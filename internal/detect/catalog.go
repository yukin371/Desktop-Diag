package detect

import (
	"github.com/yukin371/desktop-diag/internal/model"
)

// Rule 是一条判定规则：读入一次采集结果，产出零条或多条告警。
//
// 契约（违反即为缺陷）：
//   - 纯函数：不发起 IO、不修改入参、不读全局可变状态。
//   - 可以返回多条（例如"每块活动网卡各一条"）。
//   - 不得 panic；即便 panic，engine 也会兜住并记入诊断完整性告警。
type Rule func(*model.Snapshot) []model.Issue

// ruleEntry 是规则的注册项。
type ruleEntry struct {
	// ID 是稳定标识（R-01 … R-19），会出现在报告第一层，供运维人员检索。
	ID string
	// Severity 是该规则的固有等级。同一条规则不会产出两种等级 —— 需要分档时
	// 拆成两条规则（例如 R-06/R-07 之于内存），这样报告文案才能各自准确。
	Severity model.Severity
	// Category 决定告警在报告里的分组顺序。
	Category model.Category
	// Title 是第一层显示的一行结论。
	Title string
	// Condition 是人类可读的触发条件，供阶段 6 自动生成规则文档；
	// 它与 Fn 的实际实现必须同步，阶段 4 的元测试会检查二者都存在。
	Condition string
	// Deferred 表示该规则必须延后到其余全部规则求值完毕之后再跑。
	//
	// 目前只有 R-19（诊断完整性）需要它：R-19 读的是 Snapshot.Failures，
	// 而在它之后求值的规则仍可能追加失败项（尤其是被 recover 兜住的规则 panic）。
	// 若 R-19 按注册顺序就求值，这些后到的失败项会被静默漏掉 —— 而"诊断不完整"
	// 恰恰是本工具最不该漏报的一类结论。
	Deferred bool
	Fn       Rule
}

// catalog 保存全部已注册规则，**顺序即求值顺序**。
//
// 之所以用显式注册而不是把规则散在 init() 里：注册表本身就是数据，
// 阶段 4 可以对它做元测试（ID 唯一、等级合法、Condition 非空），
// 阶段 6 可以直接把它渲染成文档，避免"代码改了文档没改"。
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

// Catalog 返回全部规则的元数据副本。
//
// 返回副本而非 catalog 本身：调用方（文档生成器、测试）不应该、
// 也无法通过它改变求值行为。
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
