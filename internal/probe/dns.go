//go:build windows

// Compares system DNS with a direct public resolver without configuration writes.
package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/yukin371/desktop-diag/internal/detect"
	"github.com/yukin371/desktop-diag/internal/model"
)

// defaultDNSTimeout 是未指定超时时的解析上限，与基线 DNSQueryTimeout 一致。
const defaultDNSTimeout = detect.DNSQueryTimeout

// DNSOptions 是一次域名解析探测的参数。
type DNSOptions struct {
	Timeout  time.Duration
	Resolver string // 空 = 系统解析器；否则形如 "223.5.5.5:53" 的直连解析器
}

// DNSKind 报告该解析器配置对应哪一类探测。
func DNSKind(resolver string) model.ProbeKind {
	if resolver == "" {
		return model.ProbeDNSSystem
	}
	return model.ProbeDNSDirect
}

// DNS 解析 domain 并记录地址列表与耗时。
//
// 返回 error 的唯一情形是 domain 为空。**解析失败不是程序错误**：它本身就是一项
// 诊断结论，会写进 ProbeResult（Success=false、Err=原因），error 仍为 nil——
// 把 NXDOMAIN 当 error 会让"DNS 故障"退化成"工具执行失败"。
//
// Resolver 为空时用 Windows 系统解析器；非空时用 PreferGo 解析器直连该地址，
// 绕过系统 DNS 配置（含 DNS 客户端服务）。
func DNS(ctx context.Context, domain string, opts DNSOptions) (model.ProbeResult, error) {
	if domain == "" {
		return model.ProbeResult{}, errors.New("DNS 探测：域名为空")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultDNSTimeout
	}

	result := model.ProbeResult{
		Kind:   DNSKind(opts.Resolver),
		Target: domain,
	}

	// 系统解析栈的等待时间由系统自己决定，只有 context 能约束住它。
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resolver := net.DefaultResolver
	if opts.Resolver != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: timeout}
				return d.DialContext(ctx, "udp", opts.Resolver)
			},
		}
	}

	start := time.Now()
	addrs, err := resolver.LookupHost(ctx, domain)
	result.Duration = time.Since(start)

	if err != nil {
		result.Success = false
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			// 超时与解析失败要能区分：前者可能是 DNS 服务器无响应，后者是明确的应答。
			result.Err = fmt.Sprintf("解析超时（超过 %v）: %v", timeout, err)
		} else {
			result.Err = fmt.Sprintf("解析失败: %v", err)
		}
		return result, nil
	}

	result.Resolved = append(result.Resolved, addrs...)
	result.Success = len(result.Resolved) > 0
	if !result.Success {
		// 无错误却拿不到地址：判定不完整，必须记原因，不能静默当作成功。
		result.Err = "解析未返回任何地址"
	}
	return result, nil
}
