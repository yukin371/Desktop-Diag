//go:build windows

// 本文件验证中文详情和未经改写的原始数据附录。
package report

import (
	"strings"
	"testing"

	"github.com/yukin371/desktop-diag/internal/model"
)

// TestChineseReport 验证异常探测中文解释及第三层英文原文保留。
func TestChineseReport(t *testing.T) {
	snap := richSnapshot()
	snap.Probes = []model.ProbeResult{
		{Kind: model.ProbeICMPGateway, Incomplete: true, Sent: 1, Err: "Access is denied."},
		{Kind: model.ProbeDNSSystem, Err: "no such host"},
		{Kind: model.ProbeTCP443, Skipped: true, SkipReason: "context canceled"},
		{Kind: model.ProbeICMPGateway, Success: true, Sent: 4, Recv: 4},
	}
	snap.AddRaw("网络", "fixture", "Sent=1 Scope=Other")
	text := RenderString(snap, nil, RenderContext{})
	for _, want := range []string{"已发送=1 已接收=0", "权限不足（原始信息：Access is denied.）", "域名解析失败", "操作已取消", "最小 0 / 最大 0", "地址范围: 全局", "类型     : 以太网", "Sent=1 Scope=Other"} {
		if !strings.Contains(text, want) {
			t.Errorf("报告缺少 %q", want)
		}
	}
	for _, kind := range []string{model.IfTypeIEEE80211, model.IfTypeTunnel, model.IfTypeLoopback, model.IfTypePPP, model.IfTypeOther} {
		if interfaceTypeLabel(kind) == kind {
			t.Errorf("接口类型未翻译: %s", kind)
		}
	}
	for _, kind := range []string{model.VirtualLoopback, model.VirtualOverlay, model.VirtualOther} {
		if virtualKindLabel(kind) == kind {
			t.Errorf("虚拟类别未翻译: %s", kind)
		}
	}
}
