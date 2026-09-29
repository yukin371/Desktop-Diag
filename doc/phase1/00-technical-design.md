# Desktop-Diag 技术方案设计 v1.0（阶段 1 交付物）

| 项目 | 内容 |
| --- | --- |
| 文档状态 | 已冻结（FROZEN） |
| 日期 | 2026-09-30 |
| 上游输入 | `doc/phase0/01-requirements-baseline.md`（需求基线 v1.0，41 条功能性需求 / 19 条判定规则 / 7 条红线） |
| 下游消费者 | 阶段 2 任务拆解、阶段 3 编码、阶段 4 测试、阶段 5 工程化 |
| 设计原则 | 纯 Win32 API · 零脚本引擎 · 分层解耦 · 全部判定可单测 · 采集器注册式扩展 |

---

## 1. 设计目标与约束映射

| 设计目标 | 对应需求 | 设计手段 |
| --- | --- | --- |
| 零 PowerShell / 零 WMI / 零 COM | C-06、Q3 | `internal/winapi` 直接 syscall Win32；DNS 走 Go 标准库（Windows 下调用 `GetAddrInfoW`），无脚本宿主 |
| 全部判定规则可单元测试 | REQ-F-401、REQ-N-10 | **纯函数边界**：syscall 层只做数据获取，逻辑层（`collect`/`detect`/`report`）只接受结构体入参 |
| 新增采集域零改核心 | REQ-N-09、Q1 | 采集器注册表 + 报告分节注册表，双注册点 |
| 单文件、免安装 | C-07、REQ-N-05 | `CGO_ENABLED=0` + `-trimpath` + `-ldflags "-s -w"` 静态编译 |
| 单项失败不崩溃 | REQ-F-702、E6 | 统一降级总纲：`Collector` 失败被捕获并记入 `Snapshot.Failures`，调度不中断、状态不回滚 |
| 报告写入多级降级 | Q2、REQ-F-506 | 独立 `report.Writer`，四级候选链 + 写探针 |

---

## 2. 架构总览

### 2.1 分层结构

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/desktop-diag            进程入口：装配依赖、调用 app.Run │
│                              返回值 → os.Exit(code)          │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────┐
│  internal/app                编排层（唯一知道"顺序"的地方）    │
│   · 解析 CLI          · 逐项调度采集器                        │
│   · 调用判定引擎      · 渲染报告 + 控制台输出                 │
│   · 汇总退出码                                                │
└───┬──────────┬──────────┬──────────┬──────────┬─────────────┘
    │          │          │          │          │
┌───▼───┐ ┌────▼────┐ ┌───▼────┐ ┌───▼────┐ ┌───▼──────┐
│  cli  │ │ collect │ │ detect │ │ report │ │    ui    │
│ 参数  │ │ 采集器  │ │ 判定引擎│ │ 渲染写入│ │ 控制台呈现│
└───────┘ └────┬────┘ └───┬────┘ └───┬────┘ └────┬─────┘
               │          │          │           │
               │      ┌───▼──────────▼───┐   ┌───▼──────┐
               │      │   internal/model  │   │ winapi   │
               │      │   领域数据结构     │   │ (编码)   │
               │      └───────────────────┘   └───┬──────┘
               │                                  │
        ┌──────▼───────┐                  ┌───────▼────────┐
        │ internal/probe│                  │ internal/winapi│
        │ ICMP/DNS/TCP │                  │ Win32 syscall  │
        └──────┬───────┘                  └───────┬────────┘
               │                                  │
               └──────────┬───────────────────────┘
                          │
                   Windows 内核 / IP Helper
```

### 2.2 依赖方向（严格单向，无环）

```
cmd → app → {cli, collect, detect, report, ui}
collect → {model, winapi, probe}
probe   → {model, winapi}
detect  → {model, detect/thresholds}
report  → {model}
ui      → {model, winapi}
model   → 无内部依赖（叶子包）
winapi  → 无内部依赖（叶子包）
```

**强制约束**：`model`、`winapi` 不得 import 任何其他 internal 包；`detect` 不得 import `winapi`（否则规则无法脱离真机单测）。

---

## 3. 仓库与包结构

```
Desktop-Diag/
├─ cmd/
│  └─ desktop-diag/
│     └─ main.go                    入口，仅 ~30 行
├─ internal/
│  ├─ app/
│  │  └─ app.go                     编排层：Run(args, stdout, stderr) int（唯一知道"顺序"的地方）
│  ├─ cli/
│  │  ├─ options.go                 Options / Parse(args) / 退出码常量
│  │  └─ help.go                    -h 帮助文本
│  ├─ version/
│  │  └─ version.go                 ldflags 注入的 Version/Commit/BuildTime + String()
│  ├─ model/
│  │  ├─ snapshot.go                Snapshot / CollectFailure / RawLine
│  │  ├─ host.go                    Host / Adapter / Addr
│  │  ├─ health.go                  Health
│  │  ├─ probe.go                   ProbeResult / ProbeKind / LayerConclusion / Level
│  │  └─ issue.go                   Severity / Category / Issue
│  ├─ winapi/
│  │  ├─ doc.go                     包说明：全部为只读调用，无写系统状态能力
│  │  ├─ kernel32.go                GetComputerNameExW / GetTickCount64 / GetSystemTimes
│  │  │                             / GlobalMemoryStatusEx / GetDiskFreeSpaceExW
│  │  ├─ ntdll.go                   RtlGetVersion
│  │  ├─ iphlpapi.go                GetAdaptersAddresses / IcmpCreateFile / IcmpSendEcho
│  │  ├─ iphlpapi_types.go          IP_ADAPTER_ADDRESSES_LH 等 C 结构体 Go 映射
│  │  ├─ advapi32.go                注册表只读封装（薄封装 x/sys/windows/registry）
│  │  ├─ console.go                 GetConsoleOutputCP / SetConsoleOutputCP / IsConsole
│  │  └─ token.go                   管理员权限检测（x/sys/windows Token.IsElevated）
│  ├─ probe/
│  │  ├─ icmp.go                    IcmpSendEcho 封装：Send(n, timeout) → 统计
│  │  ├─ dns.go                     系统 DNS 解析 + 直连 223.5.5.5 解析
│  │  ├─ tcp.go                     TCP 443 候选目标探测
│  │  └─ classify.go                故障层级判定（纯函数，可单测）
│  ├─ collect/
│  │  ├─ collector.go               Collector 接口 + Registry
│  │  ├─ system.go                  REQ-F-101~103（主机信息）
│  │  ├─ network.go                 REQ-F-104~109（网卡信息）
│  │  ├─ health.go                  REQ-F-301~303（CPU/内存/磁盘）
│  │  └─ probe.go                   REQ-F-201~206（连通性探测采集器）
│  ├─ detect/
│  │  ├─ thresholds.go              ★ 全部阈值常量（唯一数值来源）
│  │  ├─ rules.go                   R-01 ~ R-19 规则函数
│  │  ├─ engine.go                  Evaluate(snapshot) []Issue，含排序
│  │  └─ catalog.go                 规则元数据（ID/等级/类别/标题模板）便于文档与测试共用
│  ├─ report/
│  │  ├─ render.go                  三层渲染（纯函数，接受 io.Writer）
│  │  ├─ writer.go                  四级降级写入链
│  │  ├─ sections.go                报告分节注册表（扩展点）
│  │  └─ templates.go               文案模板与分隔线
│  └─ ui/
│     ├─ console.go                 逐项进度输出 [n/N]、汇总
│     ├─ color.go                   ANSI 颜色 + 非 TTY/重定向自动降级
│     └─ symbols.go                 ✅ / ⚠ / ❌ 符号集（含纯 ASCII 降级）
├─ doc/
│  ├─ prd.md
│  ├─ phase0/ …phase1/ …            各阶段交付物
│  └─ architecture.md               阶段 6 汇总（由本设计文档提炼）
├─ scripts/
│  ├─ build.ps1                     本地构建 + 版本注入
│  ├─ test.ps1                      测试 + 覆盖率
│  └─ release.ps1                   交叉编译 + sha256
├─ .github/workflows/ci.yml         阶段 5
├─ go.mod                           module github.com/yukin371/desktop-diag
├─ go.sum
├─ .gitignore
├─ README.md
├─ CHANGELOG.md
└─ LICENSE
```

**模块路径决策**：`github.com/yukin371/desktop-diag`（取自本机 git 身份 `yukin371`，无远程仓库；若后续推送至其他账号，仅需 `go mod edit -module` 一次替换）。

---

## 4. 核心数据结构（`internal/model`）

> 设计要点：全部为**值语义的普通结构体**，无指针网状引用、无接口，保证可比较、可序列化、可 golden 测试。

### 4.1 快照根对象

```go
// Snapshot 是一次完整诊断的全部输入，是 collect → detect → report 之间唯一的传递介质。
type Snapshot struct {
	StartedAt time.Time
	Host      Host
	Adapters  []Adapter
	Health    Health
	Probes    []ProbeResult
	Layers    []LayerConclusion // 由 probe.classify 计算
	Failures  []CollectFailure  // 采集失败清单 → R-19
	Raw       []RawLine         // 第三层附录原始数据
}

type CollectFailure struct {
	Item    string // 人类可读的采集项名，如 "网卡信息"
	EnvVar  string // 内部标识，如 "network"
	Reason  string // 失败原因原文
	Partial bool   // 是否部分成功（如 3 块网卡中 1 块读取失败）
}

type RawLine struct {
	Section string // 分节标题
	Source  string // 数据来源，如 "GetAdaptersAddresses" / "HKLM\\...\\Interfaces"
	Line    string // 原始键值对
}
```

### 4.2 主机信息

```go
type Host struct {
	ComputerName string
	UserName     string
	IsAdmin      bool
	OSName       string    // "Windows 11 Pro for Workstations"
	OSVersion    string    // "10.0.26200"
	OSBuild      string    // "26200"
	OSArch       string    // "AMD64"
	Uptime       time.Duration
	BootTime     time.Time // StartedAt - Uptime，近似值
}
```

### 4.3 网卡信息

```go
type Adapter struct {
	Index        uint32
	Name         string   // FriendlyName，如 "以太网"
	Description  string   // 硬件描述，如 "Realtek PCIe GbE Family Controller"
	MAC          string   // "AA-BB-CC-DD-EE-FF"，无 MAC 时为空
	IfType       string   // "Ethernet" / "IEEE80211" / "Tunnel" / "Loopback" / 其他
	OperStatus   string   // "Up" / "Down" / "Testing" / "Unknown" / "Dormant" / "NotPresent" / "LowerLayerDown"
	AdminEnabled bool     // 是否未被设备管理器禁用
	IsVirtual    bool     // 虚拟网卡识别结果
	VirtualKind  string   // "VMware" / "Hyper-V" / "VirtualBox" / "TAP" / "WireGuard" / "Loopback" / ""
	IPv4         []Addr   // 地址 + 掩码 + 前缀长度
	IPv6         []Addr
	Gateways     []string // IPv4 默认网关（来自 FirstGatewayAddress + 注册表兜底）
	DNS          []string // DNS 服务器（来自 FirstDnsServerAddress + 注册表兜底）
	DNSSource    string   // "GetAdaptersAddresses" / "Registry" / "未采集"
	DHCPEnabled  bool
	DHCPKnown    bool     // 是否成功判定 DHCP 状态（注册表不可读时为 false）
}

type Addr struct {
	IP     string
	Mask   string // 点分十进制，IPv4 才填
	Prefix int    // 前缀长度
	Scope  string // IPv6: "Global" / "LinkLocal" / "SiteLocal" / "Other"
}
```

**派生辅助方法（纯函数，全部可单测）**：
- `(a Adapter) IsActive() bool` → `OperStatus == "Up"`
- `(Adapter) HasUsableIPv4() bool`
- `(Snapshot) ActivePhysicalAdapters() []Adapter` → 过滤「Up + 非虚拟 + 非 Loopback」
- `(Snapshot) IPv6OnlyLinkLocal() bool` → R-05 判定输入

### 4.4 健康度

```go
type Health struct {
	CPUPercent     float64 // 0–100，1s 窗口
	CPUKnown       bool
	MemTotalBytes  uint64
	MemAvailBytes  uint64
	MemUsedPercent float64
	MemKnown       bool
	SystemDrive    string // "C:"
	DiskTotalBytes uint64
	DiskFreeBytes  uint64
	DiskKnown      bool
}
```

### 4.5 探测结果

```go
type ProbeKind string

const (
	ProbeICMPGateway ProbeKind = "icmp-gateway"
	ProbeDNSSystem   ProbeKind = "dns-system"
	ProbeDNSDirect   ProbeKind = "dns-direct"
	ProbeTCP443      ProbeKind = "tcp-443"
)

type ProbeResult struct {
	Kind        ProbeKind
	AdapterName string        // ICMP 探测归属网卡；其他为空
	Target      string        // "192.168.1.1" / "223.5.5.5:53" / "223.5.5.5:443"
	Sent        int           // ICMP 发包数
	Recv        int
	LossPercent float64
	MinRTT      time.Duration
	AvgRTT      time.Duration
	MaxRTT      time.Duration
	Success     bool
	Resolved    []string      // DNS 探测解析结果
	Duration    time.Duration // 总耗时
	Err         string        // 失败原因原文（不含敏感信息）
	Skipped     bool
	SkipReason  string
}

type LayerConclusion struct {
	Level    string // "ok" / "wan-dns" / "wan-down" / "wan-port" / "lan-down" / "icmp-filtered" / "undetermined"
	Summary  string // 内网正常 · 外网正常，本机 DNS 故障
	Severity model.Severity
}
```

### 4.6 告警对象

```go
type Severity int

const (
	SevOK Severity = iota
	SevWarning
	SevSevere
)

type Category string

const (
	CatNetwork Category = "网络"
	CatSystem  Category = "系统"
	CatStorage Category = "存储"
	CatMeta    Category = "诊断完整性"
)

type Issue struct {
	RuleID     string   // "R-01"
	Severity   Severity
	Category   Category
	Title      string   // 第一层单行标题
	Detail     string   // 多行详情
	Evidence   []string // 关联数据，如 "IP: 169.254.13.7 (以太网)"
	Suggestion string   // 排查方向（只读建议，非自动修复）
}
```

---

## 5. 采集器接口与扩展机制

### 5.1 接口定义

```go
// Collector 是采集域的统一抽象。
// 契约：
//  1. Collect 不得 panic；不得提前终止进程；不得修改系统状态（C-01/C-02）。
//  2. Collect 失败必须通过返回值暴露，调度层负责记入 Snapshot.Failures。
//  3. Collect 只能写 Snapshot 中属于自己域的字段（避免跨域耦合）。
type Collector interface {
	// Name 返回人类可读的采集项名，用于进度输出 [n/N] 与失败清单。
	Name() string
	// Collect 执行采集，将结果写入 snap。
	Collect(ctx context.Context, snap *model.Snapshot) error
}
```

### 5.2 双注册表（满足 REQ-N-09）

```go
// collect.Registry —— 调度顺序在 register 时确定，app 不需要硬编码列表。
func Register(c Collector) { registry = append(registry, c) }
func All() []Collector     { return registry }
```

```go
// report.sections —— 第二层详情分节的渲染器注册表。
type SectionRenderer func(w io.Writer, snap *model.Snapshot)
func RegisterSection(title string, order int, r SectionRenderer)
```

**扩展一个新采集域（V1.1 启动项采集）的完整改动清单**：
| 步骤 | 文件 | 是否触碰核心 |
| --- | --- | --- |
| 1. 新增领域结构体 | `internal/model/startup.go` | 否 |
| 2. Snapshot 增加一个字段 | `internal/model/snapshot.go` | 是（1 行） |
| 3. 实现采集器 + `init()` 中 `Register` | `internal/collect/startup.go` | 否 |
| 4. 注册第二层分节渲染器 | `internal/report/sections.go` 或采集器同文件 `init()` | 否 |
| 5. 新增判定规则 + `init()` 中注册 | `internal/detect/rules.go` 或新文件 | 否 |

结论：**调度层（`app`）、判定引擎（`detect.Evaluate`）、报告骨架（`report.Render`）零改动**，仅 `Snapshot` 加 1 个字段。达标。

### 5.3 采集器清单与顺序

| # | Name | 文件 | 产出 | 可失败 |
| --- | --- | --- | --- | --- |
| 1 | 主机与系统信息 | `system.go` | `Host` | 部分（RtlGetVersion 失败可回退注册表） |
| 2 | 网络适配器信息 | `network.go` | `Adapters` | 是 |
| 3 | 系统健康度 | `health.go` | `Health` | 部分（CPU 采样失败不影响内存/磁盘） |
| 4 | 网络连通性探测 | `probe.go` | `Probes` + `Layers` | 是（无活动网卡时全部 `Skipped`） |

> 顺序即依赖顺序：判定引擎需要 `Adapters` 才能解释 `Probes`，因此网卡必须在探测之前。

---

## 6. `internal/winapi` 详细设计

### 6.1 API 映射总表（对应基线第 5 节）

| 采集项 | DLL | 函数 | 权限要求 | 备注 |
| --- | --- | --- | --- | --- |
| 计算机名 | kernel32 | `GetComputerNameExW(ComputerNameDnsHostname)` | 无 | — |
| OS 版本 | ntdll | `RtlGetVersion` | 无 | 规避 `GetVersionEx` 版本谎报 |
| OS 显示名 | advapi32 | 注册表 `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion` → `ProductName` / `DisplayVersion` | 读权限（普通用户可读） | 失败仅影响显示名 |
| 运行时长 | kernel32 | `GetTickCount64` | 无 | 返回 ms，需注意 49.7 天回绕（uint64 实际 5.8 亿年） |
| 网卡全量信息 | iphlpapi | `GetAdaptersAddresses(AF_UNSPEC, GAA_FLAG_INCLUDE_GATEWAYS\|GAA_FLAG_SKIP_ANYCAST\|GAA_FLAG_SKIP_MULTICAST)` | 无 | ★ 核心难点，见 6.2 |
| 网关 | iphlpapi | `IP_ADAPTER_ADDRESSES_LH.FirstGatewayAddress` | 无 | 需 `GAA_FLAG_INCLUDE_GATEWAYS` |
| DNS | iphlpapi | `IP_ADAPTER_ADDRESSES_LH.FirstDnsServerAddress` | 无 | 主来源 |
| DNS 兜底 | advapi32 | 注册表 `…\Tcpip\Parameters\Interfaces\{GUID}` → `NameServer` / `DhcpNameServer` | 读权限 | 见 6.4 |
| DHCP 状态 | advapi32 | 同上键 → `EnableDHCP` | 读权限 | 失败 → `DHCPKnown=false`，不误报 |
| ICMP 探测 | iphlpapi | `IcmpCreateFile` / `IcmpSendEcho` / `IcmpCloseHandle` | **无需管理员** | ★ 见 6.3 |
| CPU 占用率 | kernel32 | `GetSystemTimes` × 2（间隔 1s） | 无 | idle/kernel/user 三个 FILETIME 求差 |
| 内存 | kernel32 | `GlobalMemoryStatusEx` | 无 | `ullTotalPhys` / `ullAvailPhys` |
| 磁盘 | kernel32 | `GetDiskFreeSpaceExW` | 无 | 系统盘盘符取 `%SystemDrive%` |
| 管理员检测 | advapi32 | `x/sys/windows` → `Token.IsElevated()` | 无 | — |
| 控制台代码页 | kernel32 | `GetConsoleOutputCP` / `SetConsoleOutputCP(65001)` | 无 | 仅当 stdout 为控制台时调用 |

**依赖白名单复核**：上表仅使用 `syscall` + `golang.org/x/sys/windows` + 标准库 `net`/`os`。零第三方运行时、零脚本宿主、零 COM。✔ 满足 C-06、C-07 与基线第 5 节依赖约束。

### 6.2 核心难点一：`GetAdaptersAddresses` 结构体映射

这是全项目**最容易出错**的地方（结构体布局错位会导致内存越界或字段乱码）。设计对策：
1. 严格按 `iptypes.h`（`_IP_ADAPTER_ADDRESSES_LH`）定义，字段顺序与对齐不得调整；
2. 结构体首字段为 `Length uint32`，调用时预置为 `unsafe.Sizeof`，**用作自检**；
3. 首次调用带 `ULONG` size 参数的探测式为「先传 15KB 缓冲区，若返回 `ERROR_BUFFER_OVERFLOW` 则按返回值重新分配」；
4. 采用**双次调用**模式：第一次 `size=0` 取所需大小 → 分配 → 第二次取数据；
5. 验证手段：阶段 3 完成后，将输出与 `ipconfig /all` 逐字段人工比对（阶段 4 验收项）。

```go
// 关键字段的 Go 映射（节选，完整版在 iphlpapi_types.go）
type ipAdapterAddresses struct {
	Length                 uint32
	IfIndex                uint32
	Next                   *ipAdapterAddresses
	AdapterName            *byte
	FirstUnicastAddress    *ipAdapterUnicastAddress
	FirstAnycastAddress    uintptr
	FirstMulticastAddress  uintptr
	FirstDNSServerAddress  *ipAdapterDNSServerAddress
	DNSSuffix              *uint16
	Description            *uint16
	FriendlyName           *uint16
	PhysicalAddress        [8]byte
	PhysicalAddressLength  uint32
	Flags                  uint32
	Mtu                    uint32
	IfType                 uint32
	OperStatus             uint32
	Ipv6IfIndex            uint32
	ZoneIndices            [16]uint32
	FirstPrefix            uintptr
	TransmitLinkSpeed      uint64
	ReceiveLinkSpeed       uint64
	FirstWinsServerAddress uintptr
	FirstGatewayAddress    *ipAdapterGatewayAddress
	Ipv4Metric             uint32
	Ipv6Metric             uint32
	Luid                   uint64
	// 后续字段 MVP 不使用，但为保持 sizeof 正确必须完整声明，见实现
}
```

**结构体完整性规则**：即使 MVP 不使用 `Dhcpv6ClientDuid` 之后的字段，也必须完整声明以保证数组遍历时 `Next` 指针偏移正确。

### 6.3 核心难点二：非管理员 ICMP 探测

| 方案 | 是否需要管理员 | 评价 |
| --- | --- | --- |
| 原始套接字（`x/net/icmp` + `ip4:icmp`） | **需要** | ❌ 直接违反 REQ-F-705（普通用户必须完整可用） |
| `exec ping.exe` | 不需要 | ❌ 违反 C-06 精神（起进程 = EDR 可见行为），且输出解析脆弱 |
| **`IcmpSendEcho`** | **不需要** | ✅ 采用。Windows 官方 API，内核代发 ICMP，EDR 视角为普通网络探测 |

```go
// probe/icmp.go
func Send(dst net.IP, count int, timeout time.Duration) (Result, error) {
	h, err := winapi.IcmpCreateFile()
	if err != nil { return Result{}, err }
	defer winapi.IcmpCloseHandle(h)

	for i := 0; i < count; i++ {
		rtt, err := winapi.IcmpSendEcho(h, dst, payload, timeout)
		// 累计 Sent/Recv/RTT 序列
	}
	// 计算 LossPercent / Min / Avg / Max
}
```

- 单包超时 1s（基线 REQ-F-206），4 包，包间无额外间隔；
- 载荷固定 32 字节（内容为固定常量，**不含任何终端标识信息**，满足 C-03）；
- 若 `IcmpCreateFile` 本身失败 → 返回错误 → 计入 `Failures`，不误判为"网关不可达"（这是 R-18 存在的原因）。

### 6.4 DNS 采集的兜底与降级

```
主来源：GetAdaptersAddresses.FirstDnsServerAddress
   ↓ 为空
兜底：注册表 HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\{GUID}
        · NameServer（静态） 优先
        · DhcpNameServer（DHCP 下发） 次之
   ↓ 仍失败（键不可读）
标记 DNSSource = "未采集"
   ↓
判定引擎：DNSSource=="未采集" → 触发 R-19（诊断不完整），
          **不得**触发 R-03（DNS 未配置）  ← 对应基线场景 S-17
```

网卡 ↔ 注册表键的关联方式：`AdapterName`（形如 `{GUID}`）与注册表子键名一致，可直接匹配。

### 6.5 只读保证

`internal/winapi` 包内**不存在任何写入系统状态的 API 声明**（无 `RegSetValueEx`、无 `SetIpInterfaceEntry`、无 `CreateService`）。这将作为阶段 4 的合规验证项之一：全仓库 grep 写入类 API 名称应为 0 命中。

---

## 7. 判定引擎设计（`internal/detect`）

### 7.1 阈值常量集中管理（Q5 落地）

```go
// thresholds.go —— 全项目唯一数值来源，禁止在规则函数内出现字面量
package detect

const (
	// 网络
	APIPA_Prefix              = "169.254."
	LinkLocalV6Prefix         = "fe80:"
	ICMPPacketCount           = 4
	ICMPTimeout               = 1 * time.Second
	ICMPLossWarnPercent       = 20.0
	ICMPLatencyWarnMs         = 150
	TCPDialTimeout            = 3 * time.Second
	DNSQueryTimeout           = 3 * time.Second

	// 健康度
	MemWarnPercent            = 85.0
	MemSeverePercent          = 95.0
	CPUWarnPercent            = 85.0
	CPUSeverePercent          = 95.0
	CPUSampleWindow           = 1 * time.Second
	DiskWarnFreeBytes        = 10 * 1024 * 1024 * 1024 // 10 GiB
	DiskSevereFreeBytes      = 5 * 1024 * 1024 * 1024  // 5 GiB
)

// 无效 DNS 地址（R-04）
var InvalidDNSServers = []string{"0.0.0.0", "127.0.0.1"}

// TCP 443 候选目标（C1 决策）
var TCP443Targets = []string{"223.5.5.5:443", "223.6.6.6:443", "119.29.29.29:443"}

// DNS 探测域名（C2 决策）
const DNSProbeDomain = "www.baidu.com"
const DNSDirectResolver = "223.5.5.5:53"
```

> 注：基线写「10 GB」，实现按 GiB（1024³）计算，报告展示为 `GB`，单位口径在阶段 6 用户手册中说明。

### 7.2 规则函数签名与注册

```go
// Rule 是纯函数：输入快照，输出 0..n 条 Issue。无副作用、无 IO、无全局状态。
type Rule func(*model.Snapshot) []model.Issue

type ruleEntry struct {
	ID       string
	Severity model.Severity
	Category model.Category
	Title    string
	fn       Rule
}

var catalog []ruleEntry
func register(id string, sev model.Severity, cat model.Category, title string, fn Rule)
```

`catalog` 同时作为**测试与文档的数据源**：阶段 4 可写元测试断言「catalog 中每条规则 ID 都有对应的表驱动用例」，阶段 6 可用 `catalog` 自动生成《判定规则说明》文档，杜绝文档与代码不一致。

### 7.3 规则实现示例（体现可测性）

```go
// R-01：IPv4 地址以 169.254 开头
func r01APIPA(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range s.Adapters {
		if !a.IsActive() { continue }
		for _, addr := range a.IPv4 {
			if strings.HasPrefix(addr.IP, APIPA_Prefix) {
				out = append(out, newIssue("R-01", withEvidence(
					fmt.Sprintf("网卡: %s  地址: %s/%d", a.Name, addr.IP, addr.Prefix))))
				break // 同一网卡只报一次
			}
		}
	}
	return out
}
```

单元测试无需真机：构造 `model.Snapshot{Adapters: []model.Adapter{{...}}}` 即可。

### 7.4 故障层级判定（`probe/classify.go`，纯函数）

严格实现基线第 6.3 节矩阵：

```go
func Classify(probes []model.ProbeResult) []model.LayerConclusion
```

- 输入全部为已完成的 `ProbeResult`，**不发起任何网络请求** → 可 100% 单测覆盖 7 种矩阵行；
- 未执行的探测（`Skipped`）在矩阵中按「不可用」处理并给出 `undetermined` 结论。

### 7.5 排序与聚合

```go
func Evaluate(s *model.Snapshot) []model.Issue {
	var issues []model.Issue
	for _, e := range catalog { issues = append(issues, e.fn(s)...) }
	if len(s.Failures) > 0 { issues = append(issues, buildIncompleteIssue(s.Failures)) } // R-19
	sortIssues(issues) // 严重 → 警告；同级按 Category 固定序；同类别按 RuleID 升序
	return issues
}
```

**确定性保证（REQ-F-403、REQ-N-08）**：排序键为 `(Severity desc, CategoryOrder, RuleID asc)`，无随机源，同机器连续运行结论稳定。

---

## 8. 报告设计（`internal/report`）

### 8.1 文件与编码

| 项 | 值 |
| --- | --- |
| 文件名 | `diag_YYYYMMDD_HHMMSS.txt`；同名存在时追加 `_1`、`_2`…（REQ-F-502、S-15） |
| 编码 | UTF-8 **with BOM**（`\xEF\xBB\xBF`，REQ-F-504） |
| 换行 | CRLF（REQ-F-505） |
| 写入方式 | `os.OpenFile(O_CREATE|O_EXCL|O_WRONLY, 0644)`，`O_EXCL` 天然防止覆盖 |

### 8.2 三层结构模板

```
================================================================================
            Desktop-Diag 桌面运维一键诊断报告
================================================================================
 诊断时间    : 2026-09-30 01:23:45
 计算机名    : DESKTOP-XXXX
 运行账户    : DOMAIN\user       管理员权限: 否
 操作系统    : Windows 11 Pro for Workstations (10.0.26200) AMD64
 系统运行时长: 3 天 7 小时 12 分
 诊断工具版本: v0.1.0 (commit abc1234, built 2026-09-30T01:00:00Z)
 报告存放路径: C:\Users\user\Desktop-Diag\reports\diag_20260930_012345.txt
 路径选择说明: 未指定 -o；EXE 所在目录为只读共享，已降级至用户目录
 耗时        : 8.4 秒
================================================================================

【第一层】异常告警汇总
--------------------------------------------------------------------------------
 共 3 项：严重 1，警告 2

 [严重] 网络 - DHCP 地址获取失败，终端未获取有效内网 IP
        网卡: 以太网  地址: 169.254.13.7/16
        建议: 检查网线/无线连接与 DHCP 服务器可用性，确认网卡未被禁用

 [警告] 存储 - 系统盘空间不足，存在系统卡顿、更新失败风险
        系统盘 C:  剩余 8.2 GB / 共 476.9 GB (1.7%)
        建议: 清理临时文件与更新缓存，或将个人数据迁移至其他分区

 [警告] 诊断完整性 - 诊断不完整，1 项数据缺失，结论可能不完整
        缺失: 网络连通性探测（原因: 未检测到已连接的网络适配器）
--------------------------------------------------------------------------------

【第二层】结构化检测数据详情
--------------------------------------------------------------------------------
 ▼ 主机与系统
   计算机名   : DESKTOP-XXXX
   ...
 ▼ 网络适配器（共 4 块，已连接 1 块）
   [1] 以太网  (Realtek PCIe GbE Family Controller)
       状态     : 已连接       类型: Ethernet
       MAC      : AA-BB-CC-DD-EE-FF
       IPv4     : 169.254.13.7/16   掩码: 255.255.0.0
       网关     : 未配置
       DNS      : 未配置
       DHCP     : 已启用
   ...
 ▼ 系统健康度
   CPU 占用率 : 12.3 %
   内存       : 已用 9.1 GB / 共 15.9 GB (57.2%)，可用 6.8 GB
   系统盘 C:  : 剩余 8.2 GB / 共 476.9 GB
 ▼ 网络连通性
   网关探测(以太网 → 192.168.1.1) : 丢包 0%  延迟 平均 2 ms (min 1 / max 4)
   系统 DNS 解析 www.baidu.com    : 成功 → 110.242.68.66 (23 ms)
   直连 DNS 223.5.5.5 解析        : 成功 → 110.242.68.66 (18 ms)
   TCP 443 探测                   : 223.5.5.5:443 可达 (31 ms)
   故障层级判定                   : 内网正常 · 外网正常
--------------------------------------------------------------------------------

【第三层】原始数据附录
--------------------------------------------------------------------------------
 [来源] GetAdaptersAddresses
   IfIndex=12 FriendlyName="以太网" OperStatus=Up IfType=6 Mtu=1500
   PhysicalAddress=AA-BB-CC-DD-EE-FF Length=6
   Unicast: 169.254.13.7/16 PrefixOrigin=LinkLayer
   Gateway: (none)
   DNS: (none)

 [来源] HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\{...}
   EnableDHCP=1  DhcpIPAddress=169.254.13.7  NameServer=(absent)

 [来源] GetSystemTimes / GlobalMemoryStatusEx / GetDiskFreeSpaceExW
   ...

 [来源] IcmpSendEcho / GetAddrInfoW / net.DialTimeout
   ...

================================================================================
 报告结束 · 本工具仅执行只读诊断，未修改任何系统配置
================================================================================
```

### 8.3 四级降级写入链（Q2 落地）

```go
type candidate struct {
	Dir    string
	Label  string // "命令行 -o 指定" / "EXE 所在目录" / "用户目录" / "系统临时目录"
}

func ResolveTargets(explicit string) []candidate {
	// ① explicit（若给出）
	// ② exeDir  —— os.Executable() 所在目录
	// ③ %USERPROFILE%\Desktop-Diag\reports
	// ④ %TEMP%\Desktop-Diag\reports
}

// Write 依次尝试：确保目录存在 → O_CREATE|O_EXCL 打开 → 成功即写。
// 成功时返回实际路径与降级说明（供报告头与结论区使用）。
func Write(cands []candidate, snap *model.Snapshot, issues []model.Issue) (Outcome, error)
```

| 结果 | 行为 | 退出码 |
| --- | --- | --- |
| 第 1 候选成功 | 正常，无降级说明 | 由告警等级决定 |
| 第 2–4 候选成功 | 报告头输出 `路径选择说明: 已降级至 X（原因: Y）` | 由告警等级决定 |
| 全部失败 | 控制台打印每级失败原因；报告不生成 | **2**（REQ-F-507） |

**共享目录特殊处理（S-10）**：EXE 位于 UNC 路径（`\\server\share\…`）时，会先尝试该目录，失败则降级；同时在报告头注明「检测到 UNC 部署路径」，为运维批量收集提供线索。

---

## 9. 控制台呈现层（`internal/ui`）

### 9.1 编码策略（REQ-F-603）

```
启动时：
  if winapi.IsConsole(os.Stdout) {                  // GetConsoleMode 成功即为控制台
      winapi.SetConsoleOutputCP(65001)              // UTF-8 输出代码页
      winapi.SetConsoleCP(65001)                    // 输入代码页（无输入，一并设置保持一致）
      winapi.EnableVTProcessing(os.Stdout)          // 开启 ANSI 虚拟终端序列
  }
```

Go 源码统一以 UTF-8 编写，直接写 UTF-8 字节；在 CP=65001 控制台下中文正常显示。若 `SetConsoleOutputCP` 失败（极旧系统），降级为**纯 ASCII 输出模式**（符号与颜色全部关闭，仅保留 `[严重]/[警告]` 文本标记）。

### 9.2 颜色与符号降级矩阵

| 条件 | 颜色 | 符号 | 示例 |
| --- | --- | --- | --- |
| 控制台 + VT 开启 | ANSI 颜色 | ✅ ⚠ ❌ | `[严重]` 显示为红色 |
| 输出被重定向（非控制台） | 关闭 | 保留 Unicode | 无 ANSI 转义序列（REQ-F-602） |
| `SetConsoleOutputCP` 失败 | 关闭 | ASCII（`[OK] [!] [X]`） | 兼容极旧环境 |

### 9.3 进度输出格式（REQ-F-601、C8）

```
Desktop-Diag v0.1.0  桌面运维一键诊断工具（只读模式，不修改任何系统配置）

[1/4] 主机与系统信息 ...... 完成
[2/4] 网络适配器信息 ...... 完成（4 块，已连接 1 块）
[3/4] 系统健康度 .......... 完成
[4/4] 网络连通性探测 ...... 完成

────────────────────────────────────────
 发现 3 项异常：严重 1，警告 2
 报告已生成：C:\Users\user\Desktop-Diag\reports\diag_20260930_012345.txt
 总耗时：8.4 秒
────────────────────────────────────────
```

---

## 10. CLI 与退出码（基线第 9 节落地）

```go
type Options struct {
	OutputDir  string // -o
	Verbose    bool   // -v
	ShowVer    bool   // -version
	ShowHelp   bool   // -h / --help
}
func Parse(args []string) (Options, error)
```

| 退出码 | 常量 | 触发条件 |
| --- | --- | --- |
| 0 | `ExitOK` | 诊断完成，无严重告警 |
| 1 | `ExitFindings` | 诊断完成，存在严重告警 |
| 2 | `ExitError` | 程序自身错误（参数非法、报告无法生成等） |

`main.go` 骨架：

```go
func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))
}
```

> 说明：PRD 未要求 `-version`，但基线第 9 节已列入 CLI 契约，且它是阶段 7 版本验证的低成本手段，故保留。

---

## 11. 错误处理与降级总纲（E6 落地）

| 层级 | 失败类型 | 处理策略 | 是否影响退出码 |
| --- | --- | --- | --- |
| 参数解析 | 非法参数/目录不存在 | 打印用法 + 错误说明 | 2 |
| 采集器 | 单项失败 | 捕获 → `Snapshot.Failures` → R-19；继续下一项 | 否（除非全部失败） |
| 采集器 | **全部**采集失败 | 仍生成报告（仅含完整性告警） | 否 |
| 探测 | 无活动网卡 | 全部 `Skipped` + 原因；不产生误导性严重告警 | 否 |
| 探测 | `IcmpCreateFile` 失败 | 记入 `Failures`；**不**触发 R-14（网关不可达），改触发 R-19 | 否 |
| 判定 | 规则内部 panic | `defer recover()` 包裹每条规则，转为该规则的 `Failures` 条目 | 否 |
| 报告 | 写入降级 | 走四级链 | 否 |
| 报告 | 全部失败 | 控制台打印各级原因 | 2 |
| 控制台 | 编码设置失败 | 降级为 ASCII 模式 | 否 |

**总原则**：除「参数非法」与「报告完全无法生成」外，任何异常都不得中断诊断流程或返回 2。

---

## 12. 可测试性设计（REQ-N-10）

### 12.1 纯函数边界清单

| 包 | 可纯测的单元 | 测试方式 |
| --- | --- | --- |
| `detect` | 19 条规则 + 排序 + R-19 聚合 | 表驱动，构造 Snapshot |
| `probe/classify` | 7 行判定矩阵 + 边界 | 表驱动 |
| `collect` | `BuildAdapters(raw)`、掩码转换、前缀↔掩码互转、虚拟网卡识别、注册表值解析 | 表驱动，喂入固定 raw 结构 |
| `report` | 三层渲染、降级链选择逻辑、文件名生成与去重 | golden 文件 + 临时目录 |
| `ui` | 颜色降级判定、符号降级判定 | 表驱动（注入 io.Writer / 假 TTY 标志） |
| `cli` | 参数解析与错误 | 表驱动 |
| `model` | `IsActive` / `ActivePhysicalAdapters` / `IPv6OnlyLinkLocal` 等派生方法 | 表驱动 |

**关键设计**：所有 syscall 结果先转为**中间 raw 结构**（如 `winapi.RawAdapter`），再由 `collect` 的纯函数转换为 `model.Adapter`。这样结构体映射的正确性由集成测试保证，业务逻辑的正确性由单元测试保证，二者解耦。

### 12.2 覆盖率目标

- `detect` ≥ 90%（核心价值，最高优先级）
- `probe/classify` = 100%（矩阵有限且明确）
- `collect`（纯函数部分）≥ 80%
- `report` ≥ 75%
- 整体 ≥ 70%（REQ-N-10）

### 12.3 无法单测的部分及替代验证

| 部分 | 替代验证（阶段 4） |
| --- | --- |
| `winapi` 结构体映射 | 与 `ipconfig /all` 逐字段人工比对 |
| `IcmpSendEcho` | 真机 ping 网关比对丢包/延迟 |
| 控制台编码 | GBK 代码页下实际运行目视检查 |
| 报告 BOM/CRLF | 十六进制查看前 3 字节与行尾 |

---

## 13. 构建与依赖策略（阶段 5 前置）

### 13.1 `go.mod`

```
module github.com/yukin371/desktop-diag

go 1.26.0

require golang.org/x/sys v0.48.0   // 唯一第三方依赖
```

> ⚠ **工具链说明（实测确定，非选择）**：`golang.org/x/sys v0.48.0` 的 `go.mod` 声明 `go >= 1.26.0`。
> 本机初始工具链为 go1.25.1，因 `GOTOOLCHAIN=auto` 触发自动升级至 go1.26.x，`go.mod` 的 go 指令同步被改写为 `go 1.26.0`。
> 阶段 5 的构建脚本必须固定 `GOTOOLCHAIN` 行为（显式声明所需版本），否则离线/内网构建环境会因无法下载工具链而失败。
> 若不希望引入工具链升级要求，可将 `x/sys` 降到 v0.36.0 一类的低版本（该版本要求 go >= 1.24）——MVP 只用 `windows.Token.IsElevated` 与 `windows/registry`，
> 两者在 v0.36.0 已存在。**决策：保留 v0.48.0 + go 1.26.0**（阶段 3 实测通过），阶段 5 在构建脚本中显式声明。

### 13.2 构建参数

```powershell
$env:CGO_ENABLED = "0"      # 保证纯静态、无 MSVC 运行库依赖
$modulePath = "github.com/yukin371/desktop-diag"
go build -trimpath `
  -ldflags "-s -w -X '$modulePath/internal/version.Version=$Version' -X '$modulePath/internal/version.Commit=$Commit' -X '$modulePath/internal/version.BuildTime=$BuildTime'" `
  -o dist/Desktop-Diag.exe ./cmd/desktop-diag
```

- `-s -w`：剥离符号表与 DWARF，压缩体积（REQ-N-04 ≤ 15 MB，预期实际 6–9 MB）
- `-trimpath`：去除本地路径，避免泄露开发者目录结构，也是构建可复现性的要求
- ⚠ **`-X` 的目标必须是包内变量的完整导入路径**（`<module>/internal/version.Version`），
  不是早期草案写的 `main.version`。T-01 实测：初版写成 `main.version` 时注入静默失效，
  产物 `-version` 只打印默认值 `dev`。**判定注入是否真的生效，必须看 `BuildTime`**
  （默认值 `unknown` → 注入后为真实时间戳），因为 `Version`/`Commit` 的默认值可能与期望值混淆。

> **PowerShell 版本说明（实测确定）**：本机**只有 Windows PowerShell 5.1**，没有 `pwsh`（PowerShell 7）。
> 所有脚本因此必须用 `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\xxx.ps1` 调用，
> 且**脚本文件必须保存为 UTF-8 with BOM** —— PS 5.1 读取无 BOM 的 `.ps1` 时按 ANSI 解析，
> 含中文的脚本文案会变成乱码甚至破坏引号配对。脚本同时保持 PS 7 兼容。
> 另：`$ErrorActionPreference="Stop"` 下 PS 5.1 会把原生命令（如 git）的 stderr
> 升级为**终止错误**，调用 git 必须临时放宽并显式检查 `$LASTEXITCODE`。

### 13.3 质量门禁

| 门禁 | 命令 | 阶段 |
| --- | --- | --- |
| 格式 | `gofmt -l .` 输出为空 | 3、5 |
| 静态检查 | `go vet ./...` 零问题（REQ-N-11） | 3、5 |
| 测试 | `go test ./... -cover`，整体 ≥ 70% | 4、5 |
| 交叉编译 | `GOOS=windows GOARCH=amd64` + （可选）`arm64` | 5、7 |
| 合规扫描 | 全仓 grep 写入类 API 名称，期望 0 命中（见 6.5） | 4、5 |

---

## 14. 风险清单与对策

| ID | 风险 | 影响 | 概率 | 对策 |
| --- | --- | --- | --- | --- |
| RK-01 | `IP_ADAPTER_ADDRESSES_LH` 结构体布局错位 | 网卡数据乱码/崩溃 | 中 | 完整声明字段；`Length` 自检；双次调用；与 `ipconfig /all` 逐字段人工比对（阶段 4 强制验收项） |
| RK-02 | `IcmpSendEcho` 在企业环境被安全软件拦截 | 无法判定内网链路 | 低 | 失败记入 `Failures` 触发 R-19，**不**误报 R-14；R-18 处理"网关不通但外网可达" |
| RK-03 | CP 65001 在个别终端下中文仍异常 | 可读性 | 低 | 三级降级至纯 ASCII 模式 |
| RK-04 | 注册表 `Tcpip\Interfaces` 键不可读 | DNS 为空导致误报 R-03 | 中 | `DNSSource="未采集"` 时不触发 R-03，改触发 R-19（S-17） |
| RK-05 | 磁盘阈值单位口径（GB vs GiB）争议 | 验收争议 | 低 | 统一按 GiB 计算、显示为 GB，阶段 6 文档显式说明 |
| RK-06 | TCP 443 候选目标在国内网络环境失效 | 误报外网中断 | 中 | 候选列表 3 个 IP + 末位回退 `www.baidu.com:443`（仅当 DNS 正常时）；全部失败且网关正常才判 R-17 |
| RK-07 | 多网卡环境下 ICMP 探测无法指定出接口 | 结论归属模糊 | 中 | `IcmpSendEcho` 按目标 IP 由系统路由决定出口；报告中按目标网关归属网卡输出，并注明"实际出口由系统路由表决定" |
| RK-08 | arm64 构建下结构体对齐差异 | 可选产物失败 | 低 | arm64 列为可选目标；主目标 amd64 必须通过 |

---

## 15. 关键假设实测验证记录（验证程序 `tmpverify/main.go`）

设计文档定稿后，用一个一次性程序对本方案的 **13 项关键假设**做了真机实测（`CGO_ENABLED=0`，
Windows 11 Pro for Workstations Build 26200 / AMD64）。**13/13 全部通过，exit code 0**。

| # | 假设 | 实测结果 |
| --- | --- | --- |
| 1 | `golang.org/x/sys/windows` 提供提权判定 | ✅ `windows.Token.IsElevated` 存在 |
| 2 | `kernel32!GetTickCount64` 可调用（运行时长来源，替代 WMI `LastBootUpTime`） | ✅ 返回 108530062 ms（30.15 h） |
| 3 | `GlobalMemoryStatusEx` + 手写 `MEMORYSTATUSEX` 可得内存 | ✅ 修正布局后 load=49% total=31.4GiB avail=15.8GiB（**修正过程见下**） |
| 4 | `GetDiskFreeSpaceExW` 可得系统盘容量 | ✅ `C:` free=517.2GiB total=930.3GiB |
| 5 | `ntdll!RtlGetVersion` 可拿到真实 OS 版本（规避 `GetVersionEx` 谎报） | ✅ 10.0.26200 |
| 6 | 控制台代码页可设为 UTF-8 | ✅ 本机已是 65001；`SetConsoleOutputCP(65001)` 成功 |
| 7 | `iphlpapi!GetAdaptersAddresses` 符号可解析 | ✅ 可解析，**且确认 x/sys 无封装 → 必须手写结构体**（6.2 结论成立） |
| 8 | `IcmpCreateFile`/`IcmpCloseHandle` 可用（非管理员 ICMP 方案） | ✅ 句柄获取与关闭均成功 |
| 9 | 注册表 `Tcpip\Parameters\Interfaces` 可枚举（DNS 兜底链） | ✅ 13 个接口子键，其中 3 个含 DNS 配置 |
| 10 | `net.LookupHost` 在 `CGO_ENABLED=0` 下可用（系统 DNS 探测） | ✅ 解析成功，返回 IPv4+IPv6 |
| 11 | 自定义 `net.Resolver{PreferGo:true}` 可直连指定 DNS（绕过系统 DNS 的对照探测） | ✅ 经 `udp 223.5.5.5:53` 解析成功 |
| 12 | TCP 443 候选目标在国内网络可达（R-17 判定依赖） | ✅ `223.5.5.5:443` 可达（34–36 ms），**首选目标实测有效** |
| 13 | 工具链与依赖可在本机拉取 | ✅ `golang.org/x/sys v0.48.0`（引发 go 指令升至 1.26.0，见 13.1） |

### 15.1 ⚠ 实测抓出的真实缺陷：`MEMORYSTATUSEX` 结构体布局

第一版按"直觉对齐"写了 padding，**实测直接失败**：

```
[FAIL] kernel32!GlobalMemoryStatusEx      The parameter is incorrect.
```

- 错误布局：`Length(4) + _pad(4) + MemoryLoad(4) + _pad(4) + 6×uint64`
- 后果：`sizeof` 比真实结构体**偏大 8 字节**，`dwLength` 校验不通过，API 返回
  `ERROR_INVALID_PARAMETER (87)`，内存数据完全拿不到。
- 正确布局（**无任何填充**）：

```go
// dwLength(4) + dwMemoryLoad(4) + 6×DWORDLONG(8)
type memoryStatusEx struct {
    Length               uint32
    MemoryLoad           uint32
    TotalPhys            uint64
    AvailPhys            uint64
    TotalPageFile        uint64
    AvailPageFile        uint64
    TotalVirtual         uint64
    AvailVirtual         uint64
    AvailExtendedVirtual uint64
}
```

### 15.2 由此确立的阶段 3 强制纪律

**教训**：Win32 结构体的"看起来需要对齐填充"的直觉是**错的**——`DWORD` 后接 `DWORDLONG` 时，
自然对齐正好落在 8 字节边界，任何凭直觉添加的填充都会破坏 `dwLength`/`Length` 校验。

由此把 6.2 的验证手段升级为**阶段 3 的强制纪律**：

> `internal/winapi/iphlpapi_types.go` 中**每一个**手写结构体（`IP_ADAPTER_ADDRESSES_LH`、
> `IP_ADAPTER_UNICAST_ADDRESS_LH`、`IP_ADAPTER_DNS_SERVER_ADDRESS_XP`、
> `IP_ADAPTER_GATEWAY_ADDRESS_LH`、`ICMP_ECHO_REPLY`），
> 都必须先用同一手法（临时程序 + `unsafe.Sizeof` 打印 + 真机调用校验）验证偏移与
> `sizeof`，**验证通过后才能写进正式包**。

`IP_ADAPTER_ADDRESSES_LH` 是本项目最大的结构体（RK-01），其 `Next` 指针偏移只要错 8 字节
就会导致整条链表遍历崩溃或网卡数据乱码——这正是"先验证再落地"纪律的价值所在。

### 15.3 清理

验证程序与产物（`tmpverify/`）在阶段 2 开始前删除，不进入正式交付物。

---

## 16. 阶段 1 出口检查单

- [x] 架构分层与依赖方向明确（第 2 节，无环）
- [x] 仓库与包结构定稿（第 3 节，含模块路径决策）
- [x] 核心数据结构定稿（第 4 节，6 组结构体）
- [x] 采集器接口与**双注册扩展机制**定稿（第 5 节，满足 REQ-N-09）
- [x] Win32 API 映射表 + 两个核心难点方案（第 6 节：`GetAdaptersAddresses` 映射、非管理员 `IcmpSendEcho`）
- [x] 判定引擎与阈值集中管理方案（第 7 节，`thresholds.go` 为唯一数值源）
- [x] 报告三层模板 + 四级降级写入链（第 8 节）
- [x] 控制台编码与三级降级策略（第 9 节）
- [x] CLI 契约与退出码（第 10 节）
- [x] 错误处理总纲（第 11 节）
- [x] 可测试性设计与覆盖率目标（第 12 节）
- [x] 构建参数与质量门禁（第 13 节）
- [x] 风险清单与对策（第 14 节，8 项）
- [x] **关键假设真机实测验证（第 15 节，13/13 通过，抓出并修正 1 个结构体缺陷）**
- [x] **验证纪律写入设计（15.2：所有手写 Win32 结构体必须先验证再落地）**

**交付物**：`doc/phase1/00-technical-design.md`（本文件）

**下一步**：阶段 2 —— 任务拆解（WBS + 里程碑 + 依赖排序 + 可执行 Todo）。
