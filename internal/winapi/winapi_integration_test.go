//go:build windows

package winapi

import (
	"net"
	"os"
	"testing"
	"time"
)

// 本文件是 winapi 层的**真机集成测试**：它会真的调用 Windows API。
//
// 布局断言测试（*_types_test.go）只能证明"我们声明的结构体与 SDK 一致"，
// 证明不了"API 真的按这个结构体写数据"。本文件补上后半段：
// 用系统返回的真实数值去验证映射正确——例如把 FriendlyName 与
// net.Interfaces() 的名称逐项比对，一旦偏移错位，读到的就是乱码，
// 比对必然失败。
//
// 这些测试依赖具体机器的硬件与网络状态，因此断言只针对"物理上必然成立"的性质
// （数量非负、用量不超过总量、累计计数不回退），不做数值快照。

func TestRuntimeGetAdaptersAddresses(t *testing.T) {
	adapters, err := GetAdaptersAddresses(afUnspec, gaaFlagIncludeGateways|gaaFlagSkipAnycast|gaaFlagSkipMulticast)
	if err != nil {
		t.Fatalf("GetAdaptersAddresses 失败: %v", err)
	}
	if len(adapters) == 0 {
		t.Fatal("GetAdaptersAddresses 返回 0 个适配器，真机上不可能——布局或解析逻辑有误")
	}

	// 每个节点的 Length 已在 parseAdapters 内自检：系统写回的值必须不小于
	// 我们声明的 sizeof(ipAdapterAddressesLH)。若 SDK 布局与映射不一致，
	// 那次自检会直接返回错误，根本走不到这里。

	for _, a := range adapters {
		t.Logf("ifIndex=%-4d v6IfIndex=%-4d ifType=%-4d operStatus=%d mtu=%-5d dhcp=%v\n"+
			"        friendly=%q\n        desc=%q\n        adapterName=%q\n"+
			"        mac=%q (len=%d) dnsSuffix=%q\n        ipv4=%v\n        ipv6=%v\n        dns=%v gw=%v",
			a.IfIndex, a.Ipv6IfIndex, a.IfType, a.OperStatus, a.Mtu, a.Dhcpv4Enabled,
			a.FriendlyName, a.Description, a.AdapterName,
			a.PhysicalAddress, a.PhysicalAddressLength, a.DnsSuffix,
			a.IPv4, a.IPv6, a.DNS, a.Gateways)
	}

	// 交叉验证：Go 标准库的 net.Interfaces() 在 Windows 上同样基于
	// GetAdaptersAddresses 实现，但用的是它自己独立维护的结构体定义。
	// 两套独立映射得到同样的 IfIndex / 名称 / MAC，就说明我们的映射是对的。
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces() 失败: %v", err)
	}

	ours := make(map[uint32]RawAdapter, len(adapters))
	for _, a := range adapters {
		ours[a.IfIndex] = a
	}

	matched := 0
	for _, fi := range ifaces {
		a, ok := ours[uint32(fi.Index)]
		if !ok {
			t.Errorf("net.Interfaces() 报告 ifIndex=%d (%q)，但 GetAdaptersAddresses 未返回该适配器",
				fi.Index, fi.Name)
			continue
		}
		matched++

		if a.FriendlyName != fi.Name {
			t.Errorf("ifIndex=%d 名称不一致：winapi=%q net=%q（若为乱码，说明 FriendlyName 偏移错位）",
				fi.Index, a.FriendlyName, fi.Name)
		}
		if len(fi.HardwareAddr) > 0 {
			wantMAC := formatMAC(fi.HardwareAddr)
			if a.PhysicalAddress != wantMAC {
				t.Errorf("ifIndex=%d MAC 不一致：winapi=%q net=%q", fi.Index, a.PhysicalAddress, wantMAC)
			}
		}
	}

	if matched == 0 {
		t.Fatal("GetAdaptersAddresses 与 net.Interfaces() 没有任何一个适配器对得上，映射很可能整体错位")
	}

	// IPv4/IPv6 地址必须能通过网络库反向解析回同一串文本。
	for _, a := range adapters {
		for _, addr := range append(append([]RawAddr{}, a.IPv4...), a.IPv6...) {
			if addr.IP == "" {
				t.Errorf("ifIndex=%d 出现空地址字符串", a.IfIndex)
				continue
			}
			if net.ParseIP(addr.IP) == nil {
				t.Errorf("ifIndex=%d 解析出非法 IP %q", a.IfIndex, addr.IP)
			}
			if addr.Family == afInet {
				if p := net.ParseIP(addr.IP); p != nil && p.To4() == nil {
					t.Errorf("ifIndex=%d 标为 IPv4 但值为 %q", a.IfIndex, addr.IP)
				}
			}
		}
		for _, d := range append(append([]string{}, a.DNS...), a.Gateways...) {
			if net.ParseIP(d) == nil {
				t.Errorf("ifIndex=%d DNS/网关解析出非法 IP %q", a.IfIndex, d)
			}
		}
	}
}

func TestRuntimeGetAdaptersAddressesFoundSomethingUseful(t *testing.T) {
	adapters, err := GetAdaptersAddresses(afUnspec, gaaFlagIncludeGateways|gaaFlagSkipAnycast|gaaFlagSkipMulticast)
	if err != nil {
		t.Fatalf("GetAdaptersAddresses 失败: %v", err)
	}

	var withName, withMAC, withV4, withGW int
	for _, a := range adapters {
		if a.FriendlyName != "" {
			withName++
		}
		if a.PhysicalAddress != "" {
			withMAC++
		}
		if len(a.IPv4) > 0 {
			withV4++
		}
		if len(a.Gateways) > 0 {
			withGW++
		}
	}
	t.Logf("适配器 %d 个：有名称 %d，有 MAC %d，有 IPv4 %d，有网关 %d",
		len(adapters), withName, withMAC, withV4, withGW)

	// 真机上几乎必然至少有一个带名称的适配器（哪怕只有回环）。
	if withName == 0 {
		t.Error("没有任何适配器解析出 FriendlyName，字符串读取很可能有误")
	}
}

func TestRuntimeGlobalMemoryStatusEx(t *testing.T) {
	m, err := GlobalMemoryStatusEx()
	if err != nil {
		t.Fatalf("GlobalMemoryStatusEx 失败: %v\n"+
			"提示：若错误为 ERROR_INVALID_PARAMETER(87)，说明 MemoryStatusEx 的布局又被改出了填充字节", err)
	}

	if m.Length != MemoryStatusExSize {
		t.Errorf("Length = %d, 期望 %d", m.Length, MemoryStatusExSize)
	}
	if m.TotalPhys == 0 {
		t.Error("TotalPhys == 0，真机上不可能")
	}
	if m.AvailPhys > m.TotalPhys {
		t.Errorf("AvailPhys (%d) > TotalPhys (%d)", m.AvailPhys, m.TotalPhys)
	}
	if m.MemoryLoad > 100 {
		t.Errorf("MemoryLoad = %d, 超出 0–100", m.MemoryLoad)
	}

	// 用负载百分比反推的已用量应当与直接相减的结果大致吻合。
	computed := float64(m.TotalPhys-m.AvailPhys) * 100 / float64(m.TotalPhys)
	if diff := computed - float64(m.MemoryLoad); diff > 5 || diff < -5 {
		t.Errorf("MemoryLoad=%d%% 与 Total-Avail 推算出的 %.1f%% 相差过大", m.MemoryLoad, computed)
	}

	t.Logf("内存：已用 %d%% / 总计 %.2f GiB / 可用 %.2f GiB",
		m.MemoryLoad,
		float64(m.TotalPhys)/(1<<30),
		float64(m.AvailPhys)/(1<<30))
}

func TestRuntimeGetDiskFreeSpaceEx(t *testing.T) {
	freeToCaller, total, totalFree, err := GetDiskFreeSpaceEx(`C:\`)
	if err != nil {
		t.Fatalf(`GetDiskFreeSpaceEx("C:\") 失败: %v`, err)
	}
	if total == 0 {
		t.Error("系统盘总容量为 0，真机上不可能")
	}
	if totalFree > total {
		t.Errorf("可用空间 (%d) > 总容量 (%d)", totalFree, total)
	}
	if freeToCaller > total {
		t.Errorf("调用方可用配额 (%d) > 总容量 (%d)", freeToCaller, total)
	}
	t.Logf("C: 总 %.2f GiB / 可用 %.2f GiB / 配额 %.2f GiB",
		float64(total)/(1<<30), float64(totalFree)/(1<<30), float64(freeToCaller)/(1<<30))
}

func TestRuntimeGetSystemTimes(t *testing.T) {
	a, err := GetSystemTimes()
	if err != nil {
		t.Fatalf("GetSystemTimes 第一次调用失败: %v", err)
	}

	time.Sleep(250 * time.Millisecond)

	b, err := GetSystemTimes()
	if err != nil {
		t.Fatalf("GetSystemTimes 第二次调用失败: %v", err)
	}

	// 这三个都是开机以来的累计计数，只能单调不减。
	if b.Kernel < a.Kernel || b.User < a.User || b.Idle < a.Idle {
		t.Fatalf("累计计数出现回退：kernel %d→%d, user %d→%d, idle %d→%d",
			a.Kernel, b.Kernel, a.User, b.User, a.Idle, b.Idle)
	}

	idle := b.Idle - a.Idle
	// 关键语义：kernel 时间**包含** idle，所以总时间 = kernel + user，
	// 忙时间 = 总时间 - idle。忘记减 idle 会把空闲机器算成满载。
	total := (b.Kernel - a.Kernel) + (b.User - a.User)
	if total == 0 {
		t.Fatal("250ms 内累计时间没有增长，GetSystemTimes 可能没有真正生效")
	}
	if idle > total {
		t.Fatalf("idle (%d) 大于 total (%d)，说明 Kernel 不含 Idle 的假设在本机不成立", idle, total)
	}

	pct := float64(total-idle) * 100 / float64(total)
	if pct < 0 || pct > 100 {
		t.Errorf("算出的 CPU 占用率 %.2f%% 超出 0–100", pct)
	}
	t.Logf("CPU 占用率 ≈ %.1f%%（idle=%d, total=%d，单位 100ns）", pct, idle, total)
}

func TestRuntimeRtlGetVersion(t *testing.T) {
	v, err := RtlGetVersion()
	if err != nil {
		t.Fatalf("RtlGetVersion 失败: %v", err)
	}
	if v.OSVersionInfoSize != RtlOsVersionInfoWSize {
		t.Errorf("OSVersionInfoSize = %d, 期望 %d", v.OSVersionInfoSize, RtlOsVersionInfoWSize)
	}
	// 本工具的兼容性矩阵起点是 Windows 10 1809（内核 10.0.17763）。
	if v.MajorVersion < 10 {
		t.Errorf("主版本号 = %d，低于兼容性矩阵要求的 10", v.MajorVersion)
	}
	if v.BuildNumber < 17763 {
		t.Errorf("内部版本号 = %d，低于兼容性矩阵要求的 17763（Windows 10 1809）", v.BuildNumber)
	}
	t.Logf("内核版本 %d.%d.%d (platformID=%d)",
		v.MajorVersion, v.MinorVersion, v.BuildNumber, v.PlatformID)
}

func TestRuntimeGetTickCount64(t *testing.T) {
	a, err := GetTickCount64()
	if err != nil {
		t.Fatalf("GetTickCount64 失败: %v", err)
	}
	if a == 0 {
		t.Error("GetTickCount64 返回 0，说明系统刚启动或调用失败")
	}
	time.Sleep(20 * time.Millisecond)
	b, err := GetTickCount64()
	if err != nil {
		t.Fatalf("GetTickCount64 第二次调用失败: %v", err)
	}
	if b < a {
		t.Errorf("GetTickCount64 回退：%d → %d", a, b)
	}
	t.Logf("系统已运行 %.2f 小时（%d ms）", float64(a)/1000/3600, a)
}

func TestRuntimeGetComputerNameEx(t *testing.T) {
	name, err := GetComputerNameEx(computerNameDNSHostname)
	if err != nil {
		t.Fatalf("GetComputerNameEx 失败: %v", err)
	}
	if name == "" {
		t.Fatal("GetComputerNameEx 返回空字符串")
	}
	if len(name) > maxComputerNameLen {
		t.Errorf("计算机名长度 %d 超过缓冲区上限 %d，说明没有正确截断", len(name), maxComputerNameLen)
	}
	t.Logf("计算机名：%q", name)
}

func TestRuntimeIsElevated(t *testing.T) {
	// IsElevated 不应 panic，且结果必须与"当前进程能否打开受限资源"的直觉一致。
	// 这里不断言真值：测试可能在非提升的 shell 中运行，那本身是合法场景。
	elevated := IsElevated()
	t.Logf("当前进程是否提升：%v", elevated)

	// 补充确认：GetDiskFreeSpaceEx 属于**不需要提升**即可成功的只读 API
	// （这正是本工具能在非管理员账户下正常工作的前提之一）。
	// 因此未提升时它必须成功；若失败，说明不是权限问题而是调用方式有问题。
	if !elevated {
		if _, _, _, err := GetDiskFreeSpaceEx(`C:\`); err != nil {
			t.Errorf("未提升状态下 GetDiskFreeSpaceEx 失败（该 API 本不需要提升，应属调用错误）: %v", err)
		}
	}
}

func TestRuntimeConsole(t *testing.T) {
	cp := GetConsoleOutputCP()
	t.Logf("控制台输出代码页：%d（UTF-8 为 %d）", cp, CPUTF8)

	// GetConsoleOutputCP 在无控制台时返回 0，这也是合法情况（例如 go test 输出被重定向）。
	if cp != 0 && cp > 65535 {
		t.Errorf("代码页 %d 超出 uint16 范围，返回值解读有误", cp)
	}
}

// 固定 32 字节载荷：ICMP 回显数据长度对判断"是否为本工具发出的包"没有意义，
// 但固定长度能让不同机器的结果可比（基线 C-03：载荷不含任何终端标识）。
var icmpTestPayload = []byte("Desktop-Diag ICMP probe payload!") // 正好 32 字节

func TestRuntimeIcmpLoopback(t *testing.T) {
	if len(icmpTestPayload) != 32 {
		t.Fatalf("测试载荷应为 32 字节，实际 %d", len(icmpTestPayload))
	}

	h, err := IcmpCreateFile()
	if err != nil {
		t.Fatalf("IcmpCreateFile 失败（非管理员下本应成功，这是本工具免管理员的基石）: %v", err)
	}
	defer func() {
		if cerr := IcmpCloseHandle(h); cerr != nil {
			t.Errorf("IcmpCloseHandle 失败: %v", cerr)
		}
	}()

	res, err := IcmpSendEcho(h, net.IPv4(127, 0, 0, 1), icmpTestPayload, time.Second)
	if err != nil {
		t.Fatalf("IcmpSendEcho 到回环地址失败: %v", err)
	}
	if !res.Replied() {
		t.Fatalf("回环地址没有应答，Status=%d（回环必然可达，说明回复缓冲区或结构体解析有误）", res.Status)
	}
	t.Logf("回环应答：来自 %s，RTT=%d ms，DataSize=%d", res.Address, res.RoundTripTime, res.DataSize)
}

func TestRuntimeIcmpUnreachableTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过（需要等待超时）")
	}

	h, err := IcmpCreateFile()
	if err != nil {
		t.Fatalf("IcmpCreateFile 失败: %v", err)
	}
	defer IcmpCloseHandle(h)

	// 192.0.2.0/24 是 RFC 5737 保留的文档用网段，保证不可路由。
	const timeout = 800 * time.Millisecond
	start := time.Now()
	res, err := IcmpSendEcho(h, net.IPv4(192, 0, 2, 1), icmpTestPayload, timeout)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("超时不应被当作调用错误返回: %v", err)
	}
	if res.Replied() {
		t.Fatal("192.0.2.1 竟然应答了，说明回复解析读到了错误的内存")
	}

	// 关键断言：超时必须表现为"没收到回包"的结果，而不是 error。
	// 若这里变成 error，丢包率就会被误算成"探测失败"，R-12 网关丢包判定会失准。
	validNoReply := res.Status == ipReqTimedOut ||
		res.Status == ipDestNetUnreachable ||
		res.Status == ipDestHostUnreachable
	if !validNoReply {
		t.Errorf("无应答时的 Status = %d，既不是超时也不是不可达", res.Status)
	}

	// 超时不应远超设定值（ICMP 超时精度受系统定时器影响，给 3 倍余量）。
	if elapsed > 3*timeout {
		t.Errorf("超时 %v 远超设定值 %v，timeout 参数可能没有生效", elapsed, timeout)
	}
	t.Logf("192.0.2.1 无应答：Status=%d，耗时 %v（设定超时 %v）", res.Status, elapsed, timeout)
}

func TestRuntimeIcmpInvalidHandleFails(t *testing.T) {
	// 无效句柄必须返回 error，而不是静默产生"没收到回包"的假象——
	// 否则句柄失效会被误读成网络不通，产生误导性的严重告警。
	_, err := IcmpSendEcho(0, net.IPv4(127, 0, 0, 1), icmpTestPayload, 200*time.Millisecond)
	if err == nil {
		t.Error("传入无效句柄时 IcmpSendEcho 没有返回错误")
	} else {
		t.Logf("无效句柄按预期失败：%v", err)
	}
}

func TestRuntimeGetWindowsDirectory(t *testing.T) {
	dir, err := GetWindowsDirectory()
	if err != nil {
		t.Fatalf("GetWindowsDirectoryW 失败: %v", err)
	}
	if dir == "" {
		t.Fatal("Windows 目录为空")
	}

	// 必须形如 "X:\..."，这样调用方才能取出盘符。
	// 基线缺陷 B5 指出硬编码 "C:" 会在系统盘非 C 的机器上得出错误结论，
	// 本测试是"盘符来自系统而非写死"这一修复的证据。
	if len(dir) < 3 || dir[1] != ':' || (dir[2] != '\\' && dir[2] != '/') {
		t.Errorf("Windows 目录 %q 不是「盘符:\\...」形式，无法据此求系统盘", dir)
	}

	// 该目录应当真实存在（用 os.Stat 交叉验证，避免 API 返回了一个看似合理的假路径）。
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("Windows 目录 %q 不存在: %v", dir, err)
	}
	t.Logf("Windows 目录 = %s（据此得到系统盘 %s）", dir, dir[:2])
}
