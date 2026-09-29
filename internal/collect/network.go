//go:build windows

package collect

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// networkCollector 采集网络适配器信息。
type networkCollector struct{}

// Name 实现 Collector。
func (networkCollector) Name() string { return "网络适配器信息" }

// EnvVar 实现 envVarer。
func (networkCollector) EnvVar() string { return "network" }

// Collect 实现 Collector。
func (c networkCollector) Collect(ctx context.Context, snap *model.Snapshot) error {
	raw, err := winapi.GetAdaptersAddresses(winapi.AFUnspec, winapi.DefaultAdapterFlags)
	if err != nil {
		return fmt.Errorf("枚举网络适配器失败: %w", err)
	}

	adapters := make([]model.Adapter, 0, len(raw))
	for _, r := range raw {
		a := convertAdapter(r)

		// DNS 单独走一条兜底链，因为 GetAdaptersAddresses 在部分机器上
		// 会对"确实配了 DNS"的网卡返回空列表（这正是基线场景 S-17）。
		a.DNS, a.DNSSource = resolveDNS(r, snap)

		adapters = append(adapters, a)
	}

	// 按接口索引排序：GetAdaptersAddresses 的返回顺序在不同机器/不同
	// 运行时刻并不保证一致，而报告要求两次运行结果可比（REQ-N-08）。
	sort.SliceStable(adapters, func(i, j int) bool { return adapters[i].Index < adapters[j].Index })
	snap.Adapters = adapters

	for _, a := range adapters {
		snap.AddRaw("网络适配器", "GetAdaptersAddresses",
			fmt.Sprintf("IfIndex=%d 名称=%q 描述=%q 类型=%s 状态=%s MAC=%s 虚拟=%v 网关=%s DNS=%s(%s)",
				a.Index, a.Name, a.Description, a.IfType, a.OperStatus,
				orNone(a.MAC), a.IsVirtual, orNone(strings.Join(a.Gateways, ", ")),
				orNone(strings.Join(a.DNS, ", ")), a.DNSSource))
	}

	if len(adapters) == 0 {
		// 无网卡不是错误：这是一台需要被诊断的机器，不是采集失败。
		// 记为部分采集，让报告说明"没有任何网络适配器"，判定层则保持静默。
		snap.AddFailure(c.Name(), c.EnvVar(), "系统未返回任何网络适配器", true)
		return nil
	}
	return nil
}

// convertAdapter 把 winapi 的中立原始结构转成领域模型。
//
// 转换放在 collect 而不是 winapi：winapi 是叶子包，不能 import model；
// 而"IF_TYPE 6 叫什么名字"属于领域知识，不该混进 syscall 封装里。
func convertAdapter(r winapi.RawAdapter) model.Adapter {
	a := model.Adapter{
		Index:       r.IfIndex,
		Name:        cleanString(r.FriendlyName),
		Description: cleanString(r.Description),
		MAC:         cleanString(r.PhysicalAddress),
		IfType:      ifTypeName(r.IfType),
		OperStatus:  operStatusName(r.OperStatus),
		DHCPEnabled: r.Dhcpv4Enabled,
		DHCPKnown:   true,
		Gateways:    dropBlanks(r.Gateways),
	}

	// AdminEnabled 恒为 true，这是刻意的：GetAdaptersAddresses 不暴露管理启用位，
	// 「网卡被禁用」与「网线没插」在 IF_OPER_STATUS 上都可能是 Down。
	// 本工具宁可只说"状态 Down"，也不猜"网卡已被禁用"——后者会让运维
	// 直接去改一个可能本来就正确的配置。
	a.AdminEnabled = true

	a.IsVirtual, a.VirtualKind = detectVirtual(a.Name, a.Description, a.IfType)

	for _, v4 := range r.IPv4 {
		a.IPv4 = append(a.IPv4, model.Addr{
			IP:     v4.IP,
			Mask:   prefixToMask(v4.Prefix),
			Prefix: v4.Prefix,
			Scope:  classifyScope(v4.IP, false),
		})
	}
	for _, v6 := range r.IPv6 {
		a.IPv6 = append(a.IPv6, model.Addr{
			IP:     v6.IP,
			Mask:   fmt.Sprintf("/%d", v6.Prefix),
			Prefix: v6.Prefix,
			Scope:  classifyScope(v6.IP, true),
		})
	}
	return a
}

// resolveDNS 取得某块网卡的 DNS 服务器列表，并说明来源。
//
// 返回的 source 取值语义（**这三者必须区分清楚，否则 R-03 会误报**）：
//   - DNSSourceGetAdaptersAddresses：系统 API 直接给了 DNS，最可信。
//   - DNSSourceRegistry：API 没给，但注册表里查过了——
//     含"查了但确实没配"（合法为空，R-03 可以据此报警）。
//   - DNSSourceMissing：**没读到**（拒绝访问等）。此时我们不知道有没有配，
//     R-03 必须保持沉默，改由 R-19 报告"诊断不完整"。
func resolveDNS(r winapi.RawAdapter, snap *model.Snapshot) ([]string, string) {
	if len(r.DNS) > 0 {
		return r.DNS, model.DNSSourceGetAdaptersAddresses
	}

	// 注册表按 {GUID} 组织，没有 GUID 就定位不到子键，只能如实说"未采集"。
	if !strings.HasPrefix(r.AdapterName, "{") {
		return nil, model.DNSSourceGetAdaptersAddresses
	}

	path := winapi.RegPathTcpipInterfaces + `\` + r.AdapterName
	_, val, err := winapi.RegReadFirstString(registry.LOCAL_MACHINE, path,
		winapi.RegValueNameServer, winapi.RegValueDhcpNameServer)

	switch {
	case err == nil:
		list := winapi.ParseNameServerList(val)
		snap.AddRaw("网络适配器", `HKLM\`+path+`\{NameServer,DhcpNameServer}`,
			fmt.Sprintf("%s → %s", orNone(r.FriendlyName), orNone(strings.Join(list, ", "))))
		return list, model.DNSSourceRegistry

	case errors.Is(err, winapi.ErrRegNotFound):
		// 查过了，这台网卡确实没有配置任何 DNS。这是**结论**，不是失败。
		snap.AddRaw("网络适配器", `HKLM\`+path, "（无 NameServer / DhcpNameServer 值）")
		return nil, model.DNSSourceRegistry

	default:
		// 读不到 ≠ 没配置。必须记入诊断完整性，而不是报"DNS 未配置"。
		snap.AddFailure("网络适配器信息", "network",
			fmt.Sprintf("网卡 %s 的 DNS 配置无法读取（可能因权限受限）: %v",
				displayOr(r.FriendlyName, r.AdapterName), err), true)
		return nil, model.DNSSourceMissing
	}
}

// ifTypeName 把 IF_TYPE 数值映射为领域常量。
func ifTypeName(t uint32) string {
	switch t {
	case winapi.IfTypeEthernetCSMACD:
		return model.IfTypeEthernet
	case winapi.IfTypeIEEE80211:
		return model.IfTypeIEEE80211
	case winapi.IfTypeTunnel:
		return model.IfTypeTunnel
	case winapi.IfTypeSoftwareLoopback:
		return model.IfTypeLoopback
	case winapi.IfTypePPP:
		return model.IfTypePPP
	default:
		return model.IfTypeOther
	}
}

// operStatusName 把 IF_OPER_STATUS 数值映射为领域常量。
func operStatusName(s uint32) string {
	switch s {
	case winapi.IfOperStatusUp:
		return model.OperStatusUp
	case winapi.IfOperStatusDown:
		return model.OperStatusDown
	case winapi.IfOperStatusTesting:
		return model.OperStatusTesting
	case winapi.IfOperStatusDormant:
		return model.OperStatusDormant
	case winapi.IfOperStatusNotPresent:
		return model.OperStatusNotPresent
	case winapi.IfOperStatusLowerLayerDown:
		return model.OperStatusLowerLayerDown
	default:
		return model.OperStatusUnknown
	}
}

// classifyScope 判定一个地址的作用域。
//
// 只区分三种对诊断有意义的情况：链路本地（169.254/fe80，说明 DHCP 没拿到地址）、
// 站点本地（fec0 已废弃段与 fc00::/7 唯一本地地址）、以及其余全局地址。
// 回环地址归入 Other——它既不是"能上网"也不是"链路本地"。
func classifyScope(ip string, isV6 bool) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return model.ScopeOther
	}
	if parsed.IsLoopback() {
		return model.ScopeOther
	}
	if isV6 {
		if parsed.IsLinkLocalUnicast() {
			return model.ScopeLinkLocal
		}
		// fc00::/7（唯一本地地址）与已废弃的 fec0::/10 同属站点本地。
		if len(parsed) == net.IPv6len && (parsed[0]&0xfe) == 0xfc {
			return model.ScopeSiteLocal
		}
		if len(parsed) == net.IPv6len && parsed[0] == 0xfe && (parsed[1]&0xc0) == 0xc0 {
			return model.ScopeSiteLocal
		}
		return model.ScopeGlobal
	}
	if strings.HasPrefix(ip, model.APIPAPrefix) {
		return model.ScopeLinkLocal
	}
	return model.ScopeGlobal
}

// prefixToMask 把 IPv4 前缀长度转成点分掩码，便于报告里与 ipconfig 对照。
func prefixToMask(prefix int) string {
	if prefix < 0 || prefix > 32 {
		return ""
	}
	mask := net.CIDRMask(prefix, 32)
	return net.IP(mask).String()
}

// detectVirtual 判断一块网卡是否为虚拟/隧道网卡，并给出种类。
//
// 为什么必须判断：一台装了 VMware 或 Hyper-V 的机器会有 4~8 块虚拟网卡，
// 它们大多处于 Up 状态。若不排除，"网关探测"会去 ping VMware 的虚拟网关，
// 得出的"网关不通"对用户毫无意义——真实网络其实是好的。
func detectVirtual(name, description, ifType string) (bool, string) {
	if ifType == model.IfTypeLoopback {
		return true, model.VirtualLoopback
	}

	// 名称与描述都要看：厂商有时把标识放在描述里（"TAP-Windows Adapter V9"
	// 的 FriendlyName 常被系统改写为"以太网 2"）。
	hay := strings.ToLower(name + " " + description)

	switch {
	case strings.Contains(hay, "vmware"):
		return true, model.VirtualVMware
	case strings.Contains(hay, "hyper-v"), strings.Contains(hay, "hyperv"):
		return true, model.VirtualHyperV
	case strings.Contains(hay, "virtualbox"), strings.Contains(hay, "vbox"):
		return true, model.VirtualVirtualBox
	case strings.Contains(hay, "tap-windows"), strings.Contains(hay, "openvpn"):
		return true, model.VirtualTAP
	case strings.Contains(hay, "wireguard"), strings.Contains(hay, "wintun"):
		return true, model.VirtualWireGuard
	case strings.Contains(hay, "zerotier"), strings.Contains(hay, "tailscale"),
		strings.Contains(hay, "hamachi"), strings.Contains(hay, "radmin"),
		strings.Contains(hay, "softether"), strings.Contains(hay, "netbird"),
		strings.Contains(hay, "neorouter"):
		// 覆盖网络（overlay VPN）网卡。实测 ZeroTier 的网关是 25.255.255.254
		// ——一个 IPv4 保留段里的合成地址。把它当真实网关去 ping，
		// 只会得到"探测无法发起"或"100% 丢包"，后者足以让报告
		// 误判成内网链路中断（R-14，SEVERE）。
		return true, model.VirtualOverlay
	case strings.Contains(hay, "loopback"), strings.Contains(hay, "km-test"):
		return true, model.VirtualLoopback
	case strings.Contains(hay, "wi-fi direct"), strings.Contains(hay, "wifi direct"):
		// Wi-Fi Direct 虚拟适配器在无线网卡启用时也会是 Up，
		// 但它没有任何真实网关，混进来同样会污染网关探测结论。
		return true, model.VirtualOther
	}
	return false, ""
}

// cleanString 清掉系统字符串里可能带的空白与 NUL。
func cleanString(s string) string {
	return strings.TrimSpace(strings.TrimRight(s, "\x00"))
}

// dropBlanks 去掉空串元素。
func dropBlanks(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// orNone 把空值显示为「（无）」，避免报告里出现「网关：」这种让人怀疑采集失败的空白。
func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "（无）"
	}
	return s
}

// displayOr 在首选值为空时回落到备选值。
func displayOr(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
