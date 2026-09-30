// Package model 定义 Desktop-Diag 的领域数据结构。
//
// 本包是叶子包：不 import 任何其他 internal 包，也不做 IO 或系统调用，
// 只承载数据与作用于数据的纯函数，因此可以脱离真机在任意平台单测。
// 全部为值语义的普通结构体，无指针网状引用、无接口，可比较、可序列化。
// 本文件承载诊断快照、降级记录与采集对象的派生查询。
package model

import "time"

// Snapshot 是一次完整诊断的全部输入，是 collect → detect → report 之间唯一的传递介质。
type Snapshot struct {
	StartedAt time.Time
	Host      Host
	Adapters  []Adapter
	Health    Health
	Probes    []ProbeResult
	Layers    []LayerConclusion // 由 probe.classify 计算
	Failures  []CollectFailure  // 采集失败清单 → R-19
	Raw       []RawLine         // 第三层附录原始数据
}

// CollectFailure 记录一个采集项的失败或部分失败；任何一条都会触发 R-19，
// 这是「非管理员运行不得静默缺数据」的落地点。
type CollectFailure struct {
	Item    string // 人类可读的采集项名，如 "网卡信息"
	EnvVar  string // 内部标识，如 "network"
	Reason  string // 失败原因原文
	Partial bool   // 是否部分成功（如 3 块网卡中 1 块读取失败）
}

// RawLine 是报告第三层「原始数据附录」的一行；保留原始数据是为了结论与运维
// 经验冲突时可直接核对，而不必二次运行工具。
type RawLine struct {
	Section string // 分节标题
	Source  string // 数据来源，如 "GetAdaptersAddresses" / "HKLM\\...\\Interfaces"
	Line    string // 原始键值对
}

// ActivePhysicalAdapters 返回「真实、已启用、正在工作」的物理网卡，
// 是 R-01 / R-02 / R-03 与连通性探测归属判定的共同输入。
//
// IfType != Loopback 那一条是对虚拟标记漏判的兜底防御，三者缺一不可。
func (s *Snapshot) ActivePhysicalAdapters() []Adapter {
	var out []Adapter
	for _, a := range s.Adapters {
		if !a.IsActive() {
			continue
		}
		if a.IsVirtual {
			continue
		}
		if a.IfType == IfTypeLoopback {
			continue
		}
		out = append(out, a)
	}
	return out
}

// IPv6OnlyLinkLocal 判定 R-05 的触发条件：仅有 IPv6 链路本地地址、无全局 IPv6，且无有效 IPv4。
//
// 无活动物理网卡时返回 false —— 那是无网卡场景，由 R-19 处理。
// 只读取 a.IPv6 的 Scope，因此与 IPv4 的 APIPA 判定（R-01）互不干扰。
func (s *Snapshot) IPv6OnlyLinkLocal() bool {
	adapters := s.ActivePhysicalAdapters()
	if len(adapters) == 0 {
		return false
	}

	hasLinkLocal := false
	for _, a := range adapters {
		if a.AddressMissing {
			return false
		}
		if a.HasUsableIPv4() {
			return false
		}
		for _, addr := range a.IPv6 {
			if addr.Scope != ScopeLinkLocal {
				return false
			}
			hasLinkLocal = true
		}
	}
	return hasLinkLocal
}

// AddRaw 追加一条第三层原始数据。采集器用它记录未经加工的键值对。
func (s *Snapshot) AddRaw(section, source, line string) {
	s.Raw = append(s.Raw, RawLine{Section: section, Source: source, Line: line})
}

// AddFailure 追加一条采集失败记录（R-19 的输入）。
func (s *Snapshot) AddFailure(item, envVar, reason string, partial bool) {
	s.Failures = append(s.Failures, CollectFailure{
		Item:    item,
		EnvVar:  envVar,
		Reason:  reason,
		Partial: partial,
	})
}
