# Desktop-Diag

Desktop-Diag 是 Windows 桌面诊断工具。它采集主机、网卡、DNS、网关、TCP、ICMP、CPU、内存和系统盘信息，按固定规则生成中文诊断报告。

程序只做诊断和报告输出。运行时不修改系统配置、不修复问题、不驻留；仅在双击模式或 `-open` 时通过 Windows 文件关联打开本次报告，不启动命令解释器，也不上传诊断数据。

## 使用

双击 EXE 时，独立控制台会先显示引导：按 Enter 开始完整诊断，输入 Q 后按 Enter 退出。完成后自动打开本次报告，并等待 Enter 关闭窗口。已有终端/重定向运行不等待输入、不默认打开查看器；使用 `-open` 主动打开，`-no-open` 禁用自动打开。打开失败不影响报告保存，控制台显示手动路径。当前仅支持完整诊断，单项排查尚未实现。

运行环境：Windows x64。交付物是单个 `Desktop-Diag.exe`，依赖 Windows 自带系统 DLL，不需要 Go、PowerShell 或额外运行库。

```powershell
Desktop-Diag.exe
Desktop-Diag.exe -o C:\Users\Public\Desktop-Diag
Desktop-Diag.exe -format txt
Desktop-Diag.exe -v
Desktop-Diag.exe -version
Desktop-Diag.exe -h
```

`-o` 指定报告目录。目录不可写时，程序按报告写入降级链选择可用目录，并在报告中记录实际路径和原因。默认生成可离线阅读的单文件 HTML，文件名为 `diag_YYYYMMDD_HHMMSS.html`；使用 `-format txt` 可导出原有纯文本报告，同一秒内重复运行会追加序号，不覆盖旧报告。

`-v` 输出采集来源、原始字段和分项耗时。`-version` 只打印构建版本、提交号、构建时间和目标平台，不执行诊断。

退出码：

| 代码 | 含义 |
| --- | --- |
| `0` | 诊断完成，没有严重告警 |
| `1` | 诊断完成，存在严重告警 |
| `2` | 程序错误，例如参数非法或报告无法生成 |

## 常见问题

### 需要管理员权限吗？

普通 Medium IL 用户进程可以运行完整流程。某些企业安全策略或 Low IL 启动环境可能拒绝 ICMP，报告会保留拒绝原因和当前进程完整性等级，明确标为探测受限，并继续 DNS、TCP 和本地采集。程序不会自动提权。

### ICMP 失败是否表示网络断了？

不一定。ICMP 可能被完整性级别、防火墙或安全软件拦截。调用被拒绝时，没有有效发包或丢包证据，不应当作网关故障；请核对报告中的进程完整性等级。低完整性环境可能限制调用，等级本身不能确定具体拦截者。应结合报告中的 DNS、TCP 和其他证据判断；报告会把权限拒绝与无响应分开记录。

### 报告写在哪里？

优先使用 `-o` 指定的目录，然后尝试 EXE 所在目录、用户目录和临时目录。最终路径与降级原因写在报告头中。

### 能否在 Linux、macOS 或 arm64 真机运行？

生产程序依赖 Windows API，当前交付目标是 Windows x64。GitHub Actions 已验证 arm64 交叉编译，arm64 真机仍未完成验收。

## 报告

报告默认是自包含 HTML，双击可用浏览器阅读。开头提供各领域诊断总览（结论、严重/警告数、缺失数据），点击具体问题可跳到展开的证据与建议，并返回总览；同时提供分节导航、可折叠证据和原始数据，适配窄屏和浏览器打印；无需网络、脚本或外部资源。采集内容经过 HTML 转义。双击模式完成后自动打开本次报告，命令行可用 `-open`；`-no-open` 禁用。

`-format txt` 保留 UTF-8 BOM、CRLF 换行的纯文本输出。两种格式均包含三层内容：

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

`v0.1.0` 发布候选、校验值、支持矩阵和正式发布进度见 [阶段七发布记录](doc/phase7/README.md)。

## 许可证

见 [LICENSE](LICENSE)。变更记录见 [CHANGELOG.md](CHANGELOG.md)。


## SmartScreen 与发布签名

当前候选未签名，SmartScreen“无法识别应用”属于发布者/下载信誉提示，不等于 Defender 已判定病毒。获取可信代码签名证书后，可使用 `release.ps1 -CertificateThumbprint <证书指纹> -TimestampServer <HTTPS RFC3161 服务> -RequireSignature`；需要 Windows SDK signtool 和当前用户证书库中的私钥，工具不保存证书密码。签名在 SHA256 生成前完成，元数据记录签名状态。没有可信证书时不要把候选宣称为签名正式版；签名也不保证新文件立即获得信誉。若出现具体恶意软件检出，应记录检测名称/哈希并由发布者向 Microsoft 提交误报复核，不自动上传诊断数据、不关闭保护、不删除下载安全标记。
