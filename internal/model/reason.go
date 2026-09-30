// 本文件提供失败原因的中文展示，保留原文且不修改采集证据。
package model

import "strings"

// ChineseReason 为常见底层错误添加中文解释；未知英文错误保留为原始信息。
func ChineseReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" || strings.Contains(reason, "（原始信息：") {
		return reason
	}
	// rules 按具体原因到一般原因匹配，避免把取消误判为超时。
	rules := []struct{ keyword, explanation string }{
		{"access is denied", "访问被拒绝，权限不足"},
		{"access denied", "访问被拒绝，权限不足"},
		{"permission denied", "访问被拒绝，权限不足"},
		{"context canceled", "操作已取消"},
		{"context deadline exceeded", "操作超过允许时间"},
		{"no such host", "域名解析失败，未找到对应主机"},
		{"connection refused", "目标拒绝连接"},
		{"network is unreachable", "网络不可达"},
		{"no route to host", "无可用路由到达目标"},
		{"no space left", "磁盘空间不足"},
		{"cannot find the path", "指定路径不存在"},
		{"cannot find the file", "指定文件不存在"},
		{"timed out", "操作超时"},
		{"timeout", "操作超时"},
	}
	lower := strings.ToLower(reason)
	for _, rule := range rules {
		if strings.Contains(lower, rule.keyword) {
			return rule.explanation + "（原始信息：" + reason + "）"
		}
	}
	for _, char := range reason {
		if char >= '\u4e00' && char <= '\u9fff' {
			return reason
		}
	}
	return "未能完成操作，具体原因请参阅原始信息（原始信息：" + reason + "）"
}

// ChineseEvidence 只解释可识别的错误，普通数值和设备证据保持原样。
func ChineseEvidence(evidence string) string {
	explanation := ChineseReason(evidence)
	if strings.HasPrefix(explanation, "未能完成操作，具体原因请参阅原始信息") {
		return evidence
	}
	return explanation
}
