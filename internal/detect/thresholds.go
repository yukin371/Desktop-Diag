// Package detect 实现判定引擎：把一次采集得到的 Snapshot 变成一组 Issue。
//
// 设计约束（基线 REQ-N-08 可复现性）：
//   - 本包**只**依赖 internal/model 与标准库，绝不 import internal/winapi。
//     这样全部规则都能脱离真机、用构造出来的 Snapshot 做单元测试。
//   - 规则是纯函数：无副作用、不做 IO、不读全局可变状态。
//   - 所有数值集中在 thresholds.go，规则函数内**不得出现魔法数字**。
package detect

import (
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// 本文件是全项目**唯一**的阈值来源（基线第 6.2 节「阈值集中管理」）。
//
// 任何规则函数里出现字面量数值都视为缺陷：阶段 4 的评审会逐条核对，
// 因为散落的魔法数字会让"改阈值"变成一场寻宝，也让人无法一眼看出
// 报告里的判定标准到底是什么。

// ── 网络识别前缀 ──────────────────────────────────────────────

const (
	// APIPAPrefix 是 DHCP 自动专用地址的前缀，出现即表示未取得有效内网 IP。
	//
	// 直接复用 model 里的定义：那里是判断 Addr.IsAPIPA() 的依据，
	// 两处各写一份会在某天悄悄分叉。
	APIPAPrefix = model.APIPAPrefix

	// LinkLocalV6Prefix 是 IPv6 链路本地地址前缀，无法路由到本网段之外。
	LinkLocalV6Prefix = "fe80:"
)

// ── 探测参数 ──────────────────────────────────────────────────

const (
	// ICMPPacketCount 是单个目标的回显请求数量。
	//
	// 4 个包是为了在"采样太少先验不足"与"耗时太长"之间取平衡：
	// 每包 1s 超时，最坏情况单目标 4s，4 个探测目标合计仍在 30s 预算内。
	ICMPPacketCount = 4

	// ICMPTimeout 是单个回显请求的等待时间。
	ICMPTimeout = 1 * time.Second

	// ICMPPayloadSize 是回显载荷的固定字节数。
	//
	// 固定长度让不同机器的结果可比；且载荷是常量、**不含任何终端标识**，
	// 满足基线 C-03「零上传、不带终端身份」。
	ICMPPayloadSize = 32

	// TCPDialTimeout 是单个 TCP 443 拨号的超时。
	TCPDialTimeout = 3 * time.Second

	// DNSQueryTimeout 是单次域名解析的超时。
	DNSQueryTimeout = 3 * time.Second
)

// ── 告警阈值 ──────────────────────────────────────────────────

const (
	// ICMPLossWarnPercent：网关丢包率达到此值即告警（未达 100% 时）。
	ICMPLossWarnPercent = 20.0

	// ICMPLatencyWarnMs：网关平均往返延迟达到此毫秒数即告警。
	ICMPLatencyWarnMs = 150.0

	// MemWarnPercent / MemSeverePercent：内存占用率分档。
	MemWarnPercent   = 85.0
	MemSeverePercent = 95.0

	// CPUWarnPercent / CPUSeverePercent：CPU 占用率分档。
	CPUWarnPercent   = 85.0
	CPUSeverePercent = 95.0

	// CPUSampleWindow 是 CPU 占用率的采样窗口（两次 GetSystemTimes 的间隔）。
	//
	// 1 秒是权衡结果：太短会被瞬时抖动主导，太长会让总耗时逼近 30s 预算。
	CPUSampleWindow = 1 * time.Second

	// DiskWarnFreeBytes / DiskSevereFreeBytes：系统盘剩余空间分档。
	//
	// 按二进制 GiB（1024³）计算，但报告里显示为 GB —— 这与 Windows 自身的
	// 显示习惯一致，避免运维人员看到"与资源管理器不一致的数字"。
	DiskWarnFreeBytes   = 10 * (1 << 30)
	DiskSevereFreeBytes = 5 * (1 << 30)
)

// ── 探测目标 ──────────────────────────────────────────────────

// InvalidDNSServers 是"看起来像地址、实际无法解析域名"的占位值。
//
// 它们会被误当成配置完整的 DNS（列表非空），从而骗过"DNS 列表为空"的检查，
// 所以必须单独识别（R-04）。
var InvalidDNSServers = []string{"0.0.0.0", "127.0.0.1"}

// DNSProbeDomain 是用于验证解析能力的域名。
//
// 选 www.baidu.com 的理由：国内任意网络环境下都应能解析，且结果稳定；
// 用一个随机域名会把"域名不存在"误判成"DNS 服务故障"。
var DNSProbeDomain = "www.baidu.com"

// DNSDirectResolver 是绕过本机 DNS 配置、直连的公共解析器。
//
// 它的作用是与系统解析结果做**对照**：系统失败而它成功，
// 说明外网链路是通的，问题出在本机 DNS 配置或 DNS 客户端服务上（R-15）。
var DNSDirectResolver = "223.5.5.5:53"

// TCP443Targets 是验证公网出口可用的候选目标。
//
// 多个目标是为了抗单点：任一目标可达即认为出口正常。
// 全部是国内公共 DNS 的 443 端口，实测可达且长期稳定（阶段 1 已验证
// 223.5.5.5:443 在本机 34–36ms 可达）。
var TCP443Targets = []string{
	"223.5.5.5:443",
	"223.6.6.6:443",
	"119.29.29.29:443",
}

// ── 辅助判定 ──────────────────────────────────────────────────

// IsInvalidDNSServer 报告一个 DNS 地址是否属于无效占位值。
//
// 大小写与空白都要容忍：注册表里的值可能是 "0.0.0.0 " 这样的形式。
func IsInvalidDNSServer(ip string) bool {
	s := strings.TrimSpace(ip)
	for _, bad := range InvalidDNSServers {
		if s == bad {
			return true
		}
	}
	return false
}

// AllDNSInvalid 报告一个非空 DNS 列表是否**全部**由无效占位值组成。
//
// 依据基线 R-04 的措辞「DNS 仅含 0.0.0.0 或仅含 127.0.0.1」：
// 混合列表（例如 0.0.0.0 与 8.8.8.8 并存）仍然可解析，不算配置异常。
func AllDNSInvalid(dns []string) bool {
	if len(dns) == 0 {
		return false
	}
	for _, ip := range dns {
		if !IsInvalidDNSServer(ip) {
			return false
		}
	}
	return true
}
