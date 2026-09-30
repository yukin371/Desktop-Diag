//go:build windows

// 本文件执行有界并发网关探测与顺序 DNS/TCP 对照，保留未执行和未完成状态。
package collect

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/probe"
)

// probeCollector 通过可注入的只读探测函数采集连通性，测试无需修改真实网络。
type probeCollector struct {
	icmp func(context.Context, string, probe.ICMPOptions) (model.ProbeResult, error)
	dns  func(context.Context, string, probe.DNSOptions) (model.ProbeResult, error)
	tcp  func(context.Context, string, probe.TCPOptions) (model.ProbeResult, error)
}

// Name 返回进度与失败清单中的采集域名称。
func (probeCollector) Name() string { return "网络连通性探测" }

// EnvVar 返回稳定的采集域标识。
func (probeCollector) EnvVar() string { return "probe" }

// defaults 填入正式探测实现，不改变传入实例或全局函数。
func (c probeCollector) defaults() probeCollector {
	if c.icmp == nil {
		c.icmp = probe.ICMP
	}
	if c.dns == nil {
		c.dns = probe.DNS
	}
	if c.tcp == nil {
		c.tcp = probe.TCP
	}
	return c
}

// Collect 先收集逐接口网关证据，再执行全局 DNS/TCP；超时仍记录所有未完成目标。
func (c probeCollector) Collect(ctx context.Context, snap *model.Snapshot) error {
	c = c.defaults()
	// active 优先使用物理接口，只有虚拟默认出口时按模型约定回退。
	active := snap.ProbeAdapters()
	// probes 仅由调度线程写入，并发工作者只返回独立结果。
	probes := c.probeGateways(ctx, snap, active)
	if len(active) == 0 {
		for _, kind := range []model.ProbeKind{model.ProbeICMPGateway, model.ProbeDNSSystem, model.ProbeDNSDirect, model.ProbeTCP443} {
			probes = append(probes, skippedProbe(kind, "", "", "没有可诊断的活动接口（含默认路由虚拟出口）"))
		}
		snap.AddFailure(c.Name(), c.EnvVar(), "没有可诊断的活动接口，网络探测全部跳过；覆盖网络与无默认路由虚拟接口不在探测范围", true)
	} else {
		probes = append(probes, c.probeDNS(ctx, snap)...)
		probes = append(probes, c.probeTCP(ctx, snap)...)
	}
	snap.Probes = probes
	snap.Layers = probe.Classify(probes)
	snap.AddRaw("网络连通性", "路由语义", "ICMP 按目标网关标注所属接口，实际出接口由系统路由决定；DNS/TCP 为全局探测，不证明每个接口均正常")
	return nil
}

// probeGateways 以固定工作者数探测，完成后按输入接口顺序串行合并快照。
func (c probeCollector) probeGateways(ctx context.Context, snap *model.Snapshot, active []model.Adapter) []model.ProbeResult {
	c = c.defaults()
	// results 固定每个接口的位置，避免完成先后影响报告顺序。
	results := make([]model.ProbeResult, len(active))
	// jobs 为每个工作者分配唯一位置，不允许并发写相同元素。
	jobs := make(chan int, len(active))
	for i := range active {
		jobs <- i
	}
	close(jobs)
	// workers 限制系统句柄数和同一时刻的网络请求数。
	workers := min(detect.GatewayWorkers, len(active))
	// wait 等待工作者完成后再接触 Snapshot。
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for i := range jobs {
				results[i] = c.probeGateway(ctx, active[i])
			}
		}()
	}
	wait.Wait()
	for i, result := range results {
		snap.AddRaw("网络连通性", "IcmpSendEcho", fmt.Sprintf("接口 %d %s → %s：%s", result.AdapterIndex, result.AdapterName, result.Target, probeSummary(result)))
		if result.Incomplete || (result.Skipped && active[i].HasGateway()) {
			snap.AddFailure(c.Name(), c.EnvVar(), fmt.Sprintf("接口 %d %s 的网关探测不完整：%s", result.AdapterIndex, result.AdapterName, probeSummary(result)), true)
		}
	}
	return results
}

// probeGateway 保留接口身份；单工作者异常转为缺失证据，不让 goroutine 崩溃整个进程。
func (c probeCollector) probeGateway(ctx context.Context, a model.Adapter) (result model.ProbeResult) {
	defer func() {
		if cause := recover(); cause != nil {
			result = skippedProbe(model.ProbeICMPGateway, a.DisplayName(), a.FirstGateway(), fmt.Sprintf("网关探测内部错误：%v", cause))
			result.AdapterIndex = a.Index
		}
	}()
	// gateway 只取 IPv4，IPv6 仅展示并明确能力缺失。
	gateway := a.FirstGateway()
	if gateway == "" {
		// reason 区分配置缺失与协议能力缺失，前者交给配置规则。
		reason := "未配置 IPv4 默认网关"
		if len(a.GatewaysV6) > 0 {
			reason = "MVP 不支持 IPv6 网关 ICMP；IPv6 默认路由已采集，IPv4 网关探测未执行"
		}
		result = skippedProbe(model.ProbeICMPGateway, a.DisplayName(), "", reason)
	} else if err := ctx.Err(); err != nil {
		result = skippedProbe(model.ProbeICMPGateway, a.DisplayName(), gateway, "探测预算已结束："+err.Error())
	} else {
		// err 表示未能发起调用；不能伪造网关无回包。
		var err error
		result, err = c.icmp(ctx, gateway, probe.ICMPOptions{Count: detect.ICMPPacketCount, Timeout: detect.ICMPTimeout, AdapterName: a.DisplayName(), AdapterIndex: a.Index})
		if err != nil {
			result = skippedProbe(model.ProbeICMPGateway, a.DisplayName(), gateway, "ICMP 探测未能完成："+err.Error())
		}
	}
	result.AdapterIndex = a.Index
	return result
}

// probeDNS 执行两次解析对照，预算不足时为每个缺失项分别留痕。
func (c probeCollector) probeDNS(ctx context.Context, snap *model.Snapshot) []model.ProbeResult {
	// results 保持系统解析在直连解析之前。
	var results []model.ProbeResult
	for _, resolver := range []string{"", detect.DNSDirectResolver} {
		// kind/target 表示当前解析器与固定测试目标。
		kind, target := probe.DNSKind(resolver), detect.DNSProbeDomain
		if resolver != "" {
			target = resolver
		}
		// result/error 分别承载网络结果与调用失败。
		var result model.ProbeResult
		var err error
		if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			result, err = c.dns(ctx, detect.DNSProbeDomain, probe.DNSOptions{Timeout: detect.DNSQueryTimeout, Resolver: resolver})
		}
		if err != nil {
			result = skippedProbe(kind, "", target, "DNS 探测未能完成："+err.Error())
		}
		if ctx.Err() != nil && !result.Skipped {
			result.Incomplete = true
			result.Err = ctx.Err().Error()
		}
		c.recordProbe(snap, result, "DNS 对照")
		results = append(results, result)
	}
	return results
}

// probeTCP 首个成功即停止；预算中断时仍保留未尝试候选，不能把部分失败当全部失败。
func (c probeCollector) probeTCP(ctx context.Context, snap *model.Snapshot) []model.ProbeResult {
	// results 保持固定候选顺序，成功之后不再发起网络调用。
	var results []model.ProbeResult
	for _, target := range detect.TCP443Targets {
		// result/error 分别承载 TCP 握手与未能发起探测的原因。
		var result model.ProbeResult
		var err error
		if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			result, err = c.tcp(ctx, target, probe.TCPOptions{Timeout: detect.TCPDialTimeout})
		}
		if err != nil {
			result = skippedProbe(model.ProbeTCP443, "", target, "TCP 探测未能完成："+err.Error())
		}
		if ctx.Err() != nil && !result.Skipped {
			result.Incomplete = true
			result.Err = ctx.Err().Error()
		}
		c.recordProbe(snap, result, "TCP 443 直连（不走系统代理）")
		results = append(results, result)
		if result.Success {
			break
		}
	}
	return results
}

// recordProbe 将已完成结果与能力缺失统一写入原始附录和失败清单。
func (c probeCollector) recordProbe(snap *model.Snapshot, result model.ProbeResult, source string) {
	snap.AddRaw("网络连通性", source, result.Target+" → "+probeSummary(result))
	if result.Skipped || result.Incomplete {
		snap.AddFailure(c.Name(), c.EnvVar(), result.Kind.KindLabel()+"："+probeSummary(result), true)
	}
}

// skippedProbe 构造未执行记录；发送数保持为零，不计入丢包率。
func skippedProbe(kind model.ProbeKind, adapter, target, reason string) model.ProbeResult {
	return model.ProbeResult{Kind: kind, AdapterName: adapter, Target: target, Skipped: true, SkipReason: reason}
}

// probeSummary 将结果压成一行；未完成采样不能伪装成完整失败。
func probeSummary(r model.ProbeResult) string {
	if r.Skipped {
		return "已跳过（" + r.SkipReason + "）"
	}
	if r.Incomplete {
		return "未完成（" + r.Err + "）"
	}
	if r.Kind == model.ProbeICMPGateway {
		return fmt.Sprintf("发送 %d 收 %d 丢包 %.0f%% 平均 %v", r.Sent, r.Recv, r.LossPercent, r.AvgRTT.Round(time.Millisecond))
	}
	if r.Success {
		return fmt.Sprintf("成功（耗时 %v）解析=%s", r.Duration.Round(time.Millisecond), strings.Join(r.Resolved, ","))
	}
	return fmt.Sprintf("失败（耗时 %v）：%s", r.Duration.Round(time.Millisecond), r.Err)
}
