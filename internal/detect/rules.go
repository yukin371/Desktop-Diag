// 本文件按完整采集证据执行诊断规则，告警措辞限定目标、协议与采样窗口。
package detect

import (
	"fmt"
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// 本文件按基线 6.2 节的规则表实现 R-01 … R-19。
// register 里的 Condition 必须与实际实现一致：阶段 6 直接把它渲染成规则文档。

// init registers built-in rules in deterministic evaluation order.
func init() {
	register(ruleEntry{
		ID:        "R-01",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "DHCP 地址获取失败，终端未获取有效内网 IP",
		Condition: "任一活动物理网卡的 IPv4 地址以 " + APIPAPrefix + " 开头",
		Fn:        ruleAPIPA,
	})
	register(ruleEntry{
		ID:        "R-02",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "无网关配置，内网出口异常，无法正常联网",
		Condition: "任一活动物理网卡未配置默认网关",
		Fn:        ruleNoGateway,
	})
	register(ruleEntry{
		ID:        "R-03",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "DNS 未配置，局域网域名与网页均无法正常解析",
		Condition: "任一活动物理网卡的 DNS 列表为空，且该网卡的 DNS 来源不是「未采集」",
		Fn:        ruleNoDNS,
	})
	register(ruleEntry{
		ID:        "R-04",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "DNS 配置异常（指向无效地址），域名解析将失败",
		Condition: "任一活动物理网卡的 DNS 列表非空但全部由 " + strings.Join(InvalidDNSServers, " / ") + " 组成",
		Fn:        ruleInvalidDNS,
	})
	register(ruleEntry{
		ID:        "R-05",
		Severity:  model.SevWarning,
		Category:  model.CatNetwork,
		Title:     "网络未获得有效 IP（仅链路本地地址），无法对外通信",
		Condition: "全部活动物理网卡都没有有效 IPv4，且至少存在一个 " + LinkLocalV6Prefix + " 的 IPv6 地址",
		Fn:        ruleIPv6OnlyLinkLocal,
	})

	register(ruleEntry{
		ID:        "R-06",
		Severity:  model.SevSevere,
		Category:  model.CatSystem,
		Title:     "系统内存严重过载，存在程序闪退与系统卡死风险",
		Condition: fmt.Sprintf("内存占用率 ≥ %.0f%%", MemSeverePercent),
		Fn:        ruleMemSevere,
	})
	register(ruleEntry{
		ID:        "R-07",
		Severity:  model.SevWarning,
		Category:  model.CatSystem,
		Title:     "系统内存占用偏高，终端运行可能卡顿",
		Condition: fmt.Sprintf("内存占用率 ≥ %.0f%% 且 < %.0f%%", MemWarnPercent, MemSeverePercent),
		Fn:        ruleMemWarn,
	})
	register(ruleEntry{
		ID:        "R-08",
		Severity:  model.SevSevere,
		Category:  model.CatSystem,
		Title:     "CPU 采样窗口内占用严重偏高",
		Condition: fmt.Sprintf("CPU 占用率 ≥ %.0f%%", CPUSeverePercent),
		Fn:        ruleCPUSevere,
	})
	register(ruleEntry{
		ID:        "R-09",
		Severity:  model.SevWarning,
		Category:  model.CatSystem,
		Title:     "CPU 占用率偏高，存在系统卡顿风险",
		Condition: fmt.Sprintf("CPU 占用率 ≥ %.0f%% 且 < %.0f%%", CPUWarnPercent, CPUSeverePercent),
		Fn:        ruleCPUWarn,
	})

	register(ruleEntry{
		ID:        "R-10",
		Severity:  model.SevWarning,
		Category:  model.CatStorage,
		Title:     "系统盘空间不足，存在系统卡顿、更新失败风险",
		Condition: "系统盘剩余空间 < 10 GiB 且 ≥ 5 GiB",
		Fn:        ruleDiskWarn,
	})
	register(ruleEntry{
		ID:        "R-11",
		Severity:  model.SevSevere,
		Category:  model.CatStorage,
		Title:     "系统盘空间严重不足，存在系统异常与更新失败高风险",
		Condition: "系统盘剩余空间 < 5 GiB",
		Fn:        ruleDiskSevere,
	})

	register(ruleEntry{
		ID:        "R-12",
		Severity:  model.SevWarning,
		Category:  model.CatNetwork,
		Title:     "内网链路存在丢包，网络稳定性异常",
		Condition: fmt.Sprintf("网关 ICMP 丢包率 ≥ %.0f%% 且未达 100%%", ICMPLossWarnPercent),
		Fn:        ruleGatewayLoss,
	})
	register(ruleEntry{
		ID:        "R-13",
		Severity:  model.SevWarning,
		Category:  model.CatNetwork,
		Title:     "内网延迟偏高，访问内网资源缓慢",
		Condition: fmt.Sprintf("网关 ICMP 平均延迟 ≥ %.0f ms", ICMPLatencyWarnMs),
		Fn:        ruleGatewayLatency,
	})
	register(ruleEntry{
		ID:        "R-14",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "网关及上层探测均无响应，疑似链路或出口策略异常",
		Condition: "网关、系统/直连 DNS 和全部 TCP 候选均已完成失败，没有任何上层成功证据",
		Fn:        ruleGatewayUnreachable,
	})
	register(ruleEntry{
		ID:        "R-15",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "系统 DNS 无法解析测试域名，直连 DNS 正常",
		Condition: "系统 DNS 解析 " + DNSProbeDomain + " 失败，但直连 " + DNSDirectResolver + " 解析成功",
		Fn:        ruleLocalDNSBroken,
	})
	register(ruleEntry{
		ID:        "R-16",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "系统与直连 DNS 均无法解析测试域名",
		Condition: "系统和直连 " + DNSDirectResolver + " 解析 " + DNSProbeDomain + " 均已完成且失败",
		Fn:        ruleDNSAllBroken,
	})
	register(ruleEntry{
		ID:        "R-17",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "公网 TCP 443 候选直连失败，已探测网关可达",
		Condition: "全部公网 TCP 443 候选均已完成失败，并有网关成功响应证据",
		Fn:        ruleWANPortBlocked,
	})
	register(ruleEntry{
		ID:        "R-18",
		Severity:  model.SevWarning,
		Category:  model.CatNetwork,
		Title:     "网关 ICMP 无响应，但公网 TCP 目标可达",
		Condition: "网关 ICMP 丢包率 = 100%，但任一公网 TCP 443 目标可达",
		Fn:        ruleICMPFiltered,
	})

	// Deferred: true —— 它读 Snapshot.Failures，必须等其余规则全部跑完才能结算，
	// 否则会漏掉后到（含被 recover 兜住）的失败项。
	register(ruleEntry{
		ID:        "R-19",
		Severity:  model.SevWarning,
		Category:  model.CatMeta,
		Title:     "诊断不完整，部分数据缺失，结论可能不完整",
		Condition: "任一采集项或判定项失败/未采集",
		Deferred:  true,
		Fn:        rulePartialData,
	})
}

// ruleAPIPA flags automatic IPv4 addresses on active physical adapters.
func ruleAPIPA(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range configurationAdapters(s) {
		var bad []string
		for _, addr := range a.IPv4 {
			if addr.IsAPIPA() {
				bad = append(bad, addr.IP)
			}
		}
		if len(bad) == 0 {
			continue
		}
		out = append(out, model.Issue{
			RuleID:   "R-01",
			Severity: model.SevSevere,
			Category: model.CatNetwork,
			Title:    "DHCP 地址获取失败，终端未获取有效内网 IP",
			Detail: fmt.Sprintf("网卡「%s」获得的是自动专用地址（APIPA），说明 DHCP 请求没有得到响应。",
				a.DisplayName()),
			Evidence: []string{
				"网卡：" + a.DisplayName(),
				"自动专用地址：" + strings.Join(bad, ", "),
			},
			Suggestion: "检查网线/Wi-Fi 是否真正连通；确认 DHCP 服务器可达；" +
				"可在管理员权限下执行 ipconfig /release 与 ipconfig /renew 重新申请地址。",
		})
	}
	return out
}

// ruleNoGateway flags missing default routes while accepting IPv6 gateways.
func ruleNoGateway(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range configurationAdapters(s) {
		if a.HasGateway() {
			continue
		}
		out = append(out, model.Issue{
			RuleID:   "R-02",
			Severity: model.SevSevere,
			Category: model.CatNetwork,
			Title:    "无网关配置，内网出口异常，无法正常联网",
			Detail: fmt.Sprintf("网卡「%s」处于活动状态，但没有任何默认网关，本机无法访问本网段以外的地址。",
				a.DisplayName()),
			Evidence: []string{
				"网卡：" + a.DisplayName(),
				"IPv4：" + joinOrNone(a.AddrStrings()),
				"网关：(空)",
			},
			Suggestion: "确认是否应使用 DHCP 自动获取网关；" +
				"若为静态配置，请补上默认网关地址；若是 VPN 客户端，请确认其是否接管了默认路由。",
		})
	}
	return out
}

// ruleNoDNS flags known empty DNS configuration, excluding missing evidence.
func ruleNoDNS(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range configurationAdapters(s) {
		if a.HasDNS() {
			continue
		}
		// 关键守卫（基线场景 S-17）：DNS 来源为「未采集」时，我们**不知道**
		// 它到底有没有配置，只是没读到。此时报「DNS 未配置」是凭空断言，
		// 会让用户去改一个本来正确的设置。这种情况交给 R-19 表达。
		if a.DNSSource == model.DNSSourceMissing {
			continue
		}
		out = append(out, model.Issue{
			RuleID:   "R-03",
			Severity: model.SevSevere,
			Category: model.CatNetwork,
			Title:    "DNS 未配置，局域网域名与网页均无法正常解析",
			Detail: fmt.Sprintf("网卡「%s」的活动配置里没有任何 DNS 服务器地址，所有域名解析都会失败。",
				a.DisplayName()),
			Evidence: []string{
				"网卡：" + a.DisplayName(),
				"DNS：(空)",
				"DNS 来源：" + a.DNSSource,
			},
			Suggestion: "确认 DHCP 是否下发了 DNS；若为静态配置，请至少填写一个内网或公共 DNS 地址。",
		})
	}
	return out
}

// ruleInvalidDNS flags only unspecified DNS placeholders; local proxies are legal.
func ruleInvalidDNS(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range configurationAdapters(s) {
		if !AllDNSInvalid(a.DNS) {
			continue
		}
		out = append(out, model.Issue{
			RuleID:   "R-04",
			Severity: model.SevSevere,
			Category: model.CatNetwork,
			Title:    "DNS 配置异常（指向无效地址），域名解析将失败",
			Detail: fmt.Sprintf("网卡「%s」配置的 DNS 地址列表非空，但全部是无效占位值，"+
				"看起来「已配置」却无法解析任何域名。", a.DisplayName()),
			Evidence: []string{
				"网卡：" + a.DisplayName(),
				"DNS：" + strings.Join(a.DNS, ", "),
				"无效地址判定：" + strings.Join(InvalidDNSServers, ", "),
			},
			Suggestion: "把 DNS 改为真实可用的解析服务器地址（内网 DNS 或公共 DNS）。",
		})
	}
	return out
}

// ruleIPv6OnlyLinkLocal flags link-local-only IPv6 without usable IPv4.
func ruleIPv6OnlyLinkLocal(s *model.Snapshot) []model.Issue {
	if !s.IPv6OnlyLinkLocal() {
		return nil
	}
	var addrs []string
	for _, a := range configurationAdapters(s) {
		for _, v6 := range a.IPv6 {
			addrs = append(addrs, fmt.Sprintf("%s（%s %s）", v6.IP, a.DisplayName(), v6.Scope))
		}
	}
	return []model.Issue{{
		RuleID:   "R-05",
		Severity: model.SevWarning,
		Category: model.CatNetwork,
		Title:    "网络未获得有效 IP（仅链路本地地址），无法对外通信",
		Detail: "本机只拿到了 IPv6 链路本地地址，这类地址仅在本网段内有效，" +
			"无法路由到内网服务器或外网；同时也没有可用的 IPv4 地址。",
		Evidence: model.NormalizeEvidence(append([]string{
			"IPv6 地址清单：",
		}, addrs...)),
		Suggestion: "确认该网段是否提供 IPv4 地址；若使用 IPv6，需要路由器下发全局地址（非 fe80:: 开头）。",
	}}
}

// ruleMemSevere applies the severe threshold to known memory samples.
func ruleMemSevere(s *model.Snapshot) []model.Issue {
	if !s.Health.MemKnown || s.Health.MemUsedPercent < MemSeverePercent {
		return nil
	}
	return []model.Issue{memIssue(s, "R-06", model.SevSevere,
		"系统内存严重过载，存在程序闪退与系统卡死风险",
		"内存占用率已达到严重档，新启动的程序很可能申请不到内存。")}
}

// ruleMemWarn applies the warning band without duplicating severe findings.
func ruleMemWarn(s *model.Snapshot) []model.Issue {
	if !s.Health.MemKnown || s.Health.MemUsedPercent < MemWarnPercent || s.Health.MemUsedPercent >= MemSeverePercent {
		return nil
	}
	return []model.Issue{memIssue(s, "R-07", model.SevWarning,
		"系统内存占用偏高，终端运行可能卡顿",
		"内存占用率偏高，尚未达到严重档，但已足以影响交互流畅度。")}
}

// memIssue builds the memory finding with measured usage and threshold evidence.
func memIssue(s *model.Snapshot, id string, sev model.Severity, title, detail string) model.Issue {
	h := s.Health
	return model.Issue{
		RuleID:   id,
		Severity: sev,
		Category: model.CatSystem,
		Title:    title,
		Detail:   detail,
		Evidence: []string{
			fmt.Sprintf("内存占用率：%.1f%%", h.MemUsedPercent),
			fmt.Sprintf("已用：%s / 总计：%s", formatGiB(h.MemUsedBytes()), formatGiB(h.MemTotalBytes)),
			fmt.Sprintf("告警阈值：警告 ≥ %.0f%%，严重 ≥ %.0f%%", MemWarnPercent, MemSeverePercent),
		},
		Suggestion: "在任务管理器中按内存占用排序，关闭或重启占用最高的进程；" +
			"若长期偏高，考虑扩充物理内存。",
	}
}

// ruleCPUSevere checks the severe threshold for the short CPU sampling window.
func ruleCPUSevere(s *model.Snapshot) []model.Issue {
	if !s.Health.CPUKnown || s.Health.CPUPercent < CPUSeverePercent {
		return nil
	}
	return []model.Issue{cpuIssue(s, "R-08", model.SevSevere,
		"CPU 采样窗口内占用严重偏高",
		"本次采样窗口内 CPU 繁忙，可能影响响应；单次采样不证明长期持续满载。")}
}

// ruleCPUWarn checks the CPU warning band without duplicating severe findings.
func ruleCPUWarn(s *model.Snapshot) []model.Issue {
	if !s.Health.CPUKnown || s.Health.CPUPercent < CPUWarnPercent || s.Health.CPUPercent >= CPUSeverePercent {
		return nil
	}
	return []model.Issue{cpuIssue(s, "R-09", model.SevWarning,
		"CPU 占用率偏高，存在系统卡顿风险",
		"采样窗口内 CPU 占用率偏高，尚未达到满载。")}
}

// cpuIssue states the CPU sampling window rather than inferring sustained load.
func cpuIssue(s *model.Snapshot, id string, sev model.Severity, title, detail string) model.Issue {
	return model.Issue{
		RuleID:   id,
		Severity: sev,
		Category: model.CatSystem,
		Title:    title,
		Detail:   detail,
		Evidence: []string{
			fmt.Sprintf("CPU 占用率：%.1f%%（采样窗口 %s）", s.Health.CPUPercent, CPUSampleWindow),
			fmt.Sprintf("告警阈值：警告 ≥ %.0f%%，严重 ≥ %.0f%%", CPUWarnPercent, CPUSeverePercent),
		},
		Suggestion: "在任务管理器中按 CPU 占用排序定位占用进程；" +
			"若为杀毒扫描或系统更新，可等待其完成后再复测。",
	}
}

// ruleDiskWarn checks known system-volume free space against the warning band.
func ruleDiskWarn(s *model.Snapshot) []model.Issue {
	h := s.Health
	if !h.DiskKnown || h.DiskFreeBytes < DiskSevereFreeBytes || h.DiskFreeBytes >= DiskWarnFreeBytes {
		return nil
	}
	return []model.Issue{diskIssue(s, "R-10", model.SevWarning,
		"系统盘空间不足，存在系统卡顿、更新失败风险")}
}

// ruleDiskSevere checks known system-volume free space against the severe threshold.
func ruleDiskSevere(s *model.Snapshot) []model.Issue {
	h := s.Health
	if !h.DiskKnown || h.DiskFreeBytes >= DiskSevereFreeBytes {
		return nil
	}
	return []model.Issue{diskIssue(s, "R-11", model.SevSevere,
		"系统盘空间严重不足，存在系统异常与更新失败高风险")}
}

// diskIssue combines system-volume identity, usage and free-space evidence.
func diskIssue(s *model.Snapshot, id string, sev model.Severity, title string) model.Issue {
	h := s.Health
	usedPct := 0.0
	if h.DiskTotalBytes > 0 {
		usedPct = float64(h.DiskUsedBytes()) / float64(h.DiskTotalBytes) * 100
	}
	return model.Issue{
		RuleID:   id,
		Severity: sev,
		Category: model.CatStorage,
		Title:    title,
		Detail: fmt.Sprintf("系统盘 %s 的可用空间已低于安全线，"+
			"Windows 更新、页面文件扩展与临时文件写入都可能失败。", h.SystemDrive),
		Evidence: []string{
			"系统盘：" + h.SystemDrive,
			fmt.Sprintf("已用：%s / 总计：%s（%.1f%%）", formatGiB(h.DiskUsedBytes()), formatGiB(h.DiskTotalBytes), usedPct),
			fmt.Sprintf("剩余：%s", formatGiB(h.DiskFreeBytes)),
			fmt.Sprintf("告警阈值：警告 < %s，严重 < %s", formatGiB(DiskWarnFreeBytes), formatGiB(DiskSevereFreeBytes)),
		},
		Suggestion: "清理系统盘：删除临时文件与旧更新缓存，把大文件移出系统盘；" +
			"可在管理员权限下运行磁盘清理工具。",
	}
}

// gatewayProbes 返回全部真正执行过的网关探测。
// 排除 Skipped 是 R-12 ~ R-14 / R-18 的共同前提：没探测过不等于探测失败。
func gatewayProbes(s *model.Snapshot) []model.ProbeResult {
	var out []model.ProbeResult
	for _, p := range s.FindProbes(model.ProbeICMPGateway) {
		if !p.Executed() || p.Incomplete {
			continue
		}
		out = append(out, p)
	}
	return out
}

// gatewayFailed 报告是否有探测确实执行过、却一个回包都没收到。
func gatewayFailed(s *model.Snapshot) bool {
	for _, p := range gatewayProbes(s) {
		if p.Recv == 0 {
			return true
		}
	}
	return false
}

// tcpProbes returns TCP records that were not explicitly skipped.
func tcpProbes(s *model.Snapshot) []model.ProbeResult {
	var out []model.ProbeResult
	for _, p := range s.FindProbes(model.ProbeTCP443) {
		if p.Skipped {
			continue
		}
		out = append(out, p)
	}
	return out
}

// allTCPFailed requires completed failures for every configured candidate.
func allTCPFailed(s *model.Snapshot) bool {
	// evidence 必须覆盖全部固定目标，不能用一条失败冒充全部候选失败。
	evidence := model.AssessProbes(s.Probes, model.ProbeTCP443, TCP443Targets)
	return evidence.Complete && !evidence.Success
}

// anyTCPSuccess reports actual successful TCP evidence.
func anyTCPSuccess(s *model.Snapshot) bool {
	for _, p := range tcpProbes(s) {
		if p.Success {
			return true
		}
	}
	return false
}

// executed 报告某类探测是否真的跑过（存在非 Skipped 的结果）。
func executed(s *model.Snapshot, kind model.ProbeKind) (model.ProbeResult, bool) {
	// first 保留失败证据；有完整成功时优先返回成功，避免首条失败掩盖后续成功。
	var first model.ProbeResult
	found := false
	for _, p := range s.FindProbes(kind) {
		if p.Executed() && !p.Incomplete {
			if p.Success {
				return p, true
			}
			if !found {
				first, found = p, true
			}
		}
	}
	return first, found
}

// ruleGatewayLoss reports partial loss only for complete ICMP samples.
func ruleGatewayLoss(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, p := range gatewayProbes(s) {
		// 丢包 100% 由 R-14/R-18 表达，这里只覆盖"部分丢包"这一档。
		if p.Recv == 0 || p.LossPercent < ICMPLossWarnPercent {
			continue
		}
		out = append(out, model.Issue{
			RuleID:   "R-12",
			Severity: model.SevWarning,
			Category: model.CatNetwork,
			Title:    "内网链路存在丢包，网络稳定性异常",
			Detail: fmt.Sprintf("发往网关 %s 的回显请求有部分没有得到应答，"+
				"链路质量不稳定，可能表现为访问内网资源时快时慢。", p.Target),
			Evidence: []string{
				"探测网卡：" + p.AdapterName,
				"目标：" + p.Target,
				fmt.Sprintf("发包 %d，回包 %d，丢包率 %.0f%%", p.Sent, p.Recv, p.LossPercent),
				fmt.Sprintf("告警阈值：丢包率 ≥ %.0f%%", ICMPLossWarnPercent),
			},
			Suggestion: "检查网线、交换机端口与无线信号强度；" +
				"若仅有本机异常，可尝试更换网线或网口后复测。",
		})
	}
	return out
}

// ruleGatewayLatency checks average RTT only when replies were received.
func ruleGatewayLatency(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, p := range gatewayProbes(s) {
		if p.Recv == 0 {
			continue
		}
		if p.AvgRTT < timeMillisFromFloat(ICMPLatencyWarnMs) {
			continue
		}
		out = append(out, model.Issue{
			RuleID:   "R-13",
			Severity: model.SevWarning,
			Category: model.CatNetwork,
			Title:    "内网延迟偏高，访问内网资源缓慢",
			Detail: fmt.Sprintf("访问网关 %s 的平均往返延迟偏高，局域网内通信本身就很慢，"+
				"访问内网服务器与共享目录时感受会更明显。", p.Target),
			Evidence: []string{
				"探测网卡：" + p.AdapterName,
				"目标：" + p.Target,
				fmt.Sprintf("平均延迟 %.1f ms（最小 %.1f ms，最大 %.1f ms）",
					float64(p.AvgRTT)/float64(millisecond), float64(p.MinRTT)/float64(millisecond), float64(p.MaxRTT)/float64(millisecond)),
				fmt.Sprintf("告警阈值：平均延迟 ≥ %.0f ms", ICMPLatencyWarnMs),
			},
			Suggestion: "无线连接请靠近 AP 或改用有线；有线连接请检查交换机负载与网线质量。",
		})
	}
	return out
}

// ruleGatewayUnreachable requires complete failures across all required protocols.
func ruleGatewayUnreachable(s *model.Snapshot) []model.Issue {
	// gateway/system/direct 必须都有完整失败证据；其他接口或上层成功时不宣称整体链路中断。
	gateway := model.AssessProbes(s.Probes, model.ProbeICMPGateway, nil)
	system := model.AssessProbes(s.Probes, model.ProbeDNSSystem, nil)
	direct := model.AssessProbes(s.Probes, model.ProbeDNSDirect, nil)
	if !gateway.Complete || gateway.Success || !system.Complete || system.Success || !direct.Complete || direct.Success || !allTCPFailed(s) {
		return nil
	}
	var out []model.Issue
	for _, p := range gatewayProbes(s) {
		if p.Recv != 0 {
			continue
		}
		// 关键守卫：网关无 ICMP 响应但公网 443 可达时，是 ICMP 被安全设备拦截而非
		// 内网中断（那正是 R-18 的场景）；此处再报「内网链路中断」会与 R-18 自相矛盾。
		if anyTCPSuccess(s) {
			continue
		}
		out = append(out, model.Issue{
			RuleID:   "R-14",
			Severity: model.SevSevere,
			Category: model.CatNetwork,
			Title:    "网关及上层探测均无响应，疑似链路或出口策略异常",
			Detail: fmt.Sprintf("发往网关 %s 的 %d 个回显请求全部没有得到应答，"+
				"系统/直连 DNS 和全部公网 443 候选也均已执行失败，可能为链路、出口策略或目标服务问题。", p.Target, p.Sent),
			Evidence: []string{
				"探测网卡：" + p.AdapterName,
				"目标：" + p.Target,
				fmt.Sprintf("发包 %d，回包 %d，丢包率 100%%", p.Sent, p.Recv),
				"回显错误：" + orNone(p.Err),
			},
			Suggestion: "按「本机网线/Wi-Fi → 交换机端口 → 网关设备」的顺序排查；" +
				"确认同一网段的其他终端是否正常，以判断是单机问题还是网段问题。",
		})
	}
	return out
}

// ruleLocalDNSBroken compares failed system resolution with successful direct resolution.
func ruleLocalDNSBroken(s *model.Snapshot) []model.Issue {
	systemEvidence := model.AssessProbes(s.Probes, model.ProbeDNSSystem, nil)
	directEvidence := model.AssessProbes(s.Probes, model.ProbeDNSDirect, nil)
	sys, okSys := executed(s, model.ProbeDNSSystem)
	direct, okDirect := executed(s, model.ProbeDNSDirect)
	if !systemEvidence.Complete || systemEvidence.Success || !directEvidence.Success || !okSys || !okDirect || !direct.Success {
		return nil
	}
	return []model.Issue{{
		RuleID:   "R-15",
		Severity: model.SevSevere,
		Category: model.CatNetwork,
		Title:    "系统 DNS 无法解析测试域名，直连 DNS 正常",
		Detail: "系统解析器无法解析本次测试域名，而直连公共 DNS 成功。" +
			"该差异可能来自 DNS 配置、服务或企业策略，不证明其他域名或全部外网均不可用。",
		Evidence: []string{
			fmt.Sprintf("探测域名：%s", DNSProbeDomain),
			fmt.Sprintf("系统 DNS 解析：失败（%s）", orNone(sys.Err)),
			fmt.Sprintf("直连 %s 解析：成功（%s）", DNSDirectResolver, strings.Join(direct.Resolved, ", ")),
			"本机 DNS 服务器：" + joinOrNone(collectDNSList(s)),
		},
		Suggestion: "检查网卡 DNS 设置是否正确；确认「DNS Client」服务（Dnscache）是否正在运行；" +
			"可临时把网卡 DNS 改为公共 DNS 验证。",
	}}
}

// ruleDNSAllBroken requires complete failures from both DNS paths.
func ruleDNSAllBroken(s *model.Snapshot) []model.Issue {
	systemEvidence := model.AssessProbes(s.Probes, model.ProbeDNSSystem, nil)
	directEvidence := model.AssessProbes(s.Probes, model.ProbeDNSDirect, nil)
	// system 是不可缺少的对照证据，公共 DNS 不可用不代表企业 DNS 故障。
	system, okSystem := executed(s, model.ProbeDNSSystem)
	direct, ok := executed(s, model.ProbeDNSDirect)
	if !systemEvidence.Complete || systemEvidence.Success || !directEvidence.Complete || directEvidence.Success || !okSystem || !ok {
		return nil
	}
	return []model.Issue{{
		RuleID:   "R-16",
		Severity: model.SevSevere,
		Category: model.CatNetwork,
		Title:    "系统与直连 DNS 均无法解析测试域名",
		Detail:   "本次固定域名在系统与公共解析器均解析失败；需结合 TCP 证据检查解析策略或目标域名，不能据此断言全部互联网不可用。",
		Evidence: []string{
			fmt.Sprintf("探测域名：%s", DNSProbeDomain),
			fmt.Sprintf("系统 DNS 解析：失败（%s）", orNone(system.Err)),
			fmt.Sprintf("直连 %s 解析：失败（%s）", DNSDirectResolver, orNone(direct.Err)),
		},
		Suggestion: "先确认基础连通性（网关与公网 443 是否可达）；" +
			"若基础链路正常，可能是 DNS 端口被策略封锁，请联系网络管理员。",
	}}
}

// ruleWANPortBlocked requires all TCP candidates failed and an observed reachable gateway.
func ruleWANPortBlocked(s *model.Snapshot) []model.Issue {
	// gateway 需要实际成功证据，空列表或跳过不能说明内网正常。
	gateway := model.AssessProbes(s.Probes, model.ProbeICMPGateway, nil)
	if !allTCPFailed(s) || !gateway.Complete || !gateway.Success {
		return nil
	}
	// 成功只证明至少一个网关，不代表其他接口或所有互联网目标可用。
	var ev []string
	for _, p := range tcpProbes(s) {
		ev = append(ev, fmt.Sprintf("%s → 失败（%s）", p.Target, orNone(p.Err)))
	}
	return []model.Issue{{
		RuleID:   "R-17",
		Severity: model.SevSevere,
		Category: model.CatNetwork,
		Title:    "公网 TCP 443 候选直连失败，已探测网关可达",
		Detail: fmt.Sprintf("全部 %d 个固定公网 443 候选直连均失败，且至少一个已探测网关可达。"+
			"结果只针对这些候选；企业代理要求、出口策略和目标服务状态都可能造成差异。", len(ev)),
		Evidence: model.NormalizeEvidence(append([]string{
			"探测目标（直连，不走系统代理）：",
		}, ev...)),
		Suggestion: "确认该终端是否需要通过代理上网；" +
			"若同网段其他终端正常，请检查本机防火墙或安全软件是否拦截了出站连接。",
	}}
}

// ruleICMPFiltered explains unresponsive gateway ICMP when global TCP evidence succeeds.
func ruleICMPFiltered(s *model.Snapshot) []model.Issue {
	if !gatewayFailed(s) || !anyTCPSuccess(s) {
		return nil
	}
	var ev []string
	for _, p := range gatewayProbes(s) {
		if p.Recv == 0 {
			ev = append(ev, fmt.Sprintf("网关 %s（网卡 %s）：%d 个回显请求全部无应答", p.Target, p.AdapterName, p.Sent))
		}
	}
	for _, p := range tcpProbes(s) {
		if p.Success {
			ev = append(ev, fmt.Sprintf("公网 %s → 可达（%d ms）", p.Target, p.Duration.Milliseconds()))
		}
	}
	return []model.Issue{{
		RuleID:   "R-18",
		Severity: model.SevWarning,
		Category: model.CatNetwork,
		Title:    "网关 ICMP 无响应，但公网 TCP 目标可达",
		Detail: "至少一个公网 TCP 候选可达，网关 ICMP 未获响应，可能受 ICMP 策略影响。" +
			"实际出接口由系统路由决定，不能据此证明每块无响应网卡都能正常联网。",
		Evidence:   ev,
		Suggestion: "核对出接口与企业 ICMP 策略；若某一网卡持续异常，请单独检查该接口的链路与路由。",
	}}
}

// rulePartialData aggregates missing evidence after other rules finish.
func rulePartialData(s *model.Snapshot) []model.Issue {
	if len(s.Failures) == 0 {
		return nil
	}
	var ev []string
	for _, f := range s.Failures {
		line := "• " + f.Item
		if f.Partial {
			line += "（部分采集）"
		}
		line += "：" + f.Reason
		if f.EnvVar != "" {
			line += "（环境：" + f.EnvVar + "）"
		}
		ev = append(ev, line)
	}
	return []model.Issue{{
		RuleID:   "R-19",
		Severity: model.SevWarning,
		Category: model.CatMeta,
		Title:    fmt.Sprintf("诊断不完整，%d 项数据缺失，结论可能不完整", len(s.Failures)),
		Detail: "以下项目未能成功采集或判定。缺失项对应的规则不会触发，" +
			"因此本次结论可能在对应方向上偏于乐观，请结合完整清单判断。",
		Evidence:   ev,
		Suggestion: incompleteSuggestion(s),
	}}
}

const millisecond = int64(1000 * 1000)

// timeMillisFromFloat 把 float64 毫秒阈值归一化成 time.Duration。
// 阈值用 float64 便于书写、RTT 是 Duration，不归一化就会"拿纳秒比毫秒"。
func timeMillisFromFloat(ms float64) time.Duration {
	return time.Duration(ms * float64(millisecond))
}

// formatGiB 把字节数按 1024³ 换算、但标签写 GB，与资源管理器/磁盘管理的口径一致；
// 若改用十进制 GB，同一块盘会出现"工具说 931 GB、系统说 868 GB"的困惑。
func formatGiB(b uint64) string {
	const gib = 1 << 30
	return fmt.Sprintf("%.2f GB", float64(b)/float64(gib))
}

// joinOrNone renders an explicit placeholder for an empty evidence list.
func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "(无)"
	}
	return strings.Join(items, ", ")
}

// orNone preserves nonempty text and marks missing evidence explicitly.
func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(无)"
	}
	return s
}

// collectDNSList gathers configured DNS addresses on active physical interfaces.
func collectDNSList(s *model.Snapshot) []string {
	var out []string
	for _, a := range configurationAdapters(s) {
		out = append(out, a.DNS...)
	}
	return out
}

// configurationAdapters excludes inventory-only rows whose empty configuration is not evidence.
func configurationAdapters(s *model.Snapshot) []model.Adapter {
	var adapters []model.Adapter
	for _, a := range s.ActivePhysicalAdapters() {
		if !a.AddressMissing {
			adapters = append(adapters, a)
		}
	}
	return adapters
}

// incompleteSuggestion separates unavailable evidence from network failure and avoids blanket elevation advice.
func incompleteSuggestion(s *model.Snapshot) string {
	advice := "按缺失原因逐项核查运行环境和安全策略；ICMP 调用被拒绝表示探测受限，不是网关不响应或网络丢包。" +
		"Windows ICMP API 通常无需管理员权限；请结合 DNS/TCP 结果判断，不要仅据此关闭安全软件、修改网络配置或要求提权。"
	if s.Host.IntegrityLevel != "" {
		advice += fmt.Sprintf(" 当前进程完整性等级：%s（RID=%d）。", s.Host.IntegrityLevel, s.Host.IntegrityRID)
		if strings.HasPrefix(s.Host.IntegrityLevel, "Low") || strings.HasPrefix(s.Host.IntegrityLevel, "Untrusted") {
			advice += "低完整性环境可能限制 ICMP；可由用户在可信的普通桌面运行环境中复测。该等级不证明具体拦截者。"
		}
	} else {
		advice += " 进程完整性等级未知，不能推断具体拒绝原因。"
	}
	return advice
}
