//go:build windows

package collect

import "testing"

func TestNormalizeOSName(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		build uint32
		want  string
	}{
		{
			// 本机实测案例：Windows 11 (build 26200) 的注册表 ProductName
			// 至今仍写作 "Windows 10 Pro for Workstations"。
			// 照抄注册表会把用户的 Windows 11 报成 Windows 10。
			name:  "Windows 11 的注册表遗留写法必须被纠正",
			in:    "Windows 10 Pro for Workstations",
			build: 26200,
			want:  "Windows 11 Pro for Workstations",
		},
		{
			name:  "Windows 11 家庭版",
			in:    "Windows 10 Home",
			build: 22631,
			want:  "Windows 11 Home",
		},
		{
			name:  "边界：首个 Win11 版本号即生效",
			in:    "Windows 10 Pro",
			build: 22000,
			want:  "Windows 11 Pro",
		},
		{
			name:  "边界：差一个版本号就仍是 Windows 10",
			in:    "Windows 10 Pro",
			build: 21999,
			want:  "Windows 10 Pro",
		},
		{
			name:  "Windows 10 不得被改写",
			in:    "Windows 10 Enterprise LTSC 2021",
			build: 19044,
			want:  "Windows 10 Enterprise LTSC 2021",
		},
		{
			// Server 的 ProductName 与客户端完全不同，绝不会以 "Windows 10" 开头。
			// 这条用例是为了钉住"不要顺手把 Server 也改了"。
			name:  "Windows Server 2025 不得被改写",
			in:    "Windows Server 2025 Standard",
			build: 26100,
			want:  "Windows Server 2025 Standard",
		},
		{
			name:  "Windows Server 2022 不得被改写",
			in:    "Windows Server 2022 Datacenter",
			build: 20348,
			want:  "Windows Server 2022 Datacenter",
		},
		{
			name:  "本身就写着 Windows 11 时保持原样",
			in:    "Windows 11 Pro",
			build: 26200,
			want:  "Windows 11 Pro",
		},
		{
			name:  "空值给出明确的未知而不是空字符串",
			in:    "   ",
			build: 26200,
			want:  "未知（注册表 ProductName 为空）",
		},
		{
			name:  "两侧空白被清理",
			in:    "  Windows 10 Pro  ",
			build: 26200,
			want:  "Windows 11 Pro",
		},
		{
			// 只改写前缀。"Windows 10" 恰好是全文时应得到 "Windows 11"。
			name:  "只有 Windows 10 三个词时也能改写",
			in:    "Windows 10",
			build: 22000,
			want:  "Windows 11",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeOSName(tc.in, tc.build); got != tc.want {
				t.Errorf("normalizeOSName(%q, %d) = %q，期望 %q", tc.in, tc.build, got, tc.want)
			}
		})
	}
}

func TestFormatOSVersion(t *testing.T) {
	cases := []struct {
		name                     string
		major, minor, build, ubr uint32
		want                     string
	}{
		{name: "本机实测值", major: 10, minor: 0, build: 26200, ubr: 9457, want: "10.0.26200.9457"},
		{name: "无修订号时省略最后一段", major: 10, minor: 0, build: 19045, ubr: 0, want: "10.0.19045"},
		{
			// 读不到修订号不能写成 ".0"：那会让人以为修订号真的是 0，
			// 而真实原因是我们没读到。
			name: "ubr=0 不得写成 .0", major: 6, minor: 1, build: 7601, ubr: 0, want: "6.1.7601",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatOSVersion(tc.major, tc.minor, tc.build, tc.ubr); got != tc.want {
				t.Errorf("formatOSVersion(%d,%d,%d,%d) = %q，期望 %q",
					tc.major, tc.minor, tc.build, tc.ubr, got, tc.want)
			}
		})
	}
}

func TestArchDisplay(t *testing.T) {
	cases := map[string]string{
		"amd64": "x86-64 (64 位)",
		"arm64": "ARM64 (64 位)",
		"386":   "x86 (32 位)",
		"wasm":  "wasm", // 未知值原样返回，便于排错
	}
	for in, want := range cases {
		if got := archDisplay(in); got != want {
			t.Errorf("archDisplay(%q) = %q，期望 %q", in, got, want)
		}
	}
}
