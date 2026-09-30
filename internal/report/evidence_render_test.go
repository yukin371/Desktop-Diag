//go:build windows

// This file checks rendered configuration and probing evidence boundaries.
package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yukin371/desktop-diag/internal/model"
)

// TestUnknownManagementAndAddressDataRemainUnknown prevents fabricated disabled/empty configuration.
func TestUnknownManagementAndAddressDataRemainUnknown(t *testing.T) {
	var out bytes.Buffer
	renderAdapter(&out, 1, model.Adapter{ID: "{fixture}", Name: "LAN", OperStatus: model.OperStatusUp, AddressMissing: true, DNSSource: model.DNSSourceMissing})
	text := out.String()
	for _, want := range []string{"管理状态未知", "地址 API 未提供该接口", "{fixture}", "未采集"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "管理已禁用") || strings.Contains(text, "无 IPv4 地址") {
		t.Fatalf("unknown data fabricated: %s", text)
	}
	out.Reset()
	renderAdapter(&out, 1, model.Adapter{AdminKnown: true, OperStatus: model.OperStatusDown})
	if !strings.Contains(out.String(), "管理已禁用") {
		t.Fatalf("disabled management missing: %s", out.String())
	}
	out.Reset()
	renderProbe(&out, model.ProbeResult{Kind: model.ProbeICMPGateway, AdapterIndex: 7, AdapterName: "LAN", Sent: 1, Incomplete: true, Err: "cancelled"})
	if !strings.Contains(out.String(), "探测不完整") || !strings.Contains(out.String(), "接口 7") {
		t.Fatalf("partial probe identity lost: %s", out.String())
	}
}
