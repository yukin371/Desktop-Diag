package detect

import (
	"github.com/yukin371/desktop-diag/internal/model"
)

// Rule 是一条判定规则：读入一次采集结果，产出零条或多条告警。
//
// 契约：纯函数（不发起 IO、不改入参、不读全局可变状态）；不得 panic ——
// 即便 panic，engine 也会兜住并记入诊断完整性告警。
type Rule func(*model.Snapshot) []model.Issue

// ruleEntry 是规则的注册项。
type ruleEntry struct {
	// ID 是稳定标识（R-01 … R-19），会出现在报告第一层供运维检索。
	ID string
	// Severity 是该规则的固有等级；需要分档时拆成两条规则（如 R-06/R-07），
	// 这样报告文案才能各自准确。
	Severity model.Severity
	// Category 决定告警在报告里的分组顺序。
	Category model.Category
	// Title 是第一层显示的一行结论。
	Title string
	// Condition 是人类可读的触发条件，供阶段 6 生成规则文档；必须与 Fn 同步。
	Condition string
	// Deferred 表示该规则必须等其余全部规则求值完毕之后再跑。
	//
	// 只有 R-19（诊断完整性）需要它：R-19 读 Snapshot.Failures，而排在它后面的
	// 规则仍可能追加失败项（含被 recover 兜住的 panic），按注册顺序求值会静默漏报。
	Deferred bool
	Fn       Rule
}

// catalog 保存全部已注册规则，顺序即求值顺序。
// 用显式注册而不是散在 init() 里，是为了让注册表本身可被元测试与文档生成消费。
var catalog []ruleEntry

func register(e ruleEntry) {
	catalog = append(catalog, e)
}

// RuleInfo 是规则的只读视图，供元测试与文档生成使用。
type RuleInfo struct {
	ID        string
	Title     string
	Condition string
	Severity  model.Severity
	Category  model.Category
}

// Catalog 返回全部规则的元数据副本；调用方（文档生成器、测试）不得也无法靠它改变求值行为。
func Catalog() []RuleInfo {
	out := make([]RuleInfo, 0, len(catalog))
	for _, e := range catalog {
		out = append(out, RuleInfo{
			ID:        e.ID,
			Title:     e.Title,
			Condition: e.Condition,
			Severity:  e.Severity,
			Category:  e.Category,
		})
	}
	return out
}
