//go:build windows

package winapi

import (
	"testing"
	"unsafe"
)

// 本文件是「结构体布局纪律」的执行者（见 doc.go）：期望值取自 Windows SDK 在 amd64 下的实际布局。
// 任何一个数字对不上，都意味着 Go 侧布局与 MSVC 侧不一致，读到的是错位的内存。

func TestStructLayoutSizes(t *testing.T) {
	tests := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"rawSockaddr (SOCKADDR)", unsafe.Sizeof(rawSockaddr{}), 16},
		{"socketAddress (SOCKET_ADDRESS)", unsafe.Sizeof(socketAddress{}), 16},
		{"sockaddrIn", unsafe.Sizeof(sockaddrIn{}), 16},
		{"sockaddrIn6", unsafe.Sizeof(sockaddrIn6{}), 28},
		{"ifLuid (IF_LUID)", unsafe.Sizeof(ifLuid{}), 8},

		{"ipOptionInformation (IP_OPTION_INFORMATION)", unsafe.Sizeof(ipOptionInformation{}), 16},
		{"icmpEchoReply (ICMP_ECHO_REPLY)", unsafe.Sizeof(icmpEchoReply{}), 40},

		{"ipAdapterUnicastAddressLH", unsafe.Sizeof(ipAdapterUnicastAddressLH{}), 64},
		{"ipAdapterDnsServerAddressXP", unsafe.Sizeof(ipAdapterDnsServerAddressXP{}), 32},
		{"ipAdapterGatewayAddressLH", unsafe.Sizeof(ipAdapterGatewayAddressLH{}), 32},
		{"ipAdapterWinsServerAddressLH", unsafe.Sizeof(ipAdapterWinsServerAddressLH{}), 32},

		{"ipAdapterAddressesLH", unsafe.Sizeof(ipAdapterAddressesLH{}), 448},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: sizeof = %d, 期望 %d（与 Windows SDK 布局不符，读写将错位）",
				tt.name, tt.got, tt.want)
		}
	}
}

func TestStructLayoutAlignments(t *testing.T) {
	tests := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"socketAddress", unsafe.Alignof(socketAddress{}), 8},
		{"sockaddrIn6", unsafe.Alignof(sockaddrIn6{}), 4},
		{"icmpEchoReply", unsafe.Alignof(icmpEchoReply{}), 8},
		{"ipAdapterUnicastAddressLH", unsafe.Alignof(ipAdapterUnicastAddressLH{}), 8},
		{"ipAdapterAddressesLH", unsafe.Alignof(ipAdapterAddressesLH{}), 8},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: alignof = %d, 期望 %d", tt.name, tt.got, tt.want)
		}
	}
}

// ipAdapterAddressesLH 的逐字段偏移：Next 指针错 8 字节，链表遍历就会崩溃。
func TestStructLayoutAdapterAddressesOffsets(t *testing.T) {
	var a ipAdapterAddressesLH

	tests := []struct {
		field string
		got   uintptr
		want  uintptr
	}{
		{"Length", unsafe.Offsetof(a.Length), 0},
		{"IfIndex", unsafe.Offsetof(a.IfIndex), 4},
		{"Next", unsafe.Offsetof(a.Next), 8},
		{"AdapterName", unsafe.Offsetof(a.AdapterName), 16},
		{"FirstUnicastAddress", unsafe.Offsetof(a.FirstUnicastAddress), 24},
		{"FirstAnycastAddress", unsafe.Offsetof(a.FirstAnycastAddress), 32},
		{"FirstMulticastAddress", unsafe.Offsetof(a.FirstMulticastAddress), 40},
		{"FirstDnsServerAddress", unsafe.Offsetof(a.FirstDnsServerAddress), 48},
		{"DnsSuffix", unsafe.Offsetof(a.DnsSuffix), 56},
		{"Description", unsafe.Offsetof(a.Description), 64},
		{"FriendlyName", unsafe.Offsetof(a.FriendlyName), 72},
		{"PhysicalAddress", unsafe.Offsetof(a.PhysicalAddress), 80},
		{"PhysicalAddressLength", unsafe.Offsetof(a.PhysicalAddressLength), 88},
		{"Flags", unsafe.Offsetof(a.Flags), 92},
		{"Mtu", unsafe.Offsetof(a.Mtu), 96},
		{"IfType", unsafe.Offsetof(a.IfType), 100},
		{"OperStatus", unsafe.Offsetof(a.OperStatus), 104},
		{"Ipv6IfIndex", unsafe.Offsetof(a.Ipv6IfIndex), 108},
		{"ZoneIndices", unsafe.Offsetof(a.ZoneIndices), 112},
		{"FirstPrefix", unsafe.Offsetof(a.FirstPrefix), 176},
		{"TransmitLinkSpeed", unsafe.Offsetof(a.TransmitLinkSpeed), 184},
		{"ReceiveLinkSpeed", unsafe.Offsetof(a.ReceiveLinkSpeed), 192},
		{"FirstWinsServerAddress", unsafe.Offsetof(a.FirstWinsServerAddress), 200},
		{"FirstGatewayAddress", unsafe.Offsetof(a.FirstGatewayAddress), 208},
		{"Ipv4Metric", unsafe.Offsetof(a.Ipv4Metric), 216},
		{"Ipv6Metric", unsafe.Offsetof(a.Ipv6Metric), 220},
		{"Luid", unsafe.Offsetof(a.Luid), 224},
		{"Dhcpv4Server", unsafe.Offsetof(a.Dhcpv4Server), 232},
		{"CompartmentId", unsafe.Offsetof(a.CompartmentId), 248},
		{"NetworkGuid", unsafe.Offsetof(a.NetworkGuid), 252},
		{"ConnectionType", unsafe.Offsetof(a.ConnectionType), 268},
		{"TunnelType", unsafe.Offsetof(a.TunnelType), 272},
		{"Dhcpv6Server", unsafe.Offsetof(a.Dhcpv6Server), 280},
		{"Dhcpv6ClientDuid", unsafe.Offsetof(a.Dhcpv6ClientDuid), 296},
		// Dhcpv6ClientDuid[130] 结束于 426，ULONG 需 4 字节对齐，故 Dhcpv6ClientDuidLength 落在 428（MSVC 实测）。
		{"Dhcpv6ClientDuidLength", unsafe.Offsetof(a.Dhcpv6ClientDuidLength), 428},
		{"Dhcpv6Iaid", unsafe.Offsetof(a.Dhcpv6Iaid), 432},
		{"FirstDnsSuffix", unsafe.Offsetof(a.FirstDnsSuffix), 440},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("ipAdapterAddressesLH.%s 偏移 = %d, 期望 %d", tt.field, tt.got, tt.want)
		}
	}
}

func TestStructLayoutUnicastAddressOffsets(t *testing.T) {
	var u ipAdapterUnicastAddressLH
	tests := []struct {
		field string
		got   uintptr
		want  uintptr
	}{
		{"Length", unsafe.Offsetof(u.Length), 0},
		{"Flags", unsafe.Offsetof(u.Flags), 4},
		{"Next", unsafe.Offsetof(u.Next), 8},
		{"Address", unsafe.Offsetof(u.Address), 16},
		{"PrefixOrigin", unsafe.Offsetof(u.PrefixOrigin), 32},
		{"SuffixOrigin", unsafe.Offsetof(u.SuffixOrigin), 36},
		{"DadState", unsafe.Offsetof(u.DadState), 40},
		{"ValidLifetime", unsafe.Offsetof(u.ValidLifetime), 44},
		{"PreferredLifetime", unsafe.Offsetof(u.PreferredLifetime), 48},
		{"LeaseLifetime", unsafe.Offsetof(u.LeaseLifetime), 52},
		{"OnLinkPrefixLength", unsafe.Offsetof(u.OnLinkPrefixLength), 56},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("ipAdapterUnicastAddressLH.%s 偏移 = %d, 期望 %d", tt.field, tt.got, tt.want)
		}
	}
}

func TestStructLayoutAddressNodeOffsets(t *testing.T) {
	// DNS / Gateway / WINS 三种地址节点布局完全一致。
	var d ipAdapterDnsServerAddressXP
	if got, want := unsafe.Offsetof(d.Length), uintptr(0); got != want {
		t.Errorf("DNS 节点 Length 偏移 = %d, 期望 %d", got, want)
	}
	if got, want := unsafe.Offsetof(d.Reserved), uintptr(4); got != want {
		t.Errorf("DNS 节点 Reserved 偏移 = %d, 期望 %d", got, want)
	}
	if got, want := unsafe.Offsetof(d.Next), uintptr(8); got != want {
		t.Errorf("DNS 节点 Next 偏移 = %d, 期望 %d", got, want)
	}
	if got, want := unsafe.Offsetof(d.Address), uintptr(16); got != want {
		t.Errorf("DNS 节点 Address 偏移 = %d, 期望 %d", got, want)
	}

	var g ipAdapterGatewayAddressLH
	if got, want := unsafe.Offsetof(g.Address), uintptr(16); got != want {
		t.Errorf("网关节点 Address 偏移 = %d, 期望 %d", got, want)
	}

	var w ipAdapterWinsServerAddressLH
	if got, want := unsafe.Offsetof(w.Address), uintptr(16); got != want {
		t.Errorf("WINS 节点 Address 偏移 = %d, 期望 %d", got, want)
	}
}

func TestStructLayoutIcmpEchoReplyOffsets(t *testing.T) {
	var r icmpEchoReply
	tests := []struct {
		field string
		got   uintptr
		want  uintptr
	}{
		{"Address", unsafe.Offsetof(r.Address), 0},
		{"Status", unsafe.Offsetof(r.Status), 4},
		{"RoundTripTime", unsafe.Offsetof(r.RoundTripTime), 8},
		{"DataSize", unsafe.Offsetof(r.DataSize), 12},
		{"Reserved", unsafe.Offsetof(r.Reserved), 14},
		{"Data", unsafe.Offsetof(r.Data), 16},
		{"Options", unsafe.Offsetof(r.Options), 24},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("icmpEchoReply.%s 偏移 = %d, 期望 %d", tt.field, tt.got, tt.want)
		}
	}

	var o ipOptionInformation
	if got, want := unsafe.Offsetof(o.OptionsData), uintptr(8); got != want {
		t.Errorf("ipOptionInformation.OptionsData 偏移 = %d, 期望 %d", got, want)
	}
	if got, want := unsafe.Offsetof(o.Ttl), uintptr(0); got != want {
		t.Errorf("ipOptionInformation.Ttl 偏移 = %d, 期望 %d", got, want)
	}
}

func TestStructLayoutSocketAddressOffsets(t *testing.T) {
	var s socketAddress
	if got, want := unsafe.Offsetof(s.LpSockaddr), uintptr(0); got != want {
		t.Errorf("socketAddress.LpSockaddr 偏移 = %d, 期望 %d", got, want)
	}
	if got, want := unsafe.Offsetof(s.ISockaddrLength), uintptr(8); got != want {
		t.Errorf("socketAddress.ISockaddrLength 偏移 = %d, 期望 %d", got, want)
	}

	var in sockaddrIn
	if got, want := unsafe.Offsetof(in.Addr), uintptr(4); got != want {
		t.Errorf("sockaddrIn.Addr 偏移 = %d, 期望 %d", got, want)
	}

	var in6 sockaddrIn6
	if got, want := unsafe.Offsetof(in6.Addr), uintptr(8); got != want {
		t.Errorf("sockaddrIn6.Addr 偏移 = %d, 期望 %d", got, want)
	}
	if got, want := unsafe.Offsetof(in6.ScopeID), uintptr(24); got != want {
		t.Errorf("sockaddrIn6.ScopeID 偏移 = %d, 期望 %d", got, want)
	}
}

// 字段之间不得出现「重叠」或「未声明的额外间隙」，除已知的对齐补齐点外偏移必须严格累加。
func TestStructLayoutNoUnexpectedGaps(t *testing.T) {
	var a ipAdapterAddressesLH

	// 前 80 字节是连续的小字段，不应有任何间隙
	if got, want := unsafe.Offsetof(a.PhysicalAddress), uintptr(80); got != want {
		t.Fatalf("前段字段出现意外间隙：PhysicalAddress 偏移 %d，期望 %d", got, want)
	}
	// 88+32=120 起 ZoneIndices(64)，结束于 176，其后紧接 FirstPrefix 指针。
	if got, want := unsafe.Offsetof(a.ZoneIndices)+unsafe.Sizeof(a.ZoneIndices), uintptr(176); got != want {
		t.Fatalf("ZoneIndices 尾部偏移 = %d，期望 176（其后的 FirstPrefix 指针应紧邻）", got)
	}
	// 已知补齐点：TunnelType 结束于 276，Dhcpv6Server 需 8 字节对齐 → 280
	if got, want := unsafe.Offsetof(a.TunnelType)+uintptr(4), uintptr(276); got != want {
		t.Fatalf("TunnelType 尾部偏移 = %d，期望 276", got)
	}
	if got, want := unsafe.Offsetof(a.Dhcpv6Server), uintptr(280); got != want {
		t.Fatalf("Dhcpv6Server 偏移 = %d，期望 280（含 4 字节对齐补齐）", got)
	}
}

// Go 结构体的 sizeof 必须是其自身对齐的整数倍，否则数组化时元素之间会出现 SDK 没预期的额外补齐。
func TestStructLayoutSizeIsMultipleOfAlign(t *testing.T) {
	check := func(name string, size, align uintptr) {
		if align == 0 || size%align != 0 {
			t.Errorf("%s: sizeof=%d 不是 alignof=%d 的整数倍", name, size, align)
		}
	}
	check("ipAdapterAddressesLH", unsafe.Sizeof(ipAdapterAddressesLH{}), unsafe.Alignof(ipAdapterAddressesLH{}))
	check("ipAdapterUnicastAddressLH", unsafe.Sizeof(ipAdapterUnicastAddressLH{}), unsafe.Alignof(ipAdapterUnicastAddressLH{}))
	check("ipAdapterDnsServerAddressXP", unsafe.Sizeof(ipAdapterDnsServerAddressXP{}), unsafe.Alignof(ipAdapterDnsServerAddressXP{}))
	check("socketAddress", unsafe.Sizeof(socketAddress{}), unsafe.Alignof(socketAddress{}))
	check("icmpEchoReply", unsafe.Sizeof(icmpEchoReply{}), unsafe.Alignof(icmpEchoReply{}))
}
