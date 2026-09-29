package model

import "strings"

// Severity 是告警等级。
//
// 取值刻意从 SevOK=0 起算，使「未告警」小于任何真实告警，
// 排序时 Severity 降序即可让 SEVERE 排在最前。
type Severity int

const (
	SevOK Severity = iota
	SevWarning
	SevSevere
)

// String 返回中文等级名。
func (s Severity) String() string {
	switch s {
	case SevSevere:
		return "严重"
	case SevWarning:
		return "警告"
	case SevOK:
		return "正常"
	default:
		return "未知"
	}
}

// Category 是告警分类，也决定第一层汇总中的分组顺序。
type Category string

const (
	CatNetwork Category = "网络"
	CatSystem  Category = "系统"
	CatStorage Category = "存储"
	CatMeta    Category = "诊断完整性"
)

// CategoryOrder 给出分类的固定展示顺序。
// 顺序固定是 REQ-N-08「结论可复现」的一部分：同一份 Snapshot 必须渲染出完全相同的报告。
func CategoryOrder(c Category) int {
	switch c {
	case CatNetwork:
		return 0
	case CatSystem:
		return 1
	case CatStorage:
		return 2
	case CatMeta:
		return 3
	default:
		return 4
	}
}

// Issue 是一条诊断结论（也就是报告第一层的一个条目）。
type Issue struct {
	RuleID     string // "R-01"
	Severity   Severity
	Category   Category
	Title      string   // 第一层单行标题
	Detail     string   // 多行详情
	Evidence   []string // 关联数据，如 "IP: 169.254.13.7 (以太网)"
	Suggestion string   // 排查方向（只读建议，非自动修复）
}

// CountSeverity 统计一组 Issue 中各等级的数量。
func CountSeverity(issues []Issue) (severe, warning int) {
	for _, is := range issues {
		switch is.Severity {
		case SevSevere:
			severe++
		case SevWarning:
			warning++
		}
	}
	return severe, warning
}

// HasSevere 报告是否存在严重告警，决定进程退出码 0 还是 1。
func HasSevere(issues []Issue) bool {
	for _, is := range issues {
		if is.Severity == SevSevere {
			return true
		}
	}
	return false
}

// NormalizeEvidence 去掉证据行中的首尾空白并剔除空行，
// 保证报告渲染时不会出现空的项目符号。
func NormalizeEvidence(evidence []string) []string {
	out := make([]string, 0, len(evidence))
	for _, e := range evidence {
		e = strings.TrimSpace(e)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}
