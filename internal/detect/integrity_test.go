// Tests restricted ICMP evidence and environment-aware guidance without network IO.
package detect

import (
	"github.com/yukin371/desktop-diag/internal/model"
	"strings"
	"testing"
)

// TestDeniedICMPDoesNotReportNetworkFailure rejects outage rules on unavailable gateway evidence.
func TestDeniedICMPDoesNotReportNetworkFailure(t *testing.T) {
	for _, level := range []string{"Low（低完整性）", "Medium（中完整性）", ""} {
		s := &model.Snapshot{Host: model.Host{IntegrityLevel: level, IntegrityRID: 4096}}
		s.Failures = []model.CollectFailure{{Item: "ICMP", EnvVar: "probe", Reason: "IcmpSendEcho: Access is denied.", Partial: true}}
		s.Probes = []model.ProbeResult{
			{Kind: model.ProbeICMPGateway, Skipped: true, Incomplete: true, SkipReason: "Access is denied."},
			{Kind: model.ProbeDNSSystem, Success: true}, {Kind: model.ProbeDNSDirect, Success: true},
			{Kind: model.ProbeTCP443, Success: true},
		}
		issues := Evaluate(s)
		found := false
		for _, issue := range issues {
			switch issue.RuleID {
			case "R-12", "R-13", "R-14", "R-17", "R-18":
				t.Fatalf("denied ICMP produced %s", issue.RuleID)
			}
			if issue.RuleID == "R-19" {
				found = true
				if strings.Contains(issue.Suggestion, "以管理员身份重新运行") || !strings.Contains(issue.Suggestion, "不是网关不响应或网络丢包") {
					t.Fatalf("misleading suggestion %s", issue.Suggestion)
				}
			}
		}
		if !found {
			t.Fatal("missing incomplete warning")
		}
	}
}
