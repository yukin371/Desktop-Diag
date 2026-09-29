//go:build windows

package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// defaultTCPTimeout 是未指定超时时的连接上限，与基线 TCPDialTimeout 一致。
const defaultTCPTimeout = 3 * time.Second

// TCPOptions 是一次 TCP 直连探测的参数。
type TCPOptions struct {
	Timeout time.Duration
}

// TCP 对 target（形如 "223.5.5.5:443"）发起一次 TCP 握手探测。
//
// 返回 error 的唯一情形是 target 为空或缺少端口。**连不通不是程序错误**：拒绝、
// 超时、不可路由都是诊断结论，写进 ProbeResult（Success=false、Err=原因）后
// error 仍为 nil。
//
// 走 net.Dialer 而非 HTTP 客户端：Go 的 net.Dial 不读取 WinINet/系统代理设置，
// 因此这是真正的直连探测——结果若受代理影响，就无法区分"端口被封"与"代理异常"。
func TCP(ctx context.Context, target string, opts TCPOptions) (model.ProbeResult, error) {
	if target == "" {
		return model.ProbeResult{}, errors.New("TCP 探测：目标地址为空")
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return model.ProbeResult{}, fmt.Errorf("TCP 探测：目标 %q 不是合法的 主机:端口 形式: %w", target, err)
	}
	// 主机为空（":443"）无法拨号；端口非数字（"127.0.0.1:abc"）只会得到一个含义
	// 模糊的查找错误。两种情况都在这里提前拒绝，让调用方立刻看出是目标字符串写错了。
	if host == "" {
		return model.ProbeResult{}, fmt.Errorf("TCP 探测：目标 %q 缺少主机地址", target)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return model.ProbeResult{}, fmt.Errorf("TCP 探测：目标 %q 的端口 %q 不是数字", target, port)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTCPTimeout
	}

	result := model.ProbeResult{
		Kind:   model.ProbeTCP443,
		Target: target,
	}

	dialer := net.Dialer{Timeout: timeout}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	result.Duration = time.Since(start)

	if err != nil {
		result.Success = false
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutErr(err) {
			result.Err = fmt.Sprintf("TCP 连接超时（超过 %v）: %v", timeout, err)
		} else {
			result.Err = fmt.Sprintf("TCP 连接失败: %v", err)
		}
		return result, nil
	}
	// 只测握手，不发送任何数据：本工具不得向对端写入内容。
	conn.Close()

	result.Success = true
	return result, nil
}

// isTimeoutErr 判断错误链中是否有超时错误：net.OpError 的超时来自 Dialer.Timeout，
// 不一定会包装 context.DeadlineExceeded。
func isTimeoutErr(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}
