// 本文件将固定探测目标传给纯逻辑分类层，不执行网络或系统调用。
package probe

import (
	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
)

// Classify 按实际完成的探测证据和完整 TCP 候选列表输出一条层级结论。
func Classify(probes []model.ProbeResult) []model.LayerConclusion {
	return model.ClassifyNetwork(probes, detect.TCP443Targets)
}
