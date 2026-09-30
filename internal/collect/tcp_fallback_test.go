//go:build windows

// This file checks every configured TCP candidate and the successful short-circuit boundary.
package collect

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/probe"
)

// TestTCPFallbackCandidateEvidence checks failures cannot stand in for unattempted candidates.
func TestTCPFallbackCandidateEvidence(t *testing.T) {
	for _, successIndex := range []int{-1, 0, len(detect.TCP443Targets) - 1} {
		var attempted []string
		c := probeCollector{tcp: func(_ context.Context, target string, _ probe.TCPOptions) (model.ProbeResult, error) {
			index := len(attempted)
			attempted = append(attempted, target)
			return model.ProbeResult{Kind: model.ProbeTCP443, Target: target, Success: index == successIndex, Err: "fixture unreachable"}, nil
		}}
		s := &model.Snapshot{}
		results := c.probeTCP(context.Background(), s)
		count := len(detect.TCP443Targets)
		if successIndex >= 0 {
			count = successIndex + 1
		}
		if len(results) != count || !reflect.DeepEqual(attempted, detect.TCP443Targets[:count]) || len(s.Failures) != 0 {
			t.Fatalf("index=%d results=%+v attempted=%v", successIndex, results, attempted)
		}
		if successIndex == -1 && !model.AssessProbes(results, model.ProbeTCP443, detect.TCP443Targets).Complete {
			t.Fatal("complete failures lost")
		}
	}
	c := probeCollector{tcp: func(context.Context, string, probe.TCPOptions) (model.ProbeResult, error) {
		return model.ProbeResult{}, errors.New("invocation unavailable")
	}}
	s := &model.Snapshot{}
	results := c.probeTCP(context.Background(), s)
	if len(s.Failures) != len(detect.TCP443Targets) || model.AssessProbes(results, model.ProbeTCP443, detect.TCP443Targets).Complete {
		t.Fatalf("invocation errors misclassified: %+v", s)
	}
}
