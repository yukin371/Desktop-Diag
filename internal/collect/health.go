//go:build windows

package collect

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// healthCollector 采集系统健康度：CPU、内存、系统盘。
type healthCollector struct{}

// Name 实现 Collector。
func (healthCollector) Name() string { return "系统健康度" }

// EnvVar 实现 envVarer。
func (healthCollector) EnvVar() string { return "health" }

// Collect 实现 Collector。
//
// 三项指标相互独立，任何一项失败都不影响另外两项的输出——
// 内存能读而磁盘读不到的机器，其内存结论依然有价值。
func (c healthCollector) Collect(ctx context.Context, snap *model.Snapshot) error {
	var partial []string

	if err := collectMemory(snap); err != nil {
		partial = append(partial, "内存: "+err.Error())
	}
	if err := collectDisk(snap); err != nil {
		partial = append(partial, "系统盘: "+err.Error())
	}
	if err := collectCPU(ctx, snap); err != nil {
		// 上下文被取消（例如用户 Ctrl+C）不算采集缺陷，不要污染诊断完整性。
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			partial = append(partial, "CPU: "+err.Error())
		}
	}

	if len(partial) > 0 {
		snap.AddFailure(c.Name(), c.EnvVar(), strings.Join(partial, "；"), true)
	}
	if !snap.Health.MemKnown && !snap.Health.DiskKnown && !snap.Health.CPUKnown {
		return errors.New("系统健康度完全无法采集")
	}
	return nil
}

// collectMemory 采集物理内存使用情况。
func collectMemory(snap *model.Snapshot) error {
	m, err := winapi.GlobalMemoryStatusEx()
	if err != nil {
		return err
	}
	h := &snap.Health
	h.MemTotalBytes = m.TotalPhys
	h.MemAvailBytes = m.AvailPhys
	h.MemUsedPercent = float64(m.MemoryLoad)
	h.MemKnown = true

	snap.AddRaw("系统健康度", "kernel32!GlobalMemoryStatusEx",
		fmt.Sprintf("Load=%d%% TotalPhys=%d AvailPhys=%d", m.MemoryLoad, m.TotalPhys, m.AvailPhys))
	return nil
}

// collectDisk 采集**系统盘**的容量与可用空间。
//
// 系统盘由系统自己给出（GetWindowsDirectoryW），不是硬编码 "C:"。
// 基线缺陷 B5 明确指出：在系统盘非 C 的机器上，硬编码会得出完全相反的结论
// （例如实际系统盘只剩 2 GiB，却去报告一块空闲的数据盘）。
func collectDisk(snap *model.Snapshot) error {
	winDir, err := winapi.GetWindowsDirectory()
	if err != nil {
		return fmt.Errorf("无法确定系统盘: %w", err)
	}
	drive := filepath.VolumeName(winDir) // "C:"
	if drive == "" {
		return fmt.Errorf("Windows 目录 %q 中取不出盘符", winDir)
	}

	// 加反斜杠才是卷根路径；"C:" 在 Win32 里表示"该盘的当前目录"，
	// 传给 GetDiskFreeSpaceEx 会失败。
	free, total, _, err := winapi.GetDiskFreeSpaceEx(drive + `\`)
	if err != nil {
		return err
	}

	h := &snap.Health
	h.SystemDrive = drive
	h.DiskTotalBytes = total
	h.DiskFreeBytes = free
	h.DiskKnown = true

	snap.AddRaw("系统健康度", "kernel32!GetDiskFreeSpaceExW",
		fmt.Sprintf("%s\\ FreeBytesAvailableToCaller=%d TotalBytes=%d（来源：%s）", drive, free, total, winDir))
	return nil
}

// collectCPU 用两次 GetSystemTimes 采样求差算 CPU 占用率。
//
// 为什么用采样差而不是别的办法：
//   - GlobalMemoryStatusEx 那样的"瞬时值"对 CPU 不存在，占用率本质是一段时间的比值。
//   - 性能计数器（PDH）需要加载额外 DLL 且 API 面很大，与"尽量少动作"的红线不符。
//
// 口径注意：GetSystemTimes 的 Kernel 时间**已含 Idle**，
// 因此 total = Kernel + User，busy = total - Idle。若把 Kernel 当成"纯内核忙时"
// 再相加，算出来的占用率会系统性偏高（在本机实测会高出十几个百分点）。
func collectCPU(ctx context.Context, snap *model.Snapshot) error {
	first, err := winapi.GetSystemTimes()
	if err != nil {
		return err
	}

	// 采样窗口可被取消：用户按下 Ctrl+C 时不该再干等一秒。
	timer := time.NewTimer(detect.CPUSampleWindow)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	second, err := winapi.GetSystemTimes()
	if err != nil {
		return err
	}

	dTotal := (second.Kernel + second.User) - (first.Kernel + first.User)
	dIdle := second.Idle - first.Idle

	// 计数不可能回退；真回退说明期间发生过异常（例如计数器被重置），
	// 此时给出"未知"比给出一个错的百分比更负责。
	if dTotal == 0 || dIdle > dTotal {
		return fmt.Errorf("系统时间计数器异常（Δtotal=%d Δidle=%d）", dTotal, dIdle)
	}

	percent := float64(dTotal-dIdle) / float64(dTotal) * 100
	// 浮点误差可能给出 -0.0001 或 100.0001，钳到合法区间。
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	h := &snap.Health
	h.CPUPercent = percent
	h.CPUKnown = true

	snap.AddRaw("系统健康度", "kernel32!GetSystemTimes",
		fmt.Sprintf("采样窗口 %s：Δidle=%d Δ(kernel+user)=%d → CPU=%.1f%%",
			detect.CPUSampleWindow, dIdle, dTotal, percent))
	return nil
}
