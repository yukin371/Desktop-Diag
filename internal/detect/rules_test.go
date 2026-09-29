package detect

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// upAdapter 造一块「已连接的非虚拟以太网卡」，即进入判定范围的基准对象。
func upAdapter(name string) model.Adapter {
	return model.Adapter{
		Name:       name,
		IfType:     model.IfTypeEthernet,
		OperStatus: model.OperStatusUp,
		DNSSource:  model.DNSSourceGetAdaptersAddresses,
	}
}

func snap(adapters ...model.Adapter) *model.Snapshot {
	return &model.Snapshot{Adapters: adapters}
}

func withHealth(s *model.Snapshot, h model.Health) *model.Snapshot {
	s.Health = h
	return s
}

func withProbes(s *model.Snapshot, ps ...model.ProbeResult) *model.Snapshot {
	s.Probes = append(s.Probes, ps...)
	return s
}

// gwProbe 造一条网关 ICMP 探测结果，LossPercent 按 sent/recv 自动算出。
func gwProbe(adapter, target string, sent, recv int, avgMs float64) model.ProbeResult {
	p := model.ProbeResult{
		Kind:        model.ProbeICMPGateway,
		AdapterName: adapter,
		Target:      target,
		Sent:        sent,
		Recv:        recv,
		Duration:    time.Duration(sent) * time.Second,
		Success:     recv > 0,
	}
	if sent > 0 {
		p.LossPercent = float64(sent-recv) / float64(sent) * 100
	}
	if recv > 0 {
		p.MinRTT = time.Duration(avgMs * float64(time.Millisecond))
		p.AvgRTT = p.MinRTT
		p.MaxRTT = p.MinRTT
	}
	return p
}

func tcpProbe(target string, ok bool) model.ProbeResult {
	return model.ProbeResult{
		Kind:    model.ProbeTCP443,
		Target:  target,
		Success: ok,
		Err:     errText(ok, "connect: connection refused"),
	}
}

func dnsProbe(kind model.ProbeKind, ok bool) model.ProbeResult {
	return model.ProbeResult{
		Kind:     kind,
		Target:   DNSProbeDomain,
		Success:  ok,
		Resolved: resolvedIf(ok),
		Err:      errText(ok, "no such host"),
	}
}

func errText(ok bool, s string) string {
	if ok {
		return ""
	}
	return s
}

func resolvedIf(ok bool) []string {
	if !ok {
		return nil
	}
	return []string{"110.242.68.66", "39.156.66.10"}
}

// run 跑一条规则并返回其告警。
func run(t *testing.T, id string, s *model.Snapshot) []model.Issue {
	t.Helper()
	for _, e := range catalog {
		if e.ID == id {
			return e.Fn(s)
		}
	}
	t.Fatalf("规则 %s 未注册", id)
	return nil
}

// TestCatalogMetadata 是阶段 6 文档生成的正确性前提：
// 缺 ID、缺标题或重复 ID 都会让自动生成的规则表出现空洞。
func TestCatalogMetadata(t *testing.T) {
	cat := Catalog()
	if len(cat) != 19 {
		t.Fatalf("规则数量 = %d，基线定义的是 19 条", len(cat))
	}

	seen := map[string]bool{}
	for i, r := range cat {
		wantID := fmt.Sprintf("R-%02d", i+1)
		if r.ID != wantID {
			t.Errorf("第 %d 条规则 ID = %q，期望 %q（顺序即求值顺序，必须与基线一致）", i+1, r.ID, wantID)
		}
		if seen[r.ID] {
			t.Errorf("规则 ID 重复：%s", r.ID)
		}
		seen[r.ID] = true

		if strings.TrimSpace(r.Title) == "" {
			t.Errorf("%s 缺少标题", r.ID)
		}
		if strings.TrimSpace(r.Condition) == "" {
			t.Errorf("%s 缺少触发条件描述（阶段 6 要靠它生成文档）", r.ID)
		}
		if r.Severity != model.SevWarning && r.Severity != model.SevSevere {
			t.Errorf("%s 的等级 %v 不是告警等级", r.ID, r.Severity)
		}
		if model.CategoryOrder(r.Category) >= 4 {
			t.Errorf("%s 的分类 %q 不属于任何已知分类", r.ID, r.Category)
		}
	}
}

func TestCatalogIsCopy(t *testing.T) {
	cat := Catalog()
	cat[0].ID = "R-99"
	if Catalog()[0].ID != "R-01" {
		t.Error("Catalog() 返回的必须是副本，否则文档生成器能改掉判定行为")
	}
}

func TestRuleR01APIPA(t *testing.T) {
	a := upAdapter("以太网")
	a.IPv4 = []model.Addr{{IP: "169.254.13.7"}}

	if got := run(t, "R-01", snap(a)); len(got) != 1 {
		t.Fatalf("APIPA 地址应触发 R-01，实际 %d 条", len(got))
	}

	// 正常内网地址 + APIPA 并存：仍然要报，因为对外通信会走错地址。
	a2 := upAdapter("以太网")
	a2.IPv4 = []model.Addr{{IP: "192.168.1.10"}, {IP: "169.254.13.7"}}
	if got := run(t, "R-01", snap(a2)); len(got) != 1 {
		t.Errorf("正常地址与 APIPA 并存时仍应触发 R-01，实际 %d 条", len(got))
	}

	a3 := upAdapter("以太网")
	a3.IPv4 = []model.Addr{{IP: "192.168.1.10"}}
	if got := run(t, "R-01", snap(a3)); len(got) != 0 {
		t.Errorf("正常地址不应触发 R-01，实际 %d 条", len(got))
	}

	// 网卡未连接：即使有 APIPA 残留也不报（避免对已拔线的网卡反复告警）。
	a4 := upAdapter("以太网")
	a4.OperStatus = model.OperStatusDown
	a4.IPv4 = []model.Addr{{IP: "169.254.13.7"}}
	if got := run(t, "R-01", snap(a4)); len(got) != 0 {
		t.Errorf("未连接网卡不应触发 R-01，实际 %d 条", len(got))
	}
}

func TestRuleR02NoGateway(t *testing.T) {
	a := upAdapter("以太网")
	a.IPv4 = []model.Addr{{IP: "192.168.1.10"}}
	a.DNS = []string{"192.168.1.1"}
	if got := run(t, "R-02", snap(a)); len(got) != 1 {
		t.Fatalf("无网关应触发 R-02，实际 %d 条", len(got))
	}

	a.Gateways = []string{"192.168.1.1"}
	if got := run(t, "R-02", snap(a)); len(got) != 0 {
		t.Errorf("有网关不应触发 R-02，实际 %d 条", len(got))
	}

	a.Gateways = []string{"  "}
	if got := run(t, "R-02", snap(a)); len(got) != 1 {
		t.Errorf("网关为空白串应视为未配置并触发 R-02，实际 %d 条", len(got))
	}
}

// TestRuleR03NoDNS 同时钉住基线场景 S-17：DNS 来源为「未采集」时绝不能报
// 「DNS 未配置」，否则会让用户去改一个本来正确的设置。
func TestRuleR03NoDNS(t *testing.T) {
	a := upAdapter("以太网")
	a.Gateways = []string{"192.168.1.1"}
	if got := run(t, "R-03", snap(a)); len(got) != 1 {
		t.Fatalf("空 DNS 应触发 R-03，实际 %d 条", len(got))
	}

	a.DNSSource = model.DNSSourceMissing
	if got := run(t, "R-03", snap(a)); len(got) != 0 {
		t.Errorf("DNS 来源为「未采集」时不得触发 R-03（场景 S-17），实际 %d 条", len(got))
	}

	a.DNSSource = model.DNSSourceGetAdaptersAddresses
	a.DNS = []string{"8.8.8.8"}
	if got := run(t, "R-03", snap(a)); len(got) != 0 {
		t.Errorf("有 DNS 不应触发 R-03，实际 %d 条", len(got))
	}
}

func TestRuleR04InvalidDNS(t *testing.T) {
	tests := []struct {
		name string
		dns  []string
		want int
	}{
		{"仅 0.0.0.0", []string{"0.0.0.0"}, 1},
		{"仅 127.0.0.1", []string{"127.0.0.1"}, 1},
		{"两个无效值", []string{"0.0.0.0", "127.0.0.1"}, 1},
		{"带空白", []string{" 0.0.0.0 "}, 1},
		{"无效与有效并存则不算仅含", []string{"0.0.0.0", "8.8.8.8"}, 0},
		{"正常值", []string{"192.168.1.1"}, 0},
		{"空列表由 R-03 负责", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := upAdapter("以太网")
			a.Gateways = []string{"192.168.1.1"}
			a.DNS = tt.dns
			if got := run(t, "R-04", snap(a)); len(got) != tt.want {
				t.Errorf("R-04 命中 %d 条，期望 %d 条", len(got), tt.want)
			}
		})
	}
}

func TestRuleR05IPv6OnlyLinkLocal(t *testing.T) {
	a := upAdapter("以太网")
	a.IPv6 = []model.Addr{{IP: "fe80::1", Scope: model.ScopeLinkLocal}}
	if got := run(t, "R-05", snap(a)); len(got) != 1 {
		t.Fatalf("仅链路本地 IPv6 应触发 R-05，实际 %d 条", len(got))
	}

	a.IPv4 = []model.Addr{{IP: "192.168.1.10"}}
	if got := run(t, "R-05", snap(a)); len(got) != 0 {
		t.Errorf("有可用 IPv4 时不应触发 R-05，实际 %d 条", len(got))
	}

	b := upAdapter("以太网")
	b.IPv6 = []model.Addr{{IP: "2408:8207::1", Scope: model.ScopeGlobal}}
	if got := run(t, "R-05", snap(b)); len(got) != 0 {
		t.Errorf("有全局 IPv6 时不应触发 R-05，实际 %d 条", len(got))
	}
}

func TestRuleMemoryTiers(t *testing.T) {
	tests := []struct {
		name             string
		pct              float64
		known            bool
		wantR06, wantR07 int
	}{
		{"未知不报", 0, false, 0, 0},
		{"低占用", 40, true, 0, 0},
		{"刚好 85 进入警告档", 85, true, 0, 1},
		{"94.9 仍在警告档", 94.9, true, 0, 1},
		{"95 进入严重档", 95, true, 1, 0},
		{"99 严重档", 99, true, 1, 0},
		{"84.9 不报", 84.9, true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := withHealth(snap(), model.Health{
				MemKnown:       tt.known,
				MemUsedPercent: tt.pct,
				MemTotalBytes:  32 << 30,
				MemAvailBytes:  8 << 30,
			})
			if got := run(t, "R-06", s); len(got) != tt.wantR06 {
				t.Errorf("R-06 = %d 条，期望 %d 条", len(got), tt.wantR06)
			}
			if got := run(t, "R-07", s); len(got) != tt.wantR07 {
				t.Errorf("R-07 = %d 条，期望 %d 条", len(got), tt.wantR07)
			}
		})
	}
}

func TestRuleCPUTiers(t *testing.T) {
	tests := []struct {
		name             string
		pct              float64
		known            bool
		wantR08, wantR09 int
	}{
		// 基线场景 S-18：采样窗口内系统空闲，必须输出低占用率而不是误报。
		{"空闲不报", 0.5, true, 0, 0},
		{"未知不报", 0, false, 0, 0},
		{"85 进警告档", 85, true, 0, 1},
		{"95 进严重档", 95, true, 1, 0},
		{"84.9 不报", 84.9, true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := withHealth(snap(), model.Health{CPUKnown: tt.known, CPUPercent: tt.pct})
			if got := run(t, "R-08", s); len(got) != tt.wantR08 {
				t.Errorf("R-08 = %d 条，期望 %d 条", len(got), tt.wantR08)
			}
			if got := run(t, "R-09", s); len(got) != tt.wantR09 {
				t.Errorf("R-09 = %d 条，期望 %d 条", len(got), tt.wantR09)
			}
		})
	}
}

func TestRuleDiskTiers(t *testing.T) {
	const gib = uint64(1) << 30
	tests := []struct {
		name             string
		free             uint64
		known            bool
		wantR10, wantR11 int
	}{
		{"未知不报", 0, false, 0, 0},
		{"空间充足", 100 * gib, true, 0, 0},
		{"刚好 10GiB 不报", 10 * gib, true, 0, 0},
		{"9.9GiB 进警告档", 10*gib - 1, true, 1, 0},
		{"5GiB 仍在警告档", 5 * gib, true, 1, 0},
		{"4.9GiB 进严重档", 5*gib - 1, true, 0, 1},
		// 基线场景 S-09：剩余空间为 0 必须正常触发严重档。
		{"剩余为 0", 0, true, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := withHealth(snap(), model.Health{
				DiskKnown:      tt.known,
				SystemDrive:    "C:",
				DiskTotalBytes: 500 * gib,
				DiskFreeBytes:  tt.free,
			})
			if got := run(t, "R-10", s); len(got) != tt.wantR10 {
				t.Errorf("R-10 = %d 条，期望 %d 条", len(got), tt.wantR10)
			}
			if got := run(t, "R-11", s); len(got) != tt.wantR11 {
				t.Errorf("R-11 = %d 条，期望 %d 条", len(got), tt.wantR11)
			}
		})
	}
}

func TestRuleR12GatewayLoss(t *testing.T) {
	s := withProbes(snap(), gwProbe("以太网", "192.168.1.1", 4, 3, 5))
	if got := run(t, "R-12", s); len(got) != 1 {
		t.Errorf("25%% 丢包应触发 R-12，实际 %d 条", len(got))
	}

	s = withProbes(snap(), gwProbe("以太网", "192.168.1.1", 4, 4, 5))
	if got := run(t, "R-12", s); len(got) != 0 {
		t.Errorf("无丢包不应触发 R-12，实际 %d 条", len(got))
	}

	// 100% 丢包由 R-14/R-18 表达，R-12 不得重复报。
	s = withProbes(snap(), gwProbe("以太网", "192.168.1.1", 4, 0, 0))
	if got := run(t, "R-12", s); len(got) != 0 {
		t.Errorf("100%% 丢包不应触发 R-12（应由 R-14/R-18 表达），实际 %d 条", len(got))
	}

	// 被跳过的探测（例如无线未连接）不是丢包。
	skipped := gwProbe("WLAN", "", 0, 0, 0)
	skipped.Skipped = true
	skipped.SkipReason = "无线未连接"
	s = withProbes(snap(), skipped)
	if got := run(t, "R-12", s); len(got) != 0 {
		t.Errorf("跳过的探测不应触发 R-12，实际 %d 条", len(got))
	}
}

func TestRuleR13GatewayLatency(t *testing.T) {
	s := withProbes(snap(), gwProbe("以太网", "192.168.1.1", 4, 4, 150))
	if got := run(t, "R-13", s); len(got) != 1 {
		t.Errorf("平均 150ms 应触发 R-13，实际 %d 条", len(got))
	}

	s = withProbes(snap(), gwProbe("以太网", "192.168.1.1", 4, 4, 149.9))
	if got := run(t, "R-13", s); len(got) != 0 {
		t.Errorf("149.9ms 不应触发 R-13，实际 %d 条", len(got))
	}

	// 无回包时 AvgRTT 为 0，不得因单位换算（纳秒比毫秒）而误报。
	s = withProbes(snap(), gwProbe("以太网", "192.168.1.1", 4, 0, 0))
	if got := run(t, "R-13", s); len(got) != 0 {
		t.Errorf("无回包不应触发 R-13，实际 %d 条", len(got))
	}
}

// TestRuleR14R18MutualExclusion 钉住本次设计中最重要的一处判断：
// 「网关 ping 不通但公网 443 可达」几乎总是 ICMP 被安全设备拦截，**不是断网**；
// 此时若同时报 R-14 与 R-18，报告会自相矛盾，并把"ping 被墙了"误导成严重故障。
func TestRuleR14R18MutualExclusion(t *testing.T) {
	s := withProbes(snap(),
		gwProbe("以太网", "192.168.1.1", 4, 0, 0),
		tcpProbe("223.5.5.5:443", true),
	)
	if got := run(t, "R-14", s); len(got) != 0 {
		t.Errorf("公网可达时不得报 R-14（会与 R-18 矛盾），实际 %d 条", len(got))
	}
	if got := run(t, "R-18", s); len(got) != 1 {
		t.Errorf("公网可达时应报 R-18，实际 %d 条", len(got))
	}

	s = withProbes(snap(),
		gwProbe("以太网", "192.168.1.1", 4, 0, 0),
		tcpProbe("223.5.5.5:443", false),
		tcpProbe("223.6.6.6:443", false),
	)
	if got := run(t, "R-14", s); len(got) != 1 {
		t.Errorf("网关与公网都不通时应报 R-14，实际 %d 条", len(got))
	}
	if got := run(t, "R-18", s); len(got) != 0 {
		t.Errorf("公网不通时不应报 R-18，实际 %d 条", len(got))
	}
}

func TestRuleR15LocalDNSBroken(t *testing.T) {
	s := withProbes(snap(),
		dnsProbe(model.ProbeDNSSystem, false),
		dnsProbe(model.ProbeDNSDirect, true),
	)
	if got := run(t, "R-15", s); len(got) != 1 {
		t.Errorf("系统 DNS 失败而直连成功应触发 R-15，实际 %d 条", len(got))
	}
	if got := run(t, "R-16", s); len(got) != 0 {
		t.Errorf("直连成功时不应触发 R-16，实际 %d 条", len(got))
	}

	s = withProbes(snap(),
		dnsProbe(model.ProbeDNSSystem, true),
		dnsProbe(model.ProbeDNSDirect, true),
	)
	if got := run(t, "R-15", s); len(got) != 0 {
		t.Errorf("系统 DNS 成功时不应触发 R-15，实际 %d 条", len(got))
	}

	// 系统 DNS 被跳过（未采集）时不能报 R-15：没有对照就没有结论。
	skipped := dnsProbe(model.ProbeDNSSystem, false)
	skipped.Skipped = true
	s = withProbes(snap(), skipped, dnsProbe(model.ProbeDNSDirect, true))
	if got := run(t, "R-15", s); len(got) != 0 {
		t.Errorf("系统 DNS 被跳过时不应触发 R-15，实际 %d 条", len(got))
	}
}

func TestRuleR16DNSAllBroken(t *testing.T) {
	s := withProbes(snap(), dnsProbe(model.ProbeDNSDirect, false))
	if got := run(t, "R-16", s); len(got) != 1 {
		t.Errorf("直连 DNS 失败应触发 R-16，实际 %d 条", len(got))
	}

	s = withProbes(snap(), dnsProbe(model.ProbeDNSDirect, true))
	if got := run(t, "R-16", s); len(got) != 0 {
		t.Errorf("直连 DNS 成功不应触发 R-16，实际 %d 条", len(got))
	}

	// 完全没探测（例如无活动网卡）：不得凭空报"DNS 完全不可用"。
	if got := run(t, "R-16", snap()); len(got) != 0 {
		t.Errorf("未执行直连 DNS 探测时不应触发 R-16，实际 %d 条", len(got))
	}
}

func TestRuleR17WANPortBlocked(t *testing.T) {
	s := withProbes(snap(),
		gwProbe("以太网", "192.168.1.1", 4, 4, 5),
		tcpProbe("223.5.5.5:443", false),
		tcpProbe("223.6.6.6:443", false),
	)
	if got := run(t, "R-17", s); len(got) != 1 {
		t.Errorf("网关可达且 443 全灭应触发 R-17，实际 %d 条", len(got))
	}

	s = withProbes(snap(),
		gwProbe("以太网", "192.168.1.1", 4, 4, 5),
		tcpProbe("223.5.5.5:443", false),
		tcpProbe("223.6.6.6:443", true),
	)
	if got := run(t, "R-17", s); len(got) != 0 {
		t.Errorf("有 443 可达时不应触发 R-17，实际 %d 条", len(got))
	}

	// 网关也确实不通时，由 R-14 给出更准确的结论，R-17 不得重复。
	s = withProbes(snap(),
		gwProbe("以太网", "192.168.1.1", 4, 0, 0),
		tcpProbe("223.5.5.5:443", false),
	)
	if got := run(t, "R-17", s); len(got) != 0 {
		t.Errorf("网关不通时不应触发 R-17（应由 R-14 表达），实际 %d 条", len(got))
	}

	s = withProbes(snap(), tcpProbe("223.5.5.5:443", false))
	if got := run(t, "R-17", s); len(got) != 1 {
		t.Errorf("无网关且 443 全灭应触发 R-17，实际 %d 条", len(got))
	}

	if got := run(t, "R-17", snap()); len(got) != 0 {
		t.Errorf("未执行 443 探测时不应触发 R-17，实际 %d 条", len(got))
	}
}

func TestRuleR19PartialData(t *testing.T) {
	s := snap()
	if got := run(t, "R-19", s); len(got) != 0 {
		t.Errorf("无失败项时不应触发 R-19，实际 %d 条", len(got))
	}

	s.AddFailure("网络适配器信息", "非管理员", "GetAdaptersAddresses 返回拒绝访问", false)
	s.AddFailure("系统健康度", "", "GlobalMemoryStatusEx 失败", true)
	got := run(t, "R-19", s)
	if len(got) != 1 {
		t.Fatalf("有失败项应触发一条 R-19，实际 %d 条", len(got))
	}
	if !strings.Contains(got[0].Title, "2 项") {
		t.Errorf("标题应包含缺失项数量，实际 %q", got[0].Title)
	}
	if len(got[0].Evidence) != 2 {
		t.Errorf("证据应逐项列出失败原因，实际 %d 条", len(got[0].Evidence))
	}
	if !strings.Contains(got[0].Evidence[1], "部分采集") {
		t.Errorf("部分采集的项应被标注，实际 %q", got[0].Evidence[1])
	}
}

// TestEvaluateSortOrder 钉住报告第一层的行序：
// 等级降序 → 分类固定序 → 规则 ID 升序，且多次运行完全一致。
func TestEvaluateSortOrder(t *testing.T) {
	a := upAdapter("以太网")
	a.IPv4 = []model.Addr{{IP: "169.254.13.7"}} // R-01 网络/严重
	a.Gateways = []string{"192.168.1.1"}
	a.DNS = []string{"8.8.8.8"}

	s := withHealth(snap(a), model.Health{
		CPUKnown: true, CPUPercent: 90, // R-09 系统/警告
		DiskKnown: true, SystemDrive: "C:",
		DiskTotalBytes: 500 << 30, DiskFreeBytes: 1 << 30, // R-11 存储/严重
	})
	s.AddFailure("某项", "", "失败原因", false) // R-19 诊断完整性/警告

	got := Evaluate(s)
	var ids []string
	for _, i := range got {
		ids = append(ids, i.RuleID)
	}
	want := []string{"R-01", "R-11", "R-09", "R-19"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("排序结果 = %v，期望 %v（严重在前，同类内按分类固定序再按 ID）", ids, want)
	}

	for n := 0; n < 5; n++ {
		again := Evaluate(s)
		var ids2 []string
		for _, i := range again {
			ids2 = append(ids2, i.RuleID)
		}
		if strings.Join(ids2, ",") != strings.Join(ids, ",") {
			t.Fatalf("第 %d 次运行结果不一致：%v vs %v", n+2, ids2, ids)
		}
	}
}

func TestEvaluateNilSnapshot(t *testing.T) {
	if got := Evaluate(nil); got != nil {
		t.Errorf("nil 快照应返回 nil，实际 %v", got)
	}
}

// TestEvaluateRecoversPanic 验证单条规则崩溃不会带走整份报告：采集通常要跑十几秒，
// 用户不能因为一条规则的实现缺陷就拿到崩溃而不是报告。
func TestEvaluateRecoversPanic(t *testing.T) {
	boom := ruleEntry{
		ID: "R-99", Severity: model.SevWarning, Category: model.CatMeta,
		Title: "测试用", Condition: "测试用",
		Fn: func(*model.Snapshot) []model.Issue { panic("故意崩溃") },
	}
	saved := catalog
	catalog = append(append([]ruleEntry{}, saved...), boom)
	defer func() { catalog = saved }()

	// 回归钉子：R-99 追加在 R-19 **之后**。R-19 必须靠 Deferred 延后求值，
	// 否则它结算 Failures 时看不到这条崩溃记录，"诊断不完整"就被静默吞掉了。
	if catalog[len(catalog)-1].ID != "R-99" {
		t.Fatal("测试前提被破坏：崩溃规则必须是最后注册的")
	}
	var r19pos int
	for i, e := range catalog {
		if e.ID == "R-19" {
			r19pos = i
		}
	}
	if r19pos == len(catalog)-1 {
		t.Fatal("测试前提被破坏：R-19 必须注册在崩溃规则之前")
	}

	s := snap()
	got := Evaluate(s)

	for _, i := range got {
		if i.RuleID == "R-99" {
			t.Fatalf("崩溃的规则不应产出告警，实际 %+v", i)
		}
	}
	if len(s.Failures) != 1 {
		t.Fatalf("崩溃应被记录为一条采集失败，实际 %d 条", len(s.Failures))
	}
	if !strings.Contains(s.Failures[0].Item, "R-99") {
		t.Errorf("失败记录应指明是哪条规则，实际 %q", s.Failures[0].Item)
	}
	var sawR19 bool
	for _, i := range got {
		if i.RuleID == "R-19" {
			sawR19 = true
		}
	}
	if !sawR19 {
		t.Error("规则崩溃后应触发 R-19，让用户知道这份诊断不完整")
	}
}

// TestEvaluateRealisticHealthyMachine 用一台配置齐全的健康机器验证「零告警」。
// 这是防误报的关键用例：阈值调到会误报的水平，运维人员就会开始忽略报告。
func TestEvaluateRealisticHealthyMachine(t *testing.T) {
	a := upAdapter("以太网")
	a.IPv4 = []model.Addr{{IP: "192.168.1.10", Prefix: 24}}
	a.Gateways = []string{"192.168.1.1"}
	a.DNS = []string{"192.168.1.1", "223.5.5.5"}

	s := withHealth(snap(a), model.Health{
		CPUKnown: true, CPUPercent: 12,
		MemKnown: true, MemUsedPercent: 45, MemTotalBytes: 32 << 30, MemAvailBytes: 17 << 30,
		DiskKnown: true, SystemDrive: "C:", DiskTotalBytes: 500 << 30, DiskFreeBytes: 200 << 30,
	})
	withProbes(s,
		gwProbe("以太网", "192.168.1.1", 4, 4, 2),
		dnsProbe(model.ProbeDNSSystem, true),
		dnsProbe(model.ProbeDNSDirect, true),
		tcpProbe("223.5.5.5:443", true),
	)

	if got := Evaluate(s); len(got) != 0 {
		var msgs []string
		for _, i := range got {
			msgs = append(msgs, fmt.Sprintf("%s %s", i.RuleID, i.Title))
		}
		t.Errorf("健康机器不应产生任何告警，实际 %d 条：%s", len(got), strings.Join(msgs, " / "))
	}
}
