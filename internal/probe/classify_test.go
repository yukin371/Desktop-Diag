// This file checks configured targets against the pure classifier.
package probe

import (
	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"testing"
)

// TestClassifyUsesAllConfiguredTargets rejects incomplete TCP evidence.
func TestClassifyUsesAllConfiguredTargets(t *testing.T) {
	probes := []model.ProbeResult{
		{Kind: model.ProbeICMPGateway, Sent: 4, Recv: 4, Success: true},
		{Kind: model.ProbeDNSSystem, Success: true},
	}
	for i, target := range detect.TCP443Targets {
		probes = append(probes, model.ProbeResult{Kind: model.ProbeTCP443, Target: target})
		want := model.LevelUndetermined
		if i == len(detect.TCP443Targets)-1 {
			want = model.LevelWANPort
		}
		if got := Classify(probes); len(got) != 1 || got[0].Level != want {
			t.Fatalf("candidate %d: got %+v, want %s", i, got, want)
		}
	}
}
