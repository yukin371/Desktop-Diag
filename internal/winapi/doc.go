//go:build windows

// Package winapi 封装 Desktop-Diag 所需的全部 Windows 原生调用。
//
// # 只读保证（红线 C-01 / C-02 / C-06）
//
// 本包**只包含只读调用**，不具备修改任何系统状态的能力：
//   - 不调用任何 RegSetValueEx / RegCreateKey / RegDeleteKey
//   - 不调用 SetIpInterfaceEntry / CreateService / CreateProcess 等写入或启动类 API
//   - 不启动 powershell.exe / wmic.exe / wscript.exe / cscript.exe / cmd.exe
//   - 不发起任何 HTTP 上传（不使用 WinHTTP / WinINet / URLMON）
//
// 阶段 4 的合规扫描（任务 T-30）会对全仓 grep 上述写入类 API 名称，期望 0 命中。
// 若此处需要新增 API，必须先确认它只读取状态、不改变状态。
//
// # 结构体布局纪律（阶段 1 实测教训）
//
// 本包中每一个手写 C 结构体映射都配有布局断言测试
// （见 iphlpapi_types_test.go），断言 sizeof 与关键字段偏移等于 Windows SDK 的已知值。
// 这不是形式主义：阶段 1 实测中，MEMORYSTATUSEX 因凭直觉添加了 8 字节填充，
// 导致 dwLength 校验失败、API 返回 ERROR_INVALID_PARAMETER(87)，内存数据完全拿不到。
// IP_ADAPTER_ADDRESSES_LH 比它大 20 倍、字段多 5 倍，一旦偏移错位，
// 轻则网卡数据乱码，重则遍历链表时崩溃。
//
// 因此新增或修改结构体后**必须**跑：
//
//	go test ./internal/winapi/... -run TestStructLayout -v
//
// 但断言表里的期望值是**人写的**，人写就可能写错——该表确实写错过一次
// （Dhcpv6ClientDuidLength 被写成 426，而 ULONG 需 4 字节对齐，真实值是 428）。
// 权威来源是用 MSVC 编译真实 SDK 头文件后打印出的布局：
//
//	powershell -NoProfile -ExecutionPolicy Bypass -File tools\layout-probe\run.ps1
//
// 新增结构体时，先跑该工具取得编译器算出的偏移，再据此填断言表。
//
// # 依赖方向
//
// 本包是叶子包：只依赖标准库与 golang.org/x/sys/windows，不 import 任何其他 internal 包。
package winapi
