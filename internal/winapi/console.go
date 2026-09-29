//go:build windows

package winapi

import (
	"unsafe"
)

// 本文件封装控制台相关的只读查询与**仅影响本进程控制台显示**的设置。
//
// 关于红线 C-01 的边界：SetConsoleOutputCP / SetConsoleMode 修改的是
// 当前进程所附加控制台的代码页与模式。它们不写入磁盘、不改注册表、
// 不改系统配置，进程退出即随控制台属性失效，且仅在检测到标准输出
// 确实是控制台时才会调用（输出被重定向时完全不动）。这是"让中文正确显示"
// 所必需的最小写操作，已在阶段 0 基线的写入白名单中登记。

var (
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
	procGetConsoleOutputCP = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleCP       = kernel32.NewProc("SetConsoleCP")
)

// 控制台代码页与模式常量。
const (
	// CPUTF8 是 UTF-8 代码页。Windows 控制台默认使用 OEM 代码页
	// （简体中文系统上是 936/GBK），不切换的话写出的 UTF-8 字节会被按 GBK
	// 解释成乱码。
	CPUTF8 = 65001

	// EnableVirtualTerminalProcessing 让控制台把 ANSI 转义序列当作指令而非
	// 普通字符。没有它，"\x1b[31m" 会原样打印出来。
	enableVirtualTerminalProcessing = 0x0004
	enableProcessedOutput           = 0x0001
)

// GetConsoleMode 返回控制台句柄的当前模式。句柄不是控制台时返回错误。
//
// 这也是判断"标准输出是否真的连到控制台"的标准手段：
// 输出被重定向到文件或管道时，这里会失败。
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

// EnableVTProcessing 尝试为句柄打开 ANSI 转义序列支持。
//
// 返回是否成功。失败**不是**错误路径：在 Windows 10 1511 之前的系统、
// 或某些受限环境下无法开启，此时调用方应降级为无颜色的纯文本输出
// （见 ui 包的三级降级矩阵），而不是中断诊断。
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
