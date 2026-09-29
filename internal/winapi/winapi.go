//go:build windows

package winapi

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 本文件放 winapi 包内共用的底层辅助函数与 DLL 句柄。
//
// 所有 DLL 一律通过 windows.NewLazySystemDLL 加载：它把搜索路径限制在
// System32 目录，不会因为当前工作目录或 PATH 中存在同名 DLL 而被劫持
// （DLL 搜索顺序劫持是这类工具常见的攻击面）。

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	ntdll    = windows.NewLazySystemDLL("ntdll.dll")
	iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
)

// maxAnsiStringLen 是读取以 NUL 结尾的 ANSI 字符串时的硬上限。
// 正常网卡名（{GUID} 形式）不超过 256 字节；设上限是为了在底层数据异常时
// 不至于越界读下去——诊断工具本身绝不能因为被诊断对象的数据异常而崩溃。
const maxAnsiStringLen = 4096

// maxUTF16StringLen 同上，用于以 NUL 结尾的 UTF-16 字符串。
const maxUTF16StringLen = 4096

// callError 把 syscall 风格的错误统一包装成带 API 名称的错误值。
//
// Windows 的 LazyProc.Call 在失败时返回的 err 通常是 syscall.Errno；
// 但若 Proc 本身没找到，err 会是 ERROR_PROC_NOT_FOUND 之类的值。
// 统一在这里补上 API 名，便于报告第二层直接引用（REQ-F-703）。
func callError(api string, err error) error {
	if err == nil {
		return fmt.Errorf("%s: 调用失败（系统未返回错误码）", api)
	}
	return fmt.Errorf("%s: %w", api, err)
}

// bytePtrToString 读取以 NUL 结尾的 ANSI 字符串。
//
// 不直接用 windows.BytePtrToString 是为了显式加上长度上限（见 maxAnsiStringLen）。
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

// utf16PtrToString 读取以 NUL 结尾的 UTF-16 字符串，并加上长度上限。
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	base := unsafe.Pointer(p)
	for i := 0; i < maxUTF16StringLen; i++ {
		if *(*uint16)(unsafe.Add(base, i*2)) == 0 {
			// 长度为 i+1 个 uint16（含结尾 NUL），交给标准库转换。
			return windows.UTF16ToString(unsafe.Slice(p, i+1))
		}
	}
	// 未在限长内遇到 NUL：截断为上限长度，避免无限读。
	return windows.UTF16ToString(unsafe.Slice(p, maxUTF16StringLen))
}
