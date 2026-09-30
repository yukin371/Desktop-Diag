// 本文件验证中文解释与原始证据保留规则。
package model

import (
	"strings"
	"testing"
)

// TestChineseReason 验证常见错误、大小写、未知原因和重复渲染。
func TestChineseReason(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"Access is denied.", "权限不足"}, {"PERMISSION DENIED", "权限不足"},
		{"context canceled", "操作已取消"}, {"context deadline exceeded", "超过允许时间"},
		{"lookup example.com: no such host", "域名解析失败"}, {"connection refused", "拒绝连接"},
		{"network is unreachable", "网络不可达"}, {"no route to host", "无可用路由"},
		{"no space left on device", "磁盘空间不足"}, {"cannot find the path", "路径不存在"},
		{"cannot find the file", "文件不存在"}, {"request timed out", "操作超时"},
		{"i/o timeout", "操作超时"}, {"unexpected failure", "具体原因"},
	} {
		got := ChineseReason(tc.raw)
		if !strings.Contains(got, tc.want) || !strings.Contains(got, tc.raw) {
			t.Errorf("%q => %q", tc.raw, got)
		}
		if ChineseReason(got) != got {
			t.Errorf("重复渲染改变原文: %q", got)
		}
	}
	for _, raw := range []string{"", "未采集", "IP=192.168.1.1", "Intel Ethernet"} {
		if ChineseEvidence(raw) != raw {
			t.Errorf("普通证据被改写: %q", raw)
		}
	}
}
