//go:build windows

package collect

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// TestIntegrationRunAllCollectors 在一台真实机器上跑完整条采集链。
//
// 这是本包唯一能证明"四个采集器真的能一起跑通"的测试：
// 纯函数单测证明不了注册顺序、证明不了 winapi 与 model 的字段接得上、
// 也证明不了任何一个采集器不会 panic。而这些恰恰是最容易出错的地方。
//
// 断言只针对**物理上必然成立**的性质（非负、不超过总量、字段非空），
// 不做数值快照——同一份代码在 8 核台式机与 2 核虚拟机上结果本就不同。
func TestIntegrationRunAllCollectors(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过（会发起真实网络探测并等待 CPU 采样窗口）")
	}

	snap := &model.Snapshot{StartedAt: time.Now()}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	steps := Run(ctx, snap)

	// ── 注册顺序 ────────────────────────────────────────────────
	// 顺序不是审美问题：探测采集器依赖前三者的产出，判定引擎依赖
	// Adapters 先于 Probes 就位。顺序错了，报告会给出错误的网卡归属。
	wantOrder := []string{"主机与系统信息", "网络适配器信息", "系统健康度", "网络连通性探测"}
	if len(steps) != len(wantOrder) {
		t.Fatalf("采集步骤数 = %d，期望 %d（注册列表被改动过？）", len(steps), len(wantOrder))
	}
	for i, want := range wantOrder {
		if steps[i].Name != want {
			t.Errorf("第 %d 步是 %q，期望 %q（采集顺序有依赖关系，不能调换）", i+1, steps[i].Name, want)
		}
	}
	for _, s := range steps {
		t.Logf("[%s] 耗时 %v  err=%v", s.Name, s.Duration.Round(time.Millisecond), s.Err)
	}

	// ── 主机与系统信息 ──────────────────────────────────────────
	if snap.Host.ComputerName == "" {
		t.Error("计算机名为空：报告头无法标识这是哪台机器")
	}
	if snap.Host.OSVersion == "" {
		t.Error("操作系统版本为空")
	}
	if snap.Host.OSName == "" {
		t.Error("操作系统显示名为空")
	}
	if strings.Contains(snap.Host.OSName, "Windows 10") && snap.Host.OSBuild >= "22000" {
		t.Errorf("操作系统被报成 Windows 10 而 build=%s 已达 Windows 11 门槛：注册表遗留写法未被纠正（OSName=%q）",
			snap.Host.OSBuild, snap.Host.OSName)
	}
	if snap.Host.Uptime <= 0 {
		t.Error("运行时长非正数，GetTickCount64 结果解读有误")
	}
	t.Logf("主机：%s / %s / %s / 运行 %s / 管理员=%v",
		snap.Host.ComputerName, snap.Host.OSName, snap.Host.OSVersion,
		snap.Host.Uptime.Round(time.Second), snap.Host.IsAdmin)

	// ── 网络适配器 ──────────────────────────────────────────────
	if len(snap.Adapters) == 0 {
		t.Error("一块网卡都没采到：GetAdaptersAddresses 的调用或解析有问题")
	}
	for _, a := range snap.Adapters {
		if a.Index == 0 {
			t.Errorf("网卡 %q 的 IfIndex 为 0，结构体偏移可能错位", a.DisplayName())
		}
		t.Logf("网卡 IfIndex=%d %q 类型=%s 状态=%s 虚拟=%v(%s) IPv4=%v 网关=%v DNS=%v",
			a.Index, a.DisplayName(), a.IfType, a.OperStatus,
			a.IsVirtual, a.VirtualKind, a.AddrStrings(), a.Gateways, a.DNS)
	}

	// 回环网卡绝不能出现在"活动物理网卡"里：它有一块永远可达的假网关，
	// 混进去会让 R-14「内网链路中断」在真正断网时反而不触发。
	for _, a := range snap.ActivePhysicalAdapters() {
		if a.IfType == model.IfTypeLoopback {
			t.Errorf("回环网卡 %q 出现在活动物理网卡列表中", a.DisplayName())
		}
		if a.IsVirtual {
			t.Errorf("虚拟网卡 %q 出现在活动物理网卡列表中", a.DisplayName())
		}
		if a.OperStatus != model.OperStatusUp {
			t.Errorf("状态为 %s 的网卡 %q 出现在活动网卡列表中", a.OperStatus, a.DisplayName())
		}
	}

	// ── 系统健康度 ──────────────────────────────────────────────
	h := snap.Health
	if !h.MemKnown {
		t.Error("内存未能采集（GlobalMemoryStatusEx）")
	} else {
		if h.MemTotalBytes == 0 {
			t.Error("内存总量为 0")
		}
		if h.MemAvailBytes > h.MemTotalBytes {
			t.Errorf("可用内存 %d 大于总内存 %d", h.MemAvailBytes, h.MemTotalBytes)
		}
		if h.MemUsedPercent < 0 || h.MemUsedPercent > 100 {
			t.Errorf("内存使用率 %.1f%% 越界", h.MemUsedPercent)
		}
	}

	if !h.DiskKnown {
		t.Error("系统盘未能采集")
	} else {
		if len(h.SystemDrive) != 2 || h.SystemDrive[1] != ':' {
			t.Errorf("系统盘 %q 不是「X:」形式", h.SystemDrive)
		}
		if h.DiskTotalBytes == 0 {
			t.Error("系统盘总量为 0")
		}
		if h.DiskFreeBytes > h.DiskTotalBytes {
			t.Errorf("可用空间 %d 大于总量 %d", h.DiskFreeBytes, h.DiskTotalBytes)
		}
	}

	if !h.CPUKnown {
		t.Error("CPU 占用率未能采集")
	} else if h.CPUPercent < 0 || h.CPUPercent > 100 {
		t.Errorf("CPU 占用率 %.1f%% 越界", h.CPUPercent)
	}

	t.Logf("健康度：内存 %.1f%%（%d/%d 字节） 系统盘 %s 可用 %d/%d 字节 CPU %.1f%%",
		h.MemUsedPercent, h.MemAvailBytes, h.MemTotalBytes,
		h.SystemDrive, h.DiskFreeBytes, h.DiskTotalBytes, h.CPUPercent)

	// ── 网络连通性 ──────────────────────────────────────────────
	if len(snap.Probes) == 0 {
		t.Fatal("一条探测记录都没有：探测采集器没有产出")
	}
	for _, p := range snap.Probes {
		t.Logf("探测 %-12s 目标=%-18s 跳过=%v 成功=%v 发送=%d 收到=%d 丢包=%.0f%% 错误=%q",
			p.Kind, p.Target, p.Skipped, p.Success, p.Sent, p.Recv, p.LossPercent, p.Err)

		if p.Skipped && p.Sent != 0 {
			t.Errorf("跳过的探测 %s 却记录了发送 %d 个包：跳过的探测不得计入丢包率", p.Kind, p.Sent)
		}
		if !p.Skipped && p.Recv > p.Sent {
			t.Errorf("探测 %s 收到 %d 个包却只发了 %d 个", p.Kind, p.Recv, p.Sent)
		}
		if p.LossPercent < 0 || p.LossPercent > 100 {
			t.Errorf("探测 %s 丢包率 %.1f%% 越界", p.Kind, p.LossPercent)
		}
	}

	if len(snap.Layers) == 0 {
		t.Fatal("故障层级结论为空：Classify 没有被调用或返回了空切片")
	}
	top := snap.Layers[0]
	t.Logf("层级结论：%s（%s）/ %s", top.Level, top.Severity, top.Summary)
	if top.Level == "" || top.Summary == "" {
		t.Errorf("层级结论缺少 level 或 summary：%+v", top)
	}

	// ── 第三层原始数据 ──────────────────────────────────────────
	if len(snap.Raw) == 0 {
		t.Error("原始数据附录为空：第三层将无内容可写")
	}
	sections := map[string]int{}
	for _, r := range snap.Raw {
		sections[r.Section]++
	}
	for _, want := range []string{"主机与系统", "网络适配器", "系统健康度", "网络连通性"} {
		if sections[want] == 0 {
			t.Errorf("原始数据缺少分节 %q（实际分节：%v）", want, sections)
		}
	}
	t.Logf("原始数据 %d 行，分节统计 %v", len(snap.Raw), sections)

	// 采集失败/降级项要打印出来。它们不会让测试失败（部分采集是合法结果），
	// 但一条"因权限不足未采集"如果没人看见，就永远不会被修。
	for _, f := range snap.Failures {
		t.Logf("降级项 [%s] 部分采集=%v 原因=%s", f.Item, f.Partial, f.Reason)
	}
}

// TestIntegrationRunIsRepeatable 验证同一台机器上连跑两次的结论一致。
//
// REQ-N-08 要求结论可复现：运维拿两份报告对比时，如果同一台机器
// 两次运行的"故障层级"不同，整份报告的可信度就没了。
func TestIntegrationRunIsRepeatable(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过")
	}

	first := &model.Snapshot{StartedAt: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	Run(ctx, first)

	second := &model.Snapshot{StartedAt: time.Now()}
	Run(ctx, second)

	if len(first.Layers) == 0 || len(second.Layers) == 0 {
		t.Fatal("层级结论为空")
	}
	if first.Layers[0].Level != second.Layers[0].Level {
		t.Errorf("两次运行的故障层级不同：%s vs %s（同一台机器、几乎同一时刻，结论必须一致）",
			first.Layers[0].Level, second.Layers[0].Level)
	}
	if first.Host.ComputerName != second.Host.ComputerName {
		t.Errorf("两次运行的计算机名不同：%q vs %q", first.Host.ComputerName, second.Host.ComputerName)
	}
	// 网卡列表必须稳定排序，否则同一台机器的两次报告无法逐行对照。
	if len(first.Adapters) != len(second.Adapters) {
		t.Fatalf("两次运行的网卡数量不同：%d vs %d", len(first.Adapters), len(second.Adapters))
	}
	for i := range first.Adapters {
		if first.Adapters[i].Index != second.Adapters[i].Index {
			t.Errorf("第 %d 块网卡的 IfIndex 顺序不稳定：%d vs %d（报告要求确定性输出）",
				i, first.Adapters[i].Index, second.Adapters[i].Index)
		}
	}
}
