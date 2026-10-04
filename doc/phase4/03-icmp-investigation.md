# ICMP 拒绝原因调查

日期：2026-10-02（Asia/Shanghai）。证据类型：真机、只读 ACL/令牌查询、独立 C/Go 对照。源码为 `bdcb32e`，生产代码未修改；新增的探针只在开发时运行。

## 已确认的原因链

本机工作区 `J:\Github\Desktop-Diag` 有 `Mandatory Label\Low Mandatory Level:(OI)(CI)(NW)`，标签继承到候选 EXE 和新编译的探针。从该目录启动的 C 探针令牌为 Low IL（RID `4096`），相同字节副本从用户临时目录启动为 Medium IL（RID `8192`）。两者均为同一用户 SID，`elevated=0`、`restricted=0`，只有完整性等级出现明确差异。

Windows 的强制完整性机制会按调用方与 EXE 完整性标签限制新进程等级；因此“同一账户、未提升”并不能证明两次运行权限相同。微软机制说明：[Mandatory Integrity Control](https://learn.microsoft.com/en-us/windows/win32/secauthz/mandatory-integrity-control)。

本机 ICMP 被拒绝与 Low IL 的执行上下文对应。不能据此推断所有 Windows/安全软件均有相同限制，也未取得具体内核过滤器 ID。无需把它解释为普通用户必然不能使用 ICMP，或者把提权作为产品前提。

## 对照实验

| 项目 | 工作区路径 | 用户临时目录副本 |
| --- | --- | --- |
| 用户 SID | 同一用户，末尾 `1001` | 同一用户，末尾 `1001` |
| TokenElevation / IsTokenRestricted | `0 / 0` | `0 / 0` |
| TokenIntegrityLevel | `4096`（Low） | `8192`（Medium） |
| C：回环与网关，生产/ping 载荷、默认/显式 TTL、Echo/Echo2 | 8/8 返回 count=0、GetLastError=5 | 8/8 返回 count=1、error=0、IP_SUCCESS |
| Go：调用生产包装，回环与网关、两种载荷 | 4/4 返回 Access denied | 4/4 成功 |
| 正式候选 EXE | R-19，网关 ICMP 跳过，DNS/TCP 成功 | 无告警，网关/DNS/TCP 全部有成功证据，退出 0 |

C/Go 探针副本分别核对 SHA256 相同，正式 EXE 副本 SHA256 仍为 `016C00D52E3EEE5F89D89A35A3D3814E5AD555049F8E623F59EC77D54AC28677`。测试目标为 `127.0.0.1` 与 `192.168.31.1`，ICMP 等待上限 1 秒；进程工作目录和报告输出均仍在 J 盘，排除了报告路径不可写作为该 ICMP 差异的解释。返回 J 盘的原探针复测仍被拒绝。

带令牌查询的 C 探针 SHA256：`405CC37600335063443AFA41365D53503D5E359128D3E67D6DED8192847F7CF7`。

证据目录：`J:\Github\Desktop-Diag\e2eout\phase4-20261002\icmp-investigation\`。保留 `native-results.txt`、`native-recheck.txt`、`go-results.txt`、`native-temp-results.txt`、`go-temp-results.txt`、`context-j.txt`、`context-c.txt`、`workspace-acl.txt`、`product-acl.txt`、`product-temp-acl.txt` 和正式 EXE 的 Medium IL 诊断报告 `product-temp\diag_20261002_155152.txt`。临时副本位于 `C:\Users\yukin\AppData\Local\Temp\desktop-diag-icmp-bec244d4b83f4254aed75b3f8881e754\`，只用于本次证据对照，非部署方案。

## 排除与仍未确认项

- 独立 SDK C 程序复现同样错误；替换为 ping 载荷、显式 TTL 或 Echo2 不解决 Low IL 拒绝，排查不指向 Go 包装或载荷。
- J 盘为本地固定 NTFS 卷，不是 UNC/映射共享；候选 EXE 无 `Zone.Identifier`。
- Windows 防火墙 ActiveStore 三个配置文件默认出站 Allow；显式出站 Block 规则针对沙箱账户 SID 末尾 `1005`，与探针的 `1001` 不同。
- 本机运行 Kaspersky，但加载模块中未见其用户态 DLL，未取得其拦截报告；这些信息不能证明 Kaspersky 是本次拒绝者，也不能排除其内核策略参与。
- 未发现对应的 Security 5152/5157、CodeIntegrity 或 Defender 事件；防火墙未启用丢弃日志。WFP 枚举、审核策略查询和 Kaspersky 报告数据库受权限限制，本轮未提升或改变日志策略，因此具体过滤器/驱动尚未确认。

## 验收与产品处理

本次确认了非管理员 Medium IL 场景可以完整运行，新增环境维度应记录进程完整性等级；未提升管理员账户不等同独立标准用户账户。原 Low IL 失败记录保留，不能用成功副本覆盖历史证据。

发布/真机验收应检查交付 EXE 的完整性标签并在正常 Medium IL 环境取证。不得把开发工作区 Low IL 现象推广到所有用户，也不得在产品中自动修改 ACL、迁移自身、关闭安全软件或提升权限。Low IL/企业限制时继续其他采集，明确区分调用被拒绝与网络丢包；当前 R-19 降级仍有效，统一建议管理员重跑的文案需要后续调整。

生产源码未修改。首次普通全仓单测/gofmt/vet/覆盖率通过；包含取证 Go 包时整体 88.4%（工具包为 0%，生产覆盖率基线 88.9%）。随后工具加 `phase4diag` 标签以隔离产品统计，最终 `test.ps1 -CoverageFile dist/phase4-icmp-final-coverage.out` 退出 0，整体恢复 88.9%。竞态原始运行中除 detect 测试 EXE 被系统临时目录拒绝执行外，其余包通过；单独复跑 detect 仍为执行拒绝。该错误与已确认的 ICMP Low IL 原因分开记录。
