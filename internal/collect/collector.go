//go:build windows

// Package collect 实现只读的数据采集层。
//
// 红线：只读系统状态，唯一被"写"的是 Snapshot 这个内存结构。
//
// 容错契约：任何一项采集失败都不得中断流程，也不得让程序以错误码退出；
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
// 契约（违反即为缺陷）：不 panic、不提前终止流程、不修改系统状态、
// 只写自己负责的字段（采集器之间不得互相覆盖对方的成果）。
type Collector interface {
	// Name 返回人类可读的采集项名，用于控制台进度与报告中的失败描述。
	Name() string
	// Collect 把采集结果写入 snap。
	Collect(ctx context.Context, snap *model.Snapshot) error
}

// envVarer 是可选接口：采集器可额外提供一个稳定的内部标识，供报告检索。
// 做成可选接口而非塞进 Collector，是为了让 Collector 保持最小。
type envVarer interface {
	EnvVar() string
}

// collectors 保存已注册的采集器，**注册顺序即调度顺序**：判定引擎要先看到
// Adapters 才能解释 Probes 的归属，因此网络采集必须排在连通性探测之前。
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
	Failures int // 本项新增的完整性记录数。
}

// OK 报告该采集项是否成功。
func (s StepResult) OK() bool { return s.Err == nil && s.Failures == 0 }

// StepEvent 在采集开始与结束时同步通知观察者，Snapshot 此时只由调度线程访问。
type StepEvent struct {
	Index   int        // 从 1 开始的步骤编号。
	Total   int        // 本次采集步骤总数。
	Started bool       // true 为开始事件，false 为完成事件。
	Result  StepResult // 当前步骤的耗时、失败与部分采集状态。
}

// Observer 接收实时进度；回调必须短小，不修改采集器注册表。
type Observer func(StepEvent)

// Run 依次执行全部已注册采集器，并把失败记入 snapshot。
//
// 返回值永远不为 nil——控制台要靠它打印完整的进度行，少打一行会让用户以为
// 程序卡住了。本函数永不返回错误，它把"失败"表达为 Snapshot.Failures + StepResult.Err。
func Run(ctx context.Context, snap *model.Snapshot, observers ...Observer) []StepResult {
	return RunCollectors(ctx, snap, All(), observers...)
}

// RunCollectors 调度显式采集器列表，供应用依赖装配与故障注入测试共用。
func RunCollectors(ctx context.Context, snap *model.Snapshot, list []Collector, observers ...Observer) []StepResult {
	// steps 保留注册顺序；每一步完成后立即通知，不能等全部采集返回才打印。
	steps := make([]StepResult, 0, len(list))
	for i, c := range list {
		// event 先发开始状态，以便耗时采集期间用户仍能看到当前检测项。
		event := StepEvent{Index: i + 1, Total: len(list), Started: true, Result: StepResult{Name: c.Name()}}
		notify(observers, event)
		// failuresBefore 用于识别返回 nil 但留下部分采集记录的情况。
		failuresBefore := len(snap.Failures)
		start := time.Now()
		err := runOne(ctx, c, snap)
		if err != nil {
			snap.AddFailure(c.Name(), envVarOf(c), err.Error(), false)
		}
		if err == nil && ctx.Err() != nil && len(snap.Failures) == failuresBefore {
			err = fmt.Errorf("检测预算已结束，结果可能不完整: %w", ctx.Err())
			snap.AddFailure(c.Name(), envVarOf(c), err.Error(), true)
		}
		event.Started = false
		event.Result = StepResult{Name: c.Name(), Err: err, Duration: time.Since(start), Failures: len(snap.Failures) - failuresBefore}
		steps = append(steps, event.Result)
		notify(observers, event)
	}
	return steps
}

// notify 同步传递进度，保证下一采集器启动前上一项已经呈现。
func notify(observers []Observer, event StepEvent) {
	for _, observer := range observers {
		if observer != nil {
			observer(event)
		}
	}
}

// runOne 执行单个采集器，把 panic 转成普通错误：采集跑在流程最前端，
// 一次 panic 会让用户等了十几秒却什么都拿不到。
func runOne(ctx context.Context, c Collector, snap *model.Snapshot) (err error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("未执行采集 %s: %w", c.Name(), err)
	}
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
