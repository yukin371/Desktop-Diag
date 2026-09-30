// Package detect 把一次采集得到的 Snapshot 判成一组 Issue。
//
// 本包只依赖 internal/model 与标准库，绝不 import internal/winapi，
// 于是全部规则都能脱离真机、用构造出来的 Snapshot 单测；
// 规则是纯函数：无副作用、不做 IO、不读全局可变状态。
package detect

import (
	"strings"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// 本文件是全项目唯一的阈值来源；规则函数内出现字面量数值即视为缺陷。

const (
	// DiagnosticTimeout/CollectionTimeout 限制总预算与采集时间，给报告写入留出余量。
	DiagnosticTimeout = 30 * time.Second
	CollectionTimeout = 25 * time.Second
	// GatewayWorkers 限制网关并发，防止多网卡串行耗时线性增长。
	GatewayWorkers = 4
	// ICMPMaxPayload 是 Win32 长度参数可表示的最大载荷。
	ICMPMaxPayload = 65535
	// APIPAPrefix 是 DHCP 自动专用地址前缀，出现即表示未取得有效内网 IP；
	// 复用 model 的定义，避免两处判定悄悄分叉。
	APIPAPrefix = model.APIPAPrefix

	// LinkLocalV6Prefix 是 IPv6 链路本地地址前缀，无法路由到本网段之外。
	LinkLocalV6Prefix = "fe80:"
)

const (
	// ICMPPacketCount 是单目标回显请求数；4 个包是采样充分度与 30s 总预算的平衡。
	ICMPPacketCount = 4

	// ICMPTimeout 是单个回显请求的等待时间。
	ICMPTimeout = 1 * time.Second

	// ICMPPayloadSize 是回显载荷的固定字节数；定长可跨机比较，且不含任何终端标识（C-03）。
	ICMPPayloadSize = 32

	// TCPDialTimeout 是单个 TCP 443 拨号的超时。
	TCPDialTimeout = 3 * time.Second

	// DNSQueryTimeout 是单次域名解析的超时。
	DNSQueryTimeout = 3 * time.Second
)

const (
	// ICMPLossWarnPercent：网关丢包率达到此值即告警；100% 由 R-14/R-18 表达。
	ICMPLossWarnPercent = 20.0

	// ICMPLatencyWarnMs：网关平均往返延迟达到此毫秒数即告警。
	ICMPLatencyWarnMs = 150.0

	// MemWarnPercent / MemSeverePercent：内存占用率分档。
	MemWarnPercent   = 85.0
	MemSeverePercent = 95.0

	// CPUWarnPercent / CPUSeverePercent：CPU 占用率分档。
	CPUWarnPercent   = 85.0
	CPUSeverePercent = 95.0

	// CPUSampleWindow 是 CPU 采样窗口（两次 GetSystemTimes 的间隔）；
	// 太短会被瞬时抖动主导，太长会让总耗时逼近 30s 预算。
	CPUSampleWindow = 1 * time.Second

	// DiskWarnFreeBytes / DiskSevereFreeBytes：系统盘剩余空间分档；
	// 按二进制 GiB 计算但显示为 GB，与 Windows 自身的显示口径一致。
	DiskWarnFreeBytes   = 10 * (1 << 30)
	DiskSevereFreeBytes = 5 * (1 << 30)
)

// InvalidDNSServers 是"看起来像地址、实际无法解析域名"的占位值；
// 它们会让 DNS 列表非空、骗过 R-03 的空列表检查，因此必须单独识别（R-04）。
var InvalidDNSServers = []string{"0.0.0.0", "::"}

// DNSProbeDomain 是用于验证解析能力的域名；用真实稳定域名，
// 随机域名会把"域名不存在"误判成"DNS 服务故障"。
var DNSProbeDomain = "www.baidu.com"

// DNSDirectResolver 是绕过本机 DNS 配置、直连的公共解析器，用作系统解析的对照（R-15/R-16）。
var DNSDirectResolver = "223.5.5.5:53"

// TCP443Targets 是验证公网出口可用的候选目标；多目标抗单点，任一可达即认为出口正常。
var TCP443Targets = []string{
	"223.5.5.5:443",
	"223.6.6.6:443",
	"119.29.29.29:443",
}

// IsInvalidDNSServer 报告一个 DNS 地址是否属于无效占位值；
// 容忍首尾空白，因为注册表里的值可能是 "0.0.0.0 " 这样的形式。
func IsInvalidDNSServer(ip string) bool {
	s := strings.TrimSpace(ip)
	for _, bad := range InvalidDNSServers {
		if s == bad {
			return true
		}
	}
	return false
}

// AllDNSInvalid 报告一个非空 DNS 列表是否全部由无效占位值组成；
// 混合列表（如 0.0.0.0 与 8.8.8.8 并存）仍可解析，不算配置异常。
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
