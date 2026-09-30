//go:build windows

// Queries native Windows version information without compatibility manifest overrides.
package winapi

import (
	"unsafe"
)

// 本文件封装 ntdll.dll 中的只读查询类 API。

var procRtlGetVersion = ntdll.NewProc("RtlGetVersion")

// RtlOsVersionInfoW 对应 RTL_OSVERSIONINFOW（276 字节，对齐 4）。
// OSVersionInfoSize 必须填 276：RtlGetVersion 据此判断结构体版本，填成 EX 版的 284 会越界写。
// 不用 GetVersionEx：Windows 8.1 起未声明兼容清单的进程会被它谎报成 6.2。
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
// 返回值是 NTSTATUS，**不是** Win32 的 GetLastError 语义，失败时只能把 NTSTATUS 原样报出。
func RtlGetVersion() (*RtlOsVersionInfoW, error) {
	var v RtlOsVersionInfoW
	// Size 字段必须由本函数填写，不能交给调用方（填 276 即声明传的是非 EX 版本）。
	v.OSVersionInfoSize = uint32(unsafe.Sizeof(v))

	status, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&v)))
	if status != statusSuccess {
		return nil, &NTStatusError{API: "RtlGetVersion", Status: uint32(status)}
	}
	return &v, nil
}

// NTStatusError 表示一个 NTSTATUS 非 0 的失败。
// 不复用 syscall.Errno：NTSTATUS 与 Win32 错误码是两套编码，混用会让错误信息产生误导。
type NTStatusError struct {
	API    string
	Status uint32
}

// Error formats the native status code without discarding its numeric identity.
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
