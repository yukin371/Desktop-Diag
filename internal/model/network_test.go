// This file checks completeness independently of Windows and live networking.
package model

import (
	"strings"
	"testing"
)

// TestMixedInterfaceSuccessIsScoped prevents global DNS/TCP success claiming every interface is healthy.
func TestMixedInterfaceSuccessIsScoped(t *testing.T) {
	probes := []ProbeResult{
		{Kind: ProbeICMPGateway, AdapterIndex: 1, Sent: 4, Success: false},
		{Kind: ProbeICMPGateway, AdapterIndex: 2, Sent: 4, Success: true},
		{Kind: ProbeDNSSystem, Success: true}, {Kind: ProbeDNSDirect, Success: true},
		{Kind: ProbeTCP443, Target: "first:443", Success: true},
	}
	got := ClassifyNetwork(probes, []string{"first:443", "unattempted:443"})
	if got[0].Level != LevelOK || !strings.Contains(got[0].Summary, "不能推断每个接口均正常") {
		t.Fatalf("mixed interfaces overclaimed: %+v", got)
	}
}

// TestNetworkEvidenceMatrix distinguishes absent, skipped, partial and complete failures.
func TestNetworkEvidenceMatrix(t *testing.T) {
	makeProbes := func(gateway, system, direct, tcp bool) []ProbeResult {
		return []ProbeResult{
			{Kind: ProbeICMPGateway, Sent: 4, Success: gateway},
			{Kind: ProbeDNSSystem, Success: system},
			{Kind: ProbeDNSDirect, Success: direct},
			{Kind: ProbeTCP443, Target: "a:443", Success: tcp},
			{Kind: ProbeTCP443, Target: "b:443", Success: tcp},
		}
	}
	tests := []struct {
		name     string
		probes   []ProbeResult
		level    string
		severity Severity
	}{
		{"all-success", makeProbes(true, true, true, true), LevelOK, SevOK},
		{"icmp-filtered", makeProbes(false, true, true, true), LevelICMPFiltered, SevWarning},
		{"complete-lan-failure", makeProbes(false, false, false, false), LevelLANDown, SevSevere},
		{"system-dns-only", makeProbes(true, false, true, true), LevelWANDNS, SevSevere},
		{"complete-wan-failure", makeProbes(true, false, false, false), LevelWANDown, SevSevere},
		{"tcp-failure", makeProbes(true, true, false, false), LevelWANPort, SevWarning},
		{"no-evidence", nil, LevelUndetermined, SevWarning},
		{"gateway-only", makeProbes(false, false, false, false)[:1], LevelUndetermined, SevWarning},
		{"missing-tcp-candidate", makeProbes(true, true, true, false)[:4], LevelUndetermined, SevWarning},
		{"no-gateway", makeProbes(true, true, true, true)[1:], LevelUndetermined, SevWarning},
		{"dns-only", makeProbes(true, true, true, true)[2:3], LevelUndetermined, SevWarning},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyNetwork(tc.probes, []string{"a:443", "b:443"})
			if len(got) != 1 || got[0].Level != tc.level || got[0].Severity != tc.severity || got[0].Summary == "" {
				t.Fatalf("got %+v", got)
			}
		})
	}
	for _, index := range []int{0, 1, 2, 3, 4} {
		for _, skipped := range []bool{false, true} {
			probes := makeProbes(false, false, false, false)
			probes[index].Skipped = skipped
			probes[index].Incomplete = !skipped
			if got := ClassifyNetwork(probes, []string{"a:443", "b:443"}); got[0].Level != LevelUndetermined {
				t.Errorf("index=%d skipped=%v: %+v", index, skipped, got)
			}
		}
	}
}

// TestDuplicateTargetsCannotCompleteEvidence prevents replacing missing candidates with duplicates.
func TestDuplicateTargetsCannotCompleteEvidence(t *testing.T) {
	probes := []ProbeResult{{Kind: ProbeTCP443, Target: "a:443"}, {Kind: ProbeTCP443, Target: "a:443"}}
	got := AssessProbes(probes, ProbeTCP443, []string{"a:443", "b:443"})
	if !got.Observed || got.Complete || got.Success {
		t.Fatalf("got %+v", got)
	}
	if (ProbeResult{Kind: ProbeICMPGateway, Sent: 0}).Executed() {
		t.Fatal("zero packets cannot prove failure")
	}
	if (ProbeResult{Kind: ProbeICMPGateway, Sent: 1, Incomplete: true}).TotalLoss() {
		t.Fatal("partial calls cannot prove total loss")
	}
}

// TestUnknownAddressInventoryCannotProveLinkLocalOnly checks missing rows make IPv6 scope inconclusive.
func TestUnknownAddressInventoryCannotProveLinkLocalOnly(t *testing.T) {
	s := &Snapshot{Adapters: []Adapter{{OperStatus: OperStatusUp, AddressMissing: true}}}
	if s.IPv6OnlyLinkLocal() {
		t.Fatal("unknown addresses classified as link-local-only")
	}
}

// TestProbeAdapterSelection checks physical priority, virtual fallback and overlay exclusion.
func TestProbeAdapterSelection(t *testing.T) {
	physical := Adapter{Index: 1, OperStatus: OperStatusUp, AdminKnown: true, AdminEnabled: true, Gateways: []string{"192.168.1.1"}}
	vpn := physical
	vpn.Index = 2
	vpn.IsVirtual = true
	vpn.VirtualKind = VirtualWireGuard
	overlay := vpn
	overlay.Index = 3
	overlay.VirtualKind = VirtualOverlay
	snapshot := &Snapshot{Adapters: []Adapter{vpn, overlay, physical}}
	if got := snapshot.ProbeAdapters(); len(got) != 1 || got[0].Index != 1 {
		t.Fatalf("physical priority: %+v", got)
	}
	snapshot.Adapters = []Adapter{vpn, overlay}
	if got := snapshot.ProbeAdapters(); len(got) != 1 || got[0].Index != 2 {
		t.Fatalf("virtual fallback: %+v", got)
	}
	vpn.Gateways = nil
	snapshot.Adapters = []Adapter{vpn, overlay}
	if got := snapshot.ProbeAdapters(); len(got) != 0 {
		t.Fatalf("no route: %+v", got)
	}
	physical.AdminEnabled = false
	if physical.IsActive() {
		t.Fatal("known disabled interface active")
	}
	physical.AdminKnown = false
	if !physical.IsActive() {
		t.Fatal("unknown management status discarded usable operational interface")
	}
}
