//go:build windows

// Package probe 实现只读的连通性探测：网关 ICMP、系统/直连 DNS 解析、公网 TCP 443 直连。
//
// 只发探测包与解析请求，不改动任何系统状态。
package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// defaultICMPTimeout 是未指定超时时的每包等待上限：网关在本地链路，1 秒足够
// 区分"网关没回包"与"网关慢"，又不会把探测总时长拖长。
const defaultICMPTimeout = detect.ICMPTimeout

// maxICMPPayload 是固定载荷的硬上限：IcmpSendEcho 的长度参数是 uint16，
// 超过 65535 会在转换时静默截断，导致回复缓冲区与实际请求长度不匹配。
const maxICMPPayload = detect.ICMPMaxPayload

// defaultICMPPayload 是未指定载荷时使用的 32 字节固定载荷，内容为不含任何
// 终端标识信息的常量（基线 C-03），固定长度也让不同机器的结果可比。
var defaultICMPPayload = []byte("Desktop-Diag ICMP probe payload!")

// ICMPOptions 是一次网关 ICMP 探测的参数。
type ICMPOptions struct {
	Count        int           // 发包数
	Timeout      time.Duration // 每包超时
	Payload      []byte        // 固定载荷
	AdapterName  string        // 仅回填 ProbeResult.AdapterName；无法指定出接口
	AdapterIndex uint32        // 回填接口索引，避免同名网卡混淆。
}

// ICMP 向 target 发送 Count 个 ICMP 回显请求并统计丢包与 RTT。
//
// 返回的 error 只在"这次探测根本无法进行"时非 nil：句柄创建失败、target 不是
// 合法 IPv4 地址、Count<=0、载荷超长。**单包超时不是 error**——目标没回包是一次
// 成功的调用结果，把它当 error 会让丢包率彻底失准。
//
// AdapterName 只用于结果归属标注：IcmpSendEcho 由系统按路由表选路，无法指定出接口。
func ICMP(ctx context.Context, target string, opts ICMPOptions) (result model.ProbeResult, err error) {
	return icmpWith(ctx, target, opts, icmpAPI{winapi.IcmpCreateFile, winapi.IcmpSendEcho, winapi.IcmpCloseHandle})
}

// icmpAPI 注入句柄和单包调用，以固定输入验证预算、取消与部分失败。
type icmpAPI struct {
	open  func() (uintptr, error)
	send  func(uintptr, net.IP, []byte, time.Duration) (winapi.IcmpEchoResult, error)
	close func(uintptr) error
}

// icmpWith 只统计真正发出的请求，保留部分采样与句柄释放错误。
func icmpWith(ctx context.Context, target string, opts ICMPOptions, api icmpAPI) (result model.ProbeResult, err error) {
	start := time.Now()

	result = model.ProbeResult{
		Kind:         model.ProbeICMPGateway,
		Target:       target,
		AdapterName:  opts.AdapterName,
		AdapterIndex: opts.AdapterIndex,
	}

	if opts.Count <= 0 {
		return model.ProbeResult{}, fmt.Errorf("ICMP 探测 %q：发包数必须为正数，实际为 %d", target, opts.Count)
	}

	// 先做地址校验再开句柄：参数错了就不该占用系统资源。
	ip := net.ParseIP(target)
	if ip == nil {
		return model.ProbeResult{}, fmt.Errorf("ICMP 探测：目标 %q 不是合法 IP 地址", target)
	}
	if ip.To4() == nil {
		return model.ProbeResult{}, fmt.Errorf("ICMP 探测：目标 %q 不是 IPv4 地址；IcmpSendEcho 不支持 IPv6", target)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultICMPTimeout
	}

	payload := opts.Payload
	if len(payload) == 0 {
		payload = defaultICMPPayload
	}
	if len(payload) > maxICMPPayload {
		return model.ProbeResult{}, fmt.Errorf("ICMP 探测 %q：载荷 %d 字节超过上限 %d", target, len(payload), maxICMPPayload)
	}

	if ctx.Err() != nil {
		result.Skipped, result.Incomplete = true, true
		result.Err, result.SkipReason = ctx.Err().Error(), ctx.Err().Error()
		return result, nil
	}
	handle, err := api.open()
	if err != nil {
		return model.ProbeResult{}, fmt.Errorf("ICMP 探测 %q：打开 ICMP 句柄失败: %w", target, err)
	}
	// 句柄释放失败保留在 error 链中，由采集层记录诊断不完整。
	defer func() {
		if closeErr := api.close(handle); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("关闭 ICMP 句柄失败: %w", closeErr))
		}
	}()

	var rtts []time.Duration
	callFailures := 0
	var lastCallErr error
	// attempted 统计调用次数，调用失败不作为真正发送的网络包。
	attempted := 0

	for i := 0; i < opts.Count; i++ {
		// ctx 已取消/超时 → 提前结束，把已完成的统计如实返回，不补足到 Count。
		if ctx.Err() != nil {
			break
		}

		// packetTimeout 不超过剩余预算；同步调用最多额外等待当前这一包。
		packetTimeout := timeout
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < packetTimeout {
			packetTimeout = time.Until(deadline)
			if packetTimeout <= 0 {
				break
			}
		}
		attempted++
		echo, echoErr := api.send(handle, ip, payload, packetTimeout)
		if echoErr != nil {
			callFailures++
			lastCallErr = echoErr
			continue
		}
		result.Sent++

		// Replied 为假（典型是超时，也可能是目标不可达）只意味着没收到回包。
		if echo.Replied() {
			result.Recv++
			rtts = append(rtts, time.Duration(echo.RoundTripTime)*time.Millisecond)
		}
	}

	// 每个发出去的包都是调用级失败 → 这是"探测无法进行"，必须报错，否则句柄失效
	// 会被读成"网关不回包"，产生误导性的严重告警（对应 R-18）。
	if attempted > 0 && callFailures == attempted {
		return model.ProbeResult{}, fmt.Errorf("ICMP 探测 %q：%d 个包全部调用失败，探测无法进行: %w",
			target, callFailures, lastCallErr)
	}

	result.Duration = time.Since(start)
	result.Incomplete = callFailures > 0 || result.Sent < opts.Count
	if result.Incomplete {
		if cause := ctx.Err(); cause != nil {
			result.Err = cause.Error()
		} else if lastCallErr != nil {
			result.Err = lastCallErr.Error()
		} else {
			result.Err = "ICMP 采样未完成"
		}
	}
	if result.Sent == 0 {
		result.Skipped = true
		result.SkipReason = result.Err
	}

	// LossPercent 只在确实发过包时有意义；ctx 立即取消时 Sent=0，避免除零。
	if result.Sent > 0 {
		result.LossPercent = float64(result.Sent-result.Recv) / float64(result.Sent) * 100
	}

	// 无回包时 Min/Avg/Max 保持为 0，调用方应以 Recv 判断统计是否有效。
	if len(rtts) > 0 {
		sum := time.Duration(0)
		result.MinRTT = rtts[0]
		result.MaxRTT = rtts[0]
		for _, rtt := range rtts {
			if rtt < result.MinRTT {
				result.MinRTT = rtt
			}
			if rtt > result.MaxRTT {
				result.MaxRTT = rtt
			}
			sum += rtt
		}
		result.AvgRTT = sum / time.Duration(len(rtts))
	}

	result.Success = result.Recv > 0
	return result, nil
}
