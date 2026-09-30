// Evaluates pure rules, isolates failures and sorts findings deterministically.
package detect

import (
	"fmt"
	"sort"

	"github.com/yukin371/desktop-diag/internal/model"
)

// Evaluate 对一次采集结果跑完全部规则，返回排序后的告警列表。
//
// 输出顺序完全确定（等级降序 → 分类固定序 → 规则 ID 升序），是 REQ-N-08
// 「结论可复现」的落地：同一台机器两次运行必须能 diff。
func Evaluate(s *model.Snapshot) []model.Issue {
	if s == nil {
		return nil
	}

	var issues []model.Issue
	// 两趟求值：普通规则先跑，Deferred 规则最后跑（理由见 ruleEntry.Deferred）。
	for _, deferred := range []bool{false, true} {
		for _, e := range catalog {
			if e.Deferred != deferred {
				continue
			}
			issues = append(issues, runRule(s, e)...)
		}
	}

	sortIssues(issues)
	return issues
}

// runRule 执行单条规则，并把 panic 转成一条诊断完整性告警。
// 引擎跑在采集末端，这里崩掉会让前面几十秒的采集全部作废，少报一条远好过没有报告。
func runRule(s *model.Snapshot, e ruleEntry) (out []model.Issue) {
	defer func() {
		if r := recover(); r != nil {
			// 记入 Failures，R-19 会把它带进报告第二层，指明是哪条规则出了问题。
			s.AddFailure(
				fmt.Sprintf("判定规则 %s", e.ID),
				"",
				fmt.Sprintf("规则内部错误，该项判定已跳过: %v", r),
				false,
			)
			out = nil
		}
	}()
	return e.Fn(s)
}

// sortIssues 按「等级降序 → 分类固定序 → 规则 ID 升序」排序。
// 末级键不可省：同分类同等级可能有多条告警，否则 sort 的不稳定性会让两次运行顺序不同。
func sortIssues(issues []model.Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if ca, cb := model.CategoryOrder(a.Category), model.CategoryOrder(b.Category); ca != cb {
			return ca < cb
		}
		return a.RuleID < b.RuleID
	})
}
