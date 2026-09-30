//go:build windows

// This file verifies independent inventory sources and unknown configuration states.
package collect

import (
	"context"
	"errors"
	"testing"

	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// TestInventoryMergePreservesAddressesAndDisabledDevices covers GUID case, indices and source isolation.
func TestInventoryMergePreservesAddressesAndDisabledDevices(t *testing.T) {
	addresses := []winapi.RawAdapter{{IfIndex: 1, AdapterName: "{ABC}", FriendlyName: "LAN", AddressKnown: true, Dhcpv4Enabled: true, IPv4: []winapi.RawAddr{{IP: "192.168.1.2"}}, DNS: []string{"127.0.0.1"}}}
	inventory := []winapi.RawAdapter{
		{IfIndex: 1, AdapterName: "{abc}", AdminKnown: true, AdminEnabled: true, OperStatus: winapi.IfOperStatusUp, HardwareKnown: true, HardwareInterface: true},
		{IfIndex: 2, AdapterName: "{DEF}", AdminKnown: true, AdminEnabled: false, OperStatus: winapi.IfOperStatusDown},
		{AdapterName: "{def}", AdminKnown: true, AdminEnabled: false},
		{IfIndex: 3, AdapterName: "{GHI}", OperStatus: winapi.IfOperStatusUp},
	}
	merged := mergeAdapterInventory(addresses, inventory)
	if len(merged) != 3 {
		t.Fatalf("duplicate device: %+v", merged)
	}
	a := convertAdapter(merged[0])
	if !a.AdminKnown || !a.AdminEnabled || !a.DHCPKnown || !a.DHCPEnabled || len(a.IPv4) != 1 || len(merged[0].DNS) != 1 {
		t.Fatalf("lost source fields: %+v", a)
	}
	if addresses[0].AdminKnown {
		t.Fatal("input inventory mutated")
	}
	if a := convertAdapter(merged[1]); a.IsActive() || a.DHCPKnown || !a.AdminKnown {
		t.Fatalf("disabled configuration fabricated: %+v", a)
	}
	if a := convertAdapter(merged[2]); !a.IsActive() || a.AdminKnown || a.DHCPKnown {
		t.Fatalf("unknown management fabricated: %+v", a)
	}
	// Reused indices must not attach another device's management status to an existing GUID.
	if got := mergeAdapterInventory(addresses, []winapi.RawAdapter{{IfIndex: 1, AdapterName: "{NEW}", AdminKnown: true}}); len(got) != 2 || got[0].AdminKnown {
		t.Fatalf("index reused across device identities: %+v", got)
	}
	if got := mergeAdapterInventory(addresses, []winapi.RawAdapter{{IfIndex: 1, AdminKnown: true, AdminEnabled: true}}); len(got) != 1 || !got[0].AdminKnown {
		t.Fatalf("missing identity index fallback lost: %+v", got)
	}
}

// TestNetworkSourcesDegradeIndependently verifies permission failures keep successful inventory.
func TestNetworkSourcesDegradeIndependently(t *testing.T) {
	denied := errors.New("fixture permission denied")
	c := networkCollector{
		addresses: func(uint32, uint32) ([]winapi.RawAdapter, error) { return nil, denied },
		inventory: func() ([]winapi.RawAdapter, error) {
			return []winapi.RawAdapter{{IfIndex: 1, FriendlyName: "LAN", OperStatus: winapi.IfOperStatusUp}}, denied
		},
		disabled: func() ([]winapi.RawAdapter, error) {
			return []winapi.RawAdapter{{AdapterName: "device-instance", FriendlyName: "Disabled", AdminKnown: true}}, nil
		},
	}
	s := &model.Snapshot{}
	steps := RunCollectors(context.Background(), s, []Collector{c})
	if len(s.Adapters) != 2 || len(s.Failures) != 3 || steps[0].OK() || steps[0].Err != nil {
		t.Fatalf("partial inventory lost: snapshot=%+v steps=%+v", s, steps)
	}
	for _, a := range s.Adapters {
		if a.DHCPKnown || a.DNSSource != model.DNSSourceMissing {
			t.Errorf("unknown configuration marked known: %+v", a)
		}
	}
	for _, issue := range detect.Evaluate(s) {
		if issue.Severity == model.SevSevere {
			t.Errorf("inventory-only empty fields caused false outage: %+v", issue)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Collect(ctx, &model.Snapshot{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel cause lost: %v", err)
	}
}

// TestCollectorEventsAndCancellation checks callback order, partial results and skipped collectors.
func TestCollectorEventsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &model.Snapshot{}
	var events []StepEvent
	first := testCollector{name: "first", fn: func(s *model.Snapshot) error {
		s.AddFailure("optional", "fixture", "missing", true)
		cancel()
		return nil
	}}
	second := testCollector{name: "second", fn: func(*model.Snapshot) error { t.Fatal("cancelled collector called"); return nil }}
	steps := RunCollectors(ctx, s, []Collector{first, second}, func(e StepEvent) { events = append(events, e) })
	if len(events) != 4 || !events[0].Started || events[1].Started || !events[2].Started || events[3].Started {
		t.Fatalf("event order: %+v", events)
	}
	if steps[0].OK() || !errors.Is(steps[1].Err, context.Canceled) || len(s.Failures) != 2 {
		t.Fatalf("cancellation not recorded: %+v %+v", steps, s.Failures)
	}
}

// testCollector supplies a local callback to the scheduler.
type testCollector struct {
	name string
	fn   func(*model.Snapshot) error
}

func (c testCollector) Name() string                                       { return c.name }
func (c testCollector) Collect(_ context.Context, s *model.Snapshot) error { return c.fn(s) }
