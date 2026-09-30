// 本文件验证缺失探测、企业 DNS 策略和本地 DNS 代理不会被误判为断网。
package detect

import (
	"testing"

	"github.com/yukin371/desktop-diag/internal/model"
)

// TestIncompleteEvidenceNeverClaimsOutage 验证未完成的探测不能证明全部不可达。
func TestIncompleteEvidenceNeverClaimsOutage(t *testing.T) {
	// cases 覆盖网关失败但没有上层证据，以及只有部分 TCP 候选失败。
	cases := []struct {
		name   string
		probes []model.ProbeResult
	}{
		{"gateway-only", []model.ProbeResult{gwProbe("LAN", "192.168.1.1", 4, 0, 0)}},
		{"tcp-without-gateway", []model.ProbeResult{tcpProbe(TCP443Targets[0], false)}},
		{"partial-tcp", []model.ProbeResult{gwProbe("LAN", "192.168.1.1", 4, 4, 1), tcpProbe(TCP443Targets[0], false)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// snapshot 仅包含实际完成的证据，不用默认失败填补缺失项。
			snapshot := withProbes(snap(), tc.probes...)
			for _, id := range []string{"R-14", "R-17"} {
				if issues := run(t, id, snapshot); len(issues) != 0 {
					t.Errorf("%s 根据缺失证据产生告警: %+v", id, issues)
				}
			}
		})
	}
}

// TestPublicDNSFailureDoesNotInvalidateSystemResolver 验证公共 DNS 被拦截时保留系统解析成功证据。
func TestPublicDNSFailureDoesNotInvalidateSystemResolver(t *testing.T) {
	// snapshot 表示企业 DNS 正常而公共解析器受策略限制的机器。
	snapshot := withProbes(snap(), dnsProbe(model.ProbeDNSSystem, true), dnsProbe(model.ProbeDNSDirect, false), tcpProbe(TCP443Targets[0], true))
	if issues := run(t, "R-16", snapshot); len(issues) != 0 {
		t.Fatalf("系统 DNS 已成功，不能报告全部 DNS 不可用: %+v", issues)
	}
	// 同类证据有成功时，第一条失败不能掩盖后续成功。
	snapshot.Probes = []model.ProbeResult{dnsProbe(model.ProbeDNSSystem, false), dnsProbe(model.ProbeDNSSystem, true), dnsProbe(model.ProbeDNSDirect, false)}
	for _, id := range []string{"R-15", "R-16"} {
		if issues := run(t, id, snapshot); len(issues) != 0 {
			t.Fatalf("%s ignored success: %+v", id, issues)
		}
	}
}

// TestLoopbackDNSIsValidConfiguration 验证回环地址可用于本机 DNS 代理。
func TestLoopbackDNSIsValidConfiguration(t *testing.T) {
	// adapter 使用常见本地 DNS 代理地址，地址本身不证明服务故障。
	adapter := upAdapter("LAN")
	for _, address := range []string{"127.0.0.1", "::1"} {
		adapter.DNS = []string{address}
		if issues := run(t, "R-04", snap(adapter)); len(issues) != 0 {
			t.Fatalf("合法本地 DNS 代理 %s 被误判为无效配置: %+v", address, issues)
		}
	}
}

// TestInventoryOnlyFieldsDoNotProveConfigurationFailure 验证地址 API 缺失时不把空配置判为故障。
func TestInventoryOnlyFieldsDoNotProveConfigurationFailure(t *testing.T) {
	adapter := upAdapter("LAN")
	adapter.AddressMissing = true
	adapter.IPv6 = []model.Addr{{IP: "fe80::1", Scope: model.ScopeLinkLocal}}
	for _, id := range []string{"R-01", "R-02", "R-03", "R-04", "R-05"} {
		if issues := run(t, id, snap(adapter)); len(issues) != 0 {
			t.Fatalf("%s used absent address data: %+v", id, issues)
		}
	}
}
