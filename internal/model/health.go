package model

// Health 是系统健康度快照（REQ-F-301 ~ 303）。
//
// 三个 *Known 标志是刻意的：采集失败时**不得填 0**，否则"没采到"会被当成"占用 0%"，
// 让 R-06~R-11 静默漏判；采集失败必须走 Failures → R-19。
type Health struct {
	CPUPercent     float64 // 0–100，基于 CPUSampleWindow 窗口的双采样差值
	CPUKnown       bool
	MemTotalBytes  uint64
	MemAvailBytes  uint64
	MemUsedPercent float64
	MemKnown       bool
	SystemDrive    string // "C:"，来自 GetWindowsDirectory 而非硬编码
	DiskTotalBytes uint64
	DiskFreeBytes  uint64
	DiskKnown      bool
}

// MemUsedBytes 返回已用内存字节数。
func (h Health) MemUsedBytes() uint64 {
	if h.MemAvailBytes >= h.MemTotalBytes {
		return 0
	}
	return h.MemTotalBytes - h.MemAvailBytes
}

// DiskUsedBytes 返回系统盘已用字节数。
func (h Health) DiskUsedBytes() uint64 {
	if h.DiskFreeBytes >= h.DiskTotalBytes {
		return 0
	}
	return h.DiskTotalBytes - h.DiskFreeBytes
}
