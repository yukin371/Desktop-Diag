//go:build windows

package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	opts, err := Parse(nil)
	if err != nil {
		t.Fatalf("空参数不应报错: %v", err)
	}
	if opts.OutputDir != "" || opts.Verbose || opts.ShowVersion || opts.ShowHelp {
		t.Fatalf("默认值应为全零值，实际 %+v", opts)
	}
}

func TestParseAllOptions(t *testing.T) {
	opts, err := Parse([]string{"-o", `D:\out`, "-v"})
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if opts.OutputDir != `D:\out` {
		t.Errorf("OutputDir = %q，期望 D:\\out", opts.OutputDir)
	}
	if !opts.Verbose {
		t.Error("Verbose 应为 true")
	}
}

func TestParseVersion(t *testing.T) {
	opts, err := Parse([]string{"-version"})
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if !opts.ShowVersion {
		t.Error("ShowVersion 应为 true")
	}
}

func TestParseHelp(t *testing.T) {
	// flag 对未定义的 -h/-help 返回 ErrHelp，必须转成正常退出路径而不是错误。
	for _, arg := range []string{"-h", "-help", "--help"} {
		opts, err := Parse([]string{arg})
		if err != nil {
			t.Errorf("%s 应走正常路径，实际报错 %v", arg, err)
			continue
		}
		if !opts.ShowHelp {
			t.Errorf("%s 应置 ShowHelp", arg)
		}
	}
}

func TestParseRejectsUnknownFlag(t *testing.T) {
	_, err := Parse([]string{"--json"})
	if !errors.Is(err, ErrInvalidArgs) {
		t.Fatalf("未知参数应返回 ErrInvalidArgs，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "json") {
		t.Errorf("错误信息应包含出错参数名，实际 %q", err.Error())
	}
}

func TestParseRejectsPositionalArgs(t *testing.T) {
	_, err := Parse([]string{"diag"})
	if !errors.Is(err, ErrInvalidArgs) {
		t.Fatalf("位置参数应返回 ErrInvalidArgs，实际 %v", err)
	}
}

func TestNormalizeDir(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"空串保持空", "", ""},
		{"纯空白视为未指定", "   ", ""},
		{"去首尾引号", `"D:\out"`, `D:\out`},
		{"去单引号", `'D:\out'`, `D:\out`},
		{"去结尾反斜杠", `D:\out\`, `D:\out`},
		{"去结尾正斜杠", `D:/out/`, `D:\out`},
		// 回归钉子：C:\ 被削成 C: 会改变含义（C: 指 C 盘的当前目录）。
		{"盘符根目录不得被削成 C:", `C:\`, `C:\`},
		// UNC 的 \\server\share\ 与 C:\ 一样是卷根，结尾分隔符必须保留。
		{"UNC 卷根保留结尾分隔符", `\\server\share\`, `\\server\share\`},
		{"UNC 子目录去掉结尾分隔符", `\\server\share\dir\`, `\\server\share\dir`},
		{"正斜杠归一为反斜杠", `D:/a/b`, `D:\a\b`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeDir(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeDir(%q) = %q，期望 %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestUsageCoversContract(t *testing.T) {
	// 用法文本是 -h 的全部输出，四个选项与三个退出码缺一不可。
	for _, want := range []string{"-o", "-v", "-version", "-h, --help", "0 ", "1 ", "2 ", "只读诊断"} {
		if !strings.Contains(Usage, want) {
			t.Errorf("用法文本缺少 %q", want)
		}
	}
}

func TestNormalizeDirIsIdempotent(t *testing.T) {
	for _, in := range []string{`C:\`, `D:\out\`, `\\server\share`} {
		once := normalizeDir(in)
		if twice := normalizeDir(once); twice != once {
			t.Errorf("normalizeDir 不幂等: %q -> %q -> %q", in, once, twice)
		}
		if !filepath.IsAbs(once) {
			t.Errorf("normalizeDir(%q) 应保持绝对路径形态，实际 %q", in, once)
		}
	}
}
