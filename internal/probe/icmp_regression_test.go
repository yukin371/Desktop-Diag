//go:build windows

// This file verifies ICMP cancellation, API failures and resource cleanup with fixed responses.
package probe

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/winapi"
)

// TestICMPPartialCallsAndCloseError distinguishes invocation errors from actual network loss.
func TestICMPPartialCallsAndCloseError(t *testing.T) {
	callErr, closeErr := errors.New("send failed"), errors.New("close failed")
	calls, closed := 0, 0
	api := icmpAPI{
		open: func() (uintptr, error) { return 1, nil },
		send: func(_ uintptr, _ net.IP, payload []byte, _ time.Duration) (winapi.IcmpEchoResult, error) {
			if len(payload) != 32 {
				t.Fatalf("fixed payload length=%d", len(payload))
			}
			calls++
			if calls == 2 {
				return winapi.IcmpEchoResult{}, callErr
			}
			return winapi.IcmpEchoResult{Status: 0, RoundTripTime: 2}, nil
		},
		close: func(uintptr) error { closed++; return closeErr },
	}
	res, err := icmpWith(context.Background(), "192.168.1.1", ICMPOptions{Count: 3, AdapterIndex: 7}, api)
	if !errors.Is(err, closeErr) || closed != 1 || res.Sent != 2 || res.Recv != 2 || !res.Incomplete || res.TotalLoss() || res.AdapterIndex != 7 {
		t.Fatalf("res=%+v err=%v closes=%d", res, err, closed)
	}
	api.close = func(uintptr) error { return nil }
	api.send = func(uintptr, net.IP, []byte, time.Duration) (winapi.IcmpEchoResult, error) {
		return winapi.IcmpEchoResult{}, callErr
	}
	if _, err := icmpWith(context.Background(), "192.168.1.1", ICMPOptions{Count: 2}, api); !errors.Is(err, callErr) {
		t.Fatalf("all-call error chain lost: %v", err)
	}
}

// TestICMPCancellationAndRemainingBudget checks no handle opens before a cancelled request.
func TestICMPCancellationAndRemainingBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	api := icmpAPI{open: func() (uintptr, error) { t.Fatal("cancelled probe opened handle"); return 0, nil }}
	res, err := icmpWith(ctx, "192.168.1.1", ICMPOptions{Count: 4}, api)
	if err != nil || !res.Skipped || res.Executed() || !res.Incomplete {
		t.Fatalf("cancelled: %+v %v", res, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	calls := 0
	api = icmpAPI{open: func() (uintptr, error) { return 1, nil }, close: func(uintptr) error { return nil }, send: func(_ uintptr, _ net.IP, _ []byte, timeout time.Duration) (winapi.IcmpEchoResult, error) {
		if timeout <= 0 || timeout > 100*time.Millisecond {
			t.Fatalf("packet ignored remaining budget: %s", timeout)
		}
		calls++
		cancel()
		return winapi.IcmpEchoResult{Status: 0}, nil
	}}
	res, err = icmpWith(ctx, "192.168.1.1", ICMPOptions{Count: 4, Timeout: time.Second}, api)
	if err != nil || calls != 1 || res.Sent != 1 || !res.Incomplete || res.Skipped {
		t.Fatalf("mid-sample cancel: %+v %v calls=%d", res, err, calls)
	}
}
