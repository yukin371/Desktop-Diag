# 结构体布局取证工具（layout-probe）

## 为什么存在

`internal/winapi/iphlpapi_types.go` 里手写的 Win32 结构体，一旦字段偏移与 Windows SDK
不一致，读到的是**错位的内存**——表现为乱码网卡名、崩溃、或更糟：看起来正常但数值全错。

Go 侧的 `internal/winapi/iphlpapi_types_test.go` 用 `unsafe.Offsetof` 断言了每一个字段，
但那些期望值是**人写的**，人写就可能写错。本工具用 MSVC 编译真实的 SDK 头文件，
打印出编译器算出的布局，作为**唯一的权威来源**去核对 Go 侧的断言表。

## 它抓到过的真实缺陷

`Dhcpv6ClientDuidLength` 的偏移。手写期望值是 426（= 296 + `[130]byte`），
但 `ULONG` 需要 4 字节对齐，426 不是 4 的倍数——MSVC 与 Go 都把它放在 **428**。
纯靠算术直觉写下的 426 是**不可能成立**的。断言测试立刻失败，本工具确认了正确值。

## 用法

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools\layout-probe\run.ps1
```

脚本会用 `vswhere` 定位 Visual Studio，加载 `vcvars64.bat`，编译并运行 `layout_probe.c`，
输出 SDK 的真实布局。把它与 `iphlpapi_types_test.go` 中的期望值逐行对照。

若本机没有 Visual Studio，可退回 MinGW（`gcc`），但 **MinGW 使用自带头文件**，
与 MSVC/Windows SDK 未必逐字节一致——核对时应以 MSVC 结果为准。

## 何时必须重跑

- 新增或修改 `internal/winapi/` 下任何手写结构体之后
- 升级 Windows SDK 之后
- 排查"字段值看起来不对/乱码/崩溃"类问题时

## 注意

- 这些类型是 `typedef struct _X X`，所以 `offsetof` 必须用 typedef 名；
  写成 `struct X` 会报 C2037（未定义的结构体）。
- 源文件含中文，MSVC 需要 `/utf-8`，否则按代码页 936 解析并报 C4819。
