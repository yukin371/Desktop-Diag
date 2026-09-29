//go:build windows

package collect

import (
	"context"
	"fmt"
	"strings"

	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/probe"
)

// probeCollector 执行网络连通性探测并给出故障层级结论。
type probeCollector struct{}

// Name 实现 Collector。
func (probeCollector) Name() string { return "网络连通性探测" }

// EnvVar 实现 envVarer。
func (probeCollector) EnvVar() string { return "probe" }

// Collect 实现 Collector。
//
// 探测顺序固定为「网关 → 系统 DNS → 直连 DNS → 公网 TCP 443」，
// 这条顺序本身就是诊断逻辑：从最内层往外逐段验证，
// 因此任何一段的失败都能被后续结果区分成"这一段的问题"还是"更外层的问题"。
//
// # 最要紧的一条纪律
//
// **探测"没能发起"与"发起了但没回应"必须严格区分。**
// 前者说明是我们的工具或权限出了问题（属于诊断完整性），
// 后者才是网络故障的证据。把前者记成后者，会让一台网络完好的机器
// 被判成"内网链路中断"（SEVERE），运维据此去查交换机，白忙一场。
// 本文件用 Skipped 标记表达前者。
func (c probeCollector) Collect(ctx context.Context, snap *model.Snapshot) error {
	active := snap.ActivePhysicalAdapters()
	var probes []model.ProbeResult

	probes = append(probes, c.probeGateways(ctx, snap, active)...)

	if len(active) == 0 {
		// 没有任何活动物理网卡：不能去"探测失败"，只能如实说没探。
		probes = append(probes, skippedProbe(model.ProbeICMPGateway, "", "", "没有处于活动状态的物理网卡"))
		probes = append(probes, skippedProbe(model.ProbeDNSSystem, "", "", "没有处于活动状态的物理网卡"))
		probes = append(probes, skippedProbe(model.ProbeDNSDirect, "", "", "没有处于活动状态的物理网卡"))
		probes = append(probes, skippedProbe(model.ProbeTCP443, "", "", "没有处于活动状态的物理网卡"))
		snap.AddFailure(c.Name(), c.EnvVar(), "没有处于活动状态的物理网卡，网络探测全部跳过", true)

		snap.Probes = probes
		snap.Layers = probe.Classify(probes)
		return nil
	}

	if ctx.Err() == nil {
		probes = append(probes, c.probeDNS(ctx, snap)...)
	}
	if ctx.Err() == nil {
		probes = append(probes, c.probeTCP(ctx, snap)...)
	}

	snap.Probes = probes
	snap.Layers = probe.Classify(probes)
	return nil
}

// probeGateways 对每块活动物理网卡的网关做一次 ICMP 探测。
func (c probeCollector) probeGateways(ctx context.Context, snap *model.Snapshot, active []model.Adapter) []model.ProbeResult {
	var out []model.ProbeResult

	probed := 0
	for _, a := range active {
		gw := a.FirstGateway()
		if gw == "" {
			// 无网关由 R-02 表达，这里不产生探测记录，
			// 免得报告里出现一条"探测目标为空"的噪声。
			continue
		}
		if ctx.Err() != nil {
			out = append(out, skippedProbe(model.ProbeICMPGateway, a.DisplayName(), gw, "探测被中断"))
			continue
		}
		probed++

		r, err := probe.ICMP(ctx, gw, probe.ICMPOptions{
			Count:       detect.ICMPPacketCount,
			Timeout:     detect.ICMPTimeout,
			AdapterName: a.DisplayName(),
		})
		if err != nil {
			out = append(out, skippedProbe(model.ProbeICMPGateway, a.DisplayName(), gw,
				"ICMP 探测未能发起: "+err.Error()))
			snap.AddFailure(c.Name(), c.EnvVar(),
				fmt.Sprintf("对网关 %s 的 ICMP 探测未能发起（结果不计入丢包率，避免误判内网中断）: %v", gw, err), true)
			continue
		}
		out = append(out, r)
		snap.AddRaw("网络连通性", "IcmpSendEcho", fmt.Sprintf("%s → %s：%s", a.DisplayName(), gw, probeSummary(r)))
	}

	if probed == 0 {
		snap.AddRaw("网络连通性", "IcmpSendEcho", "（所有活动网卡均未配置网关，未发起网关探测）")
	}
	return out
}

// probeDNS 依次做系统 DNS 解析与直连 DNS 解析。
//
// 两次探测的**组合**才是判据：系统解析失败而直连成功，
// 说明本机 DNS 服务/配置有问题而网络本身通（R-15）；
// 两者都失败则是更外层的故障（R-16）。
func (c probeCollector) probeDNS(ctx context.Context, snap *model.Snapshot) []model.ProbeResult {
	var out []model.ProbeResult

	sys, err := probe.DNS(ctx, detect.DNSProbeDomain, probe.DNSOptions{
		Timeout: detect.DNSQueryTimeout,
	})
	if err != nil {
		out = append(out, skippedProbe(model.ProbeDNSSystem, "", detect.DNSProbeDomain,
			"系统 DNS 探测未能发起: "+err.Error()))
		snap.AddFailure(c.Name(), c.EnvVar(), "系统 DNS 探测未能发起: "+err.Error(), true)
	} else {
		out = append(out, sys)
		snap.AddRaw("网络连通性", "net.LookupHost(系统解析栈)",
			fmt.Sprintf("%s → %s", detect.DNSProbeDomain, probeSummary(sys)))
	}

	if ctx.Err() != nil {
		out = append(out, skippedProbe(model.ProbeDNSDirect, "", detect.DNSDirectResolver, "探测被中断"))
		return out
	}

	direct, err := probe.DNS(ctx, detect.DNSProbeDomain, probe.DNSOptions{
		Timeout:  detect.DNSQueryTimeout,
		Resolver: detect.DNSDirectResolver,
	})
	if err != nil {
		out = append(out, skippedProbe(model.ProbeDNSDirect, "", detect.DNSDirectResolver,
			"直连 DNS 探测未能发起: "+err.Error()))
		snap.AddFailure(c.Name(), c.EnvVar(), "直连 DNS 探测未能发起: "+err.Error(), true)
	} else {
		out = append(out, direct)
		snap.AddRaw("网络连通性", "net.Resolver{PreferGo:true} → "+detect.DNSDirectResolver,
			fmt.Sprintf("%s → %s", detect.DNSProbeDomain, probeSummary(direct)))
	}
	return out
}

// probeTCP 依次尝试公网 TCP 443 目标，**首个成功即停止**。
//
// 为什么可以提前收手：这里要回答的问题只有一个——"公网 443 到底通不通"。
// 只要有一个目标握手成功，答案就是"通"，再试其余目标不会改变任何结论，
// 却会在网络正常时白白多花时间。反过来，全部失败时会把候选目标全部试完，
// 报告因此能显示"三个候选目标都不可达"这一更强的证据。
func (c probeCollector) probeTCP(ctx context.Context, snap *model.Snapshot) []model.ProbeResult {
	var out []model.ProbeResult

	for i, target := range detect.TCP443Targets {
		if ctx.Err() != nil {
			out = append(out, skippedProbe(model.ProbeTCP443, "", target, "探测被中断"))
			continue
		}

		r, err := probe.TCP(ctx, target, probe.TCPOptions{Timeout: detect.TCPDialTimeout})
		if err != nil {
			out = append(out, skippedProbe(model.ProbeTCP443, "", target,
				"TCP 探测未能发起: "+err.Error()))
			snap.AddFailure(c.Name(), c.EnvVar(),
				fmt.Sprintf("对 %s 的 TCP 探测未能发起: %v", target, err), true)
			continue
		}
		out = append(out, r)
		snap.AddRaw("网络连通性", "net.DialTimeout(tcp)",
			fmt.Sprintf("%s（候选 %d/%d）→ %s", target, i+1, len(detect.TCP443Targets), probeSummary(r)))

		if r.Success {
			break
		}
	}
	return out
}

// skippedProbe 构造一条"没有真正发起"的探测记录。
//
// 关键在于 Sent 保持 0 且 Skipped 为 true：
// 判定层据此把这条记录排除在丢包率与失败率统计之外。
func skippedProbe(kind model.ProbeKind, adapter, target, reason string) model.ProbeResult {
	return model.ProbeResult{
		Kind:        kind,
		AdapterName: adapter,
		Target:      target,
		Skipped:     true,
		SkipReason:  reason,
	}
}

// probeSummary 把一条探测结果压成一行可读文本，供报告第三层附录使用。
func probeSummary(r model.ProbeResult) string {
	if r.Skipped {
		return "已跳过（" + r.SkipReason + "）"
	}
	if r.Kind == model.ProbeICMPGateway {
		return fmt.Sprintf("发送 %d 收 %d 丢包 %.0f%% 平均 %v", r.Sent, r.Recv, r.LossPercent, r.AvgRTT.Round(1_000_000))
	}
	if r.Success {
		resolved := ""
		if len(r.Resolved) > 0 {
			resolved = " 解析=" + strings.Join(r.Resolved, ",")
		}
		return fmt.Sprintf("成功（耗时 %v）%s", r.Duration.Round(1_000_000), resolved)
	}
	reason := "失败"
	if r.Err != "" {
		reason = "失败：" + r.Err
	}
	return fmt.Sprintf("%s（耗时 %v）", reason, r.Duration.Round(1_000_000))
}
