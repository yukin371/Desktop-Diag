//go:build windows

package winapi

import "golang.org/x/sys/windows"

// 本文件是 Windows SDK 中 C 结构体到 Go 的逐字段映射。
// 字段顺序、类型宽度、对齐必须与 SDK 头文件完全一致：不得合并、不得省略、不得添加填充字段，否则 Next 指针偏移错位，遍历链表直接崩溃。
// 起头的匿名联合体必须展开成两个连续字段（**不要**写成 Alignment uint64 后接子字段，那会把第二个字段推到偏移 8，整体错位 4 字节）。

// ifLuid 对应 IF_LUID（一个 8 字节联合体，用 uint64 承载）。
type ifLuid struct {
	Value uint64
}

// rawSockaddr 对应 SOCKADDR（16 字节，不透明载体）。
type rawSockaddr struct {
	Family uint16
	Data   [14]byte
}

// socketAddress 对应 SOCKET_ADDRESS。
// 64 位下：lpSockaddr(8) + iSockaddrLength(4) → 12，尾部补齐到 16，对齐 8。
type socketAddress struct {
	LpSockaddr      *rawSockaddr
	ISockaddrLength int32
}

// sockaddrIn 对应 sockaddr_in（16 字节）。
type sockaddrIn struct {
	Family uint16
	Port   uint16
	Addr   [4]byte
	Zero   [8]byte
}

// sockaddrIn6 对应 sockaddr_in6（28 字节，对齐 4）。
type sockaddrIn6 struct {
	Family   uint16
	Port     uint16
	FlowInfo uint32
	Addr     [16]byte
	ScopeID  uint32
}

// ipAdapterUnicastAddressLH 对应 IP_ADAPTER_UNICAST_ADDRESS_LH（64 字节）。
// 起头同样是联合体：Length(0) + Flags(4)。
type ipAdapterUnicastAddressLH struct {
	Length             uint32
	Flags              uint32
	Next               *ipAdapterUnicastAddressLH
	Address            socketAddress
	PrefixOrigin       uint32
	SuffixOrigin       uint32
	DadState           uint32
	ValidLifetime      uint32
	PreferredLifetime  uint32
	LeaseLifetime      uint32
	OnLinkPrefixLength uint8
	// 尾部补齐 7 字节 → sizeof 64
}

// ipAdapterDnsServerAddressXP 对应 IP_ADAPTER_DNS_SERVER_ADDRESS_XP（32 字节）。
type ipAdapterDnsServerAddressXP struct {
	Length   uint32
	Reserved uint32
	Next     *ipAdapterDnsServerAddressXP
	Address  socketAddress
}

// ipAdapterGatewayAddressLH 对应 IP_ADAPTER_GATEWAY_ADDRESS_LH（32 字节）。
type ipAdapterGatewayAddressLH struct {
	Length   uint32
	Reserved uint32
	Next     *ipAdapterGatewayAddressLH
	Address  socketAddress
}

// ipAdapterWinsServerAddressLH 对应 IP_ADAPTER_WINS_SERVER_ADDRESS_LH（32 字节）。
// MVP 不解析其内容，但它在结构体里占一个指针位置，必须声明以保证后续偏移正确。
type ipAdapterWinsServerAddressLH struct {
	Length   uint32
	Reserved uint32
	Next     *ipAdapterWinsServerAddressLH
	Address  socketAddress
}

// ipAdapterAddressesLH 对应 IP_ADAPTER_ADDRESSES_LH（amd64 下 448 字节）。
// 字段全部按 iptypes.h 的顺序声明；用不到的字段同样必须保留，否则其后的 Next 偏移错位。
type ipAdapterAddressesLH struct {
	Length  uint32 // +0   联合体低位：Length
	IfIndex uint32 // +4   联合体高位：IfIndex
	// 起头是匿名联合体：Length 在偏移 0、IfIndex 在偏移 4，不要另加 Alignment 字段。
	Next                   *ipAdapterAddressesLH         // +8
	AdapterName            *byte                         // +16  ANSI 的 {GUID}
	FirstUnicastAddress    *ipAdapterUnicastAddressLH    // +24
	FirstAnycastAddress    *byte                         // +32  未使用（GAA_FLAG_SKIP_ANYCAST）
	FirstMulticastAddress  *byte                         // +40  未使用（GAA_FLAG_SKIP_MULTICAST）
	FirstDnsServerAddress  *ipAdapterDnsServerAddressXP  // +48
	DnsSuffix              *uint16                       // +56
	Description            *uint16                       // +64
	FriendlyName           *uint16                       // +72
	PhysicalAddress        [8]byte                       // +80  MAX_ADAPTER_ADDRESS_LENGTH
	PhysicalAddressLength  uint32                        // +88
	Flags                  uint32                        // +92  DdnsEnabled/Dhcpv4Enabled/Ipv4Enabled... 位域
	Mtu                    uint32                        // +96
	IfType                 uint32                        // +100 IANA ifType
	OperStatus             uint32                        // +104 IF_OPER_STATUS
	Ipv6IfIndex            uint32                        // +108
	ZoneIndices            [16]uint32                    // +112
	FirstPrefix            *byte                         // +176
	TransmitLinkSpeed      uint64                        // +184
	ReceiveLinkSpeed       uint64                        // +192
	FirstWinsServerAddress *ipAdapterWinsServerAddressLH // +200
	FirstGatewayAddress    *ipAdapterGatewayAddressLH    // +208
	Ipv4Metric             uint32                        // +216
	Ipv6Metric             uint32                        // +220
	Luid                   ifLuid                        // +224
	Dhcpv4Server           socketAddress                 // +232
	CompartmentId          uint32                        // +248
	NetworkGuid            windows.GUID                  // +252（对齐 4）
	ConnectionType         uint32                        // +268
	TunnelType             uint32                        // +272
	Dhcpv6Server           socketAddress                 // +280（对齐 8，276→280 补齐 4 字节）
	Dhcpv6ClientDuid       [130]byte                     // +296 MAX_DHCPV6_DUID_LENGTH
	Dhcpv6ClientDuidLength uint32                        // +426
	Dhcpv6Iaid             uint32                        // +430
	FirstDnsSuffix         *byte                         // +440（对齐 8，434→440 补齐 6 字节）
	// sizeof = 448
}

// ipOptionInformation 对应 IP_OPTION_INFORMATION（16 字节）。
type ipOptionInformation struct {
	Ttl         uint8
	Tos         uint8
	Flags       uint8
	OptionsSize uint8
	OptionsData *uint8
	// 尾部补齐 8 字节（偏移 4..7），OptionsData 落在偏移 8
}

// icmpEchoReply 对应 ICMP_ECHO_REPLY（40 字节）。
type icmpEchoReply struct {
	Address       uint32              // +0  IPAddr（内存字节即地址字节，见 ipAddrFromIPv4）
	Status        uint32              // +4  IP_SUCCESS=0 表示成功
	RoundTripTime uint32              // +8  毫秒
	DataSize      uint16              // +12
	Reserved      uint16              // +14
	Data          *uint8              // +16（对齐 8，14→16 补齐 2 字节）
	Options       ipOptionInformation // +24
	// sizeof = 40
}

// Address family。
const (
	afUnspec = 0
	afInet   = 2
	afInet6  = 23
)

// GetAdaptersAddresses 的 Flags。
const (
	gaaFlagSkipAnycast     = 0x0002
	gaaFlagSkipMulticast   = 0x0004
	gaaFlagSkipDnsServer   = 0x0008
	gaaFlagIncludePrefix   = 0x0010
	gaaFlagIncludeGateways = 0x0080
)

// AFUnspec 是 AddressFamily 的 AF_UNSPEC：同时取得 IPv4 与 IPv6。
// 导出它是因为只查 IPv4 会漏掉"仅 IPv6"的机器，而那恰恰是最需要诊断报告的机器。
const AFUnspec = afUnspec

// DefaultAdapterFlags 是本工具采集网卡时使用的标准 flags 组合。
// 含 gaaFlagIncludeGateways：FirstGatewayAddress 只在这个标志下才有内容，没有它就无法做网关 ICMP 探测。
// 不含 gaaFlagSkipDnsServer：DNS 服务器列表正是 DNS 判定所需的判据。
const DefaultAdapterFlags = gaaFlagIncludeGateways | gaaFlagSkipAnycast | gaaFlagSkipMulticast

// GetAdaptersAddresses 的错误码。ERROR_SUCCESS 即 0。
const (
	errorSuccess              = 0
	errorBufferOverflow       = 111
	errorNoData               = 232
	errorAddressNotAssociated = 1228
)

// IANA ifType 取值（只列 MVP 关心的）。
const (
	ifTypeEthernetCSMACD   = 6
	ifTypePPP              = 23
	ifTypeSoftwareLoopback = 24
	ifTypeIEEE80211        = 71
	ifTypeTunnel           = 131
)

// IF_OPER_STATUS。
const (
	ifOperStatusUp             = 1
	ifOperStatusDown           = 2
	ifOperStatusTesting        = 3
	ifOperStatusUnknown        = 4
	ifOperStatusDormant        = 5
	ifOperStatusNotPresent     = 6
	ifOperStatusLowerLayerDown = 7
)

// 以下导出常量供 collect 层做映射。
// 只导出数值而不导出「中文名」：6 该叫 "Ethernet" 还是"以太网"属于领域表达，应留在 internal/collect。
const (
	IfTypeEthernetCSMACD   = ifTypeEthernetCSMACD
	IfTypePPP              = ifTypePPP
	IfTypeSoftwareLoopback = ifTypeSoftwareLoopback
	IfTypeIEEE80211        = ifTypeIEEE80211
	IfTypeTunnel           = ifTypeTunnel
)

const (
	IfOperStatusUp             = ifOperStatusUp
	IfOperStatusDown           = ifOperStatusDown
	IfOperStatusTesting        = ifOperStatusTesting
	IfOperStatusUnknown        = ifOperStatusUnknown
	IfOperStatusDormant        = ifOperStatusDormant
	IfOperStatusNotPresent     = ifOperStatusNotPresent
	IfOperStatusLowerLayerDown = ifOperStatusLowerLayerDown
)

// ICMP 状态码。
const (
	ipSuccess             = 0
	ipDestNetUnreachable  = 11002
	ipDestHostUnreachable = 11003
	ipDestProtUnreachable = 11004
	ipDestPortUnreachable = 11005
	ipReqTimedOut         = 11010
	ipBadDestination      = 11018
	ipGeneralFailure      = 11050
)

// AFT 位域在 Flags 中的含义（只列 MVP 用到的）。
const (
	adapterFlagDhcpv4Enabled = 0x00000004
)
