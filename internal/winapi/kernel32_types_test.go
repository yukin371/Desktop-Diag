//go:build windows

package winapi

import (
	"testing"
	"unsafe"
)

// kernel32 / ntdll 侧手写结构体的布局断言。
// 期望值来源与 iphlpapi_types_test.go 相同：Windows SDK 头文件 + tools/layout-probe。

func TestStructLayoutMemoryStatusEx(t *testing.T) {
	var m MemoryStatusEx

	tests := []struct {
		field string
		got   uintptr
		want  uintptr
	}{
		{"Length", unsafe.Offsetof(m.Length), 0},
		{"MemoryLoad", unsafe.Offsetof(m.MemoryLoad), 4},
		{"TotalPhys", unsafe.Offsetof(m.TotalPhys), 8},
		{"AvailPhys", unsafe.Offsetof(m.AvailPhys), 16},
		{"TotalPageFile", unsafe.Offsetof(m.TotalPageFile), 24},
		{"AvailPageFile", unsafe.Offsetof(m.AvailPageFile), 32},
		{"TotalVirtual", unsafe.Offsetof(m.TotalVirtual), 40},
		{"AvailVirtual", unsafe.Offsetof(m.AvailVirtual), 48},
		{"AvailExtendedVirtual", unsafe.Offsetof(m.AvailExtendedVirtual), 56},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("MemoryStatusEx.%s 偏移 = %d, 期望 %d", tt.field, tt.got, tt.want)
		}
	}

	// sizeof 与 dwLength 必须一致：GlobalMemoryStatusEx 会拿 dwLength 做校验，
	// 任何多余的填充都会让 API 返回 ERROR_INVALID_PARAMETER(87)。
	if got := unsafe.Sizeof(MemoryStatusEx{}); got != MemoryStatusExSize {
		t.Errorf("MemoryStatusEx sizeof = %d, 期望 %d", got, MemoryStatusExSize)
	}
	// 显式断言「没有填充」：8 + 7*8 = 64。
	if got := unsafe.Offsetof(m.AvailExtendedVirtual) + 8; got != MemoryStatusExSize {
		t.Errorf("MemoryStatusEx 尾部偏移 = %d 与 sizeof %d 不一致，说明存在意外填充", got, MemoryStatusExSize)
	}
}

// TestStructLayoutRtlOsVersionInfoW 断言的是 **非 EX** 版本（RTL_OSVERSIONINFOW，276 字节）。
//
// 注意不要把这个期望值改成 284：284 是 RTL_OSVERSIONINFOEXW 的大小，后者在
// szCSDVersion 之后还有 wServicePackMajor 等 5 个字段。RtlGetVersion 按这个
// Size 字段判断调用方传的是哪个版本，填 284 会让它按 284 字节写我们的
// 276 字节结构体，导致越界写。tools/layout-probe 已实测两者的差异。
func TestStructLayoutRtlOsVersionInfoW(t *testing.T) {
	var v RtlOsVersionInfoW

	tests := []struct {
		field string
		got   uintptr
		want  uintptr
	}{
		{"OSVersionInfoSize", unsafe.Offsetof(v.OSVersionInfoSize), 0},
		{"MajorVersion", unsafe.Offsetof(v.MajorVersion), 4},
		{"MinorVersion", unsafe.Offsetof(v.MinorVersion), 8},
		{"BuildNumber", unsafe.Offsetof(v.BuildNumber), 12},
		{"PlatformID", unsafe.Offsetof(v.PlatformID), 16},
		{"CSDVersion", unsafe.Offsetof(v.CSDVersion), 20},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("RtlOsVersionInfoW.%s 偏移 = %d, 期望 %d", tt.field, tt.got, tt.want)
		}
	}

	if got := unsafe.Sizeof(RtlOsVersionInfoW{}); got != RtlOsVersionInfoWSize {
		t.Errorf("RtlOsVersionInfoW sizeof = %d, 期望 %d", got, RtlOsVersionInfoWSize)
	}
	// CSDVersion 是 WCHAR[128]，必须正好 256 字节且紧接着 PlatformID。
	if got, want := unsafe.Sizeof(v.CSDVersion), uintptr(256); got != want {
		t.Errorf("CSDVersion 大小 = %d, 期望 %d", got, want)
	}
}
