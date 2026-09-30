//go:build windows

// Registers collection domains in their required dependency order.
package collect

// 注册顺序即执行顺序，不能调换：
//  1. 主机与系统信息 —— 报告头需要它，后续采集的失败原因也要靠 IsAdmin 解释。
//  2. 网络适配器信息 —— 判定引擎须先看到 Adapters，才能把探测结果归属到具体网卡。
//  3. 系统健康度 —— CPU 采样独占 1 秒：采样窗口混入探测自身的开销，测出的就是本工具。
//  4. 网络连通性探测 —— 最后执行，它依赖前三者的产出。
//
// 注册集中在这一处，是为了让"顺序"只有一个可改的地方。
func init() {
	Register(systemCollector{})
	Register(networkCollector{})
	Register(healthCollector{})
	Register(probeCollector{})
}
