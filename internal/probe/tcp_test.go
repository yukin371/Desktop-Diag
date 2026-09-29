//go:build windows

package probe

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// TestTCPRefusedIsNotError 覆盖"连不通是诊断结论"这一核心约定：127.0.0.1:1 无服务监听，必然被立刻拒绝。
func TestTCPRefusedIsNotError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := TCP(ctx, "127.0.0.1:1", TCPOptions{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("连接被拒绝被当作程序错误返回（会让端口封锁误报成工具故障）: %v", err)
	}
	if res.Success {
		t.Skip("127.0.0.1:1 竟然有服务监听，跳过本次断言（非本机预期环境）")
	}
	if res.Err == "" {
		t.Error("连接失败时必须写明原因到 Err")
	}
	if res.Kind != model.ProbeTCP443 {
		t.Errorf("Kind = %q，期望 %q", res.Kind, model.ProbeTCP443)
	}
	if res.Target != "127.0.0.1:1" {
		t.Errorf("Target = %q，期望原样回填目标", res.Target)
	}
	if res.Duration <= 0 {
		t.Errorf("Duration = %v，期望大于 0", res.Duration)
	}
	t.Logf("127.0.0.1:1 → Success=%v Err=%q 耗时=%v", res.Success, res.Err, res.Duration)
}

// TestTCPUnroutableTimesOut 用不可路由的保留网段验证超时路径，并确认它与"连接被拒绝"可区分。
func TestTCPUnroutableTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过（需要等待超时）")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := TCP(ctx, "192.0.2.1:443", TCPOptions{Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("连接超时不应被当作程序错误: %v", err)
	}
	if res.Success {
		t.Fatalf("不可路由目标不应连通，Err=%q", res.Err)
	}
	if res.Err == "" {
		t.Fatal("失败时必须写明原因")
	}
	if !strings.Contains(res.Err, "超时") && !strings.Contains(res.Err, "失败") {
		t.Errorf("Err 文本 %q 未说明失败性质", res.Err)
	}
	t.Logf("192.0.2.1:443 → Success=%v Err=%q 耗时=%v", res.Success, res.Err, res.Duration)
}

func TestTCPTargetErrors(t *testing.T) {
	cases := []struct {
		name   string
		target string
		want   string
	}{
		{name: "空目标", target: "", want: "目标地址为空"},
		{name: "缺少端口", target: "127.0.0.1", want: "不是合法的 主机:端口 形式"},
		{name: "缺少主机", target: ":443", want: "缺少主机地址"},
		{name: "端口非数字", target: "127.0.0.1:abc", want: "不是数字"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := TCP(context.Background(), tc.target, TCPOptions{Timeout: time.Second})
			if err == nil {
				t.Fatalf("目标 %q 应返回错误", tc.target)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误文本 %q 未包含 %q", err.Error(), tc.want)
			}
			if tc.target != "" && !strings.Contains(err.Error(), tc.target) {
				t.Errorf("错误文本 %q 未包含目标 %q", err.Error(), tc.target)
			}
			if !reflect.DeepEqual(res, model.ProbeResult{}) {
				t.Errorf("错误路径不应返回半成品结果: %+v", res)
			}
		})
	}
}

// TestTCPContextCanceled 验证 ctx 取消立即返回失败结论而非错误。
func TestTCPContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := TCP(ctx, "192.0.2.1:443", TCPOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("ctx 取消不应被当作程序错误: %v", err)
	}
	if res.Success {
		t.Error("ctx 已取消不应连通成功")
	}
	if res.Err == "" {
		t.Error("失败时必须写明原因")
	}
	if res.Kind != model.ProbeTCP443 {
		t.Errorf("Kind = %q，期望 %q", res.Kind, model.ProbeTCP443)
	}
}

// TestTCPDefaultTimeout 覆盖未指定超时时的默认值路径（目标仍是必定拒绝的地址）。
func TestTCPDefaultTimeout(t *testing.T) {
	res, err := TCP(context.Background(), "127.0.0.1:1", TCPOptions{})
	if err != nil {
		t.Fatalf("默认超时下连接被拒绝不应返回错误: %v", err)
	}
	if res.Success {
		t.Skip("127.0.0.1:1 竟然有服务监听，跳过本次断言（非本机预期环境）")
	}
	if res.Duration > defaultTCPTimeout {
		t.Errorf("耗时 %v 超过默认超时 %v", res.Duration, defaultTCPTimeout)
	}
}
