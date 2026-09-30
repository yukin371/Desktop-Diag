//go:build windows

// This file validates bounded probing and completeness without live network changes.
package collect

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/probe"
)

// probeFixture returns successful DNS and TCP dependencies and an injected gateway implementation.
func probeFixture(icmp func(context.Context, string, probe.ICMPOptions) (model.ProbeResult, error)) probeCollector {
	return probeCollector{icmp: icmp,
		dns: func(_ context.Context, domain string, o probe.DNSOptions) (model.ProbeResult, error) {
			return model.ProbeResult{Kind: probe.DNSKind(o.Resolver), Target: domain, Success: true}, nil
		},
		tcp: func(_ context.Context, target string, _ probe.TCPOptions) (model.ProbeResult, error) {
			return model.ProbeResult{Kind: model.ProbeTCP443, Target: target, Success: true}, nil
		},
	}
}

// TestGatewayWorkersAreBoundedAndOrdered blocks calls until the configured pool fills.
func TestGatewayWorkersAreBoundedAndOrdered(t *testing.T) {
	var current, maximum atomic.Int32
	entered := make(chan struct{}, detect.GatewayWorkers)
	release := make(chan struct{})
	c := probeFixture(func(_ context.Context, target string, o probe.ICMPOptions) (model.ProbeResult, error) {
		n := current.Add(1)
		defer current.Add(-1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return model.ProbeResult{Kind: model.ProbeICMPGateway, Target: target, AdapterName: o.AdapterName, Sent: 4, Recv: 4, Success: true}, nil
	})
	s := &model.Snapshot{}
	for i := 0; i < 12; i++ {
		s.Adapters = append(s.Adapters, model.Adapter{Index: uint32(i + 1), Name: "same-name", OperStatus: model.OperStatusUp, Gateways: []string{fmt.Sprintf("192.168.%d.1", i)}})
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := c.Collect(context.Background(), s); err != nil {
			t.Error(err)
		}
	}()
	for range detect.GatewayWorkers {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("worker pool failed to fill")
		}
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not finish")
	}
	if maximum.Load() != int32(detect.GatewayWorkers) {
		t.Fatalf("peak workers=%d", maximum.Load())
	}
	for i, p := range s.Probes[:12] {
		if p.AdapterIndex != uint32(i+1) || p.Target != s.Adapters[i].FirstGateway() {
			t.Fatalf("result order/identity changed: %+v", p)
		}
	}
}

// TestIPv6OnlyAndCancelledProbesRecordMissingEvidence checks R-19 without false IPv4 faults.
func TestIPv6OnlyAndCancelledProbesRecordMissingEvidence(t *testing.T) {
	c := probeFixture(func(context.Context, string, probe.ICMPOptions) (model.ProbeResult, error) {
		t.Fatal("unexpected gateway call")
		return model.ProbeResult{}, nil
	})
	s := &model.Snapshot{Adapters: []model.Adapter{{Index: 7, OperStatus: model.OperStatusUp, IPv6: []model.Addr{{IP: "2001:db8::1", Scope: model.ScopeGlobal}}, GatewaysV6: []string{"fe80::1"}}}}
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if !s.Probes[0].Skipped || !strings.Contains(s.Probes[0].SkipReason, "IPv6") || len(s.Failures) != 1 || s.Layers[0].Level == model.LevelOK {
		t.Fatalf("IPv6 evidence: %+v", s)
	}
	for _, issue := range detect.Evaluate(s) {
		if issue.RuleID == "R-02" || issue.RuleID == "R-05" {
			t.Errorf("global IPv6 route falsely flagged: %+v", issue)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Adapters[0].Gateways = []string{"192.168.1.1"}
	s.Failures = nil
	if err := c.Collect(ctx, s); err != nil {
		t.Fatal(err)
	}
	if len(s.Probes) != 1+2+len(detect.TCP443Targets) || len(s.Failures) != len(s.Probes) {
		t.Fatalf("cancelled results lost: probes=%d failures=%d", len(s.Probes), len(s.Failures))
	}
	for _, p := range s.Probes {
		if !p.Skipped {
			t.Errorf("cancelled call executed: %+v", p)
		}
	}
}

// TestGatewayPanicIsIsolated verifies worker failures become evidence gaps rather than process crashes.
func TestGatewayPanicIsIsolated(t *testing.T) {
	c := probeFixture(func(context.Context, string, probe.ICMPOptions) (model.ProbeResult, error) { panic("fixture") })
	s := &model.Snapshot{Adapters: []model.Adapter{{Index: 1, OperStatus: model.OperStatusUp, Gateways: []string{"192.168.1.1"}}}}
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if !s.Probes[0].Skipped || len(s.Failures) != 1 || s.Layers[0].Level == model.LevelOK {
		t.Fatalf("panic not recorded: %+v", s)
	}
}
