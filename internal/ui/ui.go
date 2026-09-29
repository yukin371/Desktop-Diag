//go:build windows

// Package ui 负责控制台呈现：代码页、颜色降级、进度行与结尾汇总。
package ui

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// Mode 决定告警等级如何呈现。
type Mode int

const (
	// ModePlain 不输出 ANSI 转义序列，等级以「[严重]」「[警告]」文本标记表达。
	ModePlain Mode = iota
	// ModeColor 使用 ANSI 颜色。
	ModeColor
)

// ANSI 转义序列。
const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiGreen  = "\x1b[32m"
	ansiBold   = "\x1b[1m"
)

// Console 向一个输出流打印控制台信息。
type Console struct {
	out     io.Writer
	mode    Mode
	verbose bool
}

// New 创建控制台输出器。mode 由调用方决定，便于在测试中强制纯文本。
func New(out io.Writer, mode Mode) *Console {
	return &Console{out: out, mode: mode}
}

// Setup 按 out 的真实能力准备中文显示与颜色，三级降级：
// 真实控制台且能开 VT → 颜色；真实控制台但开不了 VT → 纯文本；
// 非控制台输出（重定向、管道、测试缓冲）→ 纯文本，且完全不改控制台设置
// （REQ-F-602、REQ-F-603）。
func Setup(out io.Writer) *Console {
	f, ok := out.(*os.File)
	if !ok {
		// 非 *os.File 的写入者没有控制台句柄可探测，只能走纯文本。
		return New(out, ModePlain)
	}

	handle := f.Fd()
	if !winapi.IsConsole(handle) {
		return New(out, ModePlain)
	}
	// 失败只影响中文显示，不影响诊断结果，故有意忽略（AGENTS.md 允许的文档化例外）。
	_ = winapi.SetConsoleOutputCP(winapi.CPUTF8)
	if os.Getenv("NO_COLOR") != "" || !winapi.EnableVTProcessing(handle) {
		return New(out, ModePlain)
	}
	return New(out, ModeColor)
}

// Mode 返回当前渲染模式。
func (c *Console) Mode() Mode { return c.mode }

// SetVerbose 开启详细模式，详见 Verbosef。
func (c *Console) SetVerbose(v bool) { c.verbose = v }

// Line 打印一行普通文本。
func (c *Console) Line(format string, a ...any) {
	fmt.Fprintf(c.out, format+"\n", a...)
}

// Verbosef 仅在详细模式（-v）下打印一行，用于排障信息。
func (c *Console) Verbosef(format string, a ...any) {
	if c.verbose {
		fmt.Fprintf(c.out, format+"\n", a...)
	}
}

// Step 打印一行检测进度，形如「[2/4] 网络适配器信息 ... 完成（9ms）」。
func (c *Console) Step(index, total int, name, conclusion string, sev model.Severity) {
	fmt.Fprintf(c.out, "[%d/%d] %s ... %s\n", index, total, name, c.tag(sev, conclusion))
}

// Issue 打印一条诊断结论及其证据。
func (c *Console) Issue(is model.Issue) {
	c.Line("%s %s", c.tag(is.Severity, "["+is.RuleID+"]"), is.Title)
	for _, e := range model.NormalizeEvidence(is.Evidence) {
		c.Line("        %s", e)
	}
	if is.Suggestion != "" {
		c.Line("        建议：%s", is.Suggestion)
	}
}

// Summary 打印结尾汇总：告警数、报告路径、总耗时（REQ-F-604）。
func (c *Console) Summary(severe, warning int, reportPath string, elapsed time.Duration) {
	c.Line("")
	c.Line("严重告警：%d 项", severe)
	c.Line("一般告警：%d 项", warning)
	c.Line("报告路径：%s", reportPath)
	c.Line("总耗时：%s", elapsed.Round(time.Millisecond))
}

// tag 按当前模式给等级文本上色或补文本标记。
func (c *Console) tag(sev model.Severity, text string) string {
	if sev == model.SevOK {
		return text
	}
	mark := "[" + sev.String() + "] " + text
	if c.mode != ModeColor {
		return mark
	}
	if sev == model.SevSevere {
		return ansiRed + mark + ansiReset
	}
	return ansiYellow + mark + ansiReset
}

// Header 打印启动横幅与合规声明。
func (c *Console) Header(versionLine string) {
	c.Line("%s%s%s", c.bold(), versionLine, c.reset())
	c.Line("本工具仅执行只读诊断，不修改系统配置、不上传任何数据。")
	c.Line("")
}

func (c *Console) bold() string {
	if c.mode == ModeColor {
		return ansiBold
	}
	return ""
}

func (c *Console) reset() string {
	if c.mode == ModeColor {
		return ansiReset
	}
	return ""
}

// OK 返回绿色文本（无色模式返回原文），用于正面结论。
func (c *Console) OK(text string) string {
	if c.mode == ModeColor {
		return ansiGreen + text + ansiReset
	}
	return text
}
