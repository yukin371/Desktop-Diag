# 阶段四启动与交接

日期：2026-09-30。状态：准备完成，正式验收未开始。本文是新对话入口，不需要旧聊天记录。

## 1. 下次直接使用的任务说明

> 请从 `doc/phase4/README.md` 开始，阅读验收计划和结果台账，继续 Desktop-Diag 阶段四 T-28～T-30。先检查 Git 状态、环境和被测 EXE 身份，再执行本机可用的只读验收，记录证据并修复确认的缺陷。没有可用环境的场景保持未覆盖，不把模拟测试或交叉编译当成真机验收。遵循 AGENTS.md，不修改生产机器网络、注册表、服务、账户或安全策略。必要的破坏性场景使用已准备的隔离测试环境；缺少环境时先完成不依赖该环境的项目。代码变更后运行相关测试及最终质量门禁，同步文档。不要自动提交或发布。

## 2. 阅读顺序

1. 仓库 `AGENTS.md` 及其引用的 RTK 规则。
2. [需求基线](../phase0/01-requirements-baseline.md)：正式功能、规则、红线与 S-01～S-18。
3. [阶段三完成记录](../phase3/01-completion.md)：已完成的实现与历史验证证据。
4. [验收计划](00-validation-plan.md)：优先级、步骤、标准和出口。
5. [结果台账](01-results.md)：当前基线、待办矩阵和逐次记录模板。

技术背景见 [技术设计](../phase1/00-technical-design.md)，任务承载见 [WBS](../phase2/00-task-breakdown.md)。设计中的目录和代码块是示意，实际实现以源码为准。

## 3. 项目与代码定位

- 仓库：`J:\Github\Desktop-Diag`，Windows PowerShell；当前 master，无远程仓库。
- 代码基线：`7ef92442dfdfa7e1df2e0e732357d9fd4646fff0`。
- 最近三条提交：`ae55769` 实现/测试/中文输出，`c55fa24` 开发脚本，`7ef9244` 文档。
- `go.mod`：Go 1.26.0，唯一直接第三方依赖 `golang.org/x/sys v0.48.0`。
- 入口：`cmd/desktop-diag/main.go` → `internal/app/app.go`。
- 数据流：collect/probe → model.Snapshot → detect → report/ui。
- 采集顺序：主机、网卡、健康度、连通性；注册点在 `internal/collect/registry.go`。
- 19 条规则在 `internal/detect/rules.go`，阈值及目标在 `thresholds.go`。
- 网络分类在 `internal/model/network.go`，probe/classify 是薄包装。
- 网卡合并在 collect/network；系统调用在 winapi/network、iphlpapi 等文件。
- 报告排他创建与降级在 report/writer；编码恢复在 ui/ui。

## 4. 启动命令

在仓库根目录运行，每次原生命令后确认 `$LASTEXITCODE`；非零时停止依赖该结果的后续步骤。

```powershell
git status --short
git rev-parse HEAD
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -Version phase4-baseline
Get-FileHash .\dist\Desktop-Diag.exe -Algorithm SHA256
& .\dist\Desktop-Diag.exe -version
& .\dist\Desktop-Diag.exe -o .\e2eout\phase4-baseline
$LASTEXITCODE
```

`ExecutionPolicy Bypass` 仅用于开发子进程，不修改全局策略；脚本运行前应确认企业策略允许。构建、测试脚本会选择完整 Go 1.26.0 工具链并恢复调用方环境。离线构建须预装 Go 工具链与 go.sum 对应依赖。EXE 运行无需 Go/PowerShell。

可选补充：`scripts/test.ps1 -Race` 需要 C 编译器；`tools/layout-probe/run.ps1` 需要 MSVC/Windows SDK。缺少工具时记录未执行，不假定已经通过。

## 5. 已知边界与环境陷阱

- 本机 Windows 11 工作站专业版，历史 OS 版本 10.0.26200.9457，x64；下次需重新读取。
- 未提升进程不等于独立标准用户账户。当前没有完整普通用户身份矩阵证据。
- 本机网关 ICMP 调用被拒绝，DNS/TCP 仍可工作；正确结果是 R-19 和证据不足，不能据此判为网络中断。
- 仅支持 IPv4 网关 ICMP；IPv6 网关展示但不探测，能力缺失需留痕。
- 网关未绑定出接口，DNS/TCP 为全局探测；系统路由决定出口，不能证明每块网卡正常。
- 默认优先活动物理接口，无物理接口时才回退默认路由 VM/VPN；Overlay/Other 排除。
- TCP 443 为直连，不走系统代理；测试目标失败不代表所有互联网不可用。
- 25 秒采集预算、约 30 秒整体目标，不保证强制中断异常同步 Win32/UNC IO。
- Windows 测试临时 EXE 曾被短暂拒绝访问；记录原始失败，可重试确认，不关闭安全软件掩盖问题。
- 报告摘要/详情和正常控制台为中文，原始数据与 API/设备名称保留原文。无法使用 UTF-8 时控制台 ASCII 降级，报告仍为 UTF-8 BOM/CRLF。

## 6. 产物与证据管理

`dist/`、`e2eout/`、SDK 布局输出被 Git 忽略，新检出不保证存在。台账中的 SHA256 和报告路径用于核对本机产物，不代表已经分发或永久归档。缺少文件时重新构建、重新取证并记录新身份，不补造历史结果。

开发验收可使用外部工具及独立取证文件，但产品运行时只能生成自己的报告、stdout/stderr 和必要报告目录。不要把开发取证能力加入产品运行链。

本次只准备文档、重建候选 EXE 并执行一次冒烟，不更改系统配置、不提交、不发布。
