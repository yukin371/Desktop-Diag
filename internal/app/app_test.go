//go:build windows

package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/report"
)

// fixedNow 让报告文件名与耗时可控，避免测试依赖真实时钟。
func fixedNow() time.Time {
	return time.Date(2025, 10, 5, 12, 0, 0, 0, time.Local)
}

func TestRunHelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-h"}, &out, &errOut, fixedNow); code != ExitOK {
		t.Fatalf("帮助应返回退出码 0，实际 %d", code)
	}
	if !strings.Contains(out.String(), "合规声明") {
		t.Errorf("帮助应含合规声明，实际:\n%s", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("帮助不应写 stderr，实际:\n%s", errOut.String())
	}
}

func TestRunVersionExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-version"}, &out, &errOut, fixedNow); code != ExitOK {
		t.Fatalf("版本应返回退出码 0，实际 %d", code)
	}
	if !strings.Contains(out.String(), "Desktop-Diag") {
		t.Errorf("版本输出异常:\n%s", out.String())
	}
}

func TestRunInvalidArgsReturnsExit2(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--json"}, &out, &errOut, fixedNow); code != ExitError {
		t.Fatalf("非法参数应返回退出码 2，实际 %d", code)
	}
	if errOut.Len() == 0 {
		t.Error("非法参数应把原因写到 stderr")
	}
	if !strings.Contains(errOut.String(), "用法：") {
		t.Errorf("非法参数应附带用法，实际:\n%s", errOut.String())
	}
}

func TestRunWritesReportAndSummary(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer

	code := run([]string{"-o", dir}, &out, &errOut, fixedNow)
	if code != ExitOK && code != ExitSevere {
		t.Fatalf("诊断应正常结束，实际退出码 %d，stderr=%s", code, errOut.String())
	}

	files, err := filepath.Glob(filepath.Join(dir, "diag_*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("应在 -o 目录产出恰好 1 份报告，实际 %d 份: %v", len(files), files)
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	// REQ-F-504 / REQ-F-505：记事本双击不乱码、正常换行。
	if !bytes.HasPrefix(data, []byte(report.BOM)) {
		t.Error("报告缺少 UTF-8 BOM")
	}
	if !bytes.Contains(data, []byte("\r\n")) {
		t.Error("报告应使用 CRLF 换行")
	}
	if bytes.Contains(data, []byte("\r\r\n")) {
		t.Error("报告出现 \\r\\r\\n，记事本会多显示空行")
	}

	// REQ-F-601 / REQ-F-604：进度行与三要素汇总。
	stdout := out.String()
	for _, want := range []string{"[1/4]", "严重告警：", "一般告警：", "报告路径：" + files[0], "总耗时："} {
		if !strings.Contains(stdout, want) {
			t.Errorf("控制台输出缺少 %q，实际:\n%s", want, stdout)
		}
	}
	// REQ-F-602：重定向时必须无 ANSI 转义序列。
	if strings.Contains(stdout, "\x1b") {
		t.Errorf("重定向输出不得含转义序列:\n%q", stdout)
	}
}

func TestRunDegradesWhenExplicitDirUnusable(t *testing.T) {
	// 用一个普通文件占住路径，使其下的目录无法创建，逼迫降级链生效。
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := run([]string{"-o", filepath.Join(blocker, "sub")}, &out, &errOut, fixedNow)
	if code != ExitOK && code != ExitSevere {
		t.Fatalf("降级链应救回写入，实际退出码 %d，stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "降级") {
		t.Errorf("应提示发生降级，实际:\n%s", out.String())
	}

	// 清理降级后落在别处的报告，避免污染构建缓存目录或用户目录。
	written := reportPathFromOutput(out.String())
	if written == "" {
		t.Fatal("输出中找不到报告路径")
	}
	if err := os.Remove(written); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Logf("清理降级报告失败（不影响断言）: %v", err)
	}
}

func TestRunDegradedReportRecordsActualPath(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := run([]string{"-o", filepath.Join(blocker, "sub")}, &out, &errOut, fixedNow); code == ExitError {
		t.Fatalf("不应以退出码 2 结束，stderr=%s", errOut.String())
	}

	written := reportPathFromOutput(out.String())
	if written == "" {
		t.Fatal("输出中找不到报告路径")
	}
	defer os.Remove(written)

	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	// REQ-F-506：报告头必须标注实际路径并说明降级原因。
	if !bytes.Contains(data, []byte(written)) {
		t.Error("报告头未标注实际落盘路径")
	}
	if !bytes.Contains(data, []byte("降级")) {
		t.Error("报告头未说明降级原因")
	}
}

func TestErrTextCollapsesNewlines(t *testing.T) {
	if got := errText(errors.New("第一行\n第二行")); strings.Contains(got, "\n") {
		t.Fatalf("错误文本应压成单行，实际 %q", got)
	}
	if got := errText(nil); got != "未知原因" {
		t.Fatalf("nil 错误应给出兜底文案，实际 %q", got)
	}
}

// reportPathFromOutput 从控制台输出里取出「报告路径：」后的值。
func reportPathFromOutput(s string) string {
	const marker = "报告路径："
	i := strings.LastIndex(s, marker)
	if i < 0 {
		return ""
	}
	rest := s[i+len(marker):]
	if j := strings.IndexAny(rest, "\r\n"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}
