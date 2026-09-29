//go:build windows

package winapi

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 本文件封装 kernel32.dll 中只读的查询类 API。

// errBufferTooSmall 表示 API 返回的数据超过了调用方提供的缓冲区。
//
// 单列成哨兵而不是复用系统错误码：这种情况说明我们自己的缓冲区估算错了，
// 是程序缺陷而非环境问题，报出来应当让人一眼看出该调大常量。
var errBufferTooSmall = errors.New("缓冲区过小，返回值被截断")

var (
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceExW  = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGetComputerNameExW   = kernel32.NewProc("GetComputerNameExW")
	procGetWindowsDirectoryW = kernel32.NewProc("GetWindowsDirectoryW")
)

// COMPUTER_NAME_FORMAT 取值。
const (
	computerNameNetBIOS         = 0
	computerNameDNSHostname     = 1 // 设计与基线约定使用这一项
	computerNameDNSDomain       = 2
	computerNamePhysicalNetBIOS = 4
	computerNameMax             = 8
)

// ComputerNameDNSHostname 是 GetComputerNameEx 应当使用的那一项。
//
// 之所以导出：调用方必须**明确**选这一项。默认的 ComputerNameNetBIOS
// 有 15 字符上限，在长主机名上会被静默截断，而截断后的机器名会让运维
// 认错机器——报告里的计算机名必须能拿去跟资产台账对得上。
const ComputerNameDNSHostname = computerNameDNSHostname

// MemoryStatusEx 对应 MEMORYSTATUSEX（64 字节）。
//
// # 这个结构体曾经是本项目第一个真实缺陷
//
// 阶段 1 实测时，凭"uint32 后面接 uint64 需要填充"的直觉，在 Length 与
// MemoryLoad 之后各插入了一个 `_ uint32`，使 sizeof 从 64 变成 72，
// 于是 GlobalMemoryStatusEx 的 dwLength 校验失败，返回
// ERROR_INVALID_PARAMETER(87)——「参数错误」，内存数据一个字节都拿不到。
//
// 正确布局是 dwLength(4) + dwMemoryLoad(4) + 6×DWORDLONG(8)，**没有任何填充**：
// DWORD 之后接 DWORDLONG 时，自然对齐正好落在 8 字节边界上。
// 教训：Win32 结构体里"看起来需要对齐填充"的直觉是错的，必须实测。
// 该布局由 TestStructLayoutSizes 与 tools/layout-probe 双重看守。
type MemoryStatusEx struct {
	Length               uint32 // 必须由调用方初始化为 sizeof(MemoryStatusEx)
	MemoryLoad           uint32 // 已用物理内存百分比（0–100），由系统填充
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// MemoryStatusExSize 是 MEMORYSTATUSEX 的字节数（amd64 下为 64）。
const MemoryStatusExSize = 64

// GetTickCount64 返回系统启动至今的毫秒数。
//
// 相对 GetTickCount 的优势是不会在 49.7 天后回绕。用于计算运行时长，
// 比 WMI 的 LastBootUpTime 可靠（后者在休眠/快速启动场景下会失真）。
func GetTickCount64() (uint64, error) {
	r, _, err := procGetTickCount64.Call()
	if r == 0 {
		return 0, callError("GetTickCount64", err)
	}
	return uint64(r), nil
}

// GlobalMemoryStatusEx 返回物理内存与页面文件的用量。
//
// 注意 Length 字段由本函数负责初始化——绝不交给调用方，
// 因为忘记初始化（或初始化成错误的值）正是上面记录的那个缺陷的成因。
func GlobalMemoryStatusEx() (*MemoryStatusEx, error) {
	var m MemoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))

	r, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return nil, callError("GlobalMemoryStatusEx", err)
	}
	return &m, nil
}

// GetDiskFreeSpaceEx 查询指定卷或目录的容量信息，三个返回值单位均为字节。
//
// 参数用目录路径（如 `C:\`）而非卷名，这样对挂载点与 UNC 路径同样有效。
func GetDiskFreeSpaceEx(path string) (freeToCaller, total, totalFree uint64, err error) {
	p, err := windows.UTF16FromString(path)
	if err != nil {
		return 0, 0, 0, callError("GetDiskFreeSpaceExW/UTF16FromString", err)
	}

	var freeAvail, totalBytes, totalFreeBytes uint64
	r, _, callErr := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(&p[0])),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r == 0 {
		return 0, 0, 0, callError("GetDiskFreeSpaceExW", callErr)
	}
	return freeAvail, totalBytes, totalFreeBytes, nil
}

// SystemTimes 是 GetSystemTimes 的原始返回值，单位为 100 纳秒。
//
// 关键语义：**Kernel 含 Idle**。因此
//
//	busy  = (Kernel - Idle) + User
//	total = Kernel + User
//
// 忘记减 Idle 是最常见的 CPU 占用率算法错误，会把空闲机器算成满载。
type SystemTimes struct {
	Idle   uint64
	Kernel uint64
	User   uint64
}

// filetimeToUint64 把 FILETIME（两个 32 位半）拼成 64 位计数值。
func filetimeToUint64(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

// GetSystemTimes 读取系统的空闲/内核/用户累计时间。
//
// 单次调用得到的是**开机以来的累计值**，要算占用率必须在间隔后调用两次并求差
// （见 CPUSampleWindow）。调用方负责两次采样与计算。
func GetSystemTimes() (SystemTimes, error) {
	var idle, kernel, user windows.Filetime
	r, _, err := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r == 0 {
		return SystemTimes{}, callError("GetSystemTimes", err)
	}
	return SystemTimes{
		Idle:   filetimeToUint64(idle),
		Kernel: filetimeToUint64(kernel),
		User:   filetimeToUint64(user),
	}, nil
}

// maxComputerNameLen 是计算机名缓冲区的上限。
// MAX_COMPUTERNAME_LENGTH 只有 15（那是 NetBIOS 的限制），
// 而 DNS 主机名可以长得多，所以不能按 15 分配。
const maxComputerNameLen = 256

// GetComputerNameEx 返回指定格式的计算机名。
//
// 若名称超过 15 个 NetBIOS 字符，ComputerNameNetBIOS 会被截断，
// 所以默认使用 ComputerNameDnsHostname（format=1）。
func GetComputerNameEx(format uint32) (string, error) {
	buf := make([]uint16, maxComputerNameLen)
	size := uint32(len(buf))

	r, _, err := procGetComputerNameExW.Call(
		uintptr(format),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 {
		return "", callError("GetComputerNameExW", err)
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// maxWindowsDirLen 是 Windows 目录缓冲区的上限。
//
// GetWindowsDirectoryW 在缓冲区不足时返回所需长度（可能大于 MAX_PATH），
// 据此可以判断是否需要重试；这里给定一个足够宽裕的上限，
// 避免为极端路径反复分配。
const maxWindowsDirLen = 512

// GetWindowsDirectory 返回 Windows 安装目录，例如 "C:\Windows"。
//
// 用途：求系统盘盘符。基线缺陷 B5 指出，硬编码 "C:" 会在系统盘非 C 的机器上
// 得出完全错误的磁盘空间结论。虽然 %SystemDrive% 环境变量通常也可用，
// 但那是可被进程环境改写的值，而本工具的报告要能作为故障证据，
// 因此宁可向系统本身询问。
//
// 返回值不以反斜杠结尾（除非是根目录），与 API 原生行为一致。
func GetWindowsDirectory() (string, error) {
	buf := make([]uint16, maxWindowsDirLen)

	r, _, err := procGetWindowsDirectoryW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r == 0 {
		return "", callError("GetWindowsDirectoryW", err)
	}
	// 返回的是写入的字符数（不含结尾 NUL）；达到缓冲区上限说明被截断。
	if int(r) >= len(buf) {
		return "", callError("GetWindowsDirectoryW", errBufferTooSmall)
	}
	return windows.UTF16ToString(buf[:r]), nil
}
