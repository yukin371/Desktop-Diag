//go:build windows

// Opens only a completed local report through the Windows default file association.
package winapi

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
)

// OpenReport validates the local report before invoking its default viewer, without a command shell.
func OpenReport(path string) error {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return fmt.Errorf("自动打开仅支持本地绝对报告路径: %q", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".html" && ext != ".txt" {
		return fmt.Errorf("自动打开不支持报告格式: %q", ext)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("检查待打开报告失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("待打开报告不是普通文件: %q", path)
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("转换报告路径失败: %w", err)
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("转换报告打开操作失败: %w", err)
	}
	if err := windows.ShellExecute(0, verb, file, nil, nil, reportShowNormal); err != nil {
		return fmt.Errorf("打开报告查看器失败: %w", err)
	}
	return nil
}
