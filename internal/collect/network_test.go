//go:build windows

package collect

import (
	"testing"

	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// TestSplitGateways 钉住协议族分流：混进 IPv6 网关会让双栈机器必现 R-19 误报，
// 而整条丢弃会让仅 IPv6 的机器被 R-02 误判成无法联网。
func TestSplitGateways(t *testing.T) {
	tests := []struct {
		name           string
		in             []string
		wantV4, wantV6 []string
	}{
		{"空列表", nil, nil, nil},
		{"只有 IPv4", []string{"192.168.1.1"}, []string{"192.168.1.1"}, nil},
		{"只有 IPv6", []string{"fe80::1"}, nil, []string{"fe80::1"}},
		{"双栈按族分流", []string{"fe80::1", "192.168.1.1"}, []string{"192.168.1.1"}, []string{"fe80::1"}},
		{"去掉空白串", []string{"", "192.168.1.1", "  "}, []string{"192.168.1.1"}, nil},
		{"解析不了的归入 IPv6 侧", []string{"not-an-ip"}, nil, []string{"not-an-ip"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v4, v6 := splitGateways(tt.in)
			if !sameStrings(v4, tt.wantV4) {
				t.Errorf("IPv4 = %v，期望 %v", v4, tt.wantV4)
			}
			if !sameStrings(v6, tt.wantV6) {
				t.Errorf("IPv6 = %v，期望 %v", v6, tt.wantV6)
			}
		})
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestIfTypeName(t *testing.T) {
	cases := map[uint32]string{
		winapi.IfTypeEthernetCSMACD:   model.IfTypeEthernet,
		winapi.IfTypeIEEE80211:        model.IfTypeIEEE80211,
		winapi.IfTypeTunnel:           model.IfTypeTunnel,
		winapi.IfTypeSoftwareLoopback: model.IfTypeLoopback,
		winapi.IfTypePPP:              model.IfTypePPP,
		999:                           model.IfTypeOther,
	}
	for in, want := range cases {
		if got := ifTypeName(in); got != want {
			t.Errorf("ifTypeName(%d) = %q，期望 %q", in, got, want)
		}
	}
}

func TestOperStatusName(t *testing.T) {
	cases := map[uint32]string{
		winapi.IfOperStatusUp:             model.OperStatusUp,
		winapi.IfOperStatusDown:           model.OperStatusDown,
		winapi.IfOperStatusTesting:        model.OperStatusTesting,
		winapi.IfOperStatusDormant:        model.OperStatusDormant,
		winapi.IfOperStatusNotPresent:     model.OperStatusNotPresent,
		winapi.IfOperStatusLowerLayerDown: model.OperStatusLowerLayerDown,
		winapi.IfOperStatusUnknown:        model.OperStatusUnknown,
		4242:                              model.OperStatusUnknown,
	}
	for in, want := range cases {
		if got := operStatusName(in); got != want {
			t.Errorf("operStatusName(%d) = %q，期望 %q", in, got, want)
		}
	}
}

func TestClassifyScope(t *testing.T) {
	cases := []struct {
		ip   string
		isV6 bool
		want string
	}{
		// IPv4
		{"192.168.1.10", false, model.ScopeGlobal},
		{"10.0.0.5", false, model.ScopeGlobal},
		{"169.254.13.7", false, model.ScopeLinkLocal}, // APIPA：DHCP 没拿到地址
		{"127.0.0.1", false, model.ScopeOther},        // 回环既不是全局也不是链路本地
		// IPv6
		{"fe80::1", true, model.ScopeLinkLocal},
		{"2001:db8::1", true, model.ScopeGlobal},
		{"fc00::1", true, model.ScopeSiteLocal}, // ULA
		{"fd12:3456::1", true, model.ScopeSiteLocal},
		{"fec0::1", true, model.ScopeSiteLocal}, // 已废弃的站点本地段
		{"::1", true, model.ScopeOther},         // 回环
		// 非法输入不能 panic，落到 Other
		{"这不是 IP", false, model.ScopeOther},
		{"", false, model.ScopeOther},
	}
	for _, tc := range cases {
		if got := classifyScope(tc.ip, tc.isV6); got != tc.want {
			t.Errorf("classifyScope(%q, v6=%v) = %q，期望 %q", tc.ip, tc.isV6, got, tc.want)
		}
	}
}

func TestPrefixToMask(t *testing.T) {
	cases := map[int]string{
		24: "255.255.255.0",
		16: "255.255.0.0",
		8:  "255.0.0.0",
		32: "255.255.255.255",
		0:  "0.0.0.0",
		-1: "",
		33: "",
	}
	for in, want := range cases {
		if got := prefixToMask(in); got != want {
			t.Errorf("prefixToMask(%d) = %q，期望 %q", in, got, want)
		}
	}
}

func TestDetectVirtual(t *testing.T) {
	cases := []struct {
		name, desc, ifType string
		wantVirtual        bool
		wantKind           string
	}{
		// 真实硬件绝不能被误判为虚拟——否则网关探测会被整块跳过。
		{"以太网", "Intel(R) Ethernet Connection (2) I219-V", model.IfTypeEthernet, false, ""},
		{"WLAN", "Intel(R) Wi-Fi 6 AX200 160MHz", model.IfTypeIEEE80211, false, ""},
		{"Ethernet 2", "Realtek PCIe GbE Family Controller", model.IfTypeEthernet, false, ""},

		// 装上 VMware/Hyper-V 后会出现的虚拟网卡（会有 4~8 块）。
		{"以太网 2", "VMware Virtual Ethernet Adapter for VMnet8", model.IfTypeEthernet, true, model.VirtualVMware},
		{"vEthernet (Default Switch)", "Hyper-V Virtual Ethernet Adapter", model.IfTypeEthernet, true, model.VirtualHyperV},
		{"VirtualBox Host-Only Network", "VirtualBox Host-Only Ethernet Adapter", model.IfTypeEthernet, true, model.VirtualVirtualBox},
		{"OpenVPN TAP-Windows6", "TAP-Windows Adapter V9", model.IfTypeEthernet, true, model.VirtualTAP},
		{"wg0", "WireGuard Tunnel", model.IfTypeTunnel, true, model.VirtualWireGuard},
		{"Loopback", "Microsoft KM-TEST Loopback Adapter", model.IfTypeEthernet, true, model.VirtualLoopback},
		{"回环", "Software Loopback Interface 1", model.IfTypeLoopback, true, model.VirtualLoopback},
		{"本地连接* 1", "Microsoft Wi-Fi Direct Virtual Adapter", model.IfTypeEthernet, true, model.VirtualOther},

		// 覆盖网络（overlay VPN）：网关是保留段里的合成地址，判为物理网卡会误报内网链路中断。
		{"ZeroTier One [b9a18a606fcb86b5]", "ZeroTier One [b9a18a606fcb86b5]", model.IfTypeEthernet, true, model.VirtualOverlay},
		{"Tailscale", "Tailscale Tunnel", model.IfTypeEthernet, true, model.VirtualOverlay},
		{"Hamachi", "LogMeIn Hamachi Virtual Ethernet Adapter", model.IfTypeEthernet, true, model.VirtualOverlay},
		{"Radmin VPN", "Radmin VPN Network Adapter", model.IfTypeEthernet, true, model.VirtualOverlay},
	}
	for _, tc := range cases {
		gotVirtual, gotKind := detectVirtual(tc.name, tc.desc, tc.ifType)
		if gotVirtual != tc.wantVirtual || gotKind != tc.wantKind {
			t.Errorf("detectVirtual(%q, %q, %q) = (%v, %q)，期望 (%v, %q)",
				tc.name, tc.desc, tc.ifType, gotVirtual, gotKind, tc.wantVirtual, tc.wantKind)
		}
	}
}

func TestCleanStringAndDropBlanks(t *testing.T) {
	if got := cleanString("  以太网  \x00"); got != "以太网" {
		t.Errorf("cleanString 未清理空白/NUL: %q", got)
	}
	if got := dropBlanks([]string{"", "  ", "10.0.0.1", " ", "10.0.0.2"}); len(got) != 2 || got[0] != "10.0.0.1" {
		t.Errorf("dropBlanks = %v", got)
	}
	// 全空时返回 nil 而不是空切片：报告里据此显示「（无）」。
	if got := dropBlanks([]string{"", "  "}); got != nil {
		t.Errorf("全空输入应返回 nil，实际 %v", got)
	}
}

func TestOrNone(t *testing.T) {
	if got := orNone("  "); got != "（无）" {
		t.Errorf("orNone 空白 = %q", got)
	}
	if got := orNone("10.0.0.1"); got != "10.0.0.1" {
		t.Errorf("orNone = %q", got)
	}
	if got := displayOr("", "备选"); got != "备选" {
		t.Errorf("displayOr = %q", got)
	}
	if got := displayOr("首选", "备选"); got != "首选" {
		t.Errorf("displayOr = %q", got)
	}
}
