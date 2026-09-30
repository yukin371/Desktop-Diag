//go:build windows

// 本文件集中定义控制台代码页与模式的 Win32 常量。
package winapi

// 控制台输出编码及 VT 处理位。
const (
	CPUTF8                          = 65001
	enableVirtualTerminalProcessing = 0x0004
	enableProcessedOutput           = 0x0001
)
