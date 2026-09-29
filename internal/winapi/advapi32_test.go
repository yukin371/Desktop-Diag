//go:build windows

package winapi

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// 本文件含纯函数测试与真机注册表测试；真机部分只读，且只碰必然存在的系统键。

func TestPureParseNameServerList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"空串", "", nil},
		{"纯空白", "   \t\n ", nil},
		{"单个 IPv4", "8.8.8.8", []string{"8.8.8.8"}},
		{"逗号分隔", "8.8.8.8,1.1.1.1", []string{"8.8.8.8", "1.1.1.1"}},
		{"逗号加空格", "8.8.8.8, 1.1.1.1", []string{"8.8.8.8", "1.1.1.1"}},
		{"空格分隔", "8.8.8.8 1.1.1.1", []string{"8.8.8.8", "1.1.1.1"}},
		{"分号分隔", "8.8.8.8;1.1.1.1", []string{"8.8.8.8", "1.1.1.1"}},
		{"尾随分隔符", "8.8.8.8,", []string{"8.8.8.8"}},
		{"重复分隔符", "8.8.8.8,,,1.1.1.1", []string{"8.8.8.8", "1.1.1.1"}},
		// IPv6 含冒号，绝不能被当成分隔符切开。
		{"IPv6 不得被冒号截断", "2001:4860:4860::8888", []string{"2001:4860:4860::8888"}},
		{"IPv4 与 IPv6 混合", "8.8.8.8,2001:4860:4860::8888", []string{"8.8.8.8", "2001:4860:4860::8888"}},
		{"含制表符", "8.8.8.8\t1.1.1.1", []string{"8.8.8.8", "1.1.1.1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseNameServerList(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseNameServerList(%q) = %v，期望 %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseNameServerList(%q)[%d] = %q，期望 %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPureDropEmptyStrings(t *testing.T) {
	in := []string{"a", "", "  ", "b", "\t"}
	got := dropEmptyStrings(in)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("dropEmptyStrings = %v，期望 [a b]", got)
	}
	// 必须是新切片：返回入参的别名会让调用方后续 append 污染原切片。
	if len(in) != 5 {
		t.Errorf("dropEmptyStrings 不应修改入参切片，实际长度变成了 %d", len(in))
	}
}

func TestRuntimeRegWindowsVersion(t *testing.T) {
	name, err := RegReadString64(registry.LOCAL_MACHINE, RegPathWindowsVersion, RegValueProductName)
	if err != nil {
		t.Fatalf("读取 ProductName 失败: %v", err)
	}
	if strings.TrimSpace(name) == "" {
		t.Error("ProductName 为空，OS 显示名将无法写入报告")
	}
	t.Logf("ProductName = %q", name)

	// DisplayVersion 在部分精简版系统上可能缺失；一旦存在就必须是非空文本。
	if v, err := RegReadString64(registry.LOCAL_MACHINE, RegPathWindowsVersion, RegValueDisplayVersion); err == nil {
		if strings.TrimSpace(v) == "" {
			t.Error("DisplayVersion 存在但为空")
		}
		t.Logf("DisplayVersion = %q", v)
	}

	build, err := RegReadString64(registry.LOCAL_MACHINE, RegPathWindowsVersion, RegValueCurrentBuild)
	if err != nil {
		t.Fatalf("读取 CurrentBuild 失败: %v", err)
	}
	t.Logf("CurrentBuild = %q", build)
}

// TestRuntimeRegNotFound 验证"不存在"能被精确识别：把"读取失败"误当成"值为空"会让 DNS 告警静默漏报。
func TestRuntimeRegNotFound(t *testing.T) {
	_, err := RegReadString(registry.LOCAL_MACHINE, RegPathWindowsVersion, "DesktopDiagNoSuchValue")
	if err == nil {
		t.Fatal("读取不存在的值名应当返回错误")
	}
	if !errors.Is(err, registry.ErrNotExist) {
		t.Errorf("错误应可用 errors.Is(err, registry.ErrNotExist) 匹配，实际 %v", err)
	}
	if !errors.Is(err, ErrRegNotFound) {
		t.Errorf("错误应可用 errors.Is(err, ErrRegNotFound) 匹配，实际 %v", err)
	}

	_, err = RegReadString(registry.LOCAL_MACHINE, `SOFTWARE\DesktopDiagNoSuchVendor\NoSuchProduct`, "x")
	if err == nil {
		t.Fatal("读取不存在的键应当返回错误")
	}
	if !errors.Is(err, ErrRegNotFound) {
		t.Errorf("不存在的键应匹配 ErrRegNotFound，实际 %v", err)
	}

	if RegKeyExists(registry.LOCAL_MACHINE, `SOFTWARE\DesktopDiagNoSuchVendor\NoSuchProduct`) {
		t.Error("RegKeyExists 对不存在的键应返回 false")
	}
	if !RegKeyExists(registry.LOCAL_MACHINE, RegPathWindowsVersion) {
		t.Error("RegKeyExists 对必然存在的系统键应返回 true")
	}
}

// TestRuntimeRegEnumTcpipInterfaces 验证枚举链路：子键名就是网卡 {GUID}，collect 靠它与 AdapterName 配对。
func TestRuntimeRegEnumTcpipInterfaces(t *testing.T) {
	names, err := RegEnumSubKeys(registry.LOCAL_MACHINE, RegPathTcpipInterfaces)
	if err != nil {
		t.Fatalf("枚举网卡接口键失败: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("网卡接口键下没有任何子键，这与任何一台真实机器都不符")
	}

	guidCount := 0
	for _, n := range names {
		if strings.HasPrefix(n, "{") && strings.HasSuffix(n, "}") {
			guidCount++
		}
	}
	t.Logf("接口子键 %d 个，其中 {GUID} 形式 %d 个", len(names), guidCount)
	if guidCount == 0 {
		t.Error("没有任何 {GUID} 形式的子键，子键名解析方式可能有误")
	}
}

// TestRuntimeRegReadFirstString 验证兜底链：静态 DNS 优先，缺失时回落到下一个候选。
func TestRuntimeRegReadFirstString(t *testing.T) {
	// 第一个候选必然不存在，应当回落到 ProductName。
	used, val, err := RegReadFirstString(registry.LOCAL_MACHINE, RegPathWindowsVersion,
		"DesktopDiagNoSuchValue", RegValueProductName)
	if err != nil {
		t.Fatalf("RegReadFirstString 兜底失败: %v", err)
	}
	if used != RegValueProductName {
		t.Errorf("应回落到 %s，实际用了 %s", RegValueProductName, used)
	}
	if strings.TrimSpace(val) == "" {
		t.Error("回落得到的值不应为空")
	}

	// 全部候选都不存在时必须是 ErrRegNotFound，而不是返回空串的成功结果。
	if _, _, err := RegReadFirstString(registry.LOCAL_MACHINE, RegPathWindowsVersion,
		"DesktopDiagNoSuchValue", "DesktopDiagNoSuchValue2"); !errors.Is(err, ErrRegNotFound) {
		t.Errorf("全部候选缺失时应返回 ErrRegNotFound，实际 %v", err)
	}

	// 不提供任何候选 → 参数错误。
	if _, _, err := RegReadFirstString(registry.LOCAL_MACHINE, RegPathWindowsVersion); err == nil {
		t.Error("未提供候选值名时应返回错误")
	}

	// 键本身不存在 → 报打开失败，且可匹配 ErrRegNotFound。
	if _, _, err := RegReadFirstString(registry.LOCAL_MACHINE,
		`SOFTWARE\DesktopDiagNoSuchVendor\NoSuchProduct`, "NameServer"); !errors.Is(err, ErrRegNotFound) {
		t.Errorf("键不存在时应匹配 ErrRegNotFound，实际 %v", err)
	}
}

// TestRuntimeRegReadUint32 验证 DWORD 读取（EnableDHCP 是 DWORD）。
func TestRuntimeRegReadUint32(t *testing.T) {
	// UBR 在 Windows 10 1709+ 是 DWORD；缺失时跳过，不算失败。
	v, err := RegReadUint32(registry.LOCAL_MACHINE, RegPathWindowsVersion, RegValueUBR)
	if err != nil {
		t.Skipf("UBR 不可读（部分系统没有该值），跳过: %v", err)
	}
	t.Logf("UBR = %d", v)
}
