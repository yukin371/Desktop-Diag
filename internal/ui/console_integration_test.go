//go:build windows

package ui

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// TestSetupOnRealConsole 用 CONOUT$ 取到一个真正的控制台句柄，验证 REQ-F-602 的彩色档。
//
// go test 的子进程 stdout 常是管道，未必挂着控制台；拿不到就跳过——这属于环境缺失，
// 不是失败。重定向档（无 ANSI、保留 Unicode）由 ui_test.go 的纯文本用例覆盖。
func TestSetupOnRealConsole(t *testing.T) {
	h, err := windows.CreateFile(
		windows.StringToUTF16Ptr("CONOUT$"),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Skipf("当前进程没有控制台，跳过真实控制台用例: %v", err)
	}

	f := os.NewFile(uintptr(h), "CONOUT$")
	if f == nil {
		if err := windows.CloseHandle(h); err != nil {
			t.Errorf("释放控制台句柄失败: %v", err)
		}
		t.Fatal("把 CONOUT$ 句柄包装成 *os.File 失败")
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("关闭控制台句柄失败: %v", err)
		}
	})
	originalMode, err := winapi.GetConsoleMode(f.Fd())
	if err != nil {
		t.Fatal(err)
	}
	originalCP := winapi.GetConsoleOutputCP()
	t.Cleanup(func() {
		if err := winapi.SetConsoleMode(f.Fd(), originalMode); err != nil {
			t.Errorf("恢复测试前模式失败: %v", err)
		}
		if err := winapi.SetConsoleOutputCP(originalCP); err != nil {
			t.Errorf("恢复测试前代码页失败: %v", err)
		}
	})

	if !winapi.IsConsole(f.Fd()) {
		t.Fatal("CONOUT$ 的句柄应被判定为控制台")
	}
	if winapi.GetConsoleOutputCP() == 0 {
		t.Error("GetConsoleOutputCP() 返回 0，读不到输出代码页")
	}
	if !winapi.EnableVTProcessing(f.Fd()) {
		t.Skip("该环境无法开启 VT 处理，彩色档在此不可用")
	}

	// NO_COLOR 是文档化的显式覆盖，必须优先于自动探测。
	if os.Getenv("NO_COLOR") != "" {
		c := Setup(f)
		if c.Mode() != ModePlain {
			t.Errorf("设置了 NO_COLOR 时应降级为纯文本，实际 %v", c.Mode())
		}
		if err := c.Close(); err != nil {
			t.Errorf("恢复无色控制台失败: %v", err)
		}
		t.Setenv("NO_COLOR", "")
	}

	c := Setup(f)
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("恢复控制台失败: %v", err)
		}
	})
	if c.Mode() != ModeColor {
		t.Fatalf("真实控制台 + VT 可用时 Mode = %v，期望 ModeColor", c.Mode())
	}
	// 「彩色」必须真的带 ANSI 转义，否则只是名字好听。
	if got := c.tag(model.SevSevere, "[R-01] 示例"); got == "[严重] [R-01] 示例" {
		t.Errorf("彩色档未输出 ANSI 转义，实际 %q", got)
	}
	if cp := winapi.GetConsoleOutputCP(); cp != winapi.CPUTF8 {
		t.Errorf("Setup 后控制台输出代码页 = %d，期望 %d", cp, winapi.CPUTF8)
	}
	if err := c.Close(); err != nil {
		t.Errorf("恢复控制台失败: %v", err)
	}
}
