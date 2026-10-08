//go:build windows

// Renders self-contained, escaped HTML reports for offline reading.
package report

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/yukin371/desktop-diag/internal/model"
)

// htmlSection is a registered diagnostic section with a stable local anchor.
type htmlSection struct{ ID, Title, Content string }

// htmlFinding carries readable findings and an internally selected severity class.
type htmlFinding struct{ ID, Class, Severity, Rule, Title, Content string }

// htmlReport contains only text values; html/template escapes all collected data.
type htmlReport struct {
	Host, Time, Version, Status, Header, Completeness, Raw, Footer string
	Severe, Warning, Missing                                       int
	Findings                                                       []htmlFinding
	Overview                                                       []htmlOverview
	Sections                                                       []htmlSection
	Extras                                                         []string
}

// RenderHTML writes a complete offline document, including registered sections and extensions.
func RenderHTML(w io.Writer, snap *model.Snapshot, issues []model.Issue, ctx RenderContext, extras []string) error {
	if w == nil {
		return fmt.Errorf("HTML 报告目标为 nil")
	}
	snap = snapsOf(snap)
	severe, warning := model.CountSeverity(issues)
	ctx.PlainText = false
	data := htmlReport{
		Host: orNotCollected(snap.Host.ComputerName), Time: formatTime(generatedAt(snap, ctx)),
		Version: versionLine(ctx), Header: headerBlock(snap, ctx), Raw: layer3Content(snap),
		Footer: readOnlyFooter, Severe: severe, Warning: warning, Missing: len(snap.Failures), Extras: extras,
		Status: "未发现异常",
	}
	switch {
	case severe > 0:
		data.Status = "存在严重告警，请优先排查"
	case len(snap.Failures) > 0:
		data.Status = "诊断不完整，请结合缺失数据阅读"
	case warning > 0:
		data.Status = "存在警告，建议核查"
	}
	if len(snap.Failures) > 0 {
		var missing strings.Builder
		for _, failure := range snap.Failures {
			fmt.Fprintln(&missing, failureShort(failure))
		}
		data.Completeness = missing.String()
	}
	for index, issue := range issues {
		var content strings.Builder
		writeIssue(&content, issue)
		class := "normal"
		if issue.Severity == model.SevSevere {
			class = "severe"
		} else if issue.Severity == model.SevWarning {
			class = "warning"
		}
		data.Findings = append(data.Findings, htmlFinding{fmt.Sprintf("finding-%d", index), class, issue.Severity.String(), issue.RuleID, issue.Title, content.String()})
	}
	for index, section := range Sections() {
		data.Sections = append(data.Sections, htmlSection{fmt.Sprintf("section-%d", index), section.Title, safeRenderSection(section, snap)})
	}
	data.Overview = buildHTMLOverview(snap, issues, data.Findings, data.Sections)
	tmpl, err := template.New("report").Parse(htmlDocument)
	if err != nil {
		return fmt.Errorf("解析 HTML 报告模板失败: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("渲染 HTML 报告失败: %w", err)
	}
	if _, err := WriteCRLF(w, buf.String()); err != nil {
		return fmt.Errorf("写入 HTML 报告失败: %w", err)
	}
	return nil
}

// htmlDocument embeds all styles; native details provides interaction without scripts or resources.
const htmlDocument = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<title>诊断报告 · {{.Host}}</title>
<style>
:root{--ink:#20332f;--muted:#61716a;--paper:#f4f3ed;--line:#d4ddd3;--green:#146650;--red:#a52a30;--amber:#926117}
*{box-sizing:border-box}html{scroll-behavior:smooth;scroll-padding-top:24px}body{margin:0;background:var(--paper);color:var(--ink);font-family:"Microsoft YaHei","PingFang SC",sans-serif;line-height:1.65}a{color:var(--green);text-underline-offset:4px}a:focus-visible,summary:focus-visible{outline:3px solid var(--green);outline-offset:4px}.shell{max-width:1200px;margin:auto;padding:48px 32px}.masthead{display:flex;justify-content:space-between;border-bottom:2px solid var(--ink);padding-bottom:14px;font-size:13px;letter-spacing:.08em}.brand{font-weight:bold}.hero{padding:40px 0 26px}.eyebrow{color:var(--green);font-size:12px;letter-spacing:.15em}h1{font-family:"Microsoft YaHei",sans-serif;font-size:clamp(30px,5vw,50px);line-height:1.2;margin:12px 0 18px;letter-spacing:-.04em}.sub{color:var(--muted);overflow-wrap:anywhere}.stats{display:grid;grid-template-columns:repeat(3,1fr);gap:1px;background:var(--line);border:1px solid var(--line);margin:28px 0 36px}.stat{background:#fff;padding:20px 24px}.stat strong{font-size:34px;display:block;line-height:1.2}.stat span{font-size:13px;color:var(--muted)}.red{color:var(--red)}.amber{color:var(--amber)}.layout{display:grid;grid-template-columns:190px minmax(0,1fr);gap:40px}nav{position:sticky;top:24px;align-self:start;font-size:14px}nav a{display:block;text-decoration:none;padding:9px 0;border-bottom:1px solid var(--line)}nav p{font-size:11px;letter-spacing:.12em;color:var(--muted)}h2{font-size:23px;margin:0 0 18px}section{margin-bottom:36px;scroll-margin-top:24px}.card,details.panel{background:#fff;border:1px solid var(--line);border-radius:5px;margin:12px 0}.card{padding:22px;border-left:4px solid var(--green)}.card.severe{border-left-color:var(--red)}.card.warning,.incomplete{border-left-color:var(--amber)}.card h3{font-size:18px;margin:9px 0}.badge{font-size:12px;font-weight:bold;color:var(--green)}.severe .badge{color:var(--red)}.warning .badge{color:var(--amber)}pre{white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;font:13px/1.85 "Cascadia Mono",Consolas,"Microsoft YaHei",monospace;margin:0}.card pre{margin-top:12px}summary{cursor:pointer;padding:18px 22px;font-weight:bold}details[open]>summary{border-bottom:1px solid var(--line)}details pre{padding:22px}.note{padding:18px 22px;background:#edf3ed;border:1px solid var(--line);font-size:14px}.incomplete{background:#fff7e9;border-left:4px solid var(--amber);padding:20px;margin:18px 0}.incomplete strong{display:block;margin-bottom:10px}footer{border-top:2px solid var(--ink);padding-top:20px;margin-top:36px;font-size:12px;color:var(--muted)}
.overview-wrap{overflow-x:auto;border:1px solid var(--line);background:white;margin-bottom:36px}.overview{width:100%;border-collapse:collapse;font-size:14px}.overview caption{text-align:left;padding:20px 22px;font-size:23px;font-weight:bold}.overview th,.overview td{text-align:left;vertical-align:top;padding:14px 18px;border-top:1px solid var(--line)}.overview th{background:#edf3ed;font-size:12px;color:var(--muted)}.overview .count{white-space:nowrap}.overview .links a{display:block;margin:4px 0}.overview .status{font-weight:bold;white-space:nowrap}.overview .severe{color:var(--red)}.overview .warning{color:var(--amber)}.overview .normal{color:var(--green)}.overview-note{font-size:12px;color:var(--muted);padding:0 22px 18px;margin:0}.card:target{outline:3px solid var(--green);outline-offset:4px}.back{display:inline-block;margin-top:14px;font-size:12px}
@media(max-width:720px){.shell{padding:24px 16px}.layout{display:block}nav{position:static;margin-bottom:28px;display:flex;flex-wrap:wrap;gap:8px 18px}nav p{width:100%;margin:0}nav a{padding:6px 0}.stat{padding:16px 12px}.stat strong{font-size:28px}.masthead{flex-wrap:wrap;gap:8px}.hero{padding-top:28px}}
@media print{body{background:white}.shell{padding:0;max-width:none}nav{display:none}.layout{display:block}.card,details.panel{break-inside:avoid}details:not([open])>pre{display:block}summary{list-style:none}.stats{margin:18px 0}h1{font-size:30px}a{color:inherit}}
</style></head><body><div class="shell">
<header class="masthead"><span class="brand">DESKTOP-DIAG / 桌面诊断</span><span>{{.Time}} · 离线报告</span></header>
<div class="hero"><span class="eyebrow">DIAGNOSTIC REPORT</span><h1>{{.Host}}<br>系统诊断报告</h1><p class="sub">{{.Status}}</p>
<div class="stats" aria-label="诊断摘要"><div class="stat"><strong class="red">{{.Severe}}</strong><span>严重告警</span></div><div class="stat"><strong class="amber">{{.Warning}}</strong><span>一般警告</span></div><div class="stat"><strong>{{.Missing}}</strong><span>缺失数据项</span></div></div></div>
<section id="overview" class="overview-wrap"><table class="overview"><caption>各方面诊断总览</caption><thead><tr><th scope="col">诊断领域</th><th scope="col">结论</th><th scope="col">严重 / 警告</th><th scope="col">数据缺失</th><th scope="col">问题与详情</th></tr></thead><tbody>
{{range .Overview}}<tr><th scope="row">{{.Title}}</th><td class="status {{.Class}}">{{.Status}}</td><td class="count">{{.Severe}} / {{.Warning}}</td><td>{{.Missing}}</td><td class="links">{{range .Links}}<a href="#{{.ID}}">{{.Label}}</a>{{end}}<a href="#{{.Target}}">查看检测详情 →</a></td></tr>{{end}}
</tbody></table><p class="overview-note">数量按告警条目统计；诊断完整性行汇总全部缺失数据，与各领域缺失项重叠。未发现告警仅表示已采集数据未触发规则，不代表全部系统功能正常。</p></section>
<div class="layout"><nav aria-label="报告目录"><p>报告目录</p><a href="#overview">诊断总览</a><a href="#findings">01 / 告警与建议</a>{{range .Sections}}<a href="#{{.ID}}">{{.Title}}</a>{{end}}<a href="#raw">原始数据附录</a><a href="#context">报告信息</a></nav>
<main><section id="findings"><h2>01 / 告警与建议</h2>
{{if .Completeness}}<div id="completeness" class="incomplete"><strong>部分数据缺失 · 结论可能不完整</strong><pre>{{.Completeness}}</pre></div>{{end}}
{{range .Findings}}<article id="{{.ID}}" class="card {{.Class}}"><span class="badge">{{.Severity}} · {{.Rule}}</span><h3>{{.Title}}</h3><details open><summary>查看证据与排查建议</summary><pre>{{.Content}}</pre></details><a class="back" href="#overview">↑ 返回诊断总览</a></article>{{else}}<p class="note">未发现异常。请同时检查缺失数据项；缺失数据不能视为检测正常。</p>{{end}}</section>
{{range .Sections}}<section id="{{.ID}}"><h2>{{.Title}}</h2><details class="panel" open><summary>检测详情</summary><pre>{{.Content}}</pre></details></section>{{end}}
<section id="raw"><h2>原始数据附录</h2><details class="panel"><summary>展开 API 来源与原始记录</summary><pre>{{.Raw}}</pre></details>{{range .Extras}}<details class="panel"><summary>附加记录</summary><pre>{{.}}</pre></details>{{end}}</section>
<section id="context"><h2>报告信息</h2><details class="panel"><summary>版本、报告路径与采集环境</summary><pre>{{.Header}}</pre></details></section>
</main></div><footer>{{.Footer}}<br>{{.Version}}<br>单文件离线报告 · 不加载外部资源 · 可使用浏览器打印功能归档</footer>
</div></body></html>`
