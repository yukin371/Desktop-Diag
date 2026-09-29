//go:build windows

// Package cli 解析命令行选项，对应基线第 9 节的 CLI 契约。
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// ErrInvalidArgs 表示命令行参数非法，调用方应打印用法并返回退出码 2。
var ErrInvalidArgs = errors.New("命令行参数非法")

// Options 是解析后的命令行选项。
type Options struct {
	// OutputDir 是 -o 指定的报告输出目录，已清理但**未**转为绝对路径。
	OutputDir   string
	Verbose     bool
	ShowVersion bool
	ShowHelp    bool
}

// Usage 是 -h/--help 的输出内容，含合规声明。
const Usage = `Desktop-Diag —— 桌面运维一键诊断工具

用法：
  Desktop-Diag.exe [选项]

选项：
  -o <目录>      指定报告输出目录（优先级最高；目录不存在则尝试创建，失败则降级）
  -v             详细模式：输出各采集项原始返回值、数据来源、耗时（用于排障）
  -version       打印版本、构建提交、构建时间后退出
  -h, --help     打印本帮助与合规声明后退出

退出码：
  0   诊断完成，无严重告警
  1   诊断完成，存在严重告警
  2   程序自身错误（参数非法 / 无法生成报告）

合规声明：
  本工具仅执行只读诊断，不修改任何系统配置，不上传任何数据，无驻留、无 GUI。
  全流程无输入等待，纯文本输出。
`

// Parse 解析命令行参数。
//
// 帮助与版本请求不算错误，通过 ShowHelp / ShowVersion 返回；
// 其余失败包装 ErrInvalidArgs，调用方据此返回退出码 2。
func Parse(args []string) (Options, error) {
	var opts Options
	fs := flag.NewFlagSet("desktop-diag", flag.ContinueOnError)
	// 屏蔽 flag 自带输出：用法由调用方统一呈现，避免半截用法混进日志。
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.OutputDir, "o", "", "报告输出目录")
	fs.BoolVar(&opts.Verbose, "v", false, "详细模式")
	fs.BoolVar(&opts.ShowVersion, "version", false, "打印版本后退出")

	if err := fs.Parse(args); err != nil {
		// flag 对未定义的 -h/-help 返回 ErrHelp，此处转成正常退出路径。
		if errors.Is(err, flag.ErrHelp) {
			opts.ShowHelp = true
			return opts, nil
		}
		return Options{}, fmt.Errorf("%w: %v", ErrInvalidArgs, err)
	}
	if fs.NArg() > 0 {
		return Options{}, fmt.Errorf("%w: 不支持的位置参数 %q", ErrInvalidArgs, fs.Arg(0))
	}

	opts.OutputDir = normalizeDir(opts.OutputDir)
	return opts, nil
}

// normalizeDir 去掉用户可能连带的引号，并规整结尾分隔符。
//
// 用 filepath.Clean 而非 TrimRight：后者会把 "C:\" 削成 "C:"，
// 而 "C:" 在 Windows 上表示「C 盘的当前目录」，含义完全不同。
func normalizeDir(dir string) string {
	dir = strings.TrimSpace(dir)
	dir = strings.Trim(dir, `"'`)
	if dir == "" {
		return ""
	}
	return filepath.Clean(dir)
}
