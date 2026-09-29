//go:build windows

package winapi

import (
	"errors"
	"net"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 本文件只测不接触系统的纯函数。
// 纯函数里的长度上限、字节序、边界长度都是**安全性质**，不能只靠真机碰运气覆盖。

func TestPureFormatMAC(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"nil 返回空串", nil, ""},
		{"空切片返回空串", []byte{}, ""},
		{"单字节不得带前导冒号", []byte{0x00}, "00"},
		{"典型网卡地址", []byte{0x00, 0x1A, 0x2B, 0x3C, 0x4D, 0x5E}, "00:1A:2B:3C:4D:5E"},
		{"必须是大写", []byte{0xAB, 0xCD, 0xEF}, "AB:CD:EF"},
		{"全 F 边界", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, "FF:FF:FF:FF:FF:FF"},
		{"低半字节不得漏前导零", []byte{0x0A, 0x0B}, "0A:0B"},
		{"超出 MAC 长度也不截断", make([]byte, 10), "00:00:00:00:00:00:00:00:00:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMAC(tt.in); got != tt.want {
				t.Errorf("formatMAC(%v) = %q, 期望 %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestPureIPAddrByteOrder 双向覆盖 IPAddr 的字节序。
//
// 这是本仓最贵的一次教训：写反了 probe **不会失败**。0x7F000001 的内存字节是
// 01 00 00 7F，即 1.0.0.127 —— 一个真实存在、会正常应答的公网地址，于是回环探测
// 一直"成功"，只是 RTT 从 0ms 变成 195ms 的互联网延迟；网关那次则是在探测
// 1.0.168.192，RTT 263ms。集成测试只断言「有应答」，所以完全抓不住。
func TestPureIPAddrByteOrder(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		addr uint32
	}{
		{"回环", "127.0.0.1", 0x0100007F},
		{"网关典型值", "192.168.1.1", 0x0101A8C0},
		{"阿里 DNS", "223.5.5.5", 0x050505DF},
		{"全零", "0.0.0.0", 0x00000000},
		{"全一", "255.255.255.255", 0xFFFFFFFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip4 := net.ParseIP(tt.ip).To4()
			if ip4 == nil {
				t.Fatalf("%q 不是 IPv4 地址", tt.ip)
			}
			if got := ipAddrFromIPv4(ip4); got != tt.addr {
				t.Errorf("ipAddrFromIPv4(%s) = 0x%08X, 期望 0x%08X", tt.ip, got, tt.addr)
			}
			if got := ipv4FromIPAddr(tt.addr); got != tt.ip {
				t.Errorf("ipv4FromIPAddr(0x%08X) = %q, 期望 %q", tt.addr, got, tt.ip)
			}
		})
	}
}

func TestPureHex32(t *testing.T) {
	tests := []struct {
		in   uint32
		want string
	}{
		{0x00000000, "00000000"},
		{0xC0000001, "C0000001"},
		{0xFFFFFFFF, "FFFFFFFF"},
		{0x0000000F, "0000000F"},
	}
	for _, tt := range tests {
		if got := hex32(tt.in); got != tt.want {
			t.Errorf("hex32(0x%08X) = %q, 期望 %q", tt.in, got, tt.want)
		}
	}
}

// TestPureCallError 断言 nil 错误不会产生 "nil" 字样，否则报告里会出现 "GetFoo: <nil>" 这种无意义的诊断信息。
func TestPureCallError(t *testing.T) {
	err := callError("GetFoo", nil)
	if err == nil {
		t.Fatal("callError(api, nil) 不应返回 nil")
	}
	if strings.Contains(err.Error(), "<nil>") || strings.Contains(err.Error(), "nil") {
		t.Errorf("错误信息不应出现 nil 字样: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "GetFoo") {
		t.Errorf("错误信息必须带上 API 名，实际 %q", err.Error())
	}

	sentinel := errors.New("底层失败")
	wrapped := callError("GetBar", sentinel)
	if !errors.Is(wrapped, sentinel) {
		t.Errorf("callError 必须用 %%w 包装以保留 errors.Is 语义，实际 %v", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "GetBar") {
		t.Errorf("错误信息必须带上 API 名，实际 %q", wrapped.Error())
	}
}

func TestPureBytePtrToString(t *testing.T) {
	if got := bytePtrToString(nil); got != "" {
		t.Errorf("nil 指针应返回空串，实际 %q", got)
	}

	buf := append([]byte("网卡描述 abc"), 0)
	if got := bytePtrToString(&buf[0]); got != "网卡描述 abc" {
		t.Errorf("bytePtrToString = %q, 期望 %q", got, "网卡描述 abc")
	}

	// 长度上限：填满足够多的非零字节，必须在 maxAnsiStringLen 处停下而不是越读。
	big := make([]byte, maxAnsiStringLen+64)
	for i := range big {
		big[i] = 'A'
	}
	if got := bytePtrToString(&big[0]); len(got) != maxAnsiStringLen {
		t.Errorf("无 NUL 结尾时应截断为 %d 字节，实际 %d", maxAnsiStringLen, len(got))
	}
}

func TestPureUTF16PtrToString(t *testing.T) {
	if got := utf16PtrToString(nil); got != "" {
		t.Errorf("nil 指针应返回空串，实际 %q", got)
	}

	s, err := windows.UTF16PtrFromString("中文适配器名称 Test")
	if err != nil {
		t.Fatalf("构造 UTF-16 指针失败: %v", err)
	}
	if got := utf16PtrToString(s); got != "中文适配器名称 Test" {
		t.Errorf("utf16PtrToString = %q, 期望 %q", got, "中文适配器名称 Test")
	}

	// 长度上限：全是 'A' 且没有 NUL，必须在 maxUTF16StringLen 处停下。
	big := make([]uint16, maxUTF16StringLen+64)
	for i := range big {
		big[i] = 'A'
	}
	got := utf16PtrToString(&big[0])
	if len([]rune(got)) != maxUTF16StringLen {
		t.Errorf("无 NUL 结尾时应截断为 %d 个 UTF-16 单元，实际 %d", maxUTF16StringLen, len([]rune(got)))
	}
}

// TestPureFiletimeToUint64 断言高低位的拼接顺序：写反了绝对值离谱，但相对比较看着正常，极难察觉。
func TestPureFiletimeToUint64(t *testing.T) {
	tests := []struct {
		name string
		ft   windows.Filetime
		want uint64
	}{
		{"全零", windows.Filetime{LowDateTime: 0, HighDateTime: 0}, 0},
		{"低位单独", windows.Filetime{LowDateTime: 1, HighDateTime: 0}, 1},
		{"高位单独", windows.Filetime{LowDateTime: 0, HighDateTime: 1}, 0x1_0000_0000},
		{"高低位组合", windows.Filetime{LowDateTime: 0xFFFF_FFFF, HighDateTime: 0x0000_0001}, 0x1_FFFF_FFFF},
		{"高位不得被截断", windows.Filetime{LowDateTime: 0, HighDateTime: 0xFFFF_FFFF}, 0xFFFF_FFFF_0000_0000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filetimeToUint64(tt.ft); got != tt.want {
				t.Errorf("filetimeToUint64(%+v) = 0x%X, 期望 0x%X", tt.ft, got, tt.want)
			}
		})
	}
}

// TestPureSystemTimesMath 固化「Kernel 含 Idle」这一容易写错的口径。
// kernelTime **已包含** idleTime：busy = (kernel - idle) + user，total = kernel + user；写成 busy = kernel + user 会显著高估占用率。
func TestPureSystemTimesMath(t *testing.T) {
	st := SystemTimes{Idle: 30, Kernel: 100, User: 20}
	busy := (st.Kernel - st.Idle) + st.User
	total := st.Kernel + st.User
	if busy != 90 || total != 120 {
		t.Fatalf("口径被改动了：busy=%d total=%d，期望 90 / 120", busy, total)
	}
	pct := float64(busy) / float64(total) * 100
	if pct < 74.9 || pct > 75.1 {
		t.Errorf("CPU 占用率 = %.2f%%，期望约 75%%", pct)
	}
}

// TestPureStructLayoutSystemTimes 确认 SystemTimes 就是三个连续的 uint64。
// 它直接映射 GetSystemTimes 的三个 FILETIME 出参；一旦被加字段或改类型，上面的算术口径和 API 调用都会失配。
func TestPureStructLayoutSystemTimes(t *testing.T) {
	var st SystemTimes
	if got := unsafe.Sizeof(st); got != 24 {
		t.Fatalf("SystemTimes sizeof = %d，期望 24", got)
	}
	if got := unsafe.Offsetof(st.Idle); got != 0 {
		t.Errorf("Idle 偏移 = %d，期望 0", got)
	}
	if got := unsafe.Offsetof(st.Kernel); got != 8 {
		t.Errorf("Kernel 偏移 = %d，期望 8", got)
	}
	if got := unsafe.Offsetof(st.User); got != 16 {
		t.Errorf("User 偏移 = %d，期望 16", got)
	}
}
