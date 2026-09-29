//go:build windows

package collect

import (
	"testing"

	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

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

		// 本类机器上真实存在的虚拟网卡（装上 VMware/Hyper-V 后会出现 4~8 块）。
		{"以太网 2", "VMware Virtual Ethernet Adapter for VMnet8", model.IfTypeEthernet, true, model.VirtualVMware},
		{"vEthernet (Default Switch)", "Hyper-V Virtual Ethernet Adapter", model.IfTypeEthernet, true, model.VirtualHyperV},
		{"VirtualBox Host-Only Network", "VirtualBox Host-Only Ethernet Adapter", model.IfTypeEthernet, true, model.VirtualVirtualBox},
		{"OpenVPN TAP-Windows6", "TAP-Windows Adapter V9", model.IfTypeEthernet, true, model.VirtualTAP},
		{"wg0", "WireGuard Tunnel", model.IfTypeTunnel, true, model.VirtualWireGuard},
		{"Loopback", "Microsoft KM-TEST Loopback Adapter", model.IfTypeEthernet, true, model.VirtualLoopback},
		{"回环", "Software Loopback Interface 1", model.IfTypeLoopback, true, model.VirtualLoopback},
		{"本地连接* 1", "Microsoft Wi-Fi Direct Virtual Adapter", model.IfTypeEthernet, true, model.VirtualOther},

		// 覆盖网络（overlay VPN）。本机实测存在 ZeroTier：它是 Up 状态的
		// 软件接口，网关是 25.255.255.254（IPv4 保留段里的合成地址）。
		// 若判为物理网卡，探测会失败并被记为"诊断不完整"；
		// 更糟的情况是被算成"网关 100% 丢包"，从而误报内网链路中断。
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
