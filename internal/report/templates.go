//go:build windows

// Package report 负责 Desktop-Diag 的报告渲染与写入。
//
// 只允许依赖标准库与 internal/model。版本号与主机环境由编排层通过 RenderContext
// 注入，本包因此可以脱离真机做字节级测试。内部一律以 "\n" 生成，由 WriteCRLF 转
// CRLF（REQ-F-505），落盘时由 writer.go 写 UTF-8 BOM（REQ-F-504）。
package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

const (
	// lineWidth 是分隔线与分节标题宽度，与设计文档 8.2 的模板一致。
	lineWidth = 80
	// indentMain、indentInner、indentEvidence 依次是第一层条目、第二层字段、证据行的缩进。
	indentMain     = "  "
	indentInner    = "   "
	indentEvidence = "        "
	// kvPad 是键的补齐宽度；按 rune 计，否则中文键的冒号会错位（见 kvWidth）。
	kvPad = 12
	// notCollected 是所有缺失数据的统一文案 —— 一律显式输出，留空会被误读成「正常」。
	notCollected = "未采集"
	// emptySource 是第三层附录在 RawLine.Source 为空时的占位来源。
	emptySource = "未标注来源"
)

// 三层结构的固定标题（REQ-F-501）与第二层内置分节名。
const (
	sectionTitleLayer1  = "【第一层】异常告警汇总"
	sectionTitleLayer2  = "【第二层】结构化检测数据详情"
	sectionTitleLayer3  = "【第三层】原始数据附录"
	layer2SubHost       = "主机与系统"
	layer2SubAdapters   = "网络适配器"
	layer2SubHealth     = "系统健康度"
	layer2SubConnect    = "网络连通性与故障层级"
	layer3SubsectionRaw = "原始采集数据"
)

const (
	reportTitle = "Desktop-Diag 桌面运维一键诊断报告"
	// readOnlyFooter 是报告收尾声明，对应只读红线（C-01/C-02）。
	readOnlyFooter = "报告结束 · 本工具仅执行只读诊断，未修改任何系统配置"
	// noIssueConclusion 是无告警时的正向结论（REQ-F-404）：留空或只写「正常」都不够，
	// 必须让运维一眼看出「查过了，没问题」。
	noIssueConclusion = "✅ 未发现异常项：本次诊断覆盖的全部检查项均正常"
	// incompleteConclusion 是存在采集失败时的第一层补充提示（REQ-F-405 / Q4）。
	incompleteConclusion = "⚠ 诊断不完整：下列数据未能采集，结论可能不完整"
)

// 第二层未采集标注的文案（Q4：权限不足必须显式说明，不能静默缺数据）。
const (
	notCollectedPerm    = "⚠ 因权限不足未采集"
	notCollectedOther   = "⚠ 未采集"
	notCollectedPartial = "⚠ 部分未采集"
	permReasonPrefix    = "（原因："
	reasonSuffix        = "）"
)

// permKeywords 用于把采集失败原因归类为权限不足。
//
// 用关键词而非错误类型：CollectFailure.Reason 是跨层传递的原文（可能是 Win32
// 错误字符串，也可能是 Go 的 os 错误文本），这里只做展示层措辞选择。
var permKeywords = []string{"拒绝访问", "权限", "Access is denied", "access denied"}

// repeatRune 返回 s 重复 n 次；n<=0 时返回空串，避免宽度常量被改成 0 时 panic。
func repeatRune(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	return strings.Repeat(s, n)
}

// separator 返回一条 80 字符的主分隔线。
func separator() string { return repeatRune("=", lineWidth) }

// rule 返回一条 80 字符的细分隔线（分节内部使用）。
func rule() string { return repeatRune("-", lineWidth) }

// bannerLine 把文字居中放进一行分隔线宽度内；宽度按 rune 计。
func bannerLine(text string) string {
	runes := []rune(text)
	if len(runes) >= lineWidth {
		return text
	}
	left := (lineWidth - len(runes)) / 2
	return repeatRune(" ", left) + text
}

// field 渲染报告头的一行「键    : 值」，值空时回落「未采集」。
func field(key, value string) string {
	return kvWidth(key, orNotCollected(value), kvPad, indentMain)
}

// kv 渲染第二层的一行「键    : 值」。
func kv(key, value string) string {
	return kvWidth(key, orNotCollected(value), kvPad, indentInner)
}

// kvWidth 渲染一行「键 : 值」，键按 rune 补齐到 width。
//
// 必须按 rune 补齐：fmt 的 %-12s 按字节计宽，而一个中文字符占 3 字节，
// 「故障层级判定」这类 6 字键会直接顶满 18 字节、补不出空格，
// 于是同一节里中英文键的冒号落不到同一列。
func kvWidth(key, value string, width int, indent string) string {
	pad := width - len([]rune(key))
	if pad < 0 {
		pad = 0
	}
	return indent + key + repeatRune(" ", pad) + ": " + value
}

// orNotCollected 把空值统一替换为「未采集」。
func orNotCollected(v string) string {
	if strings.TrimSpace(v) == "" {
		return notCollected
	}
	return v
}

// joinOr 用 sep 连接非空元素；全空时返回 fallback。
func joinOr(items []string, sep, fallback string) string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s := strings.TrimSpace(it); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return strings.Join(out, sep)
}

// formatBytes 按 GiB（1024³）换算并显示为 GB（设计文档 7.1 注、RK-05），统一一位小数。
func formatBytes(b uint64) string {
	const gib = 1024 * 1024 * 1024
	return fmt.Sprintf("%.1f GB", float64(b)/float64(gib))
}

// formatPercent 统一一位小数的百分比。
func formatPercent(p float64) string {
	return fmt.Sprintf("%.1f%%", p)
}

// formatDuration 把时长格式化为「X 天 Y 小时 Z 分」（REQ-F-103）。
//
// 不足一天时不输出「0 天」，零值输出「0 分」而不是空串。
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	mins := int(d / time.Minute)

	var sb strings.Builder
	if days > 0 {
		fmt.Fprintf(&sb, "%d 天 ", days)
	}
	if days > 0 || hours > 0 {
		fmt.Fprintf(&sb, "%d 小时 ", hours)
	}
	fmt.Fprintf(&sb, "%d 分", mins)
	return sb.String()
}

// formatMillis 把时长渲染成整数毫秒；负数与零统一为 0。
func formatMillis(d time.Duration) int {
	ms := int(math.Round(float64(d) / float64(time.Millisecond)))
	if ms < 0 {
		return 0
	}
	return ms
}

// formatSeconds 渲染总耗时，保留一位小数。
func formatSeconds(d time.Duration) string {
	secs := float64(d) / float64(time.Second)
	if secs < 0 || math.IsNaN(secs) {
		secs = 0
	}
	return fmt.Sprintf("%.1f 秒", secs)
}

// formatTime 统一报告中的时间格式（本地时间，秒级）；零值输出「未采集」。
func formatTime(t time.Time) string {
	if t.IsZero() {
		return notCollected
	}
	return t.Format("2006-01-02 15:04:05")
}

// operStatusLabel 把 model.OperStatus* 翻成运维读得懂的中文（REQ-F-104）。
func operStatusLabel(s string) string {
	switch s {
	case "Up":
		return "已连接"
	case "Down":
		return "已断开"
	case "Testing":
		return "正在测试"
	case "Unknown":
		return "状态未知"
	case "Dormant":
		return "休眠（等待链路事件）"
	case "NotPresent":
		return "设备不存在"
	case "LowerLayerDown":
		return "下层链路断开"
	case "":
		return notCollected
	default:
		return s
	}
}

// severityTag 返回第一层的等级标签。
//
// 用中文标签而非符号：报告要能在记事本、Excel 与纯 ASCII 终端里阅读，
// 符号（✅ ⚠）只用于「有/无告警」这类整句结论。
func severityTag(s model.Severity) string {
	switch s {
	case model.SevSevere:
		return "[严重]"
	case model.SevWarning:
		return "[警告]"
	case model.SevOK:
		return "[正常]"
	default:
		return "[未知]"
	}
}

// sortStrings 排序并去重，使输出与 map 迭代顺序无关。
func sortStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// isPermissionReason 判断一段失败原因原文是否属于权限类。
func isPermissionReason(reason string) bool {
	for _, kw := range permKeywords {
		if strings.Contains(reason, kw) {
			return true
		}
	}
	return false
}

// failureLabel 生成第二层的未采集标注文案：Q4 要求权限类必须逐字写作
// 「⚠ 因权限不足未采集（原因：X）」，因此是否权限与是否部分成功分开判断。
func failureLabel(f model.CollectFailure) string {
	var head string
	switch {
	case isPermissionReason(f.Reason):
		head = notCollectedPerm
	case f.Partial:
		head = notCollectedPartial
	default:
		head = notCollectedOther
	}
	if strings.TrimSpace(f.Reason) == "" {
		return head
	}
	return head + permReasonPrefix + model.ChineseReason(f.Reason) + reasonSuffix
}

// failureShort 生成一行简短摘要，供第一层「诊断不完整」清单使用。
func failureShort(f model.CollectFailure) string {
	item := strings.TrimSpace(f.Item)
	if item == "" {
		item = orNotCollected(f.EnvVar)
	}
	// 清单要短：原因原文可能很长（含状态码/路径），截断避免撑爆行宽。
	reason := model.ChineseReason(f.Reason)
	if reason == "" {
		reason = "原因未知"
	}
	if r := []rune(reason); len(r) > 60 {
		reason = string(r[:60]) + "…"
	}
	return item + "（原因: " + reason + "）"
}

// probeParamsNote 是 REQ-F-206 要求的探测参数固定化说明。
const probeParamsNote = "探测参数: ICMP 4 包 / 单包超时 1s；TCP 连接超时 3s；DNS 查询超时 3s（固定值，不可配置）"
