# ICMP 拒绝访问只读取证工具

本目录仅用于阶段四开发取证，不接入 Desktop-Diag 运行链。探测目标限于回环和显式提供的 IPv4 网关；不修改账户、防火墙、文件完整性标签或系统设置。

Go 探针调用生产 `winapi` 包；C 探针独立使用 Windows SDK，比较 `IcmpSendEcho`/`IcmpSendEcho2`、默认/显式 TTL 和生产/ping 载荷。C 探针还输出进程 SID、提升标志、完整性 RID 和加载模块。API 返回数为 0 时，缓冲区中的 status/RTT 初始值不能当作成功结果。

在仓库根目录构建，逐步检查原始退出码：

```powershell
New-Item -ItemType Directory -Force e2eout/phase4-20261002/icmp-investigation
go build -tags phase4diag -trimpath -o e2eout/phase4-20261002/icmp-investigation/icmp-go.exe ./tools/phase4/icmp-diag
gcc -O2 -Wall -Wextra tools/phase4/icmp-diag/native/main.c -o e2eout/phase4-20261002/icmp-investigation/icmp-context.exe -liphlpapi -lws2_32 -ladvapi32
& e2eout/phase4-20261002/icmp-investigation/icmp-go.exe 192.168.31.1
& e2eout/phase4-20261002/icmp-investigation/icmp-context.exe 192.168.31.1
icacls e2eout/phase4-20261002/icmp-investigation/icmp-context.exe
```

使用实际网关替换示例地址。Go 工具使用 `phase4diag` 构建标签，避免取证程序混入产品的包覆盖率。

2026-10-02 的结果与哈希见 [ICMP 原因调查](../../../doc/phase4/03-icmp-investigation.md)。相同字节的工具在工作区运行时为 Low IL，用户临时目录副本为 Medium IL；本机前者被拒绝，后者成功。复制仅用于对照取证，产品不得自行迁移、提权或更改安全标签。
