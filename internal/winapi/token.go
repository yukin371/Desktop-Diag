//go:build windows

// Reads current process token elevation without requesting higher privileges.
package winapi

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

// 本文件封装进程令牌相关的只读查询。

// IsElevated 报告当前进程是否以提升后的令牌运行，用于记录提升状态；完整性等级另行查询。
// 未提升时（即使账户属于 Administrators 组）返回 false；取进程自带令牌伪句柄，无需关闭。
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// ProcessIntegrity queries the actual current process token without modifying its privileges.
func ProcessIntegrity() (string, uint32, error) {
	token := windows.GetCurrentProcessToken()
	var size uint32
	err := windows.GetTokenInformation(token, windows.TokenIntegrityLevel, nil, 0, &size)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return "", 0, fmt.Errorf("查询进程完整性缓冲区长度失败: %w", err)
	}
	if size < uint32(unsafe.Sizeof(windows.Tokenmandatorylabel{})) {
		return "", 0, fmt.Errorf("进程完整性缓冲区长度异常: %d", size)
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buffer[0], size, &size); err != nil {
		return "", 0, fmt.Errorf("读取进程完整性标签失败: %w", err)
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	sid := label.Label.Sid
	if sid == nil || !sid.IsValid() || sid.SubAuthorityCount() == 0 {
		return "", 0, fmt.Errorf("进程完整性 SID 无效")
	}
	rid := sid.SubAuthority(uint32(sid.SubAuthorityCount() - 1))
	runtime.KeepAlive(buffer)
	return integrityName(rid), rid, nil
}

// integrityName names Windows integrity bands while retaining the exact RID separately.
func integrityName(rid uint32) string {
	switch {
	case rid < integrityLow:
		return "Untrusted（不可信）"
	case rid < integrityMedium:
		return "Low（低完整性）"
	case rid < integrityHigh:
		return "Medium（中完整性）"
	case rid < integritySystem:
		return "High（高完整性）"
	case rid < integrityProtected:
		return "System（系统完整性）"
	default:
		return "Protected（受保护）"
	}
}
