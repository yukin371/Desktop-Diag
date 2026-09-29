//go:build windows

package winapi

import (
	"unsafe"
)

// 本文件封装 ntdll.dll 中的只读查询类 API。

var procRtlGetVersion = ntdll.NewProc("RtlGetVersion")

// RtlOsVersionInfoW 对应 RTL_OSVERSIONINFOW（276 字节，对齐 4）。
//
// # 为什么是"非 EX"版本
//
// RtlGetVersion 依据调用方传入的 dwOSVersionInfoSize **判断用的是哪个结构体版本**：
// RTL_OSVERSIONINFOW 是 276 字节，而 RTL_OSVERSIONINFOEXW 在 szCSDVersion 之后
// 还多 5 个字段（wServicePackMajor/Minor、wSuiteMask、wProductType、wReserved），
// 共 284 字节（已由 tools/layout-probe 用 MSVC 实测确认）。
//
// 本工具只需要主/次/内部版本号，这些字段两者完全一致且都在前 20 字节，
// 所以声明并传入非 EX 版本即可。**绝不能**把这个字段填成 284——
// 那会让 RtlGetVersion 以为缓冲区有 284 字节，而实际只有 276，造成越界写。
//
// # 为什么不用 GetVersionEx
//
// GetVersionEx 从 Windows 8.1 起会对未在应用清单中声明支持版本的进程**谎报**
// （一律返回 6.2），除非附带 manifest。RtlGetVersion 是 ntdll 的底层实现，
// 不做这层伪装。阶段 1 已在真机验证：本机返回 10.0.26200，与 `ver` 命令一致。
type RtlOsVersionInfoW struct {
	OSVersionInfoSize uint32
	MajorVersion      uint32
	MinorVersion      uint32
	BuildNumber       uint32
	PlatformID        uint32
	CSDVersion        [128]uint16
}

// RtlOsVersionInfoWSize 是 RTL_OSVERSIONINFOW 的字节数（固定 276）。
const RtlOsVersionInfoWSize = 276

// statusSuccess 是 NTSTATUS 的成功码。
const statusSuccess = 0

// RtlGetVersion 返回操作系统真实版本号。
//
// 返回值为 NTSTATUS：0 表示成功，**不是** Win32 的 GetLastError 语义，
// 所以失败时不能去读 GetLastError，只能把 NTSTATUS 原样报出。
func RtlGetVersion() (*RtlOsVersionInfoW, error) {
	var v RtlOsVersionInfoW
	// 与 MEMORYSTATUSEX 同理：Size 字段必须由本函数填写，不能交给调用方。
	// 这里填的是 sizeof(RTL_OSVERSIONINFOW)=276，即告诉系统"我传的是非 EX 版本"。
	v.OSVersionInfoSize = uint32(unsafe.Sizeof(v))

	// RtlGetVersion 返回 NTSTATUS，不经 GetLastError，故忽略第三个返回值。
	status, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&v)))
	if status != statusSuccess {
		return nil, &NTStatusError{API: "RtlGetVersion", Status: uint32(status)}
	}
	return &v, nil
}

// NTStatusError 表示一个 NTSTATUS 非 0 的失败。
//
// 单独定义而不复用 syscall.Errno，是因为 NTSTATUS 与 Win32 错误码是两套编码，
// 混用会让错误信息产生误导。
type NTStatusError struct {
	API    string
	Status uint32
}

func (e *NTStatusError) Error() string {
	return e.API + ": NTSTATUS 0x" + hex32(e.Status)
}

// hex32 以固定 8 位十六进制格式化，避免不同失败码看起来长度不一。
func hex32(v uint32) string {
	const digits = "0123456789ABCDEF"
	var buf [8]byte
	for i := 7; i >= 0; i-- {
		buf[i] = digits[v&0xF]
		v >>= 4
	}
	return string(buf[:])
}
