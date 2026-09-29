//go:build windows

// Package app 是进程编排层：串联采集、判定、报告落盘与控制台输出。
package app

import (
	"context"
	"fmt"
	"io"
	"os"
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

// globalTimeout 是整体兜底超时。
//
// 每一项探测都有自己的超时，这里只防「某次系统调用卡死导致进程永不退出」；
// 触发时已完成的采集结果仍然有效，未完成的按失败降级记录。
const globalTimeout = 60 * time.Second

// Run 执行一次完整诊断并返回进程退出码。
func Run(args []string, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr, time.Now)
}

// run 是注入时间源的 Run，便于测试固定耗时与报告文件名。
func run(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	opts, err := cli.Parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n\n%s", err, cli.Usage)
		return ExitError
	}

	switch {
	case opts.ShowHelp:
		fmt.Fprint(stdout, cli.Usage)
		return ExitOK
	case opts.ShowVersion:
		fmt.Fprintln(stdout, version.String())
		return ExitOK
	}

	return diagnose(opts, stdout, stderr, now)
}

// diagnose 跑完一次诊断流程，返回退出码。
func diagnose(opts cli.Options, stdout, stderr io.Writer, now func() time.Time) int {
	console := ui.Setup(stdout)
	console.SetVerbose(opts.Verbose)
	console.Header(version.String())

	started := now()
	snap := &model.Snapshot{StartedAt: started}

	ctx, cancel := context.WithTimeout(context.Background(), globalTimeout)
	defer cancel()

	printSteps(console, collect.Run(ctx, snap))
	issues := detect.Evaluate(snap)
	printLayers(console, snap.Layers)
	printIssues(console, issues)

	elapsed := now().Sub(started)
	outcome, err := writeReport(opts, snap, issues, elapsed, now)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return ExitError
	}
	if outcome.Degraded {
		console.Line("注意：报告已降级写入「%s」（原因：%s）", outcome.Label, outcome.Reason)
	}

	severe, warning := model.CountSeverity(issues)
	console.Summary(severe, warning, outcome.Path, elapsed)

	if model.HasSevere(issues) {
		return ExitSevere
	}
	return ExitOK
}

// printSteps 逐行打印采集进度（REQ-F-601）。
func printSteps(console *ui.Console, steps []collect.StepResult) {
	for i, st := range steps {
		sev, conclusion := model.SevOK, fmt.Sprintf("完成（%s）", st.Duration.Round(time.Millisecond))
		if !st.OK() {
			sev, conclusion = model.SevWarning, "失败："+errText(st.Err)
		}
		console.Step(i+1, len(steps), st.Name, conclusion, sev)
	}
}

// printLayers 打印链路故障层级结论（REQ-F-205）。
//
// 用「可达性」限定措辞：层级只回答哪一段不通，丢包率与延迟属于质量告警
// （R-12/R-13）。因此这里显示正常、同时又有丢包告警并存并不矛盾，
// 限定词正是为了避免读者误以为两者冲突。
func printLayers(console *ui.Console, layers []model.LayerConclusion) {
	for _, l := range layers {
		console.Line("链路可达性：%s（%s）", l.Summary, l.Level)
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
	return strings.ReplaceAll(err.Error(), "\n", " ")
}
