//go:build windows

// Verifies read-only process integrity query and Windows RID classification.
package winapi

import "testing"

// TestProcessIntegrity queries the test process, avoiding assumptions about its privilege level.
func TestProcessIntegrity(t *testing.T) {
	level, rid, err := ProcessIntegrity()
	if err != nil {
		t.Fatal(err)
	}
	if level == "" || level != integrityName(rid) {
		t.Fatalf("invalid integrity %q %d", level, rid)
	}
	for _, test := range []struct {
		rid  uint32
		want string
	}{
		{0, "Untrusted（不可信）"}, {integrityLow, "Low（低完整性）"}, {integrityMedium, "Medium（中完整性）"},
		{integrityHigh, "High（高完整性）"}, {integritySystem, "System（系统完整性）"}, {integrityProtected, "Protected（受保护）"},
	} {
		if got := integrityName(test.rid); got != test.want {
			t.Errorf("RID %d: %s", test.rid, got)
		}
	}
}
