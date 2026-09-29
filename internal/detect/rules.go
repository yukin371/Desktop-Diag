package detect

import (
	"fmt"
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// 本文件按基线 6.2 节的规则表实现 R-01 … R-19。
// register 里的 Condition 必须与实际实现一致：阶段 6 直接把它渲染成规则文档。

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
		Title:     "CPU 持续满载，系统响应严重迟缓",
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
		Title:     "无法连通网关，内网链路中断",
		Condition: "网关 ICMP 丢包率 = 100%，且公网 TCP 443 全部目标均不可达",
		Fn:        ruleGatewayUnreachable,
	})
	register(ruleEntry{
		ID:        "R-15",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "本机 DNS 服务不可用，网页无法正常访问（外网链路正常）",
		Condition: "系统 DNS 解析 " + DNSProbeDomain + " 失败，但直连 " + DNSDirectResolver + " 解析成功",
		Fn:        ruleLocalDNSBroken,
	})
	register(ruleEntry{
		ID:        "R-16",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "DNS 解析能力完全不可用，外网访问中断",
		Condition: "直连 " + DNSDirectResolver + " 解析 " + DNSProbeDomain + " 也失败",
		Fn:        ruleDNSAllBroken,
	})
	register(ruleEntry{
		ID:        "R-17",
		Severity:  model.SevSevere,
		Category:  model.CatNetwork,
		Title:     "外网访问中断（内网链路正常），疑似出口或防火墙策略问题",
		Condition: "公网 TCP 443 全部目标连接失败，且网关探测未失败（网关可达或无可探测网关）",
		Fn:        ruleWANPortBlocked,
	})
	register(ruleEntry{
		ID:        "R-18",
		Severity:  model.SevWarning,
		Category:  model.CatNetwork,
		Title:     "网关探测异常但外网可达，疑似 ICMP 被防火墙拦截（非真实故障）",
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

func ruleAPIPA(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range s.ActivePhysicalAdapters() {
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

func ruleNoGateway(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range s.ActivePhysicalAdapters() {
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

func ruleNoDNS(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range s.ActivePhysicalAdapters() {
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

func ruleInvalidDNS(s *model.Snapshot) []model.Issue {
	var out []model.Issue
	for _, a := range s.ActivePhysicalAdapters() {
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

func ruleIPv6OnlyLinkLocal(s *model.Snapshot) []model.Issue {
	if !s.IPv6OnlyLinkLocal() {
		return nil
	}
	var addrs []string
	for _, a := range s.ActivePhysicalAdapters() {
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

func ruleMemSevere(s *model.Snapshot) []model.Issue {
	if !s.Health.MemKnown || s.Health.MemUsedPercent < MemSeverePercent {
		return nil
	}
	return []model.Issue{memIssue(s, "R-06", model.SevSevere,
		"系统内存严重过载，存在程序闪退与系统卡死风险",
		"内存占用率已达到严重档，新启动的程序很可能申请不到内存。")}
}

func ruleMemWarn(s *model.Snapshot) []model.Issue {
	if !s.Health.MemKnown || s.Health.MemUsedPercent < MemWarnPercent || s.Health.MemUsedPercent >= MemSeverePercent {
		return nil
	}
	return []model.Issue{memIssue(s, "R-07", model.SevWarning,
		"系统内存占用偏高，终端运行可能卡顿",
		"内存占用率偏高，尚未达到严重档，但已足以影响交互流畅度。")}
}

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

func ruleCPUSevere(s *model.Snapshot) []model.Issue {
	if !s.Health.CPUKnown || s.Health.CPUPercent < CPUSeverePercent {
		return nil
	}
	return []model.Issue{cpuIssue(s, "R-08", model.SevSevere,
		"CPU 持续满载，系统响应严重迟缓",
		"采样窗口内 CPU 几乎全程繁忙，系统会明显卡顿甚至无响应。")}
}

func ruleCPUWarn(s *model.Snapshot) []model.Issue {
	if !s.Health.CPUKnown || s.Health.CPUPercent < CPUWarnPercent || s.Health.CPUPercent >= CPUSeverePercent {
		return nil
	}
	return []model.Issue{cpuIssue(s, "R-09", model.SevWarning,
		"CPU 占用率偏高，存在系统卡顿风险",
		"采样窗口内 CPU 占用率偏高，尚未达到满载。")}
}

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

func ruleDiskWarn(s *model.Snapshot) []model.Issue {
	h := s.Health
	if !h.DiskKnown || h.DiskFreeBytes < DiskSevereFreeBytes || h.DiskFreeBytes >= DiskWarnFreeBytes {
		return nil
	}
	return []model.Issue{diskIssue(s, "R-10", model.SevWarning,
		"系统盘空间不足，存在系统卡顿、更新失败风险")}
}

func ruleDiskSevere(s *model.Snapshot) []model.Issue {
	h := s.Health
	if !h.DiskKnown || h.DiskFreeBytes >= DiskSevereFreeBytes {
		return nil
	}
	return []model.Issue{diskIssue(s, "R-11", model.SevSevere,
		"系统盘空间严重不足，存在系统异常与更新失败高风险")}
}

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
		if p.Skipped || p.Sent == 0 {
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

func allTCPFailed(s *model.Snapshot) bool {
	probes := tcpProbes(s)
	if len(probes) == 0 {
		return false
	}
	for _, p := range probes {
		if p.Success {
			return false
		}
	}
	return true
}

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
	for _, p := range s.FindProbes(kind) {
		if !p.Skipped {
			return p, true
		}
	}
	return model.ProbeResult{}, false
}

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

func ruleGatewayUnreachable(s *model.Snapshot) []model.Issue {
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
			Title:    "无法连通网关，内网链路中断",
			Detail: fmt.Sprintf("发往网关 %s 的 %d 个回显请求全部没有得到应答，"+
				"且公网 443 也全部不可达，本机与内网的连通性已中断。", p.Target, p.Sent),
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

func ruleLocalDNSBroken(s *model.Snapshot) []model.Issue {
	sys, okSys := executed(s, model.ProbeDNSSystem)
	direct, okDirect := executed(s, model.ProbeDNSDirect)
	if !okSys || sys.Success || !okDirect || !direct.Success {
		return nil
	}
	return []model.Issue{{
		RuleID:   "R-15",
		Severity: model.SevSevere,
		Category: model.CatNetwork,
		Title:    "本机 DNS 服务不可用，网页无法正常访问（外网链路正常）",
		Detail: "用本机配置的 DNS 解析域名全部失败，而绕过本机配置、直连公共 DNS " +
			"却解析成功。这说明**外网链路是通的**，问题出在本机的 DNS 配置或 DNS 客户端服务上。",
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

func ruleDNSAllBroken(s *model.Snapshot) []model.Issue {
	direct, ok := executed(s, model.ProbeDNSDirect)
	if !ok || direct.Success {
		return nil
	}
	return []model.Issue{{
		RuleID:   "R-16",
		Severity: model.SevSevere,
		Category: model.CatNetwork,
		Title:    "DNS 解析能力完全不可用，外网访问中断",
		Detail: "连绕过本机配置、直连公共 DNS 的解析也失败了，" +
			"说明本机当前的域名解析能力完全不可用。",
		Evidence: []string{
			fmt.Sprintf("探测域名：%s", DNSProbeDomain),
			fmt.Sprintf("直连 %s 解析：失败（%s）", DNSDirectResolver, orNone(direct.Err)),
		},
		Suggestion: "先确认基础连通性（网关与公网 443 是否可达）；" +
			"若基础链路正常，可能是 DNS 端口被策略封锁，请联系网络管理员。",
	}}
}

func ruleWANPortBlocked(s *model.Snapshot) []model.Issue {
	if !allTCPFailed(s) {
		return nil
	}
	// 网关确实不通时不报本条：R-14 已给出更准确的结论，再说「内网正常、外网中断」自相矛盾。
	if gatewayFailed(s) {
		return nil
	}
	var ev []string
	for _, p := range tcpProbes(s) {
		ev = append(ev, fmt.Sprintf("%s → 失败（%s）", p.Target, orNone(p.Err)))
	}
	return []model.Issue{{
		RuleID:   "R-17",
		Severity: model.SevSevere,
		Category: model.CatNetwork,
		Title:    "外网访问中断（内网链路正常），疑似出口或防火墙策略问题",
		Detail: fmt.Sprintf("全部 %d 个公网 443 目标都无法建立连接，而网关探测未失败。"+
			"这说明本机到内网的链路是好的，问题出在内网出口或出口策略上。", len(ev)),
		Evidence: model.NormalizeEvidence(append([]string{
			"探测目标（直连，不走系统代理）：",
		}, ev...)),
		Suggestion: "确认该终端是否需要通过代理上网；" +
			"若同网段其他终端正常，请检查本机防火墙或安全软件是否拦截了出站连接。",
	}}
}

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
		Title:    "网关探测异常但外网可达，疑似 ICMP 被防火墙拦截（非真实故障）",
		Detail: "网关不回 ICMP 回显请求，但公网 443 可以连通，" +
			"说明内网其实是通的，只是 ICMP 协议被安全设备或防火墙策略拦截了。**这不是断网**。",
		Evidence:   ev,
		Suggestion: "无需处理。若需让网关探测也正常，请让网络管理员放行 ICMP 回显请求。",
	}}
}

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
		Evidence: ev,
		Suggestion: "按上述原因逐项处理；若提示权限不足，以管理员身份重新运行通常可补齐缺失项；" +
			"若已是管理员却仍提示「拒绝访问」，请检查安全软件或防火墙是否拦截了本程序。",
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

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "(无)"
	}
	return strings.Join(items, ", ")
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(无)"
	}
	return s
}

func collectDNSList(s *model.Snapshot) []string {
	var out []string
	for _, a := range s.ActivePhysicalAdapters() {
		out = append(out, a.DNS...)
	}
	return out
}
