# Desktop-Diag

Desktop-Diag 是 Windows 桌面诊断工具。它采集主机、网卡、DNS、网关、TCP、ICMP、CPU、内存和系统盘信息，按固定规则生成中文诊断报告。

程序只做诊断和报告输出。运行时不修改系统配置、不修复问题、不驻留、不启动外部命令，也不上传诊断数据。

## 使用

运行环境：Windows x64。交付物是单个 `Desktop-Diag.exe`，依赖 Windows 自带系统 DLL，不需要 Go、PowerShell 或额外运行库。

```powershell
Desktop-Diag.exe
Desktop-Diag.exe -o C:\Users\Public\Desktop-Diag
Desktop-Diag.exe -v
Desktop-Diag.exe -version
Desktop-Diag.exe -h
```

`-o` 指定报告目录。目录不可写时，程序按报告写入降级链选择可用目录，并在报告中记录实际路径和原因。默认报告文件名为 `diag_YYYYMMDD_HHMMSS.txt`，同一秒内重复运行会追加序号，不覆盖旧报告。

`-v` 输出采集来源、原始字段和分项耗时。`-version` 只打印构建版本、提交号、构建时间和目标平台，不执行诊断。

退出码：

| 代码 | 含义 |
| --- | --- |
| `0` | 诊断完成，没有严重告警 |
| `1` | 诊断完成，存在严重告警 |
| `2` | 程序错误，例如参数非法或报告无法生成 |

## 常见问题

### 需要管理员权限吗？

普通 Medium IL 用户进程可以运行完整流程。某些企业安全策略或 Low IL 启动环境可能拒绝 ICMP，报告会保留拒绝原因并继续 DNS、TCP 和本地采集。程序不会自动提权。

### ICMP 失败是否表示网络断了？

不一定。ICMP 可能被完整性级别、防火墙或安全软件拦截。应结合报告中的 DNS、TCP 和其他证据判断；报告会把权限拒绝与无响应分开记录。

### 报告写在哪里？

优先使用 `-o` 指定的目录，然后尝试 EXE 所在目录、用户目录和临时目录。最终路径与降级原因写在报告头中。

### 能否在 Linux、macOS 或 arm64 真机运行？

生产程序依赖 Windows API，当前交付目标是 Windows x64。GitHub Actions 已验证 arm64 交叉编译，arm64 真机仍未完成验收。

## 报告

报告是 UTF-8 BOM、CRLF 换行的纯文本文件，包含三层内容：

1. 异常告警汇总；
2. 主机、网卡、健康度和网络连通性的结构化详情；
3. API 来源、探测参数和原始结果附录。

单项采集失败会保留原因并继续其他采集。ICMP 被权限、完整性级别或安全策略拒绝时，报告区分“调用被拒绝”和“网络无响应”，同时保留 DNS/TCP 等其他证据。

## 支持范围和限制

当前主要验收环境是 Windows 11 x64、Go 1.26.0 构建的 amd64 EXE。GitHub Actions 已验证 Windows runner 上的 amd64/arm64 交叉编译；arm64 真机、其他 Windows/Server 版本、独立标准用户、UNC、企业代理和隔离故障场景仍需单独验收。

ICMP 网关探测只支持 IPv4。目标网关属于接口身份，但实际出口由系统路由表决定。DNS 和 TCP 是全局探测，结果不能证明每块网卡都具备独立的公网连通性。

开发工作区带有 Low Mandatory Level 标签时，进程可能因 Low IL 被系统拒绝调用 ICMP。相同 EXE 在普通 Medium IL 环境运行无需管理员权限即可成功。产品不会自动提权、迁移自身或修改安全设置。

## 开发

需要 Go 1.26.0 和 Windows PowerShell 5.1。脚本只修改当前子进程的环境变量。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/quality-gate.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -Version dev
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/release.ps1 -Version v0.1.0
```

工程结构和数据流见 [架构说明](doc/architecture.md)，阶段记录见 [doc/](doc/)。

## 许可证

见 [LICENSE](LICENSE)。变更记录见 [CHANGELOG.md](CHANGELOG.md)。
