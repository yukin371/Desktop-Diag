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

func TestDNSKind(t *testing.T) {
	if got := DNSKind(""); got != model.ProbeDNSSystem {
		t.Errorf("DNSKind(\"\") = %q，期望 %q", got, model.ProbeDNSSystem)
	}
	if got := DNSKind("223.5.5.5:53"); got != model.ProbeDNSDirect {
		t.Errorf("DNSKind(\"223.5.5.5:53\") = %q，期望 %q", got, model.ProbeDNSDirect)
	}
}

// TestDNSNonexistentDomainIsNotError 是最关键的一条：
// `.invalid` 是 RFC 2606 保留的必然不存在顶级域，解析失败是**诊断结论**而非程序错误。
func TestDNSNonexistentDomainIsNotError(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过（需要一次真实的解析失败往返）")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := DNS(ctx, "nonexistent.invalid", DNSOptions{Timeout: 8 * time.Second})
	if err != nil {
		t.Fatalf("解析失败被当作程序错误返回（会让 DNS 故障退化成工具执行失败）: %v", err)
	}
	if res.Success {
		t.Fatalf("不存在的域名却解析成功？Resolved=%v", res.Resolved)
	}
	if len(res.Resolved) != 0 {
		t.Errorf("失败时不应有解析结果，实际 %v", res.Resolved)
	}
	if res.Err == "" {
		t.Error("失败时必须写明原因到 Err，否则报告无法显示证据")
	}
	if res.Kind != model.ProbeDNSSystem {
		t.Errorf("Kind = %q，期望 %q", res.Kind, model.ProbeDNSSystem)
	}
	if res.Target != "nonexistent.invalid" {
		t.Errorf("Target = %q，期望原样回填域名", res.Target)
	}
	if res.Duration <= 0 {
		t.Errorf("Duration = %v，期望大于 0", res.Duration)
	}
	t.Logf("nonexistent.invalid → Success=%v Err=%q 耗时=%v", res.Success, res.Err, res.Duration)
}

// TestDNSEmptyDomain 是唯一应当返回 error 的输入。
func TestDNSEmptyDomain(t *testing.T) {
	res, err := DNS(context.Background(), "", DNSOptions{Timeout: time.Second})
	if err == nil {
		t.Fatal("域名为空时应返回错误")
	}
	if !strings.Contains(err.Error(), "域名为空") {
		t.Errorf("错误文本 %q 不够明确", err.Error())
	}
	if !reflect.DeepEqual(res, model.ProbeResult{}) {
		t.Errorf("错误路径不应返回半成品结果: %+v", res)
	}

	// Resolver 非空但域名为空同样应报错，而不是去连解析器。
	if _, err := DNS(context.Background(), "", DNSOptions{Resolver: "223.5.5.5:53"}); err == nil {
		t.Error("Resolver 非空且域名为空时也应返回错误")
	}
}

func TestDNSKindsAndResolvers(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过（需要真实解析器响应）")
	}

	cases := []struct {
		name     string
		resolver string
		wantKind model.ProbeKind
	}{
		{name: "系统解析器", resolver: "", wantKind: model.ProbeDNSSystem},
		{name: "直连解析器", resolver: "223.5.5.5:53", wantKind: model.ProbeDNSDirect},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			res, err := DNS(ctx, "localhost", DNSOptions{Timeout: 5 * time.Second, Resolver: tc.resolver})
			if err != nil {
				t.Fatalf("解析 localhost 返回程序错误: %v", err)
			}
			if res.Kind != tc.wantKind {
				t.Errorf("Kind = %q，期望 %q", res.Kind, tc.wantKind)
			}
			if res.Err != "" {
				t.Errorf("解析 localhost 不应失败，Err=%q", res.Err)
			}
			if !res.Success || len(res.Resolved) == 0 {
				t.Fatalf("localhost 应解析成功，实际 Success=%v Resolved=%v", res.Success, res.Resolved)
			}
			t.Logf("localhost（resolver=%q）→ %v 耗时=%v", tc.resolver, res.Resolved, res.Duration)
		})
	}
}

// TestDNSDirectResolverTimeout 覆盖"解析超时"与"解析失败"的区分：
// 192.0.2.0/24 是不可路由的保留网段，连上去只会超时。
func TestDNSDirectResolverTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过（需要等待超时）")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := DNS(ctx, "nonexistent.invalid", DNSOptions{
		Timeout:  500 * time.Millisecond,
		Resolver: "192.0.2.1:53",
	})
	if err != nil {
		t.Fatalf("解析器无响应不应被当作程序错误: %v", err)
	}
	if res.Success {
		t.Fatalf("不可路由的解析器不可能解析成功，Resolved=%v", res.Resolved)
	}
	if res.Err == "" {
		t.Fatal("失败时必须写明原因")
	}
	// 错误文本要能把"超时"和"NXDOMAIN"区分开，否则报告给不出有效排查方向。
	if !strings.Contains(res.Err, "超时") {
		t.Errorf("解析器无响应时 Err=%q，期望包含「超时」以区别于解析失败", res.Err)
	}
	if res.Kind != model.ProbeDNSDirect {
		t.Errorf("Kind = %q，期望 %q", res.Kind, model.ProbeDNSDirect)
	}
	t.Logf("192.0.2.1:53 → Success=%v Err=%q 耗时=%v", res.Success, res.Err, res.Duration)
}

// TestDNSContextCanceled 验证外部 ctx 取消能中断解析，且同样不算程序错误。
func TestDNSContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := DNS(ctx, "localhost", DNSOptions{Timeout: time.Second})
	if err != nil {
		t.Fatalf("ctx 取消不应被当作程序错误: %v", err)
	}
	if res.Success && len(res.Resolved) == 0 {
		t.Error("Success=true 却没有解析结果")
	}
	if !res.Success && res.Err == "" {
		t.Error("失败时必须写明原因")
	}
}
