//go:build windows

package winapi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// 本文件封装 iphlpapi.dll 中只读的网卡枚举与 ICMP 探测 API。

var (
	procGetAdaptersAddresses = iphlpapi.NewProc("GetAdaptersAddresses")
	procIcmpCreateFile       = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle      = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho         = iphlpapi.NewProc("IcmpSendEcho")
)

const (
	// maxAdapterCount 是遍历网卡链表时的硬上限：底层数据异常（例如链表成环）时不能无限循环，诊断工具本身不能卡死。
	maxAdapterCount = 512

	// GetAdaptersAddresses 的推荐初始缓冲区大小（Windows 文档建议 15 KB）。
	initialAdapterBufSize = 15 * 1024

	// 缓冲区不足时的增量，并保留重试次数上限。
	adapterBufGrowth  = 16 * 1024
	maxAdapterRetries = 5
)

// RawAddr 是网卡地址的中立表示。
// winapi 是叶子包，不能 import internal/model，所以这里用自己的类型，由 collect 层负责转换。
type RawAddr struct {
	IP      string // 呈现形式
	Prefix  int    // OnLinkPrefixLength，即前缀长度
	Family  uint16 // afInet / afInet6
	ScopeID uint32 // IPv6 作用域 ID

	DadState          uint32
	PrefixOrigin      uint32
	SuffixOrigin      uint32
	ValidLifetime     uint32
	PreferredLifetime uint32
	LeaseLifetime     uint32
}

// RawAdapter 是一次 GetAdaptersAddresses 遍历得到的单个适配器数据。
type RawAdapter struct {
	IfIndex     uint32
	Ipv6IfIndex uint32

	AdapterName  string // ANSI 形式的 {GUID}，用于关联注册表 Interfaces 子键
	FriendlyName string // 用户可读名称
	Description  string // 驱动描述
	DnsSuffix    string

	PhysicalAddress       string // 冒号分隔的大写十六进制
	PhysicalAddressLength uint32

	Mtu        uint32
	IfType     uint32
	OperStatus uint32
	Flags      uint32

	Dhcpv4Enabled bool

	IPv4     []RawAddr
	IPv6     []RawAddr
	DNS      []string
	Gateways []string
}

// IsUp 报告适配器是否处于 Up 状态。
func (a RawAdapter) IsUp() bool { return a.OperStatus == ifOperStatusUp }

// GetAdaptersAddresses 枚举本机所有网络适配器；family 传 afUnspec 可同时取得 IPv4 与 IPv6。
// flags 必须含 gaaFlagIncludeGateways，否则拿不到 FirstGatewayAddress 链表。
// 采用「先问大小、再取数据」的双次调用，且每次重试都递增缓冲区——调用之间网卡可能发生变化（例如 VPN 正在建立）。
func GetAdaptersAddresses(family, flags uint32) ([]RawAdapter, error) {
	size := uint32(initialAdapterBufSize)

	for attempt := 0; attempt < maxAdapterRetries; attempt++ {
		buf := make([]byte, size)

		ret, _, callErr := procGetAdaptersAddresses.Call(
			uintptr(family),
			uintptr(flags),
			0, // Reserved，必须为 0
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)),
		)

		switch uint32(ret) {
		case errorSuccess:
			return parseAdapters(buf)

		case errorBufferOverflow:
			// size 已被系统改写为所需字节数；再加一点余量应对调用间隙的变化。
			size += adapterBufGrowth
			continue

		case errorNoData:
			// 没有任何适配器（网卡全部被禁用）是合法结果，不是错误。
			return nil, nil

		default:
			return nil, callError("GetAdaptersAddresses", callErr)
		}
	}

	return nil, fmt.Errorf("GetAdaptersAddresses: 连续 %d 次缓冲区不足，网卡数量可能在剧烈变化", maxAdapterRetries)
}

// parseAdapters 遍历 GetAdaptersAddresses 写回的链表。
// 缓冲区来自系统但按原始内存解析，每一步都必须做边界校验——一旦偏移假设有误，崩溃的正是这个诊断工具本身。
func parseAdapters(buf []byte) ([]RawAdapter, error) {
	nodeSize := int(unsafe.Sizeof(ipAdapterAddressesLH{}))
	if len(buf) < nodeSize {
		return nil, fmt.Errorf("GetAdaptersAddresses: 返回缓冲区 %d 字节，小于单个节点所需的 %d 字节", len(buf), nodeSize)
	}

	base := uintptr(unsafe.Pointer(&buf[0]))
	limit := base + uintptr(len(buf))

	var out []RawAdapter
	cur := (*ipAdapterAddressesLH)(unsafe.Pointer(&buf[0]))

	for i := 0; cur != nil; i++ {
		if i >= maxAdapterCount {
			return out, fmt.Errorf("GetAdaptersAddresses: 链表超过 %d 个节点，疑似成环，已停止遍历", maxAdapterCount)
		}

		addr := uintptr(unsafe.Pointer(cur))
		// 节点必须完整落在缓冲区内。
		if addr < base || addr+uintptr(nodeSize) > limit {
			return out, errors.New("GetAdaptersAddresses: 链表节点越出返回缓冲区，已停止遍历")
		}
		// 系统写回的 Length 不得小于我们声明的结构体大小，否则后续字段偏移全部不可信。
		if int(cur.Length) < nodeSize {
			return out, fmt.Errorf("GetAdaptersAddresses: 节点 Length=%d 小于本程序预期的 %d，结构体布局不匹配，已停止遍历",
				cur.Length, nodeSize)
		}

		out = append(out, convertAdapter(cur))
		cur = cur.Next
	}

	return out, nil
}

// convertAdapter 把 SDK 结构体转成中立的 RawAdapter。
func convertAdapter(a *ipAdapterAddressesLH) RawAdapter {
	raw := RawAdapter{
		IfIndex:               a.IfIndex,
		Ipv6IfIndex:           a.Ipv6IfIndex,
		AdapterName:           bytePtrToString(a.AdapterName),
		FriendlyName:          utf16PtrToString(a.FriendlyName),
		Description:           utf16PtrToString(a.Description),
		DnsSuffix:             utf16PtrToString(a.DnsSuffix),
		PhysicalAddressLength: a.PhysicalAddressLength,
		Mtu:                   a.Mtu,
		IfType:                a.IfType,
		OperStatus:            a.OperStatus,
		Flags:                 a.Flags,
		Dhcpv4Enabled:         a.Flags&adapterFlagDhcpv4Enabled != 0,
	}

	// MAC：有效长度由 PhysicalAddressLength 决定，但可能超过数组长度（如 InfiniBand 的 20 字节地址），需夹取。
	n := a.PhysicalAddressLength
	if n > uint32(len(a.PhysicalAddress)) {
		n = uint32(len(a.PhysicalAddress))
	}
	if n > 0 {
		raw.PhysicalAddress = formatMAC(a.PhysicalAddress[:n])
	}

	for u := a.FirstUnicastAddress; u != nil; u = u.Next {
		if ip, family, scope, ok := sockaddrToAddr(u.Address); ok {
			ra := RawAddr{
				IP:                ip,
				Family:            family,
				ScopeID:           scope,
				Prefix:            int(u.OnLinkPrefixLength),
				DadState:          u.DadState,
				PrefixOrigin:      u.PrefixOrigin,
				SuffixOrigin:      u.SuffixOrigin,
				ValidLifetime:     u.ValidLifetime,
				PreferredLifetime: u.PreferredLifetime,
				LeaseLifetime:     u.LeaseLifetime,
			}
			if family == afInet {
				raw.IPv4 = append(raw.IPv4, ra)
			} else {
				raw.IPv6 = append(raw.IPv6, ra)
			}
		}
	}

	for d := a.FirstDnsServerAddress; d != nil; d = d.Next {
		if ip, _, _, ok := sockaddrToAddr(d.Address); ok {
			raw.DNS = append(raw.DNS, ip)
		}
	}

	for g := a.FirstGatewayAddress; g != nil; g = g.Next {
		if ip, _, _, ok := sockaddrToAddr(g.Address); ok {
			raw.Gateways = append(raw.Gateways, ip)
		}
	}

	return raw
}

// sockaddrToAddr 把 socketAddress 里的 sockaddr 解析成 (IP 呈现形式, address family, IPv6 作用域 ID, 是否成功)。
// 长度与 family 不符时返回 false，而不是硬读——iSockaddrLength 由系统给出，用它做一致性校验成本极低。
func sockaddrToAddr(sa socketAddress) (string, uint16, uint32, bool) {
	if sa.LpSockaddr == nil {
		return "", 0, 0, false
	}

	switch sa.LpSockaddr.Family {
	case afInet:
		if sa.ISockaddrLength < int32(unsafe.Sizeof(sockaddrIn{})) {
			return "", 0, 0, false
		}
		in := (*sockaddrIn)(unsafe.Pointer(sa.LpSockaddr))
		return net.IP(in.Addr[:]).String(), afInet, 0, true

	case afInet6:
		if sa.ISockaddrLength < int32(unsafe.Sizeof(sockaddrIn6{})) {
			return "", 0, 0, false
		}
		in6 := (*sockaddrIn6)(unsafe.Pointer(sa.LpSockaddr))
		return net.IP(in6.Addr[:]).String(), afInet6, in6.ScopeID, true
	}

	return "", 0, 0, false
}

// formatMAC 把物理地址格式化成 "00:1A:2B:3C:4D:5E" 形式的大写十六进制。
func formatMAC(b []byte) string {
	const hexDigits = "0123456789ABCDEF"
	var sb strings.Builder
	sb.Grow(len(b) * 3)
	for i, c := range b {
		if i > 0 {
			sb.WriteByte(':')
		}
		sb.WriteByte(hexDigits[c>>4])
		sb.WriteByte(hexDigits[c&0x0F])
	}
	return sb.String()
}

// ICMP

// IcmpEchoResult 是一次 IcmpSendEcho 的结果。
// Status 与 Go 的 error 是两个层次：超时（Status=ipReqTimedOut）是**调用成功**，只有调用本身失败才是 error。
// 把超时当成 error 会让丢包率计算彻底失准。
type IcmpEchoResult struct {
	Status        uint32
	RoundTripTime uint32 // 毫秒
	Address       string // 应答方 IP（点分十进制）
	DataSize      uint16
}

// Replied 报告本次探测是否收到了应答。
func (r IcmpEchoResult) Replied() bool { return r.Status == ipSuccess }

// IcmpCreateFile 打开一个 ICMP 句柄。
// 它由系统内核代发收包，因此**无需管理员权限**（原始套接字才需要提升权限）。
// 调用方必须在用完后调用 IcmpCloseHandle，否则句柄泄漏。
func IcmpCreateFile() (uintptr, error) {
	h, _, err := procIcmpCreateFile.Call()
	// INVALID_HANDLE_VALUE 是 ^uintptr(0)；IcmpCreateFile 失败时返回它。
	if h == 0 || h == ^uintptr(0) {
		return 0, callError("IcmpCreateFile", err)
	}
	return h, nil
}

// IcmpCloseHandle 关闭 IcmpCreateFile 返回的句柄。
func IcmpCloseHandle(handle uintptr) error {
	r, _, err := procIcmpCloseHandle.Call(handle)
	if r == 0 {
		return callError("IcmpCloseHandle", err)
	}
	return nil
}

// IcmpReplyBufferExtra 是回复缓冲区在 ICMP_ECHO_REPLY 与请求数据之外需要预留的字节数。
// Windows 文档要求至少 sizeof(ICMP_ECHO_REPLY) + RequestSize + 8。
const icmpReplyBufferExtra = 8

// IcmpSendEcho 向 dest 发送一个 ICMP 回显请求并等待应答。
// 仅支持 IPv4：IPAddr 参数是 32 位，IPv6 需要 Icmp6SendEcho2。
// timeout 内没有应答时返回 Status == ipReqTimedOut 的**正常结果**，不是错误。
func IcmpSendEcho(handle uintptr, dest net.IP, payload []byte, timeout time.Duration) (IcmpEchoResult, error) {
	ip4 := dest.To4()
	if ip4 == nil {
		return IcmpEchoResult{}, fmt.Errorf("IcmpSendEcho: 目标 %q 不是 IPv4 地址", dest.String())
	}
	destAddr := ipAddrFromIPv4(ip4)

	replySize := int(unsafe.Sizeof(icmpEchoReply{})) + len(payload) + icmpReplyBufferExtra
	replyBuf := make([]byte, replySize)

	var reqData unsafe.Pointer
	if len(payload) > 0 {
		reqData = unsafe.Pointer(&payload[0])
	}

	ret, _, callErr := procIcmpSendEcho.Call(
		handle,
		uintptr(destAddr),
		uintptr(reqData),
		uintptr(uint16(len(payload))),
		0, // RequestOptions：使用默认 TTL/TOS
		uintptr(unsafe.Pointer(&replyBuf[0])),
		uintptr(replySize),
		uintptr(timeout.Milliseconds()),
	)

	if ret == 0 {
		// 有些 Windows 版本在超时且无任何回包时返回 0，并把 IP_REQ_TIMED_OUT 放进 GetLastError，
		// 这里把它归一化成「超时结果」，否则丢包率会被误算成「探测失败」。
		var errno syscall.Errno
		if errors.As(callErr, &errno) {
			if uint32(errno) == ipReqTimedOut {
				return IcmpEchoResult{Status: ipReqTimedOut}, nil
			}
			return IcmpEchoResult{}, callError("IcmpSendEcho", callErr)
		}
		// GetLastError 未被设置（errno 为 0）时同样按超时处理：返回 0 且无错误码只可能是没有回包。
		return IcmpEchoResult{Status: ipReqTimedOut}, nil
	}

	reply := (*icmpEchoReply)(unsafe.Pointer(&replyBuf[0]))
	return IcmpEchoResult{
		Status:        reply.Status,
		RoundTripTime: reply.RoundTripTime,
		Address:       ipv4FromIPAddr(reply.Address),
		DataSize:      reply.DataSize,
	}, nil
}

// ipAddrFromIPv4 把 4 字节 IPv4 地址编码成 ICMP API 的 IPAddr 参数。
//
// IPAddr 在内存里就按地址本身的字节顺序存放，所以小端机器上 127.0.0.1 的数值是
// 0x0100007F（内存字节 7F 00 00 01），而不是 0x7F000001。
//
// 写反了 probe 不会失败：0x7F000001 的内存字节是 01 00 00 7F，即 1.0.0.127 —— 一个
// 真实存在、会正常应答的公网地址。于是回环探测"成功"，但 RTT 变成 195ms 的互联网
// 延迟，判定层再拿它去比 150ms 阈值，就会在每次诊断里凭空造出「内网延迟偏高」。
func ipAddrFromIPv4(ip4 []byte) uint32 {
	return binary.LittleEndian.Uint32(ip4)
}

// ipv4FromIPAddr 是 ipAddrFromIPv4 的逆运算，用于把回复包里的来源地址转成点分十进制。
func ipv4FromIPAddr(v uint32) string {
	return net.IPv4(
		byte(v),
		byte(v>>8),
		byte(v>>16),
		byte(v>>24),
	).String()
}
