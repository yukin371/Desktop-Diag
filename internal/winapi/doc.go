//go:build windows

// Package winapi 封装 Desktop-Diag 所需的全部 Windows 原生调用。
//
// 只读红线：本包只调用读取类 API，不含任何写入、进程启动或上传调用；新增 API 前必须先确认它只读取状态。
// 布局纪律：手写 C 结构体映射由布局断言测试看守，任何改动后必须跑 go test ./internal/winapi/... -run TestStructLayout。
// 依赖方向：叶子包，只依赖标准库与 golang.org/x/sys/windows。
package winapi
