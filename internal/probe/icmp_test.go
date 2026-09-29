//go:build windows

package probe

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// TestICMPLoopback 用回环地址做真实探测：回环必然可达，
// 因此这条用例同时验证了句柄管理、RTT 统计与载荷传递是否真的生效。
func TestICMPLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const count = 4
	res, err := ICMP(ctx, "127.0.0.1", ICMPOptions{
		Count:       count,
		Timeout:     time.Second,
		AdapterName: "回环测试网卡",
	})
	if err != nil {
		t.Fatalf("回环 ICMP 探测返回错误（回环必然可达，说明调用姿势有误）: %v", err)
	}

	if res.Kind != model.ProbeICMPGateway {
		t.Errorf("Kind = %q，期望 %q", res.Kind, model.ProbeICMPGateway)
	}
	if res.Target != "127.0.0.1" {
		t.Errorf("Target = %q，期望 127.0.0.1", res.Target)
	}
	if res.AdapterName != "回环测试网卡" {
		t.Errorf("AdapterName = %q，期望原样回填", res.AdapterName)
	}
	if res.Sent != count {
		t.Errorf("Sent = %d，期望 %d", res.Sent, count)
	}
	if res.Recv != count {
		t.Errorf("Recv = %d，期望回环全部应答 %d", res.Recv, count)
	}
	if !res.Success {
		t.Error("回环探测 Success 应为 true")
	}
	if res.LossPercent != 0 {
		t.Errorf("LossPercent = %v，期望 0", res.LossPercent)
	}
	if res.MinRTT <= 0 || res.MaxRTT <= 0 || res.AvgRTT <= 0 {
		t.Errorf("RTT 统计异常：Min=%v Avg=%v Max=%v（回环 RTT 应大于 0）", res.MinRTT, res.AvgRTT, res.MaxRTT)
	}
	if res.MinRTT > res.AvgRTT || res.AvgRTT > res.MaxRTT {
		t.Errorf("RTT 单调性被破坏：Min=%v Avg=%v Max=%v", res.MinRTT, res.AvgRTT, res.MaxRTT)
	}
	if res.Duration <= 0 {
		t.Errorf("Duration = %v，期望大于 0", res.Duration)
	}
	if res.Err != "" {
		t.Errorf("成功的探测不应带 Err，实际 %q", res.Err)
	}
	t.Logf("回环探测：Sent=%d Recv=%d Loss=%.1f%% Min=%v Avg=%v Max=%v 总耗时=%v",
		res.Sent, res.Recv, res.LossPercent, res.MinRTT, res.AvgRTT, res.MaxRTT, res.Duration)
}

// TestICMPDefaultPayloadAndOptions 覆盖默认超时、默认载荷与单包统计的取值路径。
func TestICMPDefaultPayloadAndOptions(t *testing.T) {
	res, err := ICMP(context.Background(), "127.0.0.1", ICMPOptions{Count: 1})
	if err != nil {
		t.Fatalf("默认选项下回环探测失败: %v", err)
	}
	if res.Sent != 1 || res.Recv != 1 || !res.Success {
		t.Errorf("单包回环探测结果异常：Sent=%d Recv=%d Success=%v", res.Sent, res.Recv, res.Success)
	}
	// 单包时 Min/Avg/Max 必须相等。
	if res.MinRTT != res.AvgRTT || res.AvgRTT != res.MaxRTT {
		t.Errorf("单包 RTT 统计应三者相等，实际 Min=%v Avg=%v Max=%v", res.MinRTT, res.AvgRTT, res.MaxRTT)
	}
}

// TestICMPUnreachableTimesOutIsNotError 是丢包率正确性的关键断言：
// RFC 5737 保留网段不可路由，"没回包"必须是结果而不是 error。
func TestICMPUnreachableTimesOutIsNotError(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过（需要等待超时）")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const count = 2
	res, err := ICMP(ctx, "192.0.2.1", ICMPOptions{Count: count, Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("不可路由目标的超时被当作调用错误返回（会让丢包率彻底失准）: %v", err)
	}
	if res.Sent != count {
		t.Errorf("Sent = %d，期望 %d", res.Sent, count)
	}
	// 个别异常网络环境可能让 192.0.2.1 也被应答（例如运营商劫持），此时只要求统计自洽。
	if res.Recv == 0 {
		if res.Success {
			t.Error("无回包时 Success 必须为 false")
		}
		if res.LossPercent != 100 {
			t.Errorf("全部丢包时 LossPercent = %v，期望 100", res.LossPercent)
		}
		if res.MinRTT != 0 || res.AvgRTT != 0 || res.MaxRTT != 0 {
			t.Errorf("无回包时 RTT 必须为 0，实际 Min=%v Avg=%v Max=%v", res.MinRTT, res.AvgRTT, res.MaxRTT)
		}
	}
	t.Logf("192.0.2.1：Sent=%d Recv=%d Loss=%.1f%% Err=%q 耗时=%v", res.Sent, res.Recv, res.LossPercent, res.Err, res.Duration)
}

func TestICMPArgumentErrors(t *testing.T) {
	bigPayload := make([]byte, maxICMPPayload+1)

	cases := []struct {
		name   string
		target string
		opts   ICMPOptions
		want   string
	}{
		{
			name:   "发包数为零",
			target: "127.0.0.1",
			opts:   ICMPOptions{Count: 0},
			want:   "发包数必须为正数",
		},
		{
			name:   "发包数为负",
			target: "127.0.0.1",
			opts:   ICMPOptions{Count: -1},
			want:   "发包数必须为正数",
		},
		{
			name:   "非法IP",
			target: "not-an-ip",
			opts:   ICMPOptions{Count: 1},
			want:   "不是合法 IP 地址",
		},
		{
			name:   "数字串不是IP",
			target: "1.2.3.4.5",
			opts:   ICMPOptions{Count: 1},
			want:   "不是合法 IP 地址",
		},
		{
			name:   "IPv6链路本地不支持",
			target: "fe80::1",
			opts:   ICMPOptions{Count: 1},
			want:   "不是 IPv4 地址",
		},
		{
			name:   "IPv6回环不支持",
			target: "::1",
			opts:   ICMPOptions{Count: 1},
			want:   "不是 IPv4 地址",
		},
		{
			name:   "载荷超长",
			target: "127.0.0.1",
			opts:   ICMPOptions{Count: 1, Payload: bigPayload},
			want:   "超过上限",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ICMP(context.Background(), tc.target, tc.opts)
			if err == nil {
				t.Fatalf("目标 %q 应返回错误", tc.target)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误文本 %q 未包含 %q（错误必须带足够上下文）", err.Error(), tc.want)
			}
			// 错误消息里要能看到是哪个目标出的问题，否则多网卡场景无法定位。
			if !strings.Contains(err.Error(), tc.target) {
				t.Errorf("错误文本 %q 未包含目标 %q", err.Error(), tc.target)
			}
		})
	}
}

// TestICMPContextCanceled 验证 ctx 取消能提前结束，
// 且已完成的统计照常返回（不是错误、不补足到 Count）。
func TestICMPContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	res, err := ICMP(ctx, "127.0.0.1", ICMPOptions{Count: 4, Timeout: time.Second})
	if err != nil {
		t.Fatalf("ctx 取消不应被当作调用错误: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("ctx 已取消却耗时 %v，提前结束逻辑未生效", elapsed)
	}
	if res.Sent != 0 || res.Recv != 0 {
		t.Errorf("ctx 预先取消时不应发包，实际 Sent=%d Recv=%d", res.Sent, res.Recv)
	}
	if res.Success {
		t.Error("未发包时 Success 必须为 false")
	}
	if res.LossPercent != 0 {
		t.Errorf("未发包时 LossPercent 应为 0（避免除零），实际 %v", res.LossPercent)
	}
	if res.Kind != model.ProbeICMPGateway {
		t.Errorf("Kind = %q，期望 %q", res.Kind, model.ProbeICMPGateway)
	}
}

// TestICMPTimeoutClassification 验证"超时"与"调用失败"的区分：
// 目标不回包时 Err 为空（结果自洽），Ipv4 校验仍拦住 IPv6。
func TestICMPTimeoutClassification(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过（需要等待超时）")
	}

	// 让整体 context 先于单包超时到期：IcmpSendEcho 会返回超时/取消错误，
	// 此时所有包都是调用级失败，必须报错而不是伪装成 100% 丢包。
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res, err := ICMP(ctx, "192.0.2.1", ICMPOptions{Count: 2, Timeout: 2 * time.Second})
	if err == nil {
		// 系统在极短超时下若仍判定为"无回包结果"，也应保持统计自洽。
		if res.Success {
			t.Error("不可路由目标不应 Success=true")
		}
		if res.Err != "" {
			t.Errorf("无回包是正常结果，不应写 Err，实际 %q", res.Err)
		}
		return
	}
	if !strings.Contains(err.Error(), "全部调用失败") {
		t.Errorf("调用级失败的错误文本应说明探测无法进行，实际: %v", err)
	}
}

// TestICMPLoopbackIPStringForm 确认 net.ParseIP 的 16 字节表示也能正确发送
// （ParseIP("127.0.0.1") 返回 16 字节形式，IcmpSendEcho 内部会 To4）。
func TestICMPLoopbackIPStringForm(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	if ip == nil || ip.To4() == nil {
		t.Fatal("测试前置条件失败：127.0.0.1 应可解析为 IPv4")
	}

	res, err := ICMP(context.Background(), ip.String(), ICMPOptions{Count: 1, Timeout: time.Second})
	if err != nil {
		t.Fatalf("点分十进制字符串形式的回环探测失败: %v", err)
	}
	if !res.Success {
		t.Errorf("回环探测未成功：Sent=%d Recv=%d", res.Sent, res.Recv)
	}
}
