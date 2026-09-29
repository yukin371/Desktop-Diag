//go:build windows

package winapi

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 本文件放包内共用的底层辅助函数与 DLL 句柄。
// DLL 一律用 NewLazySystemDLL 加载：搜索路径被限定在 System32，避免被工作目录或 PATH 中的同名 DLL 劫持。

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	ntdll    = windows.NewLazySystemDLL("ntdll.dll")
	iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
)

// maxAnsiStringLen 是读取 NUL 结尾 ANSI 字符串的硬上限，防止底层数据异常时越界读下去。
const maxAnsiStringLen = 4096

// maxUTF16StringLen 同上，用于 NUL 结尾的 UTF-16 字符串。
const maxUTF16StringLen = 4096

// callError 把 syscall 风格的错误包装成带 API 名称的错误值，供报告直接引用。
func callError(api string, err error) error {
	if err == nil {
		return fmt.Errorf("%s: 调用失败（系统未返回错误码）", api)
	}
	return fmt.Errorf("%s: %w", api, err)
}

// bytePtrToString 读取 NUL 结尾的 ANSI 字符串；不用 windows.BytePtrToString 是为了强制长度上限。
func bytePtrToString(p *byte) string {
	if p == nil {
		return ""
	}
	base := unsafe.Pointer(p)
	buf := make([]byte, 0, 64)
	for i := 0; i < maxAnsiStringLen; i++ {
		c := *(*byte)(unsafe.Add(base, i))
		if c == 0 {
			break
		}
		buf = append(buf, c)
	}
	return string(buf)
}

// utf16PtrToString 读取 NUL 结尾的 UTF-16 字符串，并强制长度上限。
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	base := unsafe.Pointer(p)
	for i := 0; i < maxUTF16StringLen; i++ {
		if *(*uint16)(unsafe.Add(base, i*2)) == 0 {
			// windows.UTF16ToString 需要含结尾 NUL 的切片，故长度为 i+1。
			return windows.UTF16ToString(unsafe.Slice(p, i+1))
		}
	}
	// 未在限长内遇到 NUL：截断为上限长度，避免无限读。
	return windows.UTF16ToString(unsafe.Slice(p, maxUTF16StringLen))
}
