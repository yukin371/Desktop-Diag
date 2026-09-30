//go:build windows

// Renders the deterministic three-layer diagnostic report.
package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// BOM 是 UTF-8 字节顺序标记（REQ-F-504）；缺了它记事本会按 ANSI 解析，中文全乱码。
const BOM = "\xEF\xBB\xBF"

// RenderContext 携带报告环境信息（版本、路径、耗时）。
//
// 刻意不放进 model.Snapshot：那是 collect→detect→report 的领域数据；
// 放在这里也让本包无需 import internal/version，版本由编排层注入。
type RenderContext struct {
	Version      string        // "v0.1.0"
	Commit       string        // "abc1234"
	BuildTime    string        // "2026-09-30T01:00:00Z"
	ExePath      string        // os.Executable() 的原始值，用于 UNC 检测
	ReportPath   string        // 报告实际落盘路径（未落盘时留空）
	PathNote     string        // 路径选择说明，例如 "已降级至用户目录（原因: ...）"
	TotalElapsed time.Duration // 采集与判定耗时，落盘后的计时在报告尾与控制台记录。
	GeneratedAt  time.Time     // 报告生成时刻（零值时回落 StartedAt）
	PlainText    bool          // 纯文本模式：报告为 TXT，默认即纯文本
}

// ReportSection 是三层结构中的一层。
type ReportSection struct {
	Title string
	Lines []string
}

// WriteCRLF 把 LF 换行统一转换为 CRLF（REQ-F-505）。
//
// 必须先归一再替换：直接 Replace("\n","\r\n") 遇到已含 \r\n 的输入会产出
// \r\r\n，记事本会多显示一个空行。
func WriteCRLF(w io.Writer, s string) (int, error) {
	normalized := strings.ReplaceAll(s, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, "\n", "\r\n")
	n, err := io.WriteString(w, normalized)
	if err == nil && n != len(normalized) {
		err = io.ErrShortWrite
	}
	return n, err
}

// Render 逐层渲染报告并写入 w，保证确定性（REQ-N-08）。
//
// 不 panic：缺失字段一律落到「未采集」，空 Snapshot 也能产出完整三层骨架。
// 输出为 "\n" 换行，CRLF 由 WriteCRLF 转换，BOM 由 writer.go 添加。
func Render(w io.Writer, snap *model.Snapshot, issues []model.Issue, ctx RenderContext) error {
	if w == nil {
		return fmt.Errorf("report: 渲染目标为 nil")
	}
	snap = snapsOf(snap)
	_, err := WriteCRLF(w, renderCore(snap, issues, ctx))
	return err
}

// RenderString 返回完整报告文本（"\n" 换行，不含 BOM），供 golden 测试使用。
func RenderString(snap *model.Snapshot, issues []model.Issue, ctx RenderContext) string {
	return renderCore(snapsOf(snap), issues, ctx)
}

// ReportSections 返回三层结构，每层的正文按行拆开，便于契约测试断言标题与顺序。
func ReportSections(snap *model.Snapshot, issues []model.Issue, ctx RenderContext) []ReportSection {
	snap = snapsOf(snap)
	return []ReportSection{
		{Title: sectionTitleLayer1, Lines: contentLines(layer1Content(snap, issues))},
		{Title: sectionTitleLayer2, Lines: contentLines(layer2Content(snap))},
		{Title: sectionTitleLayer3, Lines: contentLines(layer3Content(snap))},
	}
}

// renderCore assembles the header, three report layers and read-only footer.
func renderCore(snap *model.Snapshot, issues []model.Issue, ctx RenderContext) string {
	var sb strings.Builder

	sb.WriteString(headerBlock(snap, ctx))
	sb.WriteString("\n")
	sb.WriteString(sectionTitleLayer1)
	sb.WriteString("\n")
	sb.WriteString(rule())
	sb.WriteString("\n")
	sb.WriteString(layer1Content(snap, issues))
	sb.WriteString(rule())
	sb.WriteString("\n\n")

	sb.WriteString(sectionTitleLayer2)
	sb.WriteString("\n")
	sb.WriteString(rule())
	sb.WriteString("\n")
	sb.WriteString(layer2Content(snap))
	sb.WriteString(rule())
	sb.WriteString("\n\n")

	sb.WriteString(sectionTitleLayer3)
	sb.WriteString("\n")
	sb.WriteString(rule())
	sb.WriteString("\n")
	sb.WriteString(layer3Content(snap))

	sb.WriteString(separator())
	sb.WriteString("\n")
	sb.WriteString(bannerLine(" " + readOnlyFooter + " "))
	sb.WriteString("\n")
	sb.WriteString(separator())
	sb.WriteString("\n")

	return sb.String()
}

// headerBlock 渲染报告头（REQ-F-503 + Q2 路径说明 + S-10 UNC 提示）。
func headerBlock(snap *model.Snapshot, ctx RenderContext) string {
	var sb strings.Builder

	sb.WriteString(separator())
	sb.WriteString("\n")
	sb.WriteString(bannerLine(reportTitle))
	sb.WriteString("\n")
	sb.WriteString(separator())
	sb.WriteString("\n")

	sb.WriteString(field("诊断时间", formatTime(generatedAt(snap, ctx))))
	sb.WriteString("\n")
	sb.WriteString(field("计算机名", snap.Host.ComputerName))
	sb.WriteString("\n")
	sb.WriteString(field("运行账户", accountLine(snap.Host)))
	sb.WriteString("\n")
	sb.WriteString(field("操作系统", osSummary(snap.Host)))
	sb.WriteString("\n")
	sb.WriteString(field("系统运行时长", formatDuration(snap.Host.Uptime)))
	sb.WriteString("\n")
	sb.WriteString(field("诊断工具版本", versionLine(ctx)))
	sb.WriteString("\n")
	// REQ-F-506：实际路径与降级说明必须出现在报告头。
	sb.WriteString(field("报告存放路径", ctx.ReportPath))
	sb.WriteString("\n")
	sb.WriteString(field("路径选择说明", pathNote(ctx)))
	sb.WriteString("\n")
	sb.WriteString(field("采集与判定耗时", formatSeconds(ctx.TotalElapsed)))
	sb.WriteString("\n")

	// S-10：UNC 部署路径要在报告头留线索，否则收集时无法判断报告为何落到本地。
	if IsUNCPath(ctx.ExePath) || IsUNCPath(ctx.ReportPath) {
		sb.WriteString(field("部署路径提示", "检测到 UNC 网络共享路径（"+firstUNCPath(ctx.ExePath, ctx.ReportPath)+"）"+
			"：共享目录不可写时报告会自动降级到本地目录，收集时请以上述「报告存放路径」为准"))
		sb.WriteString("\n")
	}
	if ctx.PlainText {
		sb.WriteString(field("输出模式", "纯文本（无颜色、无 ANSI 转义序列）"))
		sb.WriteString("\n")
	}

	sb.WriteString(separator())
	sb.WriteString("\n")
	return sb.String()
}

// generatedAt 取编排层显式给出的 GeneratedAt，其次回落快照的 StartedAt。
// 两条路都不读 time.Now()，渲染因此保持确定性。
func generatedAt(snap *model.Snapshot, ctx RenderContext) time.Time {
	if !ctx.GeneratedAt.IsZero() {
		return ctx.GeneratedAt
	}
	return snap.StartedAt
}

// osSummary combines available operating-system name, version and architecture.
func osSummary(h model.Host) string {
	name := strings.TrimSpace(h.OSName)
	if name == "" {
		name = notCollected
	}
	if v := strings.TrimSpace(h.OSVersion); v != "" {
		name += " (" + v + ")"
	}
	if a := strings.TrimSpace(h.OSArch); a != "" {
		name += " " + a
	}
	return name
}

// versionLine renders tool version and available build metadata.
func versionLine(ctx RenderContext) string {
	v := strings.TrimSpace(ctx.Version)
	if v == "" {
		v = "dev"
	}
	commit := strings.TrimSpace(ctx.Commit)
	if commit == "" {
		commit = "unknown"
	}
	built := strings.TrimSpace(ctx.BuildTime)
	if built == "" {
		built = "unknown"
	}
	return fmt.Sprintf("%s (commit %s, built %s)", v, commit, built)
}

// pathNote explains report destination selection and degradation.
func pathNote(ctx RenderContext) string {
	if s := strings.TrimSpace(ctx.PathNote); s != "" {
		return s
	}
	// 正常流程由 Writer.Write 填好；为空说明调用方绕过了写入器。
	return "未说明（调用方未提供路径信息）"
}

// layer1Content renders ordered findings and the diagnostic completeness summary.
func layer1Content(snap *model.Snapshot, issues []model.Issue) string {
	var sb strings.Builder

	severe, warning := model.CountSeverity(issues)
	if len(issues) == 0 {
		sb.WriteString(indentMain + noIssueConclusion + "\n")
	} else {
		fmt.Fprintf(&sb, "%s共 %d 项：严重 %d，警告 %d\n", indentMain, len(issues), severe, warning)
		for _, is := range issues {
			writeIssue(&sb, is)
		}
	}

	// REQ-F-405 / Q4：有采集失败就必须给「诊断不完整」提示，哪怕一条告警都没有 ——
	// 否则缺数据会被读成「系统正常」。
	if len(snap.Failures) > 0 {
		fmt.Fprintf(&sb, "\n%s共 %d 项未采集：\n", indentMain, len(snap.Failures))
		for _, f := range snap.Failures {
			fmt.Fprintf(&sb, "%s- %s\n", indentMain, failureShort(f))
		}
		sb.WriteString(indentMain + "说明: 上述项目的数据缺失，相关结论未参与判定；如需完整诊断请以管理员身份重试。\n")
	}

	return sb.String()
}

// writeIssue 渲染一条告警：标题 + 证据 + 建议（REQ-F-402）。
func writeIssue(sb *strings.Builder, is model.Issue) {
	tag := severityTag(is.Severity)
	cat := strings.TrimSpace(string(is.Category))
	title := strings.TrimSpace(is.Title)
	if title == "" {
		title = notCollected
	}

	fmt.Fprintf(sb, "\n%s %s - %s\n", indentMain+tag, orNotCollected(cat), title)
	if rid := strings.TrimSpace(is.RuleID); rid != "" {
		fmt.Fprintf(sb, "%s规则: %s\n", indentEvidence, rid)
	}
	if detail := strings.TrimSpace(is.Detail); detail != "" {
		for _, line := range strings.Split(detail, "\n") {
			fmt.Fprintf(sb, "%s%s\n", indentEvidence, line)
		}
	}
	// 证据统一去空白、剔空行，避免出现只有项目符号没有内容的空行。
	for _, ev := range model.NormalizeEvidence(is.Evidence) {
		fmt.Fprintf(sb, "%s%s\n", indentEvidence, model.ChineseEvidence(ev))
	}
	if sug := strings.TrimSpace(is.Suggestion); sug != "" {
		fmt.Fprintf(sb, "%s建议: %s\n", indentEvidence, sug)
	} else {
		// 没有建议就明确写「无」：留空会让读者以为渲染漏了一行。
		fmt.Fprintf(sb, "%s建议: 无需处理（该项为提示性结论）\n", indentEvidence)
	}
}

// layer2Content 按注册表渲染各分节（REQ-N-09），对分节列表零硬编码。
func layer2Content(snap *model.Snapshot) string {
	var sb strings.Builder

	list := Sections()
	if len(list) == 0 {
		sb.WriteString(indentInner + notCollected + "（未注册任何分节渲染器）\n")
		return sb.String()
	}

	for _, sec := range list {
		fmt.Fprintf(&sb, " ▼ %s\n", sec.Title)
		// 单个分节渲染器出错不得毁掉整份报告，降级成该节的一行提示。
		sb.WriteString(safeRenderSection(sec, snap))
		sb.WriteString("\n")
	}

	// 第三层没有原始数据时，第二层必须显式说明原因，不能让读者以为工具忘了输出。
	if len(snap.Raw) == 0 {
		sb.WriteString(indentInner + "注: 本次诊断未采集到任何原始数据，第三层附录为空。\n")
	}
	return sb.String()
}

// safeRenderSection 渲染单个分节，捕获渲染器的 panic。
func safeRenderSection(sec sectionEntry, snap *model.Snapshot) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("%s%s（渲染失败: %v）\n", indentInner, notCollected, r)
		}
	}()
	var sb strings.Builder
	sec.Render(&sb, snap)
	return sb.String()
}

// layer3Content 按 Section 分组、组内按 Source 标注来源。
//
// Section 与 Source 都按字典序排列：Raw 的填充顺序取决于采集器执行顺序，
// 照抄会把调度顺序泄漏进报告，破坏可复现性（REQ-N-08）。
func layer3Content(snap *model.Snapshot) string {
	if len(snap.Raw) == 0 {
		// 无原始数据不是错误，但必须给出可读结论（REQ-F-508 离线可读）。
		return indentInner + notCollected + "（本次诊断未产生原始数据；可能所有采集项均失败，详见第一层「诊断完整性」）\n"
	}

	type group struct {
		source string
		lines  []string
	}
	type bucket struct {
		section string
		groups  []*group
	}

	bySection := make(map[string]*bucket, len(snap.Raw))
	bySource := make(map[string]map[string]*group, len(snap.Raw))

	for _, rl := range snap.Raw {
		sName := strings.TrimSpace(rl.Section)
		if sName == "" {
			sName = layer3SubsectionRaw
		}
		src := strings.TrimSpace(rl.Source)
		if src == "" {
			src = emptySource
		}
		b, ok := bySection[sName]
		if !ok {
			b = &bucket{section: sName}
			bySection[sName] = b
			bySource[sName] = make(map[string]*group)
		}
		g, ok := bySource[sName][src]
		if !ok {
			g = &group{source: src}
			bySource[sName][src] = g
			b.groups = append(b.groups, g)
		}
		g.lines = append(g.lines, rl.Line)
	}

	names := make([]string, 0, len(bySection))
	for n := range bySection {
		names = append(names, n)
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, n := range names {
		b := bySection[n]
		fmt.Fprintf(&sb, " [分节] %s\n", b.section)

		sort.SliceStable(b.groups, func(i, j int) bool { return b.groups[i].source < b.groups[j].source })
		for _, g := range b.groups {
			if g.source != b.section {
				fmt.Fprintf(&sb, "  [来源] %s\n", g.source)
			}
			for _, line := range g.lines {
				// 原始行按原样输出（不 Trim），损坏的原始数据本身也是线索。
				fmt.Fprintf(&sb, "    %s\n", line)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// IsUNCPath 判断路径是否为 UNC 形式（\\server\share\...）。
//
// 反斜杠与正斜杠都要认：调用方可能传入已被 Clean 的值。
func IsUNCPath(p string) bool {
	s := strings.TrimSpace(p)
	return strings.HasPrefix(s, `\\`) || strings.HasPrefix(s, "//")
}

// firstUNCPath 返回第一个非空的 UNC 路径（EXE 路径优先，其次报告路径）。
func firstUNCPath(paths ...string) string {
	for _, p := range paths {
		if IsUNCPath(p) {
			return strings.TrimSpace(p)
		}
	}
	return ""
}

// contentLines 把一段正文按行拆开并去掉首尾空行，供契约测试断言标题与顺序。
func contentLines(block string) []string {
	trimmed := strings.Trim(block, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
