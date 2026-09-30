//go:build windows

// Wraps host, memory, disk and CPU-time Windows queries.
package winapi

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 本文件封装 kernel32.dll 中只读的查询类 API。

// errBufferTooSmall 表示 API 返回的数据超过了调用方提供的缓冲区。
// 单列成哨兵而不是复用系统错误码：这属于程序缺陷（常量估小了），不是环境问题。
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
// 导出让调用方必须**明确**选它：默认的 NetBIOS 名有 15 字符上限，会被静默截断。
const ComputerNameDNSHostname = computerNameDNSHostname

// MemoryStatusEx 对应 MEMORYSTATUSEX（64 字节）。
// 布局是 Length(4) + MemoryLoad(4) + 6×DWORDLONG(8)，**没有任何填充**：加填充会让 dwLength 校验失败、API 返回 87。
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
// 不像 GetTickCount 那样在 49.7 天后回绕，比 WMI 的 LastBootUpTime 可靠（后者在休眠/快速启动下会失真）。
func GetTickCount64() (uint64, error) {
	r, _, err := procGetTickCount64.Call()
	if r == 0 {
		return 0, callError("GetTickCount64", err)
	}
	return uint64(r), nil
}

// GlobalMemoryStatusEx 返回物理内存与页面文件的用量。
// Length 字段由本函数负责初始化，不交给调用方——忘记初始化正是 87 错误的成因。
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
// **Kernel 含 Idle**：busy = (Kernel - Idle) + User，total = Kernel + User；忘记减 Idle 会把空闲机器算成满载。
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
// 单次调用得到的是开机以来的累计值，算占用率必须在间隔后调用两次求差；两次采样与计算由调用方负责。
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

// maxComputerNameLen 不能用 MAX_COMPUTERNAME_LENGTH(15)：那是 NetBIOS 的限制，DNS 主机名可以长得多。
const maxComputerNameLen = 256

// GetComputerNameEx 返回指定格式的计算机名。
// NetBIOS 格式会把超过 15 字符的名称截断，故默认使用 ComputerNameDnsHostname（format=1）。
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

// maxWindowsDirLen 给定足够宽裕的上限，避免为极端路径反复分配缓冲区。
const maxWindowsDirLen = 512

// GetWindowsDirectory 返回 Windows 安装目录（如 "C:\Windows"），返回值不以反斜杠结尾。
// 用途是求系统盘盘符：硬编码 "C:" 会在系统盘非 C 的机器上得出完全错误的磁盘空间结论。
// 不读 %SystemDrive% 环境变量是因为它可被进程改写，而报告要能作为故障证据。
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
