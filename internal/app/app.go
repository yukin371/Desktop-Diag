//go:build windows

// Package app 是进程编排层：串联采集、判定、报告落盘与控制台输出。
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/cli"
	"github.com/yukin371/desktop-diag/internal/collect"
	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/report"
	"github.com/yukin371/desktop-diag/internal/ui"
	"github.com/yukin371/desktop-diag/internal/version"
)

// 退出码语义，详见基线第 9 节。
const (
	ExitOK     = 0 // 诊断完成，无严重告警
	ExitSevere = 1 // 诊断完成，存在严重告警
	ExitError  = 2 // 程序自身错误（参数非法 / 无法生成报告）
)

// Run 执行一次完整诊断并返回进程退出码。
func Run(args []string, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr, time.Now)
}

// run 是注入时间源的 Run，便于测试固定耗时与报告文件名。
func run(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	return runWith(args, stdout, stderr, now, collect.All(), writeReport)
}

// reportWriter 注入报告落盘，以验证退出码和包含落盘的耗时。
type reportWriter func(cli.Options, *model.Snapshot, []model.Issue, time.Duration, func() time.Time) (report.Outcome, error)

// runWith 使用局部依赖，不改动全局采集器注册表。
func runWith(args []string, stdout, stderr io.Writer, now func() time.Time, collectors []collect.Collector, write reportWriter) (code int) {
	console, errorConsole := ui.Setup(stdout), ui.Setup(stderr)
	defer func() {
		// stderr 与 stdout 共享代码页，必须按设置的逆序恢复。
		if err := errorConsole.Close(); err != nil {
			// 恢复后代码页可能已不再是 UTF-8，错误通知只写 ASCII。
			console.Line("Console restoration failed: %s", strconv.QuoteToASCII(err.Error()))
			code = ExitError
		}
		if err := console.Close(); err != nil {
			errorConsole.Line("Console restoration failed: %s", strconv.QuoteToASCII(err.Error()))
			code = ExitError
		}
		if console.Err() != nil || errorConsole.Err() != nil {
			code = ExitError
		}
	}()
	opts, err := cli.Parse(args)
	if err != nil {
		errorConsole.Line("%v\n\n%s", model.ChineseReason(err.Error()), cli.Usage)
		return ExitError
	}

	switch {
	case opts.ShowHelp:
		console.Line("%s", strings.TrimSuffix(cli.Usage, "\n"))
		return ExitOK
	case opts.ShowVersion:
		console.Line("%s", version.String())
		return ExitOK
	}

	return diagnoseWith(opts, console, errorConsole, now, collectors, write)
}

// diagnose 跑完一次诊断流程，返回退出码。
func diagnoseWith(opts cli.Options, console, errorConsole *ui.Console, now func() time.Time, collectors []collect.Collector, write reportWriter) int {
	console.SetVerbose(opts.Verbose)
	console.Header(version.String())
	if err := console.Err(); err != nil {
		errorConsole.Line("%s", model.ChineseReason(err.Error()))
		return ExitError
	}

	started := now()
	snap := &model.Snapshot{StartedAt: started}

	ctx, cancel := context.WithTimeout(context.Background(), detect.CollectionTimeout)
	defer cancel()

	collect.RunCollectors(ctx, snap, collectors, func(event collect.StepEvent) {
		if event.Started {
			console.Step(event.Index, event.Total, event.Result.Name, "检测中", model.SevOK)
		} else {
			printStep(console, event.Index, event.Total, event.Result)
			console.Verbosef("详细：%s 耗时=%s，缺失项=%d", event.Result.Name, event.Result.Duration, event.Result.Failures)
		}
		if console.Err() != nil {
			cancel()
		}
	})
	for _, raw := range snap.Raw {
		console.Verbosef("来源：%s / %s\n%s", raw.Section, raw.Source, raw.Line)
	}
	issues := detect.Evaluate(snap)
	printLayers(console, snap.Layers)
	printIssues(console, issues)

	elapsed := now().Sub(started)
	outcome, err := write(opts, snap, issues, elapsed, now)
	if err != nil {
		errorConsole.Line("%s", model.ChineseReason(err.Error()))
		return ExitError
	}
	if outcome.Degraded {
		console.Line("注意：报告已降级写入「%s」（原因：%s）", outcome.Label, model.ChineseReason(outcome.Reason))
	}

	severe, warning := model.CountSeverity(issues)
	console.Summary(severe, warning, outcome.Path, now().Sub(started))
	if err := console.Err(); err != nil {
		errorConsole.Line("%s", model.ChineseReason(err.Error()))
		return ExitError
	}

	if model.HasSevere(issues) {
		return ExitSevere
	}
	return ExitOK
}

// printStep 区分完整成功、部分采集和采集失败。
func printStep(console *ui.Console, index, total int, st collect.StepResult) {
	sev, conclusion := model.SevOK, fmt.Sprintf("完成（%s）", st.Duration.Round(time.Millisecond))
	if st.Err != nil {
		sev, conclusion = model.SevWarning, "失败："+errText(st.Err)
	} else if st.Failures > 0 {
		sev, conclusion = model.SevWarning, fmt.Sprintf("部分采集（缺失 %d 项）", st.Failures)
	}
	console.Step(index, total, st.Name, conclusion, sev)
}

// printLayers 打印链路故障层级结论（REQ-F-205）。
//
// 用「可达性」限定措辞：层级只回答哪一段不通，丢包率与延迟属于质量告警
// （R-12/R-13）。因此这里显示正常、同时又有丢包告警并存并不矛盾，
// 限定词正是为了避免读者误以为两者冲突。
func printLayers(console *ui.Console, layers []model.LayerConclusion) {
	for _, l := range layers {
		console.Line("链路可达性：%s", l.Summary)
	}
}

// printIssues 打印第一层告警汇总。
func printIssues(console *ui.Console, issues []model.Issue) {
	console.Line("")
	if len(issues) == 0 {
		console.Line("%s", console.OK("未发现异常，各检测项均正常。"))
		return
	}
	for _, is := range issues {
		console.Issue(is)
	}
}

// writeReport 渲染并落盘报告，返回最终写入结果。
func writeReport(opts cli.Options, snap *model.Snapshot, issues []model.Issue, elapsed time.Duration, now func() time.Time) (report.Outcome, error) {
	// ExePath 只用于判断 EXE 是否位于 UNC 路径；取不到时留空，视为本地路径。
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}

	wr := report.Writer{
		Now:       now,
		StartedAt: snap.StartedAt,
		RenderContext: report.RenderContext{
			Version:      version.Version,
			Commit:       version.Commit,
			BuildTime:    version.BuildTime,
			ExePath:      exe,
			TotalElapsed: elapsed,
			GeneratedAt:  now(),
		},
	}
	return wr.Write(snap, issues, report.ResolveTargets(opts.OutputDir))
}

// errText 把错误压成单行，避免多行错误把进度行撑散。
func errText(err error) string {
	if err == nil {
		return "未知原因"
	}
	return strings.ReplaceAll(model.ChineseReason(err.Error()), "\n", " ")
}
