//go:build windows

// Package collect 实现只读的数据采集层。
//
// # 红线边界
//
// 本包只读取系统状态，绝不修改：不写注册表、不改网卡配置、不启停服务、
// 不写任何临时文件。唯一被"写"的是 Snapshot 这个内存结构。
//
// # 容错契约
//
// 采集是整条流水线里最可能失败的一环（权限不足、WMI/注册表被拦截、
// 网卡被禁用、磁盘满……）。因此本包遵守一条铁律：
//
//	任何一项采集失败都不得中断流程，也不得让程序以错误码退出。
//
// 失败一律记入 Snapshot.Failures，由 R-19「诊断不完整」呈现给用户。
// 只报告采到的部分，远胜于因为一项读不到就什么都不报告。
package collect

import (
	"context"
	"fmt"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// Collector 是一个采集器。
//
// 契约（违反即为缺陷）：
//   - 不 panic。即便 panic，Run 也会兜住并记入诊断完整性告警。
//   - 不提前终止流程、不返回致命错误。
//   - 不修改系统状态。
//   - 只写自己负责的字段：采集器之间不得互相覆盖对方的成果。
type Collector interface {
	// Name 返回人类可读的采集项名，用于控制台进度与报告中的失败描述。
	Name() string
	// Collect 把采集结果写入 snap。
	Collect(ctx context.Context, snap *model.Snapshot) error
}

// envVarer 是可选接口：采集器可额外提供一个稳定的内部标识。
//
// 之所以做成可选接口而不是塞进 Collector：设计要求 Collector 保持最小，
// 而 EnvVar 只是报告里给运维人员的检索线索，缺了也不影响功能。
type envVarer interface {
	EnvVar() string
}

// collectors 保存已注册的采集器，**注册顺序即调度顺序**。
//
// 顺序有实际意义：判定引擎要先看到 Adapters 才能解释 Probes 的归属，
// 因此网络采集必须排在连通性探测之前。
var collectors []Collector

// Register 注册一个采集器。
func Register(c Collector) {
	collectors = append(collectors, c)
}

// All 返回已注册采集器的副本，供元测试与文档生成使用。
func All() []Collector {
	return append([]Collector(nil), collectors...)
}

// StepResult 是一项采集的执行结果，供控制台打印进度。
type StepResult struct {
	Name     string
	Err      error
	Duration time.Duration
}

// OK 报告该采集项是否成功。
func (s StepResult) OK() bool { return s.Err == nil }

// Run 依次执行全部已注册采集器，并把失败记入 snapshot。
//
// 返回值永远是 4 项（或注册数量）逐步结果，**永不为 nil**——
// 控制台要靠它打印完整的进度行，少打一行会让用户以为程序卡住了。
//
// 本函数永不返回错误：它把"失败"表达为 Snapshot.Failures + StepResult.Err。
func Run(ctx context.Context, snap *model.Snapshot) []StepResult {
	steps := make([]StepResult, 0, len(collectors))
	for _, c := range collectors {
		start := time.Now()
		err := runOne(ctx, c, snap)
		steps = append(steps, StepResult{
			Name:     c.Name(),
			Err:      err,
			Duration: time.Since(start),
		})
		if err != nil {
			snap.AddFailure(c.Name(), envVarOf(c), err.Error(), false)
		}
	}
	return steps
}

// runOne 执行单个采集器，把 panic 转成普通错误。
//
// 为什么规则已经要求"不 panic"还要兜这一层：采集跑在流程最前端，
// 一次 panic 会让用户等了十几秒却什么都拿不到。诊断工具的价值在于
// "尽量给出能给的结论"，而不是"要么全对要么全无"。
func runOne(ctx context.Context, c Collector, snap *model.Snapshot) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("采集过程内部错误: %v", r)
		}
	}()
	return c.Collect(ctx, snap)
}

// envVarOf 返回采集器的内部标识；未实现 envVarer 时返回空串。
func envVarOf(c Collector) string {
	if e, ok := c.(envVarer); ok {
		return e.EnvVar()
	}
	return ""
}
