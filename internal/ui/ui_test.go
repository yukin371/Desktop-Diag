//go:build windows

package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

const esc = "\x1b"

func TestPlainModeNeverEmitsEscapes(t *testing.T) {
	// REQ-F-602：输出被重定向时不得出现 ANSI 转义序列。
	var buf bytes.Buffer
	c := New(&buf, ModePlain)

	c.Header("Desktop-Diag v0.1.0")
	c.Step(1, 4, "主机与系统信息", "完成（7ms）", model.SevOK)
	c.Step(2, 4, "网络适配器信息", "失败（权限不足）", model.SevWarning)
	c.Issue(model.Issue{
		RuleID:     "R-14",
		Severity:   model.SevSevere,
		Title:      "内网链路中断",
		Evidence:   []string{"网关 192.168.0.1 丢包 100%", "  "},
		Suggestion: "检查网线与交换机端口",
	})
	c.Summary(1, 1, `C:\reports\diag.txt`, 2190*time.Millisecond)

	if strings.Contains(buf.String(), esc) {
		t.Fatalf("纯文本模式不得输出转义序列:\n%q", buf.String())
	}
}

func TestColorModeEmitsEscapes(t *testing.T) {
	var buf bytes.Buffer
	c := New(&buf, ModeColor)

	c.Step(1, 4, "系统健康度", "内存不足", model.SevSevere)
	if !strings.Contains(buf.String(), esc) {
		t.Fatalf("颜色模式应输出转义序列:\n%q", buf.String())
	}
	if !strings.Contains(buf.String(), ansiRed) {
		t.Errorf("严重告警应使用红色，实际 %q", buf.String())
	}
	if !strings.Contains(buf.String(), ansiReset) {
		t.Errorf("着色后必须复位，否则污染后续输出：%q", buf.String())
	}
}

func TestSeverityTags(t *testing.T) {
	cases := []struct {
		name string
		sev  model.Severity
		want string
	}{
		{"正常不加标记", model.SevOK, "完成"},
		{"警告加文本标记", model.SevWarning, "[警告] 完成"},
		{"严重加文本标记", model.SevSevere, "[严重] 完成"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			c := New(&buf, ModePlain)
			c.Step(1, 1, "项", "完成", tc.sev)
			got := strings.TrimSpace(buf.String())
			if got != "[1/1] 项 ... "+tc.want {
				t.Fatalf("实际 %q", got)
			}
		})
	}
}

func TestStepFormat(t *testing.T) {
	var buf bytes.Buffer
	c := New(&buf, ModePlain)
	c.Step(2, 4, "网络适配器信息", "完成（9ms）", model.SevOK)

	if got, want := strings.TrimSpace(buf.String()), "[2/4] 网络适配器信息 ... 完成（9ms）"; got != want {
		t.Fatalf("实际 %q，期望 %q", got, want)
	}
}

func TestSummaryHasThreeElements(t *testing.T) {
	// REQ-F-604：告警数、报告路径、总耗时三要素缺一不可。
	var buf bytes.Buffer
	c := New(&buf, ModePlain)
	c.Summary(2, 3, `C:\reports\diag_20251005_120000.txt`, 2190*time.Millisecond)
	out := buf.String()

	for _, want := range []string{"严重告警：2", "一般告警：3", `C:\reports\diag_20251005_120000.txt`, "2.19s"} {
		if !strings.Contains(out, want) {
			t.Errorf("汇总缺少 %q，实际:\n%s", want, out)
		}
	}
}

func TestVerbosefIsGated(t *testing.T) {
	var buf bytes.Buffer
	c := New(&buf, ModePlain)

	c.Verbosef("不应出现")
	if buf.Len() != 0 {
		t.Fatalf("未开详细模式却输出了内容: %q", buf.String())
	}

	c.SetVerbose(true)
	c.Verbosef("来源=%s", "GetAdaptersAddresses")
	if !strings.Contains(buf.String(), "来源=GetAdaptersAddresses") {
		t.Fatalf("详细模式应输出内容，实际 %q", buf.String())
	}
}

func TestIssueDropsBlankEvidence(t *testing.T) {
	var buf bytes.Buffer
	c := New(&buf, ModePlain)
	c.Issue(model.Issue{
		RuleID:   "R-03",
		Severity: model.SevWarning,
		Title:    "DNS 未配置",
		Evidence: []string{"", "   ", "以太网：无 DNS"},
	})

	out := buf.String()
	if strings.Count(out, "以太网：无 DNS") != 1 {
		t.Fatalf("证据应恰好出现一次，实际:\n%s", out)
	}
	if strings.Contains(out, "\n        \n") {
		t.Fatalf("空证据行应被剔除，实际:\n%q", out)
	}
}

func TestSetupFallsBackToPlainForNonFileWriter(t *testing.T) {
	// 测试用的 bytes.Buffer 没有控制台句柄可探测，必须降级为纯文本。
	var buf bytes.Buffer
	c := Setup(&buf)
	if c == nil {
		t.Fatal("Setup 不应返回 nil")
	}
	if c.Mode() != ModePlain {
		t.Errorf("非文件输出应降级为纯文本，实际 %v", c.Mode())
	}

	c.Line("中文 %s", "测试")
	if strings.Contains(buf.String(), esc) {
		t.Fatalf("降级后不得出现转义序列: %q", buf.String())
	}
}

func TestColorHelpersSwitchOnMode(t *testing.T) {
	plain := New(&bytes.Buffer{}, ModePlain)
	if plain.OK("x") != "x" {
		t.Errorf("纯文本模式 OK 应返回原文，实际 %q", plain.OK("x"))
	}
	if plain.Mode() != ModePlain {
		t.Error("Mode 应报告纯文本")
	}

	var buf bytes.Buffer
	color := New(&buf, ModeColor)
	if got := color.OK("正常"); got != ansiGreen+"正常"+ansiReset {
		t.Errorf("颜色模式 OK 应着色，实际 %q", got)
	}
	color.Header("横幅")
	if !strings.Contains(buf.String(), ansiBold) {
		t.Errorf("颜色模式横幅应加粗，实际 %q", buf.String())
	}
}

func TestHeaderOmitsEscapesInPlainMode(t *testing.T) {
	var buf bytes.Buffer
	c := New(&buf, ModePlain)
	c.Header("Desktop-Diag dev")

	out := buf.String()
	if strings.Contains(out, esc) {
		t.Fatalf("纯文本横幅不应含转义序列: %q", out)
	}
	if !strings.Contains(out, "只读诊断") {
		t.Errorf("横幅应含合规声明，实际 %q", out)
	}
}
