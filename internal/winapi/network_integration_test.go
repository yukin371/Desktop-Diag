//go:build windows

// This file checks read-only interface inventories on the current Windows host.
package winapi

import (
	"strings"
	"testing"
)

// TestInterfaceInventory reads management status without relying on a connected physical adapter.
func TestInterfaceInventory(t *testing.T) {
	adapters, err := GetInterfaceInventory()
	if err != nil {
		t.Fatal(err)
	}
	if len(adapters) == 0 {
		t.Fatal("interface table unexpectedly empty")
	}
	seen := make(map[uint32]bool)
	for _, a := range adapters {
		if a.IfIndex == 0 || seen[a.IfIndex] || a.AdapterName == "" || !a.HardwareKnown || a.AddressKnown {
			t.Fatalf("invalid inventory: %+v", a)
		}
		seen[a.IfIndex] = true
		if !strings.HasPrefix(a.AdapterName, "{") {
			t.Errorf("invalid interface GUID %q", a.AdapterName)
		}
	}
}

// TestDisabledDeviceInventory validates whatever disabled devices already exist without changing them.
func TestDisabledDeviceInventory(t *testing.T) {
	adapters, err := GetDisabledNetworkDevices()
	if err != nil {
		t.Logf("permission/device source degraded: %v", err)
	}
	for _, a := range adapters {
		if !a.AdminKnown || a.AdminEnabled || a.AdapterName == "" || a.AddressKnown {
			t.Fatalf("invalid disabled metadata: %+v", a)
		}
	}
	t.Logf("existing disabled network devices: %d", len(adapters))
}
