//go:build windows

// Validates viewer guardrails without launching an application.
package winapi

import "testing"

// TestOpenReportRejectsNonReports prevents arbitrary commands, remote resources and absent files.
func TestOpenReportRejectsNonReports(t *testing.T) {
	for _, path := range []string{"https://example.test", `\\server\share\file.html`, `C:\Windows\System32\cmd.exe`, "relative.html", `C:\missing-desktop-diag-report.html`} {
		if err := OpenReport(path); err == nil {
			t.Fatalf("accepted unsafe/nonexistent report %q", path)
		}
	}
}
