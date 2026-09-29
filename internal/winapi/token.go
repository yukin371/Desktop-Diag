//go:build windows

package winapi

import (
	"golang.org/x/sys/windows"
)

// 本文件封装进程令牌相关的只读查询。

// IsElevated 报告当前进程是否以提升后的令牌运行，是权限自适应的唯一判据。
// 未提升时（即使账户属于 Administrators 组）返回 false；取进程自带令牌伪句柄，无需关闭。
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
