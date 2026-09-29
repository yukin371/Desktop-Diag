package detect

import (
	"fmt"
	"sort"

	"github.com/yukin371/desktop-diag/internal/model"
)

// Evaluate 对一次采集结果跑完全部规则，返回排序后的告警列表。
//
// 输出顺序是完全确定的（等级降序 → 分类固定序 → 规则 ID 升序），
// 这是基线 REQ-N-08「结论可复现」的具体落地：同一台机器连续跑两次，
// 报告第一层的行序必须一致，否则运维人员无法用 diff 对比两次诊断。
func Evaluate(s *model.Snapshot) []model.Issue {
	if s == nil {
		return nil
	}

	var issues []model.Issue
	// 两趟求值：先跑普通规则，再跑 Deferred 规则。
	// 理由见 ruleEntry.Deferred —— 结算"诊断是否完整"必须等所有规则都跑完。
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

// runRule 执行单条规则，并把 panic 转化成一条诊断完整性告警。
//
// 为什么规则已经要求"不得 panic"还要兜这一层：判定引擎跑在采集流程的末端，
// 一旦这里崩掉，前面几十秒的采集结果就全废了，用户拿到的是崩溃而不是报告。
// 诊断工具"少报一条"远比"整个跑不出来"可接受。
func runRule(s *model.Snapshot, e ruleEntry) (out []model.Issue) {
	defer func() {
		if r := recover(); r != nil {
			// 记入 Snapshot 的采集失败清单，于是 R-19 会自动把它带进报告第二层，
			// 用户能看到究竟是哪条规则出了问题，而不是一个静默消失的结论。
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
//
// 规则 ID 作为最后的比较键不可省略：同一分类下可能有多条同等级告警
// （例如多块网卡各触发一次 R-01），没有这个键，sort.Slice 的不稳定性
// 会让两次运行的顺序不同。
func sortIssues(issues []model.Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity // SEVERE 在前
		}
		if ca, cb := model.CategoryOrder(a.Category), model.CategoryOrder(b.Category); ca != cb {
			return ca < cb
		}
		return a.RuleID < b.RuleID
	})
}
