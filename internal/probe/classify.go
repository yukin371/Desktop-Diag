//go:build windows

package probe

import "github.com/yukin371/desktop-diag/internal/model"

// Classify 按基线第 6.3 节的判定矩阵，从已完成探测中得出**一条**故障层级结论。
//
// 纯函数：只读入参，不发网络请求、无副作用、无全局状态，因此可以 100% 单测覆盖。
//
// 求值顺序（命中即返回）：
//
//  1. 无任何数据（空切片 / 全部 Skipped）        → undetermined，第 7 行「无法判定」
//  2. G通 ∧ S通 ∧ D通 ∧ TCP通（G 可为缺失）      → ok，第 1 行「内网与外网均正常」
//  3. G不通 ∧ (D通 ∨ TCP通)                      → icmp-filtered，第 6 行「内网探测异常」
//  4. G不通 ∧ D不通 ∧ TCP不通                    → lan-down，第 5 行「内网链路异常」
//  5. G通 ∧ S失败 ∧ D成功                       → wan-dns，第 2 行「本机 DNS 故障」
//  6. G通 ∧ S失败 ∧ D失败 ∧ TCP不通              → wan-down，第 3 行「外网异常」
//  7. (S成功 ∨ D成功) ∧ 已做TCP ∧ TCP不通        → wan-port，第 4 行「外网端口受限」
//  8. 其余组合                                   → undetermined，兜底，非矩阵行
//
// 第 3、4、6、7 步只接受"执行过的探测"作为证据：Skipped 或缺失的条目一律视为
// 未探测（于是不满足对应的失败条件），否则漏做一项探测就会凭空生成严重告警。
//
// 之所以只返回一条而不是多条：层级是"故障定位"的互斥结论
// （内网断 / 外网断 / DNS 故障 / 端口封锁），同时成立多条会让报告自相矛盾；
// 值得并列提示的信息已合并进对应行的摘要文案
// （例如第 3 行同时说明了"上层链路可达"，第 5 行同时说明了"直连 DNS 正常"）。
//
// probes 为空、或全部 Skipped 时返回 undetermined：Skipped 表示"未探测"，
// 绝不能当成"探测失败"来告警（model.ProbeResult 的文档明确区分了这两种语义）。
func Classify(probes []model.ProbeResult) []model.LayerConclusion {
	// 先收集各类探测的非 Skipped 结果：同类可能有多条（多网卡 ICMP、多目标 TCP）。
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

	// TCP 探测可能覆盖多个目标：只要有一个目标握手成功，就说明 443 端口本身
	// 不是被全线封锁的，因此按"成功优先"取一条代表。
	tcpProbed := len(tcps) > 0
	tcpOK := false
	for _, p := range tcps {
		if p.Success {
			tcpOK = true
			break
		}
	}

	// 第 5/6 行都建立在"已经做过 DNS 探测"之上：Skipped 或缺失的 DNS 条目
	// 是"未探测"而非"解析失败"，把它当作失败会凭空造出 DNS 故障告警。
	dnsProbed := len(sysDNS) > 0 || len(directDNS) > 0

	switch {
	case len(icmps) == 0 && len(sysDNS) == 0 && len(directDNS) == 0 && !tcpProbed:
		// 第 1 行：三类探测全无数据（空切片、或全部 Skipped）。
		return []model.LayerConclusion{{
			Level:    model.LevelUndetermined,
			Summary:  "探测数据不足，无法确定故障层级",
			Severity: model.SevWarning,
		}}

	case tcpOK && systemDNS.Success && direct.Success && (gateway.Success || len(icmps) == 0):
		// 第 2 行：各层都通。网关无探测数据不阻断本行的 OK 结论——
		// 缺少一项探测不足以否定其余全部证据。
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
		// 第 6 行（基线矩阵第 3 行「通/失败/失败/失败」）：内网与网关都正常，
		// 但两种解析方式都失败 → 故障在出口之外，不是本机 DNS 配置问题。
		// 本行必须排在第 7 行之前：两种 DNS 都失败时，"外网整体异常"是比
		// "443 端口可疑"更根本的结论。
		//
		// `!tcpOK` 不可省略：TCP 443 握手成功等于已经证明 IP 层能到达公网，
		// 此时再断言"外网访问中断"就是在推翻自己刚拿到的证据，会把一次
		// "53 端口被拦截 / 域名解析故障"误报成严重断网。有 TCP 成功证据时
		// 落到第 8 步，如实报"数据不足"。
		return []model.LayerConclusion{{
			Level:    model.LevelWANDown,
			Summary:  "内网链路正常但外网访问中断",
			Severity: model.SevSevere,
		}}

	case (systemDNS.Success || direct.Success) && tcpProbed && !tcpOK:
		// 第 7 步：域名能解析但 443 连不上 → 端口封锁或代理异常。
		// 这里要求 tcpProbed 为真：没有 TCP 证据时不能指控"端口不可达"，
		// 那属于"证据不足"（交给第 8 步），否则会把漏做探测说成端口封锁。
		return []model.LayerConclusion{{
			Level:    model.LevelWANPort,
			Summary:  "DNS 解析正常但 443 端口不可达，可能存在端口封锁或代理异常",
			Severity: model.SevWarning,
		}}

	default:
		// 第 8 步（兜底）：只有系统 DNS 且失败、或网关可通但缺失 DNS 证据等组合。
		// 这些情形都不足以指向任何一个具体层级，如实报"无法判定"。
		return []model.LayerConclusion{{
			Level:    model.LevelUndetermined,
			Summary:  "探测数据不足，无法确定故障层级",
			Severity: model.SevWarning,
		}}
	}
}
