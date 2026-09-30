//go:build windows

// Package winapi 封装 Desktop-Diag 所需的全部 Windows 原生调用。
//
// 只读红线：系统与设备查询只读；控制台代码页/模式仅在会话内设置并由 ui 恢复。
// 不含配置写入、进程启动或上传调用；新增 API 前必须核对其副作用。
// 布局纪律：手写 C 结构体映射由布局断言测试看守，任何改动后必须跑 go test ./internal/winapi/... -run TestStructLayout。
// 依赖方向：叶子包，只依赖标准库与 golang.org/x/sys/windows。
package winapi
