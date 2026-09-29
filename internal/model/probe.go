package model

import "time"

// ProbeKind 标识一类连通性探测。
type ProbeKind string

const (
	ProbeICMPGateway ProbeKind = "icmp-gateway" // 网关可达性（R-12~R-14、R-18）
	ProbeDNSSystem   ProbeKind = "dns-system"   // 使用系统配置的 DNS 解析（R-15）
	ProbeDNSDirect   ProbeKind = "dns-direct"   // 直连 223.5.5.5:53 解析，绕过系统 DNS（R-15/R-16）
	ProbeTCP443      ProbeKind = "tcp-443"      // 公网 TCP 443 直连（R-17/R-18，不走系统代理）
)

// KindLabel 返回探测类型的中文短标签，供控制台与报告使用。
func (k ProbeKind) KindLabel() string {
	switch k {
	case ProbeICMPGateway:
		return "网关 ICMP"
	case ProbeDNSSystem:
		return "系统 DNS 解析"
	case ProbeDNSDirect:
		return "直连 DNS 解析"
	case ProbeTCP443:
		return "公网 TCP 443"
	default:
		return string(k)
	}
}

// ProbeResult 是一次探测的完整结果。
//
// Skipped=true（探测不适用，如无活动网卡）与 Success=false（确实执行了但失败）
// 是两种语义，混用会把"无网卡"误报成"网络中断"，规则一律以 Skipped 为准。
type ProbeResult struct {
	Kind        ProbeKind
	AdapterName string // ICMP 探测归属网卡；其他探测为空
	Target      string // "192.168.1.1" / "223.5.5.5:53" / "223.5.5.5:443"
	Sent        int    // ICMP 发包数
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

// Executed 报告该探测是否真的执行过（既未跳过也非空结果）。
func (p ProbeResult) Executed() bool {
	return !p.Skipped
}

// TotalLoss 报告 ICMP 探测是否全部丢包（R-14 的输入）。
func (p ProbeResult) TotalLoss() bool {
	return p.Kind == ProbeICMPGateway && p.Sent > 0 && p.Recv == 0
}

// 层级结论取值，对应基线第 6 节的 7 行判定矩阵。
const (
	LevelOK           = "ok"
	LevelWANDNS       = "wan-dns"
	LevelWANDown      = "wan-down"
	LevelWANPort      = "wan-port"
	LevelLANDown      = "lan-down"
	LevelICMPFiltered = "icmp-filtered"
	LevelUndetermined = "undetermined"
)

// LevelOrder 给出层级的展示优先级（用于报告排序，数值小的在前）。
func LevelOrder(level string) int {
	switch level {
	case LevelLANDown:
		return 0
	case LevelWANDown:
		return 1
	case LevelWANDNS:
		return 2
	case LevelWANPort:
		return 3
	case LevelICMPFiltered:
		return 4
	case LevelOK:
		return 6
	case LevelUndetermined:
		return 5
	default:
		return 7
	}
}

// LayerConclusion 是判定矩阵输出的一行结论。
type LayerConclusion struct {
	Level    string // 见 Level* 常量
	Summary  string // 人类可读摘要，如 "内网正常 · 外网正常，本机 DNS 故障"
	Severity Severity
}

// FindProbe 按探测类型返回第一条匹配结果，未找到时 ok=false；
// 供规则函数与报告渲染复用，避免各处重复写循环。
func (s *Snapshot) FindProbe(kind ProbeKind) (ProbeResult, bool) {
	for _, p := range s.Probes {
		if p.Kind == kind {
			return p, true
		}
	}
	return ProbeResult{}, false
}

// FindProbes 按探测类型返回全部匹配结果（ICMP 可能多网卡多条）。
func (s *Snapshot) FindProbes(kind ProbeKind) []ProbeResult {
	var out []ProbeResult
	for _, p := range s.Probes {
		if p.Kind == kind {
			out = append(out, p)
		}
	}
	return out
}
