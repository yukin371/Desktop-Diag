//go:build windows

package probe

import "github.com/yukin371/desktop-diag/internal/model"

// Classify 按基线第 6.3 节的判定矩阵，从已完成探测中得出**一条**故障层级结论。
//
// 纯函数：只读入参、不发网络请求、无副作用，因此可全覆盖单测。只返回一条是刻意的：
// 层级结论互斥，同时成立多条会让报告自相矛盾。Skipped 表示"未探测"而非"探测失败"，
// 任何分支都不得把它当作失败证据，否则漏做一项探测就会凭空生成严重告警。
func Classify(probes []model.ProbeResult) []model.LayerConclusion {
	// 同类探测可能有多条（多网卡 ICMP、多目标 TCP），这里只收非 Skipped 的。
	var icmps, sysDNS, directDNS, tcps []model.ProbeResult
	for _, p := range probes {
		if p.Skipped {
			continue
		}
		switch p.Kind {
		case model.ProbeICMPGateway:
			icmps = append(icmps, p)
		case model.ProbeDNSSystem:
			sysDNS = append(sysDNS, p)
		case model.ProbeDNSDirect:
			directDNS = append(directDNS, p)
		case model.ProbeTCP443:
			tcps = append(tcps, p)
		}
	}

	// G：最好的一条网关探测。有回包优先，否则取第一条非 Skipped（失败的）。
	var gateway model.ProbeResult
	if len(icmps) > 0 {
		gateway = icmps[0]
		for _, p := range icmps {
			if p.Success {
				gateway = p
				break
			}
		}
	}

	// S/D：系统 DNS 与直连 DNS 各取首条（正常情况下各只有一条）。
	var systemDNS, direct model.ProbeResult
	if len(sysDNS) > 0 {
		systemDNS = sysDNS[0]
	}
	if len(directDNS) > 0 {
		direct = directDNS[0]
	}

	// TCP 探测可能覆盖多个目标：只要一个握手成功，就说明 443 本身没被全线封锁，
	// 因此按"成功优先"取一条代表。
	tcpProbed := len(tcps) > 0
	tcpOK := false
	for _, p := range tcps {
		if p.Success {
			tcpOK = true
			break
		}
	}

	// 缺失的 DNS 条目是"未探测"而非"解析失败"，把它当失败会凭空造出 DNS 故障告警。
	dnsProbed := len(sysDNS) > 0 || len(directDNS) > 0

	switch {
	case len(icmps) == 0 && len(sysDNS) == 0 && len(directDNS) == 0 && !tcpProbed:
		// 第 1 行：空切片、或全部 Skipped。
		return []model.LayerConclusion{{
			Level:    model.LevelUndetermined,
			Summary:  "探测数据不足，无法确定故障层级",
			Severity: model.SevWarning,
		}}

	case tcpOK && systemDNS.Success && direct.Success && (gateway.Success || len(icmps) == 0):
		// 第 2 行：各层都通。网关无探测数据不阻断本行的 OK 结论——缺一项证据
		// 不足以否定其余全部证据。
		return []model.LayerConclusion{{
			Level:    model.LevelOK,
			Summary:  "网络链路正常",
			Severity: model.SevOK,
		}}

	case len(icmps) > 0 && !gateway.Success && (direct.Success || tcpOK):
		// 第 3 行：网关 ICMP 不通但上层可达 → 大概率是 ICMP 被拦截，不是真断网。
		return []model.LayerConclusion{{
			Level:    model.LevelICMPFiltered,
			Summary:  "网关 ICMP 无响应但上层链路可达，疑似 ICMP 被防火墙或安全软件拦截，并非真实断网",
			Severity: model.SevWarning,
		}}

	case len(icmps) > 0 && !gateway.Success && !direct.Success && !tcpOK:
		// 第 4 行：网关不通且所有上层探测都不通 → 断网就在本地。
		return []model.LayerConclusion{{
			Level:    model.LevelLANDown,
			Summary:  "网关不可达，本地网络中断或网关设备故障",
			Severity: model.SevSevere,
		}}

	case dnsProbed && gateway.Success && !systemDNS.Success && direct.Success:
		// 第 5 行：直连 DNS 能解析、系统 DNS 不能 → 本机 DNS 配置或 DNS 客户端服务异常。
		return []model.LayerConclusion{{
			Level:    model.LevelWANDNS,
			Summary:  "系统 DNS 解析失败但直连 DNS 正常，本机 DNS 配置或 DNS 客户端服务异常",
			Severity: model.SevSevere,
		}}

	case dnsProbed && gateway.Success && !systemDNS.Success && !direct.Success && !tcpOK:
		// 第 6 行（基线矩阵第 3 行：通/失败/失败/失败）：内网与网关都正常，但两种
		// 解析方式都失败 → 故障在出口之外，不是本机 DNS 配置问题。本行须排在第 7 步
		// 之前：DNS 全失败比"443 端口可疑"更根本。
		//
		// `!tcpOK` 不可省略：TCP 443 握手成功即证明 IP 层能到达公网，此时再断言
		// "外网访问中断"就是在推翻自己刚拿到的证据，会把"53 端口被拦截 / 域名解析
		// 故障"误报成严重断网；有 TCP 成功证据时落到第 8 步，如实报"数据不足"。
		return []model.LayerConclusion{{
			Level:    model.LevelWANDown,
			Summary:  "内网链路正常但外网访问中断",
			Severity: model.SevSevere,
		}}

	case (systemDNS.Success || direct.Success) && tcpProbed && !tcpOK:
		// 第 7 步：域名能解析但 443 连不上 → 端口封锁或代理异常。tcpProbed 不可省：
		// 没有 TCP 证据时不能指控"端口不可达"，那属于证据不足（交给第 8 步）。
		return []model.LayerConclusion{{
			Level:    model.LevelWANPort,
			Summary:  "DNS 解析正常但 443 端口不可达，可能存在端口封锁或代理异常",
			Severity: model.SevWarning,
		}}

	default:
		// 第 8 步兜底：证据不足以指向任何具体层级，如实报"无法判定"。
		return []model.LayerConclusion{{
			Level:    model.LevelUndetermined,
			Summary:  "探测数据不足，无法确定故障层级",
			Severity: model.SevWarning,
		}}
	}
}
