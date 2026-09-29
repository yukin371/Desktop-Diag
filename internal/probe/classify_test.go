//go:build windows

package probe

import (
	"reflect"
	"testing"

	"github.com/yukin371/desktop-diag/internal/model"
)

// icmpProbe 是构造 ICMP 探测结果的测试辅助函数。
func icmpProbe(success bool, skipped bool) model.ProbeResult {
	return model.ProbeResult{
		Kind:    model.ProbeICMPGateway,
		Target:  "192.168.1.1",
		Sent:    4,
		Recv:    boolToInt(success) * 4,
		Success: success,
		Skipped: skipped,
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// dnsProbe 构造 DNS 探测结果。kind 由 resolver 是否为空决定，与实现保持一致。
func dnsProbe(resolver string, success bool) model.ProbeResult {
	return model.ProbeResult{
		Kind:     DNSKind(resolver),
		Target:   "www.example.com",
		Success:  success,
		Resolved: []string{"93.184.216.34"},
	}
}

func tcpProbe(target string, success bool) model.ProbeResult {
	return model.ProbeResult{
		Kind:    model.ProbeTCP443,
		Target:  target,
		Success: success,
	}
}

func allSkipped() []model.ProbeResult {
	return []model.ProbeResult{
		{Kind: model.ProbeICMPGateway, Skipped: true, SkipReason: "无活动网卡"},
		{Kind: model.ProbeDNSSystem, Skipped: true, SkipReason: "无活动网卡"},
		{Kind: model.ProbeDNSDirect, Skipped: true, SkipReason: "无活动网卡"},
		{Kind: model.ProbeTCP443, Skipped: true, SkipReason: "无活动网卡"},
	}
}

// TestClassify 逐行覆盖 7 条判定矩阵及各类边界。
func TestClassify(t *testing.T) {
	cases := []struct {
		name        string
		probes      []model.ProbeResult
		wantLevel   string
		wantSummary string
		wantSev     model.Severity
	}{
		{
			name:        "空切片_数据不足",
			probes:      nil,
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
		{
			name:        "全Skipped_数据不足",
			probes:      allSkipped(),
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
		{
			name: "第1行_仅Skipped与未知Kind",
			probes: []model.ProbeResult{
				{Kind: "unknown-kind", Success: true},
				{Kind: model.ProbeTCP443, Skipped: true},
			},
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
		{
			name: "第2行_四类探测全通",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", true),
				dnsProbe("223.5.5.5:53", true),
				tcpProbe("223.5.5.5:443", true),
			},
			wantLevel:   model.LevelOK,
			wantSummary: "网络链路正常",
			wantSev:     model.SevOK,
		},
		{
			name: "第2行_无网关探测但其余全通",
			probes: []model.ProbeResult{
				dnsProbe("", true),
				dnsProbe("223.5.5.5:53", true),
				tcpProbe("223.5.5.5:443", true),
			},
			wantLevel:   model.LevelOK,
			wantSummary: "网络链路正常",
			wantSev:     model.SevOK,
		},
		{
			name: "第3行_网关不通但直连DNS通",
			probes: []model.ProbeResult{
				icmpProbe(false, false),
				dnsProbe("", false),
				dnsProbe("223.5.5.5:53", true),
				tcpProbe("223.5.5.5:443", false),
			},
			wantLevel:   model.LevelICMPFiltered,
			wantSummary: "网关 ICMP 无响应但上层链路可达，疑似 ICMP 被防火墙或安全软件拦截，并非真实断网",
			wantSev:     model.SevWarning,
		},
		{
			name: "第3行_网关不通但TCP通",
			probes: []model.ProbeResult{
				icmpProbe(false, false),
				tcpProbe("223.5.5.5:443", true),
			},
			wantLevel:   model.LevelICMPFiltered,
			wantSummary: "网关 ICMP 无响应但上层链路可达，疑似 ICMP 被防火墙或安全软件拦截，并非真实断网",
			wantSev:     model.SevWarning,
		},
		{
			name: "第4行_网关与上层全不通",
			probes: []model.ProbeResult{
				icmpProbe(false, false),
				dnsProbe("", false),
				dnsProbe("223.5.5.5:53", false),
				tcpProbe("223.5.5.5:443", false),
			},
			wantLevel:   model.LevelLANDown,
			wantSummary: "网关不可达，本地网络中断或网关设备故障",
			wantSev:     model.SevSevere,
		},
		{
			name: "第4行_仅网关不通无其他证据",
			probes: []model.ProbeResult{
				icmpProbe(false, false),
			},
			wantLevel:   model.LevelLANDown,
			wantSummary: "网关不可达，本地网络中断或网关设备故障",
			wantSev:     model.SevSevere,
		},
		{
			name: "第5行_系统DNS失败直连DNS正常",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", false),
				dnsProbe("223.5.5.5:53", true),
				tcpProbe("223.5.5.5:443", true),
			},
			wantLevel:   model.LevelWANDNS,
			wantSummary: "系统 DNS 解析失败但直连 DNS 正常，本机 DNS 配置或 DNS 客户端服务异常",
			wantSev:     model.SevSevere,
		},
		{
			name: "第6行_系统DNS与直连DNS全失败_外网异常",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", false),
				dnsProbe("223.5.5.5:53", false),
				tcpProbe("223.5.5.5:443", false),
			},
			wantLevel:   model.LevelWANDown,
			wantSummary: "内网链路正常但外网访问中断",
			wantSev:     model.SevSevere,
		},
		{
			name: "第6行_DNS全失败_无TCP探测",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", false),
				dnsProbe("223.5.5.5:53", false),
			},
			wantLevel:   model.LevelWANDown,
			wantSummary: "内网链路正常但外网访问中断",
			wantSev:     model.SevSevere,
		},
		{
			name: "第7行_DNS正常但443不通",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", true),
				dnsProbe("223.5.5.5:53", true),
				tcpProbe("223.5.5.5:443", false),
			},
			wantLevel:   model.LevelWANPort,
			wantSummary: "DNS 解析正常但 443 端口不可达，可能存在端口封锁或代理异常",
			wantSev:     model.SevWarning,
		},
		{
			name: "第7行_仅直连DNS正常且未做TCP探测",
			probes: []model.ProbeResult{
				dnsProbe("223.5.5.5:53", true),
			},
			// 没有 TCP 证据就不能断定"端口不可达"，如实报数据不足。
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
		{
			name: "第8行_仅系统DNS且失败",
			probes: []model.ProbeResult{
				dnsProbe("", false),
			},
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
		{
			name: "第8行_网关可通但DNS探测全缺失",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				tcpProbe("223.5.5.5:443", true),
			},
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
		{
			name: "第8行_网关可通且支持DNS探测缺失但TCP通",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", false),
				dnsProbe("223.5.5.5:53", false),
				tcpProbe("223.5.5.5:443", true),
			},
			// 第 6 步要求 TCP 不通；TCP 通时不能声称"外网中断"，如实报数据不足。
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
		{
			name: "第8行_仅系统DNS成功且未做TCP探测",
			probes: []model.ProbeResult{
				dnsProbe("", true),
			},
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
		{
			name: "同Kind多条ICMP_有一条成功则取成功条",
			probes: []model.ProbeResult{
				{Kind: model.ProbeICMPGateway, Target: "192.168.1.1", Sent: 4, Recv: 0, Success: false},
				{Kind: model.ProbeICMPGateway, Target: "10.0.0.1", Sent: 4, Recv: 4, Success: true},
				dnsProbe("", false),
				dnsProbe("223.5.5.5:53", true),
			},
			wantLevel:   model.LevelWANDNS,
			wantSummary: "系统 DNS 解析失败但直连 DNS 正常，本机 DNS 配置或 DNS 客户端服务异常",
			wantSev:     model.SevSevere,
		},
		{
			name: "同Kind多条ICMP_全部失败走第4行",
			probes: []model.ProbeResult{
				{Kind: model.ProbeICMPGateway, Target: "192.168.1.1", Sent: 4, Recv: 0, Success: false},
				{Kind: model.ProbeICMPGateway, Target: "10.0.0.1", Sent: 4, Recv: 0, Success: false},
				tcpProbe("223.5.5.5:443", false),
			},
			wantLevel:   model.LevelLANDown,
			wantSummary: "网关不可达，本地网络中断或网关设备故障",
			wantSev:     model.SevSevere,
		},
		{
			name: "同Kind多条ICMP_成功的Skipped条不参与",
			probes: []model.ProbeResult{
				{Kind: model.ProbeICMPGateway, Target: "192.168.1.1", Skipped: true},
				{Kind: model.ProbeICMPGateway, Target: "10.0.0.1", Sent: 4, Recv: 4, Success: true},
				dnsProbe("", true),
				dnsProbe("223.5.5.5:53", true),
				tcpProbe("223.5.5.5:443", true),
			},
			wantLevel:   model.LevelOK,
			wantSummary: "网络链路正常",
			wantSev:     model.SevOK,
		},
		{
			name: "TCP多目标_部分成功即视为端口可达",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", true),
				dnsProbe("223.5.5.5:53", true),
				tcpProbe("223.5.5.5:443", false),
				tcpProbe("119.29.29.29:443", true),
			},
			wantLevel:   model.LevelOK,
			wantSummary: "网络链路正常",
			wantSev:     model.SevOK,
		},
		{
			name: "TCP多目标_全部失败走第6行",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", true),
				tcpProbe("223.5.5.5:443", false),
				tcpProbe("119.29.29.29:443", false),
			},
			wantLevel:   model.LevelWANPort,
			wantSummary: "DNS 解析正常但 443 端口不可达，可能存在端口封锁或代理异常",
			wantSev:     model.SevWarning,
		},
		{
			name: "第7行_系统DNS正常但TCP探测全被跳过",
			probes: []model.ProbeResult{
				icmpProbe(true, false),
				dnsProbe("", true),
				{Kind: model.ProbeTCP443, Target: "223.5.5.5:443", Skipped: true},
				{Kind: model.ProbeTCP443, Target: "119.29.29.29:443", Skipped: true},
			},
			// 全部 TCP 探测被跳过 = 从未做 TCP 探测，按"证据不足"处理；
			// 若这里被算成 443 不通，跳过探测就会变成误报。
			wantLevel:   model.LevelUndetermined,
			wantSummary: "探测数据不足，无法确定故障层级",
			wantSev:     model.SevWarning,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.probes)

			if len(got) != 1 {
				t.Fatalf("Classify 必须返回恰好一条结论，实际 %d 条：%+v", len(got), got)
			}
			if got[0].Level != tc.wantLevel {
				t.Errorf("Level = %q，期望 %q", got[0].Level, tc.wantLevel)
			}
			if got[0].Summary != tc.wantSummary {
				t.Errorf("Summary = %q，期望 %q", got[0].Summary, tc.wantSummary)
			}
			if got[0].Severity != tc.wantSev {
				t.Errorf("Severity = %v，期望 %v", got[0].Severity, tc.wantSev)
			}
		})
	}
}

// TestClassifyIsPure 确认判定不修改入参（纯函数契约：无副作用）。
func TestClassifyIsPure(t *testing.T) {
	probes := []model.ProbeResult{
		icmpProbe(false, false),
		dnsProbe("", false),
		dnsProbe("223.5.5.5:53", true),
		tcpProbe("223.5.5.5:443", false),
		{Kind: model.ProbeTCP443, Target: "1.1.1.1:443", Skipped: true},
	}
	before := make([]model.ProbeResult, len(probes))
	copy(before, probes)

	first := Classify(probes)
	second := Classify(probes)

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("结论条数异常：%d / %d", len(first), len(second))
	}
	if first[0] != second[0] {
		t.Errorf("同输入两次判定结果不一致：%+v vs %+v", first[0], second[0])
	}
	for i := range probes {
		if !reflect.DeepEqual(probes[i], before[i]) {
			t.Errorf("第 %d 条入参被修改：%+v → %+v", i, before[i], probes[i])
		}
	}
}
