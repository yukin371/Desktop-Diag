//go:build windows

package winapi

import (
	"golang.org/x/sys/windows"
)

// 本文件封装进程令牌相关的只读查询。

// IsElevated 报告当前进程是否以管理员（提升后）令牌运行。
//
// 这是权限自适应（REQ-F-701）的唯一判据。注意：
//   - 只判断"是否提升"，不尝试提升（红线 C-01/C-02：不修改任何系统状态）
//   - 仍处于 Administrators 组但未提升时返回 false——这正是"以管理员身份运行"
//     与"以管理员账户登录"的区别，也是本工具最需要区分的情形
//
// 用 windows.GetCurrentProcessToken 而不是重新 OpenProcessToken：
// 前者返回进程自带令牌的伪句柄，无需关闭，也没有句柄泄漏风险。
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
