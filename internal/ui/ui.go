//go:build windows

// Package ui 负责实时控制台呈现、ASCII 降级、输出错误与共享控制台状态恢复。
package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// Mode 决定文本编码与告警颜色。
type Mode int

// 控制台模式与 ANSI 呈现常量。
const (
	ModePlain  Mode = iota // UTF-8 纯文本，不含 ANSI。
	ModeColor              // UTF-8 彩色控制台。
	ModeASCII              // 代码页设置失败时仅输出七位 ASCII。
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiGreen  = "\x1b[32m"
	ansiBold   = "\x1b[1m"
)

// Console 记录输出错误，并在 Close 时恢复共享控制台原始状态。
type Console struct {
	out     io.Writer
	mode    Mode
	verbose bool
	err     error
	restore func() error
	notice  string
}

// consoleAPI 注入控制台能力，便于验证失败降级与恢复而不改变真实控制台。
type consoleAPI struct {
	getMode func(uintptr) (uint32, error)
	setMode func(uintptr, uint32) error
	getCP   func() uint32
	setCP   func(uint32) error
}

// New 创建显式模式输出器，默认无系统状态需要恢复。
func New(out io.Writer, mode Mode) *Console { return &Console{out: out, mode: mode} }

// Setup 仅为真实控制台设置 UTF-8/VT；非控制台不改变系统状态。
func Setup(out io.Writer) *Console {
	return setup(out, consoleAPI{winapi.GetConsoleMode, winapi.SetConsoleMode, winapi.GetConsoleOutputCP, winapi.SetConsoleOutputCP})
}

// setup 保存原代码页和模式；能力不足以可读 ASCII 降级，不吞掉失败原因。
func setup(out io.Writer, api consoleAPI) *Console {
	// console 缺省保持 UTF-8 纯文本，适用于缓冲、文件与管道。
	console := New(out, ModePlain)
	// file 必须有真实控制台句柄才允许设置共享控制台。
	file, ok := out.(*os.File)
	if !ok {
		return console
	}
	// handle/originalMode 保存该输出句柄独立的模式。
	handle := file.Fd()
	originalMode, modeErr := api.getMode(handle)
	if modeErr != nil {
		return console
	}
	// originalCP 是 stdout/stderr 共享的输出代码页，必须按设置的逆序恢复。
	originalCP := api.getCP()
	if originalCP == 0 {
		console.mode = ModeASCII
		console.notice = "Console output code page unavailable; ASCII fallback active."
		return console
	}
	if err := api.setCP(winapi.CPUTF8); err != nil {
		console.mode = ModeASCII
		console.notice = "Cannot set console UTF-8: " + err.Error() + ". ASCII fallback active; read the UTF-8 report."
		return console
	}
	// modeChanged 只在 VT 设置成功后标记，失败不声称已经修改模式。
	modeChanged := false
	if os.Getenv("NO_COLOR") == "" {
		if err := api.setMode(handle, winapi.VTOutputMode(originalMode)); err == nil {
			console.mode = ModeColor
			modeChanged = true
		} else {
			console.notice = "无法启用控制台颜色，已使用中文纯文本输出：" + model.ChineseReason(err.Error())
		}
	}
	console.restore = func() error {
		// restoreErr 同时保留模式与代码页恢复失败，不因一项失败跳过另一项。
		var restoreErr error
		if modeChanged {
			if err := api.setMode(handle, originalMode); err != nil {
				restoreErr = fmt.Errorf("恢复控制台模式失败: %w", err)
			}
		}
		if err := api.setCP(originalCP); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复控制台输出代码页失败: %w", err))
		}
		return restoreErr
	}
	return console
}

// Close 恢复共享状态；重复关闭不再执行 Win32 调用。
func (c *Console) Close() error {
	if c.restore == nil {
		return nil
	}
	// restore 移出字段后执行，使 Close 保持幂等。
	restore := c.restore
	c.restore = nil
	return restore()
}

// Mode 返回实际输出模式。
func (c *Console) Mode() Mode { return c.mode }

// Err 返回首次输出错误，调用方必须检查并决定退出码。
func (c *Console) Err() error { return c.err }

// SetVerbose 开启详细数据来源与原始字段输出。
func (c *Console) SetVerbose(enabled bool) { c.verbose = enabled }

// write 完整写入输出，短写与底层错误均记录上下文。
func (c *Console) write(text string) {
	if c.err != nil {
		return
	}
	if c.mode == ModeASCII {
		text = asciiText(text)
	}
	// written/err 同时检查 nil 错误下的短写。
	written, err := io.WriteString(c.out, text)
	if err == nil && written != len(text) {
		err = io.ErrShortWrite
	}
	if err != nil {
		c.err = fmt.Errorf("写入控制台输出失败: %w", err)
	}
}

// asciiText 保留英文标记与路径 ASCII 部分，将非 ASCII 字符显式转义避免乱码。
func asciiText(text string) string {
	// output 保留换行及 ASCII 原文，非 ASCII 字符在报告中仍保持 UTF-8。
	var output strings.Builder
	for _, char := range text {
		if char <= 127 {
			output.WriteRune(char)
		} else {
			// quoted 是单个字符的 ASCII Unicode 转义，不包含外侧引号。
			quoted := strconv.QuoteToASCII(string(char))
			output.WriteString(quoted[1 : len(quoted)-1])
		}
	}
	return output.String()
}

// Line 打印普通文本并记录输出错误。
func (c *Console) Line(format string, args ...any) { c.write(fmt.Sprintf(format, args...) + "\n") }

// Verbosef 仅在详细模式输出数据来源与字段。
func (c *Console) Verbosef(format string, args ...any) {
	if c.verbose {
		c.Line(format, args...)
	}
}

// Step 打印当前步骤；ASCII 模式仍可读出进度和告警等级。
func (c *Console) Step(index, total int, name, conclusion string, sev model.Severity) {
	if c.mode == ModeASCII {
		// names 翻译固定采集域，动态设备身份使用 Unicode 转义。
		names := map[string]string{"主机与系统信息": "Host/system", "网络适配器信息": "Network adapters", "系统健康度": "System health", "网络连通性探测": "Connectivity"}
		if label, ok := names[name]; ok {
			name = label
		}
		conclusion = strings.NewReplacer("检测中", "Running", "部分采集", "Partial", "完成", "Done", "失败", "Failed").Replace(conclusion)
	}
	c.Line("[%d/%d] %s ... %s", index, total, name, c.tag(sev, conclusion))
}

// Issue 打印规则身份、证据和建议；完整中文说明始终保留在 UTF-8 报告中。
func (c *Console) Issue(issue model.Issue) {
	c.Line("%s %s", c.tag(issue.Severity, "["+issue.RuleID+"]"), issue.Title)
	for _, evidence := range model.NormalizeEvidence(issue.Evidence) {
		c.Line("        %s", model.ChineseEvidence(evidence))
	}
	if issue.Suggestion != "" {
		if c.mode == ModeASCII {
			c.Line("        Advice: %s", issue.Suggestion)
		} else {
			c.Line("        建议：%s", issue.Suggestion)
		}
	}
}

// Summary 打印告警数、报告路径与包含落盘的实际总耗时。
func (c *Console) Summary(severe, warning int, path string, elapsed time.Duration) {
	c.Line("")
	if c.mode == ModeASCII {
		c.Line("Severe findings: %d", severe)
		c.Line("Warnings: %d", warning)
		c.Line("Report path: %s", path)
		c.Line("Total elapsed: %s", elapsed.Round(time.Millisecond))
		return
	}
	c.Line("严重告警：%d 项", severe)
	c.Line("一般告警：%d 项", warning)
	c.Line("报告路径：%s", path)
	c.Line("总耗时：%s", elapsed.Round(time.Millisecond))
}

// tag 使用与输出模式一致的可读等级标记。
func (c *Console) tag(sev model.Severity, text string) string {
	if sev == model.SevOK {
		return text
	}
	// mark 在无色模式保留等级，避免依赖颜色才能识别严重项。
	mark := "[" + sev.String() + "] " + text
	if c.mode == ModeASCII {
		if sev == model.SevSevere {
			return "[SEVERE] " + text
		}
		return "[WARNING] " + text
	}
	if c.mode != ModeColor {
		return mark
	}
	if sev == model.SevSevere {
		return ansiRed + mark + ansiReset
	}
	return ansiYellow + mark + ansiReset
}

// Header 打印启动横幅与编码降级说明。
func (c *Console) Header(version string) {
	c.Line("%s%s%s", c.bold(), version, c.reset())
	if c.mode == ModeASCII {
		c.Line("Read-only diagnostics. No system configuration changes or telemetry uploads.")
	} else {
		c.Line("本工具仅执行只读诊断，不修改系统配置、不上传业务或诊断数据。")
	}
	if c.notice != "" {
		c.Line("%s", c.notice)
	}
	c.Line("")
}

// bold 仅在彩色模式生成加粗转义。
func (c *Console) bold() string {
	if c.mode == ModeColor {
		return ansiBold
	}
	return ""
}

// reset 仅在彩色模式恢复 ANSI 状态。
func (c *Console) reset() string {
	if c.mode == ModeColor {
		return ansiReset
	}
	return ""
}

// OK 标记正常结果，纯文本与 ASCII 模式不生成转义序列。
func (c *Console) OK(text string) string {
	if c.mode == ModeColor {
		return ansiGreen + text + ansiReset
	}
	return text
}
