//go:build windows

// Queries and temporarily sets shared console output state for restoration by ui.
package winapi

import (
	"unsafe"
)

// 本文件封装控制台查询，以及仅影响本进程控制台的代码页与模式设置。
// 代码页与模式会影响共享控制台；调用方必须保存并恢复，进程退出不保证自动恢复。

var (
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
	procGetConsoleOutputCP = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleCP       = kernel32.NewProc("SetConsoleCP")
)

// VTOutputMode 返回启用 VT 所需的输出模式，不改变控制台。
func VTOutputMode(mode uint32) uint32 {
	return mode | enableVirtualTerminalProcessing | enableProcessedOutput
}

// GetConsoleMode 返回控制台句柄的当前模式；句柄不是控制台（输出被重定向）时返回错误。
func GetConsoleMode(handle uintptr) (uint32, error) {
	var mode uint32
	r, _, err := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return 0, callError("GetConsoleMode", err)
	}
	return mode, nil
}

// IsConsole 报告句柄是否连接到一个控制台。
func IsConsole(handle uintptr) bool {
	_, err := GetConsoleMode(handle)
	return err == nil
}

// SetConsoleMode 设置控制台句柄模式。
func SetConsoleMode(handle uintptr, mode uint32) error {
	r, _, err := procSetConsoleMode.Call(handle, uintptr(mode))
	if r == 0 {
		return callError("SetConsoleMode", err)
	}
	return nil
}

// GetConsoleOutputCP 返回当前控制台输出代码页。
func GetConsoleOutputCP() uint32 {
	r, _, _ := procGetConsoleOutputCP.Call()
	return uint32(r)
}

// SetConsoleOutputCP 设置控制台输出代码页。
func SetConsoleOutputCP(cp uint32) error {
	r, _, err := procSetConsoleOutputCP.Call(uintptr(cp))
	if r == 0 {
		return callError("SetConsoleOutputCP", err)
	}
	return nil
}

// SetConsoleCP 设置控制台输入代码页。
func SetConsoleCP(cp uint32) error {
	r, _, err := procSetConsoleCP.Call(uintptr(cp))
	if r == 0 {
		return callError("SetConsoleCP", err)
	}
	return nil
}

// EnableVTProcessing 尝试为句柄打开 ANSI 转义序列支持，返回是否成功。
// 返回 false **不是**错误路径：老系统或受限环境无法开启，调用方应降级为无颜色的纯文本输出。
func EnableVTProcessing(handle uintptr) bool {
	mode, err := GetConsoleMode(handle)
	if err != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true // 已开启
	}
	// 同时置上 ENABLE_PROCESSED_OUTPUT：某些系统上仅设 VT 位不生效。
	if err := SetConsoleMode(handle, mode|enableVirtualTerminalProcessing|enableProcessedOutput); err != nil {
		return false
	}
	return true
}
