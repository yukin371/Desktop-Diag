//go:build windows

package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/yukin371/desktop-diag/internal/model"
)

// addrKeyPad 是 IPv4/IPv6 地址行的键列宽度（比 kvPad 窄，给地址值留位置）。
const addrKeyPad = 9

// writeLine 输出一行并补上换行。
//
// 统一走这里而不是直接 Fprintf，是为了让每一行的换行符只有一处产出。
func writeLine(w io.Writer, line string) {
	fmt.Fprintln(w, line)
}

// SectionRenderer 渲染第二层中的一节检测详情。
//
// 必须是纯函数：只读 snap、只写 w、不 panic；数据缺失时输出「未采集」文案，
// 不得静默跳过（Q4）。
type SectionRenderer func(w io.Writer, snap *model.Snapshot)

// sectionEntry 是分节注册表的一项。
type sectionEntry struct {
	Title  string
	Order  int
	Render SectionRenderer
}

// 锁保护运行期注册（测试与 V1.1 采集域可能在 init 之后调用 RegisterSection）。
var (
	sectionsMu sync.RWMutex
	sections   []sectionEntry
)

// RegisterSection 注册一个第二层分节（REQ-N-09 的扩展点）。
//
// order 小的在前，同 order 按标题排序：排序键必须完全由注册参数决定，
// 否则注册顺序会泄漏进报告字节，破坏可复现性（REQ-N-08）。
//
// 标题为空或 r 为 nil 时忽略；重复标题覆盖前一项。
func RegisterSection(title string, order int, r SectionRenderer) {
	if strings.TrimSpace(title) == "" || r == nil {
		return
	}
	sectionsMu.Lock()
	defer sectionsMu.Unlock()
	for i := range sections {
		if sections[i].Title == title {
			sections[i] = sectionEntry{Title: title, Order: order, Render: r}
			return
		}
	}
	sections = append(sections, sectionEntry{Title: title, Order: order, Render: r})
}

// Sections 返回已注册分节的副本，按 (Order, Title) 升序。
func Sections() []sectionEntry {
	sectionsMu.RLock()
	out := make([]sectionEntry, len(sections))
	copy(out, sections)
	sectionsMu.RUnlock()

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// 第二层内置分节的顺序号；留 10 的间隔，V1.1 的新域插在中间即可。
const (
	orderHost     = 10
	orderAdapters = 20
	orderHealth   = 30
	orderProbe    = 40
)

// 内置分节在 init() 中注册，V1.1 新增采集域只需再调一次 RegisterSection。
func init() {
	RegisterSection(layer2SubHost, orderHost, renderHostSection)
	RegisterSection(layer2SubAdapters, orderAdapters, renderAdapterSection)
	RegisterSection(layer2SubHealth, orderHealth, renderHealthSection)
	RegisterSection(layer2SubConnect, orderProbe, renderProbeSection)
}

func renderHostSection(w io.Writer, snap *model.Snapshot) {
	h := snap.Host

	osLine := strings.TrimSpace(h.OSName)
	if osLine == "" {
		osLine = notCollected
	}
	if v := strings.TrimSpace(h.OSVersion); v != "" {
		osLine += " (" + v + ")"
	}
	if a := strings.TrimSpace(h.OSArch); a != "" {
		osLine += " " + a
	}

	writeKV(w, "计算机名", orNotCollected(h.ComputerName))
	writeKV(w, "运行账户", accountLine(h))
	writeKV(w, "操作系统", osLine)
	if b := strings.TrimSpace(h.OSBuild); b != "" {
		writeKV(w, "系统内部版本", b)
	}
	writeKV(w, "系统运行时长", formatDuration(h.Uptime))
	if !h.BootTime.IsZero() {
		writeKV(w, "开机时间(近似)", formatTime(h.BootTime))
	}
	writeKV(w, "本次诊断时间", formatTime(snap.StartedAt))

	// 采集失败清单只在主机节标注一次：主机信息是权限不足最集中的地方。
	writeFailures(w, snap.Failures)
}

// accountLine 渲染「user （管理员权限: 否）」形式的运行账户一行。
//
// 管理员标记紧跟账户名，不做列对齐：账户名长度不可控，
// 强行对齐反而会出现「CORP\alice            : 管理员权限」这种双冒号。
func accountLine(h model.Host) string {
	user := strings.TrimSpace(h.UserName)
	if user == "" {
		user = notCollected
	}
	admin := "否"
	if h.IsAdmin {
		admin = "是"
	}
	return user + " （管理员权限: " + admin + "）"
}

// 网络适配器

func renderAdapterSection(w io.Writer, snap *model.Snapshot) {
	total := len(snap.Adapters)
	active := 0
	for _, a := range snap.Adapters {
		if a.IsActive() {
			active++
		}
	}

	// 标题行带汇总，符合 REQ-F-104「枚举全部网卡（含已禁用）」的可核对性。
	fmt.Fprintf(w, "%s共 %d 块，已连接 %d 块\n", indentInner, total, active)
	fmt.Fprintf(w, "%s\n", kv("IPv6 概况", adapterV6Summary(snap)))

	if total == 0 {
		// 无网卡是有效结论（S-01），必须显式说明，不能留空节。
		fmt.Fprintf(w, "%s%s\n", indentInner, "本机未枚举到任何网络适配器")
		return
	}

	for i, a := range snap.Adapters {
		// 各块之间用一条细分隔线隔开：多网卡时这份清单很长，
		// 没有分隔线会让相邻两块（尤其是第二块的「状态」行）粘在一起难以定位。
		if i > 0 {
			fmt.Fprintf(w, "%s%s\n", indentInner, rule())
		}
		renderAdapter(w, i+1, a)
	}
}

// adapterV6Summary 汇总 IPv6 概况（REQ-F-108）。
func adapterV6Summary(snap *model.Snapshot) string {
	v6 := 0
	for _, a := range snap.Adapters {
		v6 += len(a.IPv6)
	}
	if v6 == 0 {
		return "无地址"
	}
	if snap.IPv6OnlyLinkLocal() {
		return fmt.Sprintf("%d 个地址（仅链路本地）", v6)
	}
	return fmt.Sprintf("%d 个地址", v6)
}

func renderAdapter(w io.Writer, n int, a model.Adapter) {
	name := a.DisplayName()
	fmt.Fprintf(w, "%s[%d] %s\n", indentInner, n, name)

	desc := strings.TrimSpace(a.Description)
	if desc != "" && desc != name {
		fmt.Fprintf(w, "%s描述     : %s\n", indentInner, desc)
	}

	state := operStatusLabel(a.OperStatus)
	if !a.AdminEnabled {
		// AdminEnabled=false 表示设备管理器里被禁用；这与「链路 Down」是两回事。
		state += "（设备已被禁用）"
	} else if !a.IsActive() {
		state = "已断开"
	}
	fmt.Fprintf(w, "%s状态     : %s\n", indentInner, state)

	fmt.Fprintf(w, "%s类型     : %s\n", indentInner, orNotCollected(a.IfType))
	if a.Index != 0 {
		fmt.Fprintf(w, "%s接口索引 : %d\n", indentInner, a.Index)
	}

	if a.IsVirtual {
		kind := strings.TrimSpace(a.VirtualKind)
		if kind == "" {
			kind = "未知类型"
		}
		fmt.Fprintf(w, "%s虚拟网卡 : 是（%s）\n", indentInner, kind)
	} else {
		fmt.Fprintf(w, "%s虚拟网卡 : 否\n", indentInner)
	}

	fmt.Fprintf(w, "%sMAC      : %s\n", indentInner, orNotCollected(a.MAC))
	renderAddrs(w, "IPv4", a.IPv4, "无 IPv4 地址")
	renderAddrs(w, "IPv6", a.IPv6, "无 IPv6 地址")

	// REQ-F-106 / REQ-F-107：无网关、无 DNS 必须显式写「未配置」而不是空白。
	gateway := joinOr(a.Gateways, ", ", "未配置")
	if a.IsActive() && !a.HasGateway() {
		gateway = "未配置"
	}
	fmt.Fprintf(w, "%s网关     : %s\n", indentInner, gateway)

	// IPv6 网关单独一行：本工具只对 IPv4 网关做探测，合并进上一行会让人以为它也测过。
	if v6 := joinOr(a.GatewaysV6, ", ", ""); v6 != "" {
		fmt.Fprintf(w, "%sIPv6 网关: %s\n", indentInner, v6)
	}

	dns := joinOr(a.DNS, ", ", "未配置")
	switch a.DNSSource {
	case model.DNSSourceMissing:
		// S-17：读不到 ≠ 没配置。这里必须区分，否则运维会去查一个并不存在的配置问题。
		dns = notCollected + "（注册表接口键不可读，无法判定是否配置了 DNS）"
	case "":
		if !a.HasDNS() {
			dns = "未配置"
		}
	default:
		dns = fmt.Sprintf("%s  [来源: %s]", dns, a.DNSSource)
	}
	fmt.Fprintf(w, "%sDNS      : %s\n", indentInner, dns)

	if a.DHCPKnown {
		on := "未启用"
		if a.DHCPEnabled {
			on = "已启用"
		}
		fmt.Fprintf(w, "%sDHCP     : %s\n", indentInner, on)
	} else {
		fmt.Fprintf(w, "%sDHCP     : %s（注册表不可读，无法判定）\n", indentInner, notCollected)
	}
}

// renderAddrs 渲染一组地址（IPv4 带掩码/前缀，IPv6 带 Scope）。
func renderAddrs(w io.Writer, label string, addrs []model.Addr, emptyText string) {
	if len(addrs) == 0 {
		writeLine(w, kvWidth(label, emptyText, addrKeyPad, indentInner))
		return
	}
	for i, ad := range addrs {
		key := label
		if i > 0 {
			// 续行占满键列（而不是打一个空键），否则第二条地址会左移、与 ipconfig 对不上。
			key = strings.Repeat(" ", len([]rune(label)))
		}
		writeLine(w, kvWidth(key, formatAddr(ad), addrKeyPad, indentInner))
	}
}

// formatAddr 渲染单个地址。
//
// IPv4 显示点分掩码 + 前缀长度（与 ipconfig 可逐字段比对）；
// IPv6 显示前缀长度 + Scope（REQ-F-108 的「仅链路本地」标记即来自 Scope）。
func formatAddr(a model.Addr) string {
	s := strings.TrimSpace(a.IP)
	if s == "" {
		return notCollected
	}
	if a.Prefix > 0 {
		s += fmt.Sprintf("/%d", a.Prefix)
	}
	if m := strings.TrimSpace(a.Mask); m != "" {
		s += "  掩码: " + m
	}
	if sc := strings.TrimSpace(a.Scope); sc != "" {
		s += "  Scope: " + scopeLabel(sc)
	}
	if a.IsAPIPA() {
		// 169.254 是 DHCP 失败的产物，就地标注可免去用户对照第一层的麻烦。
		s += "  （APIPA 自动专用地址，DHCP 未成功）"
	}
	return s
}

func scopeLabel(s string) string {
	switch s {
	case model.ScopeGlobal:
		return "Global 全局"
	case model.ScopeLinkLocal:
		return "LinkLocal 链路本地"
	case model.ScopeSiteLocal:
		return "SiteLocal 站点本地"
	case model.ScopeOther:
		return "Other"
	default:
		return s
	}
}

// 系统健康度

func renderHealthSection(w io.Writer, snap *model.Snapshot) {
	h := snap.Health

	if h.CPUKnown {
		writeKV(w, "CPU 占用率", formatPercent(h.CPUPercent))
	} else {
		// 采集失败不得显示 0%（model.Health 的注释明确了这一点）。
		writeKV(w, "CPU 占用率", notCollected+"（采样失败）")
	}

	if h.MemKnown {
		writeKV(w, "内存总量", formatBytes(h.MemTotalBytes))
		writeKV(w, "内存已用", fmt.Sprintf("%s / 共 %s (%s)",
			formatBytes(h.MemUsedBytes()), formatBytes(h.MemTotalBytes), formatPercent(h.MemUsedPercent)))
		writeKV(w, "内存可用", formatBytes(h.MemAvailBytes))
	} else {
		writeKV(w, "内存", notCollected+"（GlobalMemoryStatusEx 调用失败）")
	}

	if h.DiskKnown {
		drive := strings.TrimSpace(h.SystemDrive)
		if drive == "" {
			drive = notCollected
		}
		used := formatBytes(h.DiskUsedBytes())
		pct := 0.0
		if h.DiskTotalBytes > 0 {
			pct = float64(h.DiskUsedBytes()) / float64(h.DiskTotalBytes) * 100
		}
		writeKV(w, "系统盘", fmt.Sprintf("%s  已用 %s / 共 %s (%s)，剩余 %s",
			drive, used, formatBytes(h.DiskTotalBytes), formatPercent(pct), formatBytes(h.DiskFreeBytes)))
	} else {
		writeKV(w, "系统盘", notCollected+"（GetDiskFreeSpaceExW 调用失败）")
	}
}

// 网络连通性与故障层级

func renderProbeSection(w io.Writer, snap *model.Snapshot) {
	if len(snap.Probes) == 0 {
		writeKV(w, "连通性探测", notCollected+"（本次诊断未产生任何探测结果）")
	} else {
		// 按 model.Probes 的既有顺序渲染：顺序由 detect/probe 层固定，
		// 这里不重新排序，避免报告与判定引擎看到不同的顺序。
		for _, p := range snap.Probes {
			renderProbe(w, p)
		}
	}

	fmt.Fprintf(w, "%s%s\n", indentInner, probeParamsNote)

	if len(snap.Layers) == 0 {
		writeKV(w, "故障层级判定", notCollected+"（缺少探测结果，无法定位故障层级）")
	} else {
		layers := append([]model.LayerConclusion(nil), snap.Layers...)
		// 层级结论可能多条（多网卡场景）。固定按 LevelOrder 排序，
		// 保证与输入切片顺序无关 —— 这是 REQ-N-08 可复现性的一部分。
		sort.SliceStable(layers, func(i, j int) bool {
			li, lj := model.LevelOrder(layers[i].Level), model.LevelOrder(layers[j].Level)
			if li != lj {
				return li < lj
			}
			return layers[i].Level < layers[j].Level
		})
		for i, l := range layers {
			key := "故障层级判定"
			if i > 0 {
				// 续行占满键列，避免第 2 条结论以 ":" 开头（多网卡时很常见）。
				key = strings.Repeat(" ", len([]rune("故障层级判定")))
			}
			writeLine(w, kvWidth(key, layerTag(l)+orNotCollected(l.Summary), kvPad, indentInner))
		}
	}

	// 明确出口归属的局限（RK-07）：多网卡时 IcmpSendEcho 由系统路由决定出口。
	if len(snap.Adapters) > 1 {
		fmt.Fprintf(w, "%s注：ICMP 探测的实际出口由系统路由表决定，报告按探测归属网卡分组。\n", indentInner)
	}
}

func renderProbe(w io.Writer, p model.ProbeResult) {
	label := probeLabel(p)

	if p.Skipped {
		// Skipped 与成功/失败是两种语义（model.ProbeResult 注释）。
		reason := strings.TrimSpace(p.SkipReason)
		if reason == "" {
			reason = "未说明原因"
		}
		writeLine(w, kvWidth(label, "已跳过（原因: "+reason+"）", kvPad, indentInner))
		return
	}

	if !p.Success {
		errText := strings.TrimSpace(p.Err)
		if errText == "" {
			errText = "未返回成功且未提供失败原因"
		}
		writeLine(w, kvWidth(label, "失败（"+errText+"）", kvPad, indentInner))
		return
	}

	switch p.Kind {
	case model.ProbeICMPGateway:
		value := fmt.Sprintf("丢包 %s　延迟 平均 %d ms (min %d / max %d)　共 %d/%d 包",
			formatPercent(p.LossPercent),
			formatMillis(p.AvgRTT), formatMillis(p.MinRTT), formatMillis(p.MaxRTT), p.Recv, p.Sent)
		writeLine(w, kvWidth(label, value, kvPad, indentInner))
	case model.ProbeDNSSystem, model.ProbeDNSDirect:
		value := fmt.Sprintf("成功 → %s (%d ms)", joinOr(p.Resolved, ", ", "无解析结果"), formatMillis(p.Duration))
		writeLine(w, kvWidth(label, value, kvPad, indentInner))
	default:
		value := fmt.Sprintf("%s 可达 (%d ms)", orNotCollected(p.Target), formatMillis(p.Duration))
		writeLine(w, kvWidth(label, value, kvPad, indentInner))
	}
}

// probeLabel 生成「类型(归属 → 目标)」形式的短标签。
func probeLabel(p model.ProbeResult) string {
	label := p.Kind.KindLabel()
	if p.AdapterName != "" {
		label += "(" + p.AdapterName + ")"
	}
	if t := strings.TrimSpace(p.Target); t != "" {
		label += " → " + t
	}
	return label
}

// layerTag 给层级结论加上等级标记，让严重结论在一屏内可定位。
func layerTag(l model.LayerConclusion) string {
	switch l.Severity {
	case model.SevSevere:
		return "[严重] "
	case model.SevWarning:
		return "[警告] "
	case model.SevOK:
		return ""
	default:
		return ""
	}
}

// 未采集标注

// writeFailures 在第二层就地标注采集失败项（REQ-F-703、Q4）。
//
// 只在有失败项时输出；失败清单本身已在第一层汇总，这里给出的是
// 「哪一节缺了什么、为什么缺」，方便运维对照着看具体字段。
func writeFailures(w io.Writer, failures []model.CollectFailure) {
	if len(failures) == 0 {
		return
	}
	fmt.Fprintf(w, "%s%s\n", indentInner, incompleteConclusion)
	for _, f := range failures {
		fmt.Fprintf(w, "%s- %s：%s\n", indentInner, orNotCollected(f.Item), failureLabel(f))
	}
}

// writeKV 输出一行第二层键值对。
func writeKV(w io.Writer, key, value string) {
	writeLine(w, kv(key, value))
}
