package model

import (
	"testing"
	"time"
)

func TestAddrIsAPIPA(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"169.254.13.7", true},
		{"169.254.0.1", true},
		{"169.253.1.1", false},
		{"192.168.1.10", false},
		{"10.0.0.1", false},
		{"", false},
		{"fe80::1", false},
	}
	for _, tt := range tests {
		if got := (Addr{IP: tt.ip}).IsAPIPA(); got != tt.want {
			t.Errorf("Addr{IP:%q}.IsAPIPA() = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestAddrIsLoopback(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.1.2.3", true},
		{"::1", true},
		{"192.168.1.1", false},
		{"169.254.1.1", false},
		{"fe80::1", false},
		{"not-an-ip", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := (Addr{IP: tt.ip}).IsLoopback(); got != tt.want {
			t.Errorf("Addr{IP:%q}.IsLoopback() = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestAdapterIsActive(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{OperStatusUp, true},
		{OperStatusDown, false},
		{OperStatusDormant, false},
		{OperStatusNotPresent, false},
		{OperStatusLowerLayerDown, false},
		{"", false},
	}
	for _, tt := range tests {
		if got := (Adapter{OperStatus: tt.status}).IsActive(); got != tt.want {
			t.Errorf("Adapter{OperStatus:%q}.IsActive() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestAdapterHasUsableIPv4(t *testing.T) {
	tests := []struct {
		name string
		addr []Addr
		want bool
	}{
		{"无地址", nil, false},
		{"正常内网地址", []Addr{{IP: "192.168.1.10"}}, true},
		{"仅 APIPA", []Addr{{IP: "169.254.13.7"}}, false},
		{"仅回环", []Addr{{IP: "127.0.0.1"}}, false},
		{"APIPA + 回环", []Addr{{IP: "169.254.13.7"}, {IP: "127.0.0.1"}}, false},
		{"APIPA + 正常", []Addr{{IP: "169.254.13.7"}, {IP: "10.1.1.2"}}, true},
		{"公网地址", []Addr{{IP: "100.64.0.5"}}, true},
	}
	for _, tt := range tests {
		a := Adapter{IPv4: tt.addr}
		if got := a.HasUsableIPv4(); got != tt.want {
			t.Errorf("%s: HasUsableIPv4() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestAdapterHasGatewayAndDNS(t *testing.T) {
	tests := []struct {
		name            string
		gateways, dns   []string
		wantGW, wantDNS bool
	}{
		{"都为空", nil, nil, false, false},
		{"空字符串不算有", []string{""}, []string{"  "}, false, false},
		{"都有", []string{"192.168.1.1"}, []string{"8.8.8.8"}, true, true},
	}
	for _, tt := range tests {
		a := Adapter{Gateways: tt.gateways, DNS: tt.dns}
		if got := a.HasGateway(); got != tt.wantGW {
			t.Errorf("%s: HasGateway() = %v, want %v", tt.name, got, tt.wantGW)
		}
		if got := a.HasDNS(); got != tt.wantDNS {
			t.Errorf("%s: HasDNS() = %v, want %v", tt.name, got, tt.wantDNS)
		}
	}
}

func TestAdapterFirstGateway(t *testing.T) {
	tests := []struct {
		name string
		gw   []string
		want string
	}{
		{"无网关", nil, ""},
		{"跳过空串", []string{"", "192.168.1.1"}, "192.168.1.1"},
		{"取第一个", []string{"10.0.0.1", "10.0.0.2"}, "10.0.0.1"},
	}
	for _, tt := range tests {
		if got := (Adapter{Gateways: tt.gw}).FirstGateway(); got != tt.want {
			t.Errorf("%s: FirstGateway() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestActivePhysicalAdapters(t *testing.T) {
	up := func(a Adapter) Adapter { a.OperStatus = OperStatusUp; return a }

	tests := []struct {
		name     string
		adapters []Adapter
		want     []string
	}{
		{
			name:     "全部为空",
			adapters: nil,
			want:     nil,
		},
		{
			name: "只保留 Up 的物理网卡",
			adapters: []Adapter{
				up(Adapter{Name: "以太网", IfType: IfTypeEthernet}),
				{Name: "无线", OperStatus: OperStatusDown, IfType: IfTypeIEEE80211},
				up(Adapter{Name: "VMware", IfType: IfTypeEthernet, IsVirtual: true, VirtualKind: VirtualVMware}),
				up(Adapter{Name: "回环", IfType: IfTypeLoopback, IsVirtual: true, VirtualKind: VirtualLoopback}),
			},
			want: []string{"以太网"},
		},
		{
			name: "回环即使未被标记为虚拟也要排除",
			adapters: []Adapter{
				up(Adapter{Name: "Loopback", IfType: IfTypeLoopback}),
			},
			want: nil,
		},
		{
			name: "多块物理网卡全部保留且保持原顺序",
			adapters: []Adapter{
				up(Adapter{Name: "A", IfType: IfTypeEthernet}),
				up(Adapter{Name: "B", IfType: IfTypeIEEE80211}),
			},
			want: []string{"A", "B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Snapshot{Adapters: tt.adapters}
			got := s.ActivePhysicalAdapters()
			if len(got) != len(tt.want) {
				t.Fatalf("得到 %d 块网卡，期望 %d 块", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Name != tt.want[i] {
					t.Errorf("第 %d 块 = %q, 期望 %q", i, got[i].Name, tt.want[i])
				}
			}
		})
	}
}

// ActivePhysicalAdapters 不得改动底层切片（必须返回副本语义，避免调用方误改快照）。
func TestActivePhysicalAdaptersDoesNotMutate(t *testing.T) {
	s := &Snapshot{Adapters: []Adapter{
		{Name: "A", OperStatus: OperStatusUp, IfType: IfTypeEthernet},
		{Name: "B", OperStatus: OperStatusDown, IfType: IfTypeEthernet},
	}}
	got := s.ActivePhysicalAdapters()
	got[0].Name = "被改名"
	if s.Adapters[0].Name != "A" {
		t.Fatal("ActivePhysicalAdapters 返回的元素与原快照共享了可变状态")
	}
}

func TestIPv6OnlyLinkLocal(t *testing.T) {
	upEth := func(a Adapter) Adapter {
		a.OperStatus = OperStatusUp
		a.IfType = IfTypeEthernet
		return a
	}
	ll := func(ip string) Addr { return Addr{IP: ip, Scope: ScopeLinkLocal} }
	gl := func(ip string) Addr { return Addr{IP: ip, Scope: ScopeGlobal} }

	tests := []struct {
		name     string
		adapters []Adapter
		want     bool
	}{
		{
			name:     "无网卡 → false（属 S-01，由 R-19 处理）",
			adapters: nil,
			want:     false,
		},
		{
			name:     "仅 IPv6 链路本地、无 IPv4 → true",
			adapters: []Adapter{upEth(Adapter{IPv6: []Addr{ll("fe80::a1b2")}})},
			want:     true,
		},
		{
			name: "仅有链路本地 IPv6 但有 APIPA IPv4 → true（APIPA 不算有效 IPv4）",
			adapters: []Adapter{upEth(Adapter{
				IPv4: []Addr{{IP: "169.254.13.7"}},
				IPv6: []Addr{ll("fe80::a1b2")},
			})},
			want: true,
		},
		{
			name: "有全局 IPv6 → false",
			adapters: []Adapter{upEth(Adapter{
				IPv6: []Addr{ll("fe80::a1b2"), gl("2408:873d::1")},
			})},
			want: false,
		},
		{
			name: "有有效 IPv4 → false",
			adapters: []Adapter{upEth(Adapter{
				IPv4: []Addr{{IP: "192.168.1.10"}},
				IPv6: []Addr{ll("fe80::a1b2")},
			})},
			want: false,
		},
		{
			name: "只有 IPv4 没有任何 IPv6 → false（不能凭空判定链路本地）",
			adapters: []Adapter{upEth(Adapter{
				IPv4: []Addr{{IP: "169.254.13.7"}},
			})},
			want: false,
		},
		{
			name:     "IPv6 全被禁用（空列表）→ false",
			adapters: []Adapter{upEth(Adapter{})},
			want:     false,
		},
		{
			name: "非活动网卡上的链路本地地址不参与判定",
			adapters: []Adapter{
				{OperStatus: OperStatusDown, IfType: IfTypeEthernet, IPv6: []Addr{ll("fe80::1")}},
			},
			want: false,
		},
		{
			name: "虚拟网卡上的全局 IPv6 不应解除链路本地判定",
			adapters: []Adapter{upEth(Adapter{
				IsVirtual: true, VirtualKind: VirtualHyperV,
				IPv6: []Addr{ll("fe80::1"), gl("2408::1")},
			})},
			want: false, // 虚拟网卡被过滤后无活动物理网卡 → false
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Snapshot{Adapters: tt.adapters}
			if got := s.IPv6OnlyLinkLocal(); got != tt.want {
				t.Errorf("IPv6OnlyLinkLocal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHealthUsedBytes(t *testing.T) {
	h := Health{MemTotalBytes: 100, MemAvailBytes: 40, DiskTotalBytes: 1000, DiskFreeBytes: 250}
	if got := h.MemUsedBytes(); got != 60 {
		t.Errorf("MemUsedBytes() = %d, want 60", got)
	}
	if got := h.DiskUsedBytes(); got != 750 {
		t.Errorf("DiskUsedBytes() = %d, want 750", got)
	}

	// 异常输入不得下溢成巨大正数（uint64 溢出的经典陷阱）
	bad := Health{MemTotalBytes: 10, MemAvailBytes: 20, DiskTotalBytes: 5, DiskFreeBytes: 9}
	if got := bad.MemUsedBytes(); got != 0 {
		t.Errorf("异常内存输入 MemUsedBytes() = %d, want 0", got)
	}
	if got := bad.DiskUsedBytes(); got != 0 {
		t.Errorf("异常磁盘输入 DiskUsedBytes() = %d, want 0", got)
	}
}

func TestProbeResultSemantics(t *testing.T) {
	skipped := ProbeResult{Kind: ProbeICMPGateway, Skipped: true, SkipReason: "无活动网卡"}
	if skipped.Executed() {
		t.Error("Skipped=true 时 Executed() 应为 false")
	}
	if skipped.TotalLoss() {
		t.Error("Skipped 的探测不得被判定为 100% 丢包")
	}

	totalLoss := ProbeResult{Kind: ProbeICMPGateway, Sent: 4, Recv: 0}
	if !totalLoss.TotalLoss() {
		t.Error("Sent=4 Recv=0 应判定为全部丢包")
	}

	partial := ProbeResult{Kind: ProbeICMPGateway, Sent: 4, Recv: 3}
	if partial.TotalLoss() {
		t.Error("部分丢包不得判定为 100% 丢包")
	}

	dns := ProbeResult{Kind: ProbeDNSSystem, Sent: 0, Recv: 0}
	if dns.TotalLoss() {
		t.Error("DNS 探测不得被判定为丢包")
	}
}

func TestSnapshotFindProbe(t *testing.T) {
	s := &Snapshot{Probes: []ProbeResult{
		{Kind: ProbeICMPGateway, Target: "192.168.1.1"},
		{Kind: ProbeICMPGateway, Target: "10.0.0.1"},
		{Kind: ProbeDNSSystem, Target: "223.5.5.5:53"},
	}}

	first, ok := s.FindProbe(ProbeICMPGateway)
	if !ok || first.Target != "192.168.1.1" {
		t.Errorf("FindProbe 应返回第一条匹配，得到 %+v ok=%v", first, ok)
	}

	if _, ok := s.FindProbe(ProbeTCP443); ok {
		t.Error("不存在的探测类型应返回 ok=false")
	}

	if got := s.FindProbes(ProbeICMPGateway); len(got) != 2 {
		t.Errorf("FindProbes(icmp) 应返回 2 条，得到 %d", len(got))
	}
	if got := s.FindProbes(ProbeTCP443); len(got) != 0 {
		t.Errorf("FindProbes(tcp) 应返回 0 条，得到 %d", len(got))
	}
}

func TestProbeKindLabel(t *testing.T) {
	for _, k := range []ProbeKind{ProbeICMPGateway, ProbeDNSSystem, ProbeDNSDirect, ProbeTCP443} {
		if k.KindLabel() == "" {
			t.Errorf("ProbeKind(%q).KindLabel() 返回空串", k)
		}
	}
	if got := ProbeKind("unknown").KindLabel(); got != "unknown" {
		t.Errorf("未知类型的 KindLabel() = %q, want %q", got, "unknown")
	}
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		sev  Severity
		want string
	}{
		{SevOK, "正常"},
		{SevWarning, "警告"},
		{SevSevere, "严重"},
		{Severity(99), "未知"},
	}
	for _, tt := range tests {
		if got := tt.sev.String(); got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.sev, got, tt.want)
		}
	}
}

// Severity 的序关系必须满足 SevOK < SevWarning < SevSevere，排序逻辑依赖它。
func TestSeverityOrdering(t *testing.T) {
	if !(SevOK < SevWarning && SevWarning < SevSevere) {
		t.Fatal("Severity 常量顺序被破坏，降序排序会失效")
	}
}

func TestCountSeverityAndHasSevere(t *testing.T) {
	issues := []Issue{
		{RuleID: "R-01", Severity: SevSevere},
		{RuleID: "R-07", Severity: SevWarning},
		{RuleID: "R-09", Severity: SevWarning},
		{RuleID: "R-19", Severity: SevWarning},
	}
	severe, warning := CountSeverity(issues)
	if severe != 1 || warning != 3 {
		t.Errorf("CountSeverity = (%d,%d), want (1,3)", severe, warning)
	}
	if !HasSevere(issues) {
		t.Error("含 SevSevere 时 HasSevere 应为 true")
	}

	onlyWarn := []Issue{{Severity: SevWarning}}
	if HasSevere(onlyWarn) {
		t.Error("仅警告时 HasSevere 应为 false（退出码应为 0 而非 1）")
	}
	if HasSevere(nil) {
		t.Error("空列表 HasSevere 应为 false")
	}

	mixed := []Issue{{Severity: SevOK}, {Severity: SevSevere}}
	s2, w2 := CountSeverity(mixed)
	if s2 != 1 || w2 != 0 {
		t.Errorf("SevOK 不应计入告警，得到 (%d,%d), want (1,0)", s2, w2)
	}
}

func TestCategoryOrderIsTotalAndDistinct(t *testing.T) {
	cats := []Category{CatNetwork, CatSystem, CatStorage, CatMeta}
	seen := map[int]Category{}
	for _, c := range cats {
		o := CategoryOrder(c)
		if prev, dup := seen[o]; dup {
			t.Fatalf("分类 %q 与 %q 的排序值相同 (%d)，排序将不确定", c, prev, o)
		}
		seen[o] = c
	}
	if CategoryOrder(CatNetwork) >= CategoryOrder(CatSystem) {
		t.Error("网络应排在系统之前")
	}
	if CategoryOrder(CatMeta) <= CategoryOrder(CatStorage) {
		t.Error("诊断完整性应排在其他分类之后")
	}
}

func TestLevelOrderCoverage(t *testing.T) {
	levels := []string{
		LevelOK, LevelWANDNS, LevelWANDown, LevelWANPort,
		LevelLANDown, LevelICMPFiltered, LevelUndetermined,
	}
	seen := map[int]string{}
	for _, l := range levels {
		o := LevelOrder(l)
		if prev, dup := seen[o]; dup {
			t.Fatalf("层级 %q 与 %q 的排序值相同 (%d)", l, prev, o)
		}
		seen[o] = l
	}
	if LevelOrder("不存在的层级") != 7 {
		t.Error("未知层级应回退到最大排序值")
	}
}

func TestNormalizeEvidence(t *testing.T) {
	got := NormalizeEvidence([]string{"  IP: 1.2.3.4  ", "", "   ", "掩码: 255.255.255.0"})
	if len(got) != 2 {
		t.Fatalf("得到 %d 条证据，期望 2 条：%v", len(got), got)
	}
	if got[0] != "IP: 1.2.3.4" {
		t.Errorf("首尾空白未去除：%q", got[0])
	}
	if got := NormalizeEvidence(nil); len(got) != 0 {
		t.Errorf("nil 输入应返回空切片，得到 %v", got)
	}
}

func TestSnapshotAddRawAndFailure(t *testing.T) {
	var s Snapshot
	s.AddRaw("网络适配器", "GetAdaptersAddresses", "Index=1 Name=以太网")
	s.AddFailure("网卡信息", "network", "GetAdaptersAddresses 失败: 拒绝访问", true)

	if len(s.Raw) != 1 || s.Raw[0].Section != "网络适配器" {
		t.Errorf("AddRaw 未正确写入：%+v", s.Raw)
	}
	if len(s.Failures) != 1 {
		t.Fatalf("AddFailure 未正确写入：%+v", s.Failures)
	}
	f := s.Failures[0]
	if f.Item != "网卡信息" || f.EnvVar != "network" || !f.Partial {
		t.Errorf("AddFailure 字段不正确：%+v", f)
	}
}

func TestSnapshotStartedAt(t *testing.T) {
	now := time.Now()
	s := Snapshot{StartedAt: now, Host: Host{Uptime: 2 * time.Hour}}
	if s.StartedAt != now {
		t.Fatal("StartedAt 未正确保存")
	}
	if s.Host.Uptime != 2*time.Hour {
		t.Fatal("Uptime 未正确保存")
	}
}
