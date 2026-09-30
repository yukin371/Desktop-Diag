# 阶段 3 完成交付记录

| 项目 | 内容 |
| --- | --- |
| 日期 | 2026-09-30 |
| 范围 | 用户要求修复实现缺口、完成阶段 3、补测试；承接阶段 0–3 审计 |
| 代码 | 基于 `fa48dbc` 的阶段三实现、测试与中文展示补齐；按实现、开发脚本和文档分别提交，未发布 |
| 结论 | 阶段 3 编码、回归测试与本机端到端闭环完成；进入阶段 4 场景与兼容性验收 |
| 环境 | Windows 11 10.0.26200.9457 / amd64；Go 1.26.0；x/sys v0.48.0；当前进程未提升权限 |

## 1. 审计缺口关闭

原始问题及首次验证保留在 [00-implementation-review.md](00-implementation-review.md)，不覆盖原始审计证据。

| ID | 落实内容 | 主要回归证据 |
| --- | --- | --- |
| A-01 | 调度同步发出开始/结束事件，下一项执行前呈现上一项结果 | `app.TestRealtimeProgressAndVerbose` 在第二采集器执行时断言此前输出，而非仅看最终文本 |
| A-02 | `-v` 输出来源、原始字段、分项耗时；帮助/版本不采集、不落盘 | 同输入比较详细/默认输出；`TestHelpVersionDoNotCollectOrWrite` |
| A-03 | 三个只读来源按 GUID/索引合并；管理未知与禁用分离；仅接口清单的空 IP/网关/DNS 不参与配置失败判定 | 清单合并/独立失败/未知地址回归；本机接口表/设备枚举测试 |
| A-04 | 编码失败使用完整 ASCII 输出，英文关键字段与动态中文转义 | `ui.TestConsoleSetupModesAndRestore`、`TestASCIIAndWriteErrors` 检查所有字节 ≤127 |
| A-05 | R-16 要求系统和直连均完整失败；回环 DNS 代理合法 | 系统成功/直连受限、同类先失败后成功、单独 IPv4/IPv6 回环配置回归 |
| A-06 | 共享证据模型区分缺失/跳过/中断/完整失败；TCP 全失败需全部不同候选 | `model.TestNetworkEvidenceMatrix`、重复候选/部分 TCP 回归；R-14/R-17 缺证据不触发 |
| A-07 | IPv6 默认路由保留，IPv4 ICMP 能力不足明确跳过并计入 R-19 | `TestIPv6OnlyAndCancelledProbesRecordMissingEvidence`；全局 IPv6 不误报 R-02/R-05，不声称完整 ok |
| A-08 | 25s 采集预算、4 个网关工作者、剩余包预算与完整取消留痕；控制台耗时含落盘关闭 | 并发阻塞测试检查峰值及顺序；ICMP 预算/中途取消；应用注入 2s 落盘验证总耗时 3s；十次真机计时 |
| A-09 | 无活动物理接口时使用有默认路由的 VM/VPN；排除 Overlay/Other | `model.TestProbeAdapterSelection`；变更 CR-03；真实 VM 平台验证留阶段 4 |
| A-10 | ICMP 保留接口索引与名称，同名设备不混淆；报告/附录声明未绑定出接口 | 并发结果归属/顺序断言；全局成功不证明每块接口正常 |
| A-11 | CPU 结论限定 1s 窗口；DNS/TCP 限定域名、候选与直连协议 | 规则目录/报告与基线同步；企业策略失败不等于全互联网中断 |
| A-12 | 短写、控制台输出、连接/报告关闭与清理错误均检查；清理失败立即停止降级，保留原因并退出 2 | 关闭/删除/短写与二级候选守卫回归；正式链仍只创建报告，无写入探针 |
| A-13 | StepResult 汇总新增完整性记录，部分采集不显示完整完成 | 调度事件与应用进度回归；权限失败固定输入 |
| A-14 | stdout/stderr 分别设置，LIFO 恢复共享代码页及各句柄模式，恢复错误返回 2 | 模拟双句柄恢复顺序、双错误合并、Close 幂等；真实控制台测试恢复原状态（无控制台则明确跳过） |
| A-15 | 文件/函数职责说明补齐，默认探测超时引用统一阈值，model/detect 保持纯逻辑 | gofmt/vet；纯分类及模型跨平台构建；Win32 SDK 布局探针与断言测试 |
| A-16 | 构建/测试脚本固定完整 Go 1.26.0、子进程 PATH，结束恢复调用方环境 | `scripts/build.ps1`、`scripts/test.ps1`；构建自检只运行 `-version`，不隐式诊断/生成报告 |

代码修复关闭不等于把全部生产兼容场景验收关闭。A-08 的性能边界、A-09 的 VM 实测与 A-14 的真实控制台矩阵继续由阶段 4 承载。

## 2. 自动化验证

复现命令（仅影响本次 PowerShell 子进程，未修改全局执行策略）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1 -Race
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -Version phase3
powershell -NoProfile -ExecutionPolicy Bypass -File tools/layout-probe/run.ps1
```

`test.ps1` 检查原始退出码、gofmt、vet、完整覆盖率与可选竞态；覆盖率低于门禁返回非零。脚本选择 Go 1.26.0；离线运行须提前安装此工具链和 go.sum 锁定的依赖。竞态检查另外需要本机 C 编译器，本机已有 MinGW GCC。

| 检查 | 结果 |
| --- | --- |
| 完整单测 / go vet / gofmt | 通过 |
| 全仓竞态检查 | `go test -race -p 1 ./... -count=1` 通过 |
| 跨平台编译 | CGO 关闭时 Linux 的 model/detect/probe 分类子集与 Windows arm64 入口编译通过；非 arm64 真机运行证据 |
| 整体语句覆盖率 | 88.9%，包含 main/version，无包百分比平均 |
| app / cli / collect | 87.5% / 100.0% / 87.2% |
| detect / model / probe | 97.9% / 92.7% / 90.6% |
| report / ui / winapi | 94.1% / 96.5% / 68.4% |
| Classify / ClassifyNetwork | 均为 100% 语句覆盖；另有缺失/中断/多候选证据矩阵，不将语句覆盖等同分支正确 |
| collect 纯函数 | 门禁清单逐函数 ≥80%；probeSummary 88.9%，其余 16 项均 100%；不以含真实 IO 的包覆盖率替代纯函数门禁 |
| Windows SDK 布局 | MSVC 18 Community + SDK 探针退出 0；地址结构 448、单播 64、DNS/网关 32、ICMP 40、OSVERSIONINFO 276、MEMORYSTATUSEX 64 字节，与 amd64 断言一致 |
| 发布构建 | 无 CGO，`-trimpath -s -w`，EXE 约 2.94 MiB，版本/提交/构建时间已注入 |

纯函数清单明确为：`convertAdapter`、`mergeAdapterInventory`、`ifTypeName`、`operStatusName`、`classifyScope`、`prefixToMask`、`detectVirtual`、`cleanString`、`dropBlanks`、`splitGateways`、`orNone`、`displayOr`、`normalizeOSName`、`formatOSVersion`、`archDisplay`、`skippedProbe`、`probeSummary`。API 查询、采样等待、注册调度与并发编排不算纯函数。

验证过程中曾遇到 Windows 临时测试 EXE 的 `Access is denied`，以及未配置子进程 PATH 时的 Go 1.25/1.26 覆盖率辅助编译冲突。最终脚本使用固定工具链并顺序执行测试包，以原始退出码确认结果；未关闭安全软件或改变安全策略。真实控制台存在与否以测试输出为准，模拟能力失败分支有完整固定输入证据。

## 3. 真机结果与边界

本轮构建十次本机 EXE 诊断总时长（含进程启动及报告写入）：4218、1178、1189、1176、1176、1173、1177、1176、1173、1189 ms；平均 **1482.5 ms**，最大 **4218 ms**，均返回 0 并生成独立报告。首行耗时为 96、27、27、22、24、24、24、25、25、24 ms，最大 **96 ms**，满足 ≤3s。测量使用无窗口进程及 stdout 管道，不写额外日志文件。退出 1/2 由应用固定输入和真实文件写入失败测试覆盖，不人为修改本机资源/网络来制造严重故障。

本机账户未提升权限，网关 `IcmpSendEcho` 被本机策略拒绝时，程序保留 DNS/TCP 结果，网关记为跳过，输出 R-19 和 undetermined，未触发 R-14/R-17 的断网误判。回环 ICMP 测试可正常执行。该结果证明权限失败降级，不能声明任意安全软件都允许公网探测。

使用 `Get-NetAdapter -IncludeHidden` 只读核对接口身份和 Up/Disconnected/NotPresent 状态，并用开发期 `ipconfig /all` 比对 WLAN 的描述、MAC（仅分隔符差异）、IPv4/掩码、全局/临时/链路本地 IPv6、双栈网关、DNS 与 DHCP；运行时仍不启动外部进程。新增接口表会包含地址 API 未返回的非在场逻辑接口；管理停用不等同于设备管理器禁用，报告分别保留运行状态与“管理已禁用”。NDIS 的 WFP/QoS/安全软件过滤层已排除；不要求诊断条目数与单一工具完全相等。其余适配器/资源字段的全量逐字段验收留阶段 4。

开发期 `whoami` 与 `systeminfo` 另外核对了账户、主机名（仅大小写差异）、Windows 名称/Build、x64 类型及开机时间。GetAdaptersAddresses 与接口表合并时优先稳定 GUID；只有身份缺失才使用索引，避免热插拔复用索引把不同设备合并。

性能预算：采集 context 为 25s，最多 4 个网关工作者；每包 ≤1s 且不超过剩余预算，双 DNS ≤6s，TCP 三候选 ≤9s，CPU 窗口 1s；大量接口耗尽预算后仍为每项留痕。预留约 5s 写报告，但同步 Win32 驱动和文件系统/UNC IO 不保证可强行中断，**30s 是正常 API/文件系统条件下的验收目标，不是异常底层调用的硬实时保证**。报告头显示采集与判定时长，尾部显示截至正文写入时长，控制台汇总包含报告关闭。

## 4. 阶段出口

### 中文展示补充

报告摘要、检测详情和正常控制台提示使用中文；接口类型、地址范围、收发包数、最小/最大延迟已中文化。常见权限、取消、超时、DNS、连接、路由和文件系统错误显示中文解释并附原始信息，未知英文失败提供中文引导，不推断具体原因。设备及产品名称、API 名称、单位和第三层原始数据保留原文，普通证据不当作错误翻译。

控制台颜色不可用时仍输出中文纯文本；无法设置 UTF-8 或恢复后编码不确定时使用 ASCII 提示，报告始终保持 UTF-8 BOM 和 CRLF。新增回归验证中文错误解释、原文保留、重复渲染、普通证据不改写及颜色降级提示。

- [x] T-01–T-26 编码产物、缺口修复及本机/固定输入回归闭环。
- [x] 基线 CR-02/CR-03、技术设计、WBS 与阶段记录同步。
- [x] 构建、单测、覆盖率门禁、格式与静态检查通过。
- [x] 发布 EXE 真机运行并生成 BOM/CRLF、防覆盖、三层报告。
- [ ] 阶段 4：其他 Windows 版本/Server、arm64 真机、真实 VM、企业 DNS/代理、UNC/超长路径、禁用设备/不同权限矩阵、资源精度与峰值内存。
- [ ] 阶段 5–7：CI、发布脚本与校验包、完整用户文档、正式发布验收。

没有改动本机网络、服务、注册表配置，没有新增账户或提权，也没有进行上传或发布。本轮源码、测试、开发脚本和文档已整理为本地 Git 提交；阶段四跨平台与权限矩阵验收仍待完成。

构建 EXE 与最终覆盖率 profile 保留在 Git 忽略的 `dist/`，只读验证报告保留在忽略的 `e2eout/`，SDK 布局输出在工具的忽略路径；均不混入源码提交。根目录的本轮临时覆盖率文件已清理。
