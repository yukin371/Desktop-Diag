# Desktop-Diag 任务拆解（阶段 2 交付物）

| 项目 | 内容 |
| --- | --- |
| 文档版本 | v1.2（2026-09-30 阶段 3 完成交付） |
| 状态 | **FROZEN** |
| 上游输入 | `doc/phase0/01-requirements-baseline.md`（FROZEN）、`doc/phase1/00-technical-design.md`（FROZEN） |
| 拆解粒度 | 工作包（WP）→ 任务（T）→ 验收标准（DoD） |
| 任务总数 | 37 个任务 / 12 个工作包 / 6 个里程碑（M0–M5） |

---

## 1. 拆解原则

1. **按依赖分层，不按需求编号分层**。同一层的任务可并行；跨层任务有严格先后。
2. **每个任务都能独立验证**。"写完了"不算完成，必须有可执行的验证命令或可观测的输出现象。
3. **可测性前置于实现**。纯函数（`probe/classify.go`、`detect/rules.go`）先立接口与测试骨架，再填实现。
4. **高风险任务前置**。`winapi` 结构体映射（RK-01）是本项目唯一"错了会崩"的地方，排在所有业务逻辑之前单独验证。
5. **验证纪律继承阶段 1**：任何手写 Win32 结构体，先跑一次性验证程序确认偏移与 `sizeof`，再写进正式包（见阶段 1 文档 15.2）。

---

## 2. 工作包与里程碑总览

```
里程碑                工作包                                 任务        对应阶段
─────────────────────────────────────────────────────────────────────────────
M0 基线就绪    WP-01 仓库骨架与构建基线              T-01 ~ T-02    阶段 3
               WP-02 winapi 层                       T-03 ~ T-06    阶段 3
               WP-03 probe 层                        T-07 ~ T-10    阶段 3
               WP-04 collect 层                      T-11 ~ T-15    阶段 3
M1 数据可采    ← 到此为止：能跑出完整 Snapshot
               WP-05 detect 层                       T-16 ~ T-19    阶段 3
               WP-06 report 层                       T-20 ~ T-22    阶段 3
               WP-07 ui / cli / app / main 接线      T-23 ~ T-26    阶段 3
M2 可交付      ← 到此为止：MVP 功能完整，能出报告
               WP-08 单元测试与覆盖率达标            T-27           阶段 4
               WP-09 集成验收与场景演练              T-28 ~ T-30    阶段 4
M3 质量闭环    ← 到此为止：测试与合规全绿
               WP-10 工程化流水线                    T-31 ~ T-33    阶段 5
M4 流水线就绪  ← 到此为止：一键构建可复现
               WP-11 文档整理 + PRD 回写             T-34 ~ T-35    阶段 6
               WP-12 版本发布                        T-36 ~ T-37    阶段 7
M5 发布完成    ← 交付 v0.1.0
```

---

## 3. WBS 详细拆解

### WP-01 仓库骨架与构建基线

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-01** | 仓库骨架 + 版本注入 | `.gitignore`、目录树、`cmd/desktop-diag/main.go`（stub）、`internal/version/version.go`、`scripts/build.ps1` | `go build` 成功；`Desktop-Diag.exe -version` 打印出正确版本/提交/构建时间（证明 ldflags 注入链路通）；`gofmt -l` 空 | — |
| **T-02** | `internal/model` 领域结构 | `snapshot.go` `host.go` `health.go` `probe.go` `issue.go` | 字段语义与设计一致；已记录 GatewaysV6 等落地差异，不以设计示意字段名作为实现验收；派生纯函数 `IsActive()` / `HasUsableIPv4()` / `ActivePhysicalAdapters()` / `IPv6OnlyLinkLocal()` 单测通过；`go vet` 零问题 | T-01 |

> **T-01 说明**：`go.mod` 已存在（`module github.com/yukin371/desktop-diag`，`go 1.26.0`，`require golang.org/x/sys v0.48.0`），本任务只需补 `.gitignore` 与目录骨架。
> `.gitignore` 必须排除 `dist/`、`*.exe`、覆盖率产物，但**不得**排除 `go.sum`。

### WP-02 `internal/winapi` 层（高风险，前置）

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-03** | ★ 结构体验证程序（一次性） | 临时程序，打印每个手写结构体的 `unsafe.Sizeof` 与关键字段偏移，并真机调用校验 | 4 个结构体（`IP_ADAPTER_ADDRESSES_LH`、`IP_ADAPTER_UNICAST_ADDRESS_LH`、`IP_ADAPTER_DNS_SERVER_ADDRESS_XP`、`IP_ADAPTER_GATEWAY_ADDRESS_LH`）+ `ICMP_ECHO_REPLY` 的 `sizeof`/偏移**实测正确**；`GetAdaptersAddresses` 双次调用返回 `NO_ERROR` 且 `Length` 自检通过；`IcmpSendEcho` 能收到真实回包 | T-02 |
| **T-04** | `winapi` 基础 API | `doc.go`、`kernel32.go`、`ntdll.go`、`token.go`、`console.go` | 6 个 API 真机调用成功：计算机名、系统盘容量、运行时长、OS 版本、内存、管理员标志、控制台代码页读取/设置；`doc.go` 声明"本包仅含只读调用" | T-03 |
| **T-05** | `iphlpapi_types.go` + `iphlpapi.go` | 结构体映射 + `GetAdaptersAddresses` 链表遍历 → `RawAdapter` 列表；`IcmpSendEcho` 封装 | 返回的网卡数量/名称/MAC/IP/网关/DNS 与 `ipconfig /all` **逐字段人工比对一致**；无 panic、无内存越界；`go vet` 零问题 | T-03 |
| **T-06** | `advapi32.go` 注册表只读封装 | 读 `CurrentVersion`（OS 名）、`Tcpip\Parameters\Interfaces\{GUID}`（DNS/DHCP） | 能取到 `ProductName`/`DisplayVersion`；对当前机器实际存在的接口按 GUID 关联，不硬性要求恰好 3 个 DNS 接口；键不存在或不可读时返回**可识别的错误**而非静默空值 | T-04 |

> **T-03 是全局关键路径的第一个卡点**。阶段 1 已验证 `MEMORYSTATUSEX` 的错误填充会导致 `ERROR_INVALID_PARAMETER(87)`,
> `IP_ADAPTER_ADDRESSES_LH` 比它大 20 倍、字段多 5 倍，**绝不允许凭记忆手写**。

### WP-03 `internal/probe` 层

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-07** | `probe/icmp.go` | ICMP 探测：`Send(target, count, timeout) → ProbeResult` | 对 `127.0.0.1` 探测 4 包收 4 包、丢包率 0%；对不可达地址丢包率 100%；**不依赖管理员权限**；`IcmpCreateFile` 失败时返回错误而非伪造丢包 | T-05 |
| **T-08** | `probe/dns.go` | 系统 DNS 解析 + 直连 `223.5.5.5:53` 对照解析 | 系统解析成功时 `Resolved` 非空；构造 `PreferGo` + 自定义 `Dial` 的直连解析可用；超时 3s 生效；解析失败时 `Err` 有可读原因 | T-04 |
| **T-09** | `probe/tcp.go` | TCP 443 候选目标探测（含超时与直连，**不走系统代理**） | 实测 `223.5.5.5:443` 可达并记录 RTT（阶段 1 实测 34–36 ms）；不可达目标在 3s 内失败；按候选列表顺序回退 | T-04 |
| **T-10** | `probe/classify.go` | 故障层级判定纯函数 `Classify([]ProbeResult) []LayerConclusion` | 7 行判定矩阵**逐行单测覆盖**，覆盖率 **100%**；函数内**无任何网络/IO 调用**（可脱离真机测试） | T-07, T-08, T-09 |

### WP-04 `internal/collect` 层

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-11** | `collect/collector.go` | `Collector` 接口 + `Register`/`All` 注册表 + 调度器 | 调度器逐项调用并计时；**单个采集器 panic 不终止整体**（`recover` 后转 `CollectFailure`）；`ctx` 取消能中断 | T-02 |
| **T-12** | `collect/system.go` | REQ-F-101~103 主机信息采集 | 计算机名/用户名/管理员标志/OS 名/版本/Build/架构/运行时长 全部填充；与 `systeminfo`、`whoami` 输出一致 | T-04, T-06 |
| **T-13** | `collect/network.go` | REQ-F-104~109 网卡信息采集 | 活动物理网卡被正确识别；虚拟网卡被标记 `IsVirtual`+`VirtualKind`；APIPA/IPv6 链路本地判定正确；DNS 走三级兜底链且 `DNSSource` 标注来源 | T-05, T-06 |
| **T-14** | `collect/health.go` | REQ-F-301~303 CPU/内存/磁盘 | CPU 用 `GetSystemTimes` 双采样求差（1s 窗口）；内存与磁盘与任务管理器/资源管理器**误差 < 2%**；采集失败时 `*Known=false` 而非填 0 | T-04 |
| **T-15** | `collect/probe.go` | REQ-F-201~206 连通性探测采集器 | 无活动网卡时全部 `Skipped=true` 且带 `SkipReason`，**不产生误导性严重告警**；正常时产出 4 类探测结果 | T-07, T-08, T-09, T-10, T-13 |

### WP-05 `internal/detect` 层

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-16** | `detect/thresholds.go` | 全部阈值常量（唯一数值来源） | 常量值与基线第 6 节**逐条一致**；`rg` 运行时代码确认规则函数内**无裸字面量**阈值 | T-02 |
| **T-17** | `detect/rules.go` + `catalog.go` | R-01 ~ R-19 共 19 条规则 + 元数据目录 | 19 条规则全部注册；每条规则有独立单测（构造 Snapshot → 断言 Issue）；`catalog` 可枚举出 ID/等级/类别/标题 | T-16 |
| **T-18** | `detect/engine.go` | `Evaluate(s)`：聚合 + 排序 + `recover` | 输出按 `(Severity desc, CategoryOrder, RuleID asc)` 排序，**多次运行结果完全一致**（REQ-N-08）；任一条规则 panic 不影响其他规则，且转为 Failures 触发 R-19 | T-17 |
| **T-19** | `detect` 单元测试 | 规则与引擎测试集 | `detect` 包覆盖率 **≥ 90%**；`probe/classify` 覆盖率 **100%**；`go test ./internal/detect/... ./internal/probe/...` 全绿 | T-18 |

### WP-06 `internal/report` 层

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-20** | `report/templates.go` + `sections.go` | 文案模板 + 分节注册表 | `RegisterSection(title, order, renderer)` 可用；分节按 `order` 排序渲染；无告警时的"未发现异常"文案正确 | T-02 |
| **T-21** | `report/render.go` | 三层渲染（纯函数，写 `io.Writer`） | 三层结构与设计文档 8.2 **模板逐节一致**；报告头含路径选择说明；报告尾含"未修改任何系统配置"声明；渲染函数可对 `bytes.Buffer` 单测 | T-18, T-20 |
| **T-22** | `report/writer.go` | 四级降级写入链 + 文件落盘 | ①`-o` ②EXE 目录 ③用户配置文件目录 `%USERPROFILE%\Desktop-Diag\reports`（不是 Documents 已知文件夹） ④`%TEMP%` **逐级降级实测生效**并记录降级原因；文件为 UTF-8 **with BOM** + **CRLF**；`O_CREATE\|O_EXCL` 保证同秒连续运行不覆盖（追加 `_1`/`_2`）；全部失败时返回错误由调用方转退出码 2 | T-21 |

### WP-07 `internal/ui` / `cli` / `app` / `main` 接线

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-23** | `ui/symbols.go` `color.go` `console.go` | 控制台呈现层含三级降级 | 控制台+VT → 彩色 + ✅⚠❌；重定向 → 无颜色无 ANSI 转义但保留 Unicode；`SetConsoleOutputCP` 失败 → 纯 ASCII `[OK]/[!]/[X]`；三种模式**均需实测**（重定向用 `> file` 触发） | T-04, T-02 |
| **T-24** | `cli/options.go` + `help.go` | CLI 解析 + 帮助文本 | `-o`/`-v`/`-version`/`-h`/`--help` 全部生效；非法参数返回错误 → 退出码 2；`-h` 输出可读帮助 | T-01 |
| **T-25** | `app/app.go` | 编排层 `Run(args, stdout, stderr) int` | 完整串起 解析→采集→判定→渲染→写报告→输出；退出码语义正确（0 无严重 / 1 有严重 / 2 程序错误）；**总耗时 ≤ 30s**（典型 ≤ 15s）；启动到首行输出 **≤ 3s** | T-11, T-12, T-14, T-15, T-19, T-22, T-23, T-24 |
| **T-26** | `cmd/desktop-diag/main.go` | 入口接线 | `os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))`；**真机端到端跑通一次并生成报告** | T-25 |

### WP-08 / WP-09 测试（阶段 4）

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-27** | 单元测试补齐 | `collect` 纯函数、`report`、`ui`、`cli` 测试 | 覆盖率：整体 **≥ 70%**、`detect` ≥ 90%、`classify` 100%、`collect` 纯函数 ≥ 80%、`report` ≥ 75%；`go vet ./...` 零问题 | T-26 |
| **T-28** | 集成验收测试 | 与真实系统输出的比对证据 | ① 网卡数据 vs `ipconfig /all` 逐字段一致；② ICMP vs 真机 `ping` 一致；③ 报告 BOM/CRLF 经十六进制查看确认；④ GBK 代码页下中文目视正常；⑤ 退出码三态实测 | T-27 |
| **T-29** | 场景演练 S-01 ~ S-18 | 场景测试结果记录（含无法构造场景的说明） | 18 个边界场景逐项演练：能构造的实测通过，无法构造的（如断网、虚拟机）给出替代验证或明确标注"未覆盖 + 理由" | T-28 |
| **T-30** | 合规扫描 | 红线验证报告 | 运行时代码中的实际禁用 API 调用 **0 命中**（排除文档和测试中的名称引用）：`RegSetValueEx`/`RegCreateKey`/`SetIpInterfaceEntry`/`CreateService`/`InternetOpen` 等写入与联网类 API；复核运行时代码无 `powershell`/`wmic`/`wscript`/`cscript`/`cmd.exe /c` 进程启动；开发用 PowerShell 脚本不属运行时违规；复核 os/exec 等进程启动点；EXE 不是 DLL 导入项，不能用导入表证明没有启动外部进程 | T-27 |

### WP-10 工程化流水线（阶段 5）

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-31** | `scripts/build.ps1` / `test.ps1` / `release.ps1` | 三个可执行脚本 | `build.ps1` 产出带版本注入的 EXE；`test.ps1` 输出覆盖率并**在低于门禁时非零退出**；`release.ps1` 产出 EXE + `sha256` 校验文件 | T-26 |
| **T-32** | `.github/workflows/ci.yml` | CI 流水线 | 在 `windows-latest` 上跑通：gofmt 检查 → `go vet` → `go test -cover` → 覆盖率门禁 → `go build`；显式声明 Go 版本与 `GOTOOLCHAIN`（见阶段 1 文档 13.1） | T-31 |
| **T-33** | 质量门禁自动化 | 门禁脚本/步骤 | 5 项门禁（gofmt / vet / 覆盖率 / 交叉编译 amd64 / 禁用 API/进程调用与报告写入白名单扫描）全部可由脚本一键执行并给出明确通过/失败 | T-31, T-30 |

### WP-11 / WP-12 文档与发布（阶段 6、7）

| ID | 任务 | 产出 | DoD（验收标准） | 依赖 |
| --- | --- | --- | --- | --- |
| **T-34** | 项目文档 | `README.md`、`CHANGELOG.md`、`LICENSE`、`doc/architecture.md` | README 含：用途、**只读不修声明**、使用方法、退出码表、报告结构说明、常见问题；架构文档由阶段 1 设计文档提炼 | T-26 |
| **T-35** | PRD 回写修订 | 更新 `doc/prd.md` | 阶段 0 评审的 D1–D5 一致性瑕疵全部修订；补充第 9 节编号连续性；记录基线澄清及实施差距，不无记录改变已冻结范围 | T-34 |
| **T-36** | 版本发布 | `v0.1.0` tag + 发布产物 | 构建产物 `Desktop-Diag.exe` ≤ 15 MB（预期 6–9 MB）、单文件无依赖；`sha256` 校验文件；git tag 打上并记录构建元数据 | T-28, T-29, T-30, T-32, T-33, T-35 |
| **T-37** | 发布说明 | `doc/phase7/` 发布记录 | 记录版本号、产物校验值、支持的系统矩阵、已知限制（V1.1 顺延项：5.6 启动项与磁盘占用分析） | T-36 |

---

## 4. 依赖关系与关键路径

### 4.1 依赖关系复核

任务表依赖是执行与验收的约束；未提供任务工期估算，不声明已经计算出最短工期或关键路径。原路径把 T-15 → T-18、T-28 → T-32 等关系直接连线，与任务表不一致，本轮替换为以下汇合关系。

```text
T-01 → T-02 → T-03 → T-04/T-05
T-04 → T-06 → T-12；T-05/T-06 → T-13；T-04 → T-14
T-05 → T-07；T-04 → T-08/T-09；T-07/T-08/T-09 → T-10
T-07/T-08/T-09/T-10/T-13 → T-15
T-02 → T-11/T-16/T-20；T-16 → T-17 → T-18 → T-19
T-18/T-20 → T-21 → T-22；T-04/T-02 → T-23；T-01 → T-24
T-11/T-12/T-14/T-15/T-19/T-22/T-23/T-24 → T-25 → T-26
T-26 → T-27 → T-28 → T-29；T-27 → T-30
T-26 → T-31 → T-32；T-31/T-30 → T-33
T-26 → T-34 → T-35
T-28/T-29/T-30/T-32/T-33/T-35 → T-36 → T-37
```

T-03 的 Win32 布局验证仍是高风险前置任务。T-35 的 PRD 文字修订已在本轮提前完成，T-34 及阶段 6 的整体交付仍未完成。

### 4.2 并行机会

- **WP-03（T-07~T-10）** 与 **WP-02 的 T-06** 无相互依赖，可并行。
- **T-16（阈值常量）** 只依赖 T-02，可与整个 WP-02 并行推进 —— 这是把 `detect` 从 `winapi` 解耦的直接收益。
- **T-20（报告模板）** 只依赖 T-02，可与 WP-03/WP-04 并行。

### 4.3 阻塞风险点

| 风险点 | 任务 | 若不通过的影响 | 预案 |
| --- | --- | --- | --- |
| ★ 结构体映射错误 | T-03 / T-05 | 网卡数据全错或进程崩溃，RK-01 成真 | 阶段 1 已验证方法论；双次调用 + `Length` 自检 + `ipconfig /all` 比对三重保险 |
| `IcmpSendEcho` 被安全软件拦截 | T-07 | 无法判定内网链路 | 失败记 `Failures` 触发 R-19，**不**误报网关不可达；报告如实说明 |
| TCP 443 候选目标不可达 | T-09 | R-17 误报外网中断 | 3 个候选 IP 逐个回退；阶段 1 已实测首选 `223.5.5.5:443` 有效（34–36 ms） |
| 非管理员环境下采集受限 | T-12~T-14 | 报告缺数据 | 权限自适应即为此设计；缺数据须触发 R-19 而非静默 |

---

## 5. 可执行 Todo 清单（阶段 3 起逐项执行）

按执行顺序排列；`[x]` 表示阶段 3 编码与本机/固定输入验证完成，跨 OS/权限/UNC 等场景仍由阶段 4 承载，不将其勾选冒充完整兼容性验收。最新证据见 [phase3 交付记录](../phase3/01-completion.md)，早期差距见审计。

### 阶段 3 · 编码开发

- [x] **T-01** 仓库骨架 + 版本注入链路（`.gitignore` / 目录树 / `main.go` stub / `version.go` / `build.ps1`）
- [x] **T-02** `internal/model` 领域结构、共享证据与派生纯函数单测
- [x] **T-03** ★ 手写 Win32 结构体验证程序（5 个结构体 `sizeof`/偏移 + 真机调用）
- [x] **T-04** `winapi`：`doc.go` / `kernel32.go` / `ntdll.go` / `token.go` / `console.go`
- [x] **T-05** `winapi`：`iphlpapi_types.go` + `iphlpapi.go`（链表遍历 + ICMP）→ 与 `ipconfig /all` 比对
- [x] **T-06** `winapi`：`advapi32.go` 注册表只读封装（OS 名 + DNS/DHCP 兜底）
- [x] **T-07** `probe/icmp.go`（4 包 × 1s，无管理员权限）
- [x] **T-08** `probe/dns.go`（系统解析 + 直连 223.5.5.5 对照）
- [x] **T-09** `probe/tcp.go`（3 候选目标，直连不走代理）
- [x] **T-10** `probe/classify.go` 7 行判定矩阵 + 100% 单测
- [x] **T-11** `collect/collector.go` 接口 + 注册表 + 调度器（panic 隔离）
- [x] **T-12** `collect/system.go` 主机信息
- [x] **T-13** `collect/network.go` 网卡信息 + DNS 三级兜底
- [x] **T-14** `collect/health.go` CPU 双采样 / 内存 / 磁盘
- [x] **T-15** `collect/probe.go` 连通性探测采集器
- [x] **T-16** `detect/thresholds.go` 阈值常量（唯一数值源）
- [x] **T-17** `detect/catalog.go` + `rules.go` R-01~R-19
- [x] **T-18** `detect/engine.go` 聚合 + 确定性排序 + panic 隔离
- [x] **T-19** `detect` + `probe` 单元测试（≥90% / 100%）
- [x] **T-20** `report/templates.go` + `sections.go` 分节注册表
- [x] **T-21** `report/render.go` 三层渲染
- [x] **T-22** `report/writer.go` 四级降级写入链（BOM + CRLF + `O_EXCL`）
- [x] **T-23** `ui/` 控制台三级降级（颜色 / Unicode / ASCII）
- [x] **T-24** `cli/options.go` + `help.go`
- [x] **T-25** `app/app.go` 编排 + 退出码 + 耗时达标
- [x] **T-26** `cmd/desktop-diag/main.go` 接线 + **端到端真机跑通**

### 阶段 4 · 测试

阶段四启动入口、验收步骤、场景矩阵和结果台账见 [`doc/phase4/README.md`](../phase4/README.md)、[`00-validation-plan.md`](../phase4/00-validation-plan.md) 与 [`01-results.md`](../phase4/01-results.md)。

- [x] **T-27** 单元测试补齐，覆盖率达标（整体 ≥70%）
- [x] **T-28** 集成验收测试已收口（本机完成 I-01/I-03/I-04/I-06/I-07；GBK 目视、完整接口归档保留为环境限制）
- [x] **T-29** 场景演练已登记并收口（可执行项目完成；未取得隔离环境的 S-01～S-18 项目保持未覆盖）
- [x] **T-30** 合规扫描（运行时代码禁用 API 与外部进程扫描 0 命中；见 `doc/phase4/02-redline.md`）

### 阶段 5 · 工程化流水线

- [x] **T-31** `scripts/` 三个脚本（build / test / release）已实现；`release.ps1` 本机发布冒烟通过
- [x] **T-32** `.github/workflows/ci.yml` 已实现，待 Windows runner 验证
- [x] **T-33** 5 项质量门禁自动化已实现，本机通过；待 Windows runner 验证

### 阶段 6 · 文档整理

- [ ] **T-34** `README.md` / `CHANGELOG.md` / `LICENSE` / `doc/architecture.md`
- [x] **T-35** PRD 回写修订（D1–D5 + 第 9 节编号；本轮提前完成，T-34/阶段 6 整体未完成）

### 阶段 7 · 版本发布

- [ ] **T-36** `v0.1.0` 构建 + tag + sha256
- [ ] **T-37** 发布说明与已知限制记录

---

## 6. 需求覆盖矩阵（REQ → 任务）

| 需求组 | 需求编号 | 承载任务 |
| --- | --- | --- |
| 主机与网卡采集 | REQ-F-101 ~ 109 | T-05, T-06, T-12, T-13 |
| 网络连通性探测 | REQ-F-201 ~ 206 | T-07, T-08, T-09, T-15 |
| 系统健康度 | REQ-F-301 ~ 303 | T-04, T-14 |
| 判定引擎 | REQ-F-401 ~ 405 | T-10, T-16, T-17, T-18 |
| 三层报告 | REQ-F-501 ~ 508 | T-20, T-21, T-22 |
| 控制台与 CLI | REQ-F-601 ~ 605 | T-23, T-24, T-25 |
| 权限降级 | REQ-F-701 ~ 705 | T-11, T-13, T-15, T-25 |
| 非功能 REQ-N-01~11 | 性能/体积/单文件/免安装/健壮性/可复现/可扩展/覆盖率/静态检查 | T-01(T-31 体积), T-19, T-25(性能), T-27, T-33 |
| 红线 C-01 ~ C-07 | 只读/只诊/零上传/无驻留/无 GUI/无脚本引擎/无外部依赖 | T-04(`doc.go`), T-09(不走代理), T-24(无 GUI), T-30(合规扫描) |

**覆盖结论**：7 组功能需求（41 条）已有任务承载；原非功能矩阵仅聚合列举，遗漏了具体验收动作，按第 8 节补齐。

---

## 7. 出口检查单

- [x] WBS 拆解到可独立验证的任务粒度（37 个任务）
- [x] 每个任务有明确 DoD（可执行命令或可观测现象）
- [x] 工作包与里程碑划分完成（12 WP / 6 里程碑）
- [x] 任务表与依赖关系已补正并检查无环；无工期估算，不声明已计算关键路径
- [x] 并行机会已识别（3 处）
- [x] 阻塞风险点与预案已登记（4 项）
- [x] 可执行 Todo 清单产出（按阶段 3~7 分组）
- [x] 需求覆盖矩阵已复核，并补齐非功能验收承载与阶段 3 缺口
- [ ] 原基线最终确认记录未留存；当前用户已要求修复与完成阶段 3，CR-02/CR-03 已记录，不补造原审批

**交付物**：`doc/phase2/00-task-breakdown.md`（本文件）

**阶段四收口**：T-28～T-30 已完成本机可执行验收并形成带明确环境限制的收口；未取得的标准用户、隔离场景、其他 Windows/Server 与 arm64 真机保持未覆盖，详见 `doc/phase4/README.md` 与 `doc/phase4/01-results.md`。不得以覆盖率代替场景矩阵。

## 8. 验收承载补充与当前状态

阶段 3 编码出口已完成；第五节对应本机和固定输入验证，完整场景矩阵留阶段 4。最新证据见 [phase3 交付记录](../phase3/01-completion.md)。T-35 的 D1–D5 PRD 修订已提前完成，T-34 用户文档尚未完成，因此不将整个阶段 6 标为完成。

| 需求 | 验收承载 | 必须补齐的动作 |
| --- | --- | --- |
| REQ-N-01/02 | T-25、T-28 | 记录首行与首条进度时间；多网卡最坏预算、10 次耗时及落盘耗时 |
| REQ-N-03 | T-28 | 采样峰值内存并记录口径（工作集/私有字节），≤50 MB |
| REQ-N-04/05 | T-31、T-36、T-28 | 测量 EXE 体积；空目录单文件运行及版本注入 |
| REQ-N-06、REQ-F-705 | T-28、T-29 | 普通用户身份真机运行并记录身份与报告路径 |
| REQ-N-07 | T-27、T-29 | 采集/探测/写入故障注入，确认退出码语义 |
| REQ-N-08 | T-28、T-29 | 连续 3 次比较，剔除时间和动态采样字段 |
| REQ-N-09 | T-27、T-34 | 以测试采集域/规则/分节证明注册扩展无需改调度与报告骨架 |
| REQ-N-10 | T-19、T-27 | 整体按语句加权统计，不取包覆盖率平均；collect 纯函数门禁单独定义 |
| REQ-N-11 | T-27、T-32、T-33 | gofmt、vet、测试与门禁非零退出检查 |
| C-01–07 | T-30 | 检查实际系统写入与进程调用、报告清理白名单及控制台设置恢复；不扫描文档词语定罪 |
| Windows/权限/UNC/路径矩阵 | T-28、T-29、T-37 | 区分目标支持与实测支持，未覆盖项写入发布限制 |

已补正依赖表：T-10 依赖 T-07/T-08/T-09（完整探测结果契约）；T-15 包括 T-10（调用 Classify）；T-25 包括 T-12/T-14/T-15（完整采集链）；T-36 包括 T-28/T-29/T-30/T-32/T-33/T-35（场景、合规、CI 与文档门禁）。第 4 节已替换错误连线。
