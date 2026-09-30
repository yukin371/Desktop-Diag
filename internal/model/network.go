// 本文件实现不依赖操作系统的探测对象选择与连通性分类。
package model

// ProbeAdapters 优先选择活动物理接口；仅无物理接口时接受有默认路由的虚拟机或 VPN 接口。
func (s *Snapshot) ProbeAdapters() []Adapter {
	// adapters 保持快照顺序，避免并发探测改变报告排序。
	adapters := s.ActivePhysicalAdapters()
	if len(adapters) > 0 {
		return adapters
	}
	for _, a := range s.Adapters {
		if !a.IsActive() || !a.HasGateway() || a.IfType == IfTypeLoopback || a.VirtualKind == VirtualOverlay || a.VirtualKind == VirtualOther {
			continue
		}
		adapters = append(adapters, a)
	}
	return adapters
}

// ClassifyNetwork 按完整探测证据分类；结论仅覆盖目标可达性，不宣称所有接口均正常。
func ClassifyNetwork(probes []ProbeResult, tcpTargets []string) []LayerConclusion {
	// gateway/system/direct/tcp 保留未执行与执行失败的差异。
	gateway := AssessProbes(probes, ProbeICMPGateway, nil)
	system := AssessProbes(probes, ProbeDNSSystem, nil)
	direct := AssessProbes(probes, ProbeDNSDirect, nil)
	tcp := AssessProbes(probes, ProbeTCP443, tcpTargets)
	// result 缺省为证据不足；成功只能证明对应协议和目标。
	result := LayerConclusion{Level: LevelUndetermined, Summary: "探测数据不足，无法确定故障层级", Severity: SevWarning}
	switch {
	case gateway.Complete && gateway.Success && system.Success && direct.Success && tcp.Success:
		result = LayerConclusion{LevelOK, "网关、DNS 与公网 TCP 测试均存在成功证据；实际出接口由系统路由决定，不能推断每个接口均正常", SevOK}
	case gateway.Complete && !gateway.Success && (system.Success || direct.Success || tcp.Success):
		result = LayerConclusion{LevelICMPFiltered, "网关 ICMP 无响应但上层探测有成功证据，可能为 ICMP 策略限制；不能推断每个接口均正常", SevWarning}
	case gateway.Complete && !gateway.Success && system.Complete && !system.Success && direct.Complete && !direct.Success && tcp.Complete && !tcp.Success:
		result = LayerConclusion{LevelLANDown, "已探测网关及 DNS/TCP 目标均无成功响应，疑似本地链路或出口策略异常", SevSevere}
	case system.Complete && !system.Success && direct.Success:
		result = LayerConclusion{LevelWANDNS, "系统 DNS 未能解析测试域名，而直连 DNS 成功；请检查本机 DNS 配置或企业策略", SevSevere}
	case gateway.Complete && gateway.Success && system.Complete && !system.Success && direct.Complete && !direct.Success && tcp.Complete && !tcp.Success:
		result = LayerConclusion{LevelWANDown, "已探测网关可达，但测试域名及公网 TCP 候选均不可达；请检查出口策略与目标服务", SevSevere}
	case gateway.Complete && gateway.Success && (system.Success || direct.Success) && tcp.Complete && !tcp.Success:
		result = LayerConclusion{LevelWANPort, "DNS 测试有成功证据，但全部公网 TCP 443 候选直连失败；可能受代理或出口策略限制", SevWarning}
	}
	return []LayerConclusion{result}
}
