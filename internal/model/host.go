// 本文件定义主机、网卡与地址模型及诊断对象的选择规则。
package model

import (
	"net"
	"strings"
	"time"
)

// Host 描述运行本工具的终端主机与账户。
type Host struct {
	ComputerName string
	UserName     string
	IsAdmin      bool
	OSName       string // "Windows 11 Pro for Workstations"
	OSVersion    string // "10.0.26200"
	OSBuild      string // "26200"
	OSArch       string // "AMD64"
	Uptime       time.Duration
	BootTime     time.Time // StartedAt - Uptime，近似值
}

// IfType 取值来自 IP_ADAPTER_ADDRESSES_LH.IfType（IANA ifType）；只保留 MVP 关心的少数几类，
// 其余统一落入 IfTypeOther，避免规则函数里出现裸字符串。
const (
	IfTypeEthernet  = "Ethernet"  // IF_TYPE_ETHERNET_CSMACD (6)
	IfTypeIEEE80211 = "IEEE80211" // IF_TYPE_IEEE80211 (71)
	IfTypeTunnel    = "Tunnel"    // IF_TYPE_TUNNEL (131)
	IfTypeLoopback  = "Loopback"  // IF_TYPE_SOFTWARE_LOOPBACK (24)
	IfTypePPP       = "PPP"       // IF_TYPE_PPP (23)
	IfTypeOther     = "Other"
)

// OperStatus 取值来自 IF_OPER_STATUS；IsActive 只认 OperStatusUp。
const (
	OperStatusUp             = "Up"
	OperStatusDown           = "Down"
	OperStatusTesting        = "Testing"
	OperStatusUnknown        = "Unknown"
	OperStatusDormant        = "Dormant"
	OperStatusNotPresent     = "NotPresent"
	OperStatusLowerLayerDown = "LowerLayerDown"
)

// Scope 取值；IPv4 也填 Scope：169.254.0.0/16 → LinkLocal，127.0.0.0/8 → Other，其余 → Global。
// IPv6OnlyLinkLocal 只读 a.IPv6 的 Scope，两种协议族的语义不会互相污染。
const (
	ScopeGlobal    = "Global"
	ScopeLinkLocal = "LinkLocal"
	ScopeSiteLocal = "SiteLocal"
	ScopeOther     = "Other"
)

const (
	VirtualVMware     = "VMware"
	VirtualHyperV     = "Hyper-V"
	VirtualVirtualBox = "VirtualBox"
	VirtualTAP        = "TAP"
	VirtualWireGuard  = "WireGuard"
	VirtualLoopback   = "Loopback"
	// VirtualOverlay 是覆盖网络（ZeroTier、Tailscale、Hamachi、Radmin VPN 等）网卡。
	//
	// 必须单独归类：这类网卡是 Up 状态的软件接口，且自带一个合成网关地址
	// （实测 ZeroTier 为 25.255.255.254，属 IPv4 保留段），对它做 ICMP 探测只会得到
	// "100% 丢包"，进而被误读成"内网链路中断"。
	VirtualOverlay = "Overlay"
	VirtualOther   = "Other"
)

// DNSSource 取值。注册表 Tcpip 接口键不可读时必须是 DNSSourceMissing，此时触发 R-19
// 而**不得**触发 R-03，否则会把"读不到"误报成"没配置"。
const (
	DNSSourceGetAdaptersAddresses = "GetAdaptersAddresses"
	DNSSourceRegistry             = "Registry"
	DNSSourceMissing              = "未采集"
)

// APIPAPrefix 是 IPv4 自动专用 IP 寻址（APIPA）前缀，R-01 判定依据。
const APIPAPrefix = "169.254."

// Addr 是一个网卡地址，同时承载 IPv4 与 IPv6 的差异字段。
type Addr struct {
	IP     string
	Mask   string // 点分十进制，仅 IPv4 填写
	Prefix int    // 前缀长度
	Scope  string // 见 Scope* 常量
}

// IsAPIPA 报告该地址是否落在 169.254.0.0/16（R-01 的输入）。
func (a Addr) IsAPIPA() bool {
	return strings.HasPrefix(a.IP, APIPAPrefix)
}

// IsLoopback 报告该地址是否为回环地址（127.0.0.0/8 或 ::1）。
func (a Addr) IsLoopback() bool {
	ip := net.ParseIP(a.IP)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// Adapter 描述一块网络适配器。
type Adapter struct {
	Index          uint32
	ID             string // 接口 GUID 或禁用设备实例 ID，用于稳定关联。
	Name           string // FriendlyName，如 "以太网"
	Description    string // 硬件描述，如 "Realtek PCIe GbE Family Controller"
	MAC            string // "AA-BB-CC-DD-EE-FF"，无 MAC 时为空
	IfType         string // 见 IfType* 常量
	OperStatus     string // 见 OperStatus* 常量
	AdminEnabled   bool   // 接口管理启用状态，不推断一定由设备管理器禁用。
	AdminKnown     bool   // 是否已读取管理状态，未知不得伪装成已禁用。
	AddressMissing bool   // 地址 API 未返回该接口；空 IP/网关/DNS 不证明未配置。
	IsVirtual      bool   // 虚拟网卡识别结果
	VirtualKind    string // 见 Virtual* 常量，非虚拟时为空
	IPv4           []Addr
	IPv6           []Addr
	Gateways       []string // IPv4 默认网关（FirstGatewayAddress + 注册表兜底）
	// GatewaysV6 是 IPv6 默认网关，仅供报告展示与「是否存在默认路由」判断。
	// 必须与 Gateways 分开：混进去会让只支持 IPv4 的 ICMP 探测失败并误报 R-19；
	// 整条丢弃又会让仅 IPv6 的机器被 R-02 误判为「无网关配置」。
	GatewaysV6  []string
	DNS         []string // DNS 服务器（FirstDnsServerAddress + 注册表兜底）
	DNSSource   string   // 见 DNSSource* 常量
	DHCPEnabled bool
	DHCPKnown   bool // 地址 API 是否提供 DHCP 状态；仅接口清单时为 false。
}

// IsActive 报告链路是否已建立（R-01/R-02/R-03 的第一道门槛）。
func (a Adapter) IsActive() bool {
	return a.OperStatus == OperStatusUp && (!a.AdminKnown || a.AdminEnabled)
}

// HasUsableIPv4 报告该网卡上是否存在「可用于对外通信」的 IPv4 地址：
// 非 APIPA（DHCP 失败产物）且非回环。该定义使 R-01 与 R-05 不会互相矛盾。
func (a Adapter) HasUsableIPv4() bool {
	for _, addr := range a.IPv4 {
		ip := net.ParseIP(addr.IP)
		if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || addr.IsAPIPA() || addr.IsLoopback() {
			continue
		}
		return true
	}
	return false
}

// HasAnyIPv4 报告该网卡上是否存在任何 IPv4 地址（含 APIPA 与回环）。
func (a Adapter) HasAnyIPv4() bool {
	return len(a.IPv4) > 0
}

// HasDNS 报告 DNS 列表是否非空（R-03 的输入）。
func (a Adapter) HasDNS() bool {
	for _, d := range a.DNS {
		if strings.TrimSpace(d) != "" {
			return true
		}
	}
	return false
}

// HasGateway 报告该网卡是否配置了默认网关，IPv4 与 IPv6 都算（R-02 的输入）。
// 只看 IPv4 会把「仅 IPv6 出口」的机器误判成无法联网（R-02，SEVERE）。
func (a Adapter) HasGateway() bool {
	for _, g := range a.Gateways {
		if strings.TrimSpace(g) != "" {
			return true
		}
	}
	for _, g := range a.GatewaysV6 {
		if strings.TrimSpace(g) != "" {
			return true
		}
	}
	return false
}

// FirstGateway 返回第一个非空 IPv4 网关，无则返回空串。
// 只返回 IPv4：返回值会直接交给只支持 IPv4 的 ICMP 探测。
func (a Adapter) FirstGateway() string {
	for _, g := range a.Gateways {
		if strings.TrimSpace(g) != "" {
			return g
		}
	}
	return ""
}

// AddrStrings 返回该网卡的 IP 地址纯文本列表（不含掩码），用于证据展示。
func (a Adapter) AddrStrings() []string {
	out := make([]string, 0, len(a.IPv4)+len(a.IPv6))
	for _, addr := range a.IPv4 {
		out = append(out, addr.IP)
	}
	for _, addr := range a.IPv6 {
		out = append(out, addr.IP)
	}
	return out
}

// DisplayName 返回面向用户的网卡名称：优先 FriendlyName（网络连接面板里显示的名字），
// 为空时回落到硬件描述；两者都空时给明确占位符而不是空串 —— 报告里出现「网卡「」」
// 会让用户无法判断是哪块网卡出了问题。
func (a Adapter) DisplayName() string {
	if s := strings.TrimSpace(a.Name); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.Description); s != "" {
		return s
	}
	return "未命名网卡"
}
