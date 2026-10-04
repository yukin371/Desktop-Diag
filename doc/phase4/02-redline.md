# 阶段四 T-30 红线复核

日期：2026-10-02。源码提交：`bdcb32ea7d10573c7c44010277488f51457ae05e`。复核对象为 `internal` 与 `cmd` 中非测试 Go 源码，测试、脚本和文档名称不计入运行时结论。

## 扫描结果

按验收计划执行：

```powershell
rg -n 'RegSetValue|RegCreateKey|SetIpInterfaceEntry|CreateService|InternetOpen|os/exec|exec\.Command|CreateProcess|ShellExecute|powershell|wmic|wscript|cscript|cmd\.exe' internal cmd -g '*.go' -g '!**/*_test.go'
```

结果：无匹配（`rg` 退出码 1，表示 0 命中）。未发现注册表写入、网络配置写入、服务创建、WinINet、外部进程启动或脚本引擎调用。

第二组候选调用扫描命中仅为允许行为：

- `internal/report/writer.go`：`MkdirAll`、`OpenFile(O_CREATE|O_EXCL)`、`CreateTemp`、`Remove`，用于报告目录、排他报告文件和失败清理。
- `internal/ui` / `internal/winapi/console.go`：读取并恢复控制台模式和代码页。
- `internal/probe` / `internal/winapi/iphlpapi.go`：DNS、TCP、ICMP 诊断探测。
- `internal/collect/probe.go`：把 API 名称作为报告原始数据标签，不是进程启动。

## 人工复核

报告写入只发生在 `report.Writer`，使用候选目录降级链、`O_EXCL` 防覆盖和失败清理；控制台状态在 `ui` 中按栈恢复。源码未发现 `os/exec`、`CreateProcess`、`ShellExecute` 或 `cmd.exe /c`。EXE 未做 DLL 导入表推断；本结论基于源码调用关系和一次真实运行观察。

结论：T-30 静态红线审查通过。该结论不替代其他 Windows 版本、普通用户、UNC 或企业安全策略的运行验收。
