//go:build windows

package report

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

// 测试辅助

// fixedTime 是测试统一使用的时间，避免任何用例依赖真实时钟。
var fixedTime = time.Date(2026, 9, 30, 1, 23, 45, 0, time.Local)

// newTestWriter 返回一个时间可注入的 Writer。
func newTestWriter() Writer {
	return Writer{
		RenderContext: RenderContext{
			Version:      "v0.1.0",
			Commit:       "abc1234",
			BuildTime:    "2026-09-30T01:00:00Z",
			GeneratedAt:  fixedTime,
			TotalElapsed: 8400 * time.Millisecond,
			PlainText:    true,
		},
		Now: func() time.Time { return fixedTime },
	}
}

// richSnapshot 构造一份「数据齐全」的快照，用于版式与内容断言。
func richSnapshot() *model.Snapshot {
	snap := &model.Snapshot{StartedAt: fixedTime}
	snap.Host = model.Host{
		ComputerName: "DESKTOP-TEST",
		UserName:     `CORP\alice`,
		IsAdmin:      false,
		OSName:       "Windows 10 专业版",
		OSVersion:    "10.0.19045",
		OSBuild:      "19045.4291",
		OSArch:       "amd64",
		Uptime:       3*24*time.Hour + 7*time.Hour + 5*time.Minute,
		BootTime:     fixedTime.Add(-3 * 24 * time.Hour),
	}
	snap.Adapters = []model.Adapter{
		{
			Index:        12,
			Name:         "以太网",
			Description:  "Intel(R) Ethernet Connection I219-V",
			MAC:          "AA-BB-CC-DD-EE-FF",
			IfType:       model.IfTypeEthernet,
			OperStatus:   model.OperStatusUp,
			AdminEnabled: true,
			IPv4:         []model.Addr{{IP: "192.168.1.20", Mask: "255.255.255.0", Prefix: 24, Scope: model.ScopeGlobal}},
			IPv6:         []model.Addr{{IP: "fe80::1", Prefix: 64, Scope: model.ScopeLinkLocal}},
			Gateways:     []string{"192.168.1.1"},
			DNS:          []string{"223.5.5.5", "119.29.29.29"},
			DNSSource:    model.DNSSourceGetAdaptersAddresses,
			DHCPEnabled:  true,
			DHCPKnown:    true,
		},
		{
			Index:        5,
			Name:         "WLAN",
			IfType:       model.IfTypeIEEE80211,
			OperStatus:   model.OperStatusDown,
			AdminEnabled: false,
		},
	}
	snap.Health = model.Health{
		CPUPercent:     37.5,
		CPUKnown:       true,
		MemTotalBytes:  16 * 1024 * 1024 * 1024,
		MemAvailBytes:  6 * 1024 * 1024 * 1024,
		MemUsedPercent: 62.5,
		MemKnown:       true,
		SystemDrive:    `C:`,
		DiskTotalBytes: 476 * 1024 * 1024 * 1024,
		DiskFreeBytes:  120 * 1024 * 1024 * 1024,
		DiskKnown:      true,
	}
	snap.Probes = []model.ProbeResult{
		{
			Kind: model.ProbeICMPGateway, AdapterName: "以太网", Target: "192.168.1.1",
			Sent: 4, Recv: 4, LossPercent: 0,
			MinRTT: 2 * time.Millisecond, AvgRTT: 3 * time.Millisecond, MaxRTT: 5 * time.Millisecond,
			Success: true, Duration: 320 * time.Millisecond,
		},
		{
			Kind: model.ProbeDNSSystem, AdapterName: "以太网", Target: "www.example.com",
			Success: true, Resolved: []string{"93.184.216.34"}, Duration: 41 * time.Millisecond,
		},
		{Kind: model.ProbeTCP443, AdapterName: "WLAN", Target: "1.1.1.1", Skipped: true, SkipReason: "该网卡未连接"},
	}
	snap.Layers = []model.LayerConclusion{
		{Level: model.LevelOK, Summary: "本机到网关与公网连通性正常", Severity: model.SevOK},
	}
	snap.Raw = []model.RawLine{
		{Section: "网络适配器", Source: "GetAdaptersAddresses", Line: "AdapterName: 以太网"},
		{Section: "网络适配器", Source: "GetAdaptersAddresses", Line: "IfType: 6"},
		{Section: "系统健康度", Source: "GetSystemTimes", Line: "CPU: 37.5%"},
		{Section: "系统健康度", Source: "注册表 HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", Line: "ProductName=Windows 10 专业版"},
	}
	return snap
}

// issueFixture 构造两条告警（一条严重带证据、一条警告无建议）。
func issueFixture() []model.Issue {
	return []model.Issue{
		{
			RuleID: "R-05", Severity: model.SevSevere, Category: model.CatNetwork,
			Title:      "DNS 服务器无响应",
			Detail:     "系统 DNS 解析超时。\n已尝试 3 个服务器。",
			Evidence:   []string{"223.5.5.5 超时", "  ", "119.29.29.29 超时"},
			Suggestion: "检查 DNS 配置，或改用 223.5.5.5 / 119.29.29.29。",
		},
		{
			RuleID: "R-12", Severity: model.SevWarning, Category: model.CatStorage,
			Title: "系统盘剩余空间偏低",
		},
	}
}

// 文件字节契约：BOM 与 CRLF

// TestWriteFileHasBOMAndCRLF 是 REQ-F-504 / REQ-F-505 的落盘级验证。
//
// 断言的是真实字节，而不是「我记得写过」：记事本按 ANSI 解析无 BOM 的 UTF-8
// 会让整份中文报告乱码，这个错误只在真机上看得到，必须在这里拦住。
func TestWriteFileHasBOMAndCRLF(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWriter()

	out, err := wr.Write(richSnapshot(), issueFixture(), []Candidate{
		{Level: LevelTempDir, Label: "测试目录", Dir: dir},
	})
	if err != nil {
		t.Fatalf("Write 失败: %v", err)
	}

	raw, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatalf("读回报告失败: %v", err)
	}

	if len(raw) < 3 {
		t.Fatalf("报告过短，只有 %d 字节", len(raw))
	}
	if got := raw[:3]; string(got) != BOM {
		t.Errorf("报告前 3 字节应为 UTF-8 BOM %q，实际 %q", BOM, string(got))
	}
	if !strings.Contains(string(raw[3:]), "\r\n") {
		t.Error("报告中未找到 CRLF；REQ-F-505 要求所有换行都是 \\r\\n")
	}
	if strings.Contains(string(raw), "\r\r\n") {
		t.Error("报告中出现 \\r\\r\\n —— WriteCRLF 归一化有缺陷（记事本会多显示空行）")
	}
	if strings.Contains(strings.ReplaceAll(string(raw[3:]), "\r\n", ""), "\n") {
		t.Error("报告中存在裸 LF 换行")
	}
}

// TestWriteCRLFNormalizesAllLineEndings 覆盖三种输入形态。
func TestWriteCRLFNormalizesAllLineEndings(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"裸LF", "a\nb", "a\r\nb"},
		{"已是CRLF", "a\r\nb", "a\r\nb"},
		{"裸CR", "a\rb", "a\r\nb"},
		{"混合", "a\r\nb\nc\rd", "a\r\nb\r\nc\r\nd"},
		{"无换行", "abc", "abc"},
		{"空串", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sb strings.Builder
			if _, err := WriteCRLF(&sb, c.in); err != nil {
				t.Fatalf("WriteCRLF 失败: %v", err)
			}
			if got := sb.String(); got != c.want {
				t.Errorf("输入 %q: 期望 %q，实际 %q", c.in, c.want, got)
			}
			if strings.Contains(sb.String(), "\r\r") {
				t.Errorf("输入 %q 产生了重复回车: %q", c.in, sb.String())
			}
		})
	}
}

// 文件名与撞名（最重要的一条）

func TestReportFileNameLayout(t *testing.T) {
	got := ReportFileName(fixedTime)
	want := "diag_20260930_012345.txt"
	if got != want {
		t.Fatalf("文件名格式错误：期望 %q，实际 %q", want, got)
	}
}

// TestWriteNeverOverwritesExistingReport 是本包最关键的一条测试。
//
// 同一秒内重复运行必须得到 diag_..._1.txt，且**前一份报告字节不能被改动**。
// 覆盖会直接销毁上一份现场证据 —— 那正是运维最需要的东西。
func TestWriteNeverOverwritesExistingReport(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWriter()
	cands := []Candidate{{Level: LevelTempDir, Label: "测试目录", Dir: dir}}

	first, err := wr.Write(richSnapshot(), issueFixture(), cands)
	if err != nil {
		t.Fatalf("第一次 Write 失败: %v", err)
	}
	if filepath.Base(first.Path) != "diag_20260930_012345.txt" {
		t.Fatalf("第一份报告文件名错误: %s", filepath.Base(first.Path))
	}
	firstBytes, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatalf("读回第一份报告失败: %v", err)
	}

	// 第二次：完全相同的时刻（同秒重复运行）。
	second, err := wr.Write(richSnapshot(), issueFixture(), cands)
	if err != nil {
		t.Fatalf("第二次 Write 失败: %v", err)
	}
	if filepath.Base(second.Path) != "diag_20260930_012345_1.txt" {
		t.Fatalf("撞名后应产出 _1 后缀，实际 %s", filepath.Base(second.Path))
	}
	if second.Path == first.Path {
		t.Fatal("两次写入落到了同一路径 —— 存在覆盖报告的风险")
	}

	afterBytes, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatalf("第一份报告消失或不可读: %v", err)
	}
	if string(afterBytes) != string(firstBytes) {
		t.Fatal("第一份报告被第二次运行改写了 —— 违反 REQ-F-502 绝不覆盖")
	}
	if len(afterBytes) == 0 {
		t.Fatal("第一份报告为空")
	}

	// 第三次：继续递增到 _2，且前两份都还在。
	third, err := wr.Write(richSnapshot(), issueFixture(), cands)
	if err != nil {
		t.Fatalf("第三次 Write 失败: %v", err)
	}
	if filepath.Base(third.Path) != "diag_20260930_012345_2.txt" {
		t.Fatalf("第三次应为 _2 后缀，实际 %s", filepath.Base(third.Path))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("列目录失败: %v", err)
	}
	if len(entries) != 3 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("期望 3 份报告，实际 %d 份: %v", len(entries), names)
	}
}

// TestReserveReportFileUsesExclusiveCreate 直接验证占位用的是 O_EXCL 语义：
// 预先放一个同名文件（带哨兵内容），占位必须跳过它而不是截断它。
func TestReserveReportFileUsesExclusiveCreate(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "diag_20260930_012345.txt")
	sentinel := []byte("上一份报告的内容，不许动")
	if err := os.WriteFile(existing, sentinel, 0o644); err != nil {
		t.Fatalf("预置文件失败: %v", err)
	}

	f, full, err := reserveReportFile(dir, "diag_20260930_012345.txt")
	if err != nil {
		t.Fatalf("占位失败: %v", err)
	}
	defer f.Close()

	if full == existing {
		t.Fatal("占位返回了已存在的路径 —— 会覆盖已有报告")
	}
	if filepath.Base(full) != "diag_20260930_012345_1.txt" {
		t.Fatalf("期望 _1 后缀，实际 %s", filepath.Base(full))
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("读回预置文件失败: %v", err)
	}
	if string(got) != string(sentinel) {
		t.Fatalf("预置文件被改写：期望 %q，实际 %q", sentinel, got)
	}
}

// 四级降级链

// TestResolveTargetsOrder 验证候选链的固定优先级顺序（Q2）。
func TestResolveTargetsOrder(t *testing.T) {
	cands := ResolveTargets(`D:\reports`)
	if len(cands) != 4 {
		t.Fatalf("显式传 -o 时应得到 4 级候选，实际 %d", len(cands))
	}
	wantLevels := []int{LevelExplicit, LevelExeDir, LevelUserDir, LevelTempDir}
	for i, want := range wantLevels {
		if cands[i].Level != want {
			t.Errorf("第 %d 级应为 %d，实际 %d", i+1, want, cands[i].Level)
		}
	}
	if cands[0].Dir != `D:\reports` {
		t.Errorf("第 ① 级应为用户指定目录，实际 %q", cands[0].Dir)
	}

	// 未传 -o 时跳过第 ① 级，顺序不变。
	cands = ResolveTargets("")
	if len(cands) != 3 {
		t.Fatalf("未传 -o 时应得到 3 级候选，实际 %d", len(cands))
	}
	if cands[0].Level != LevelExeDir {
		t.Errorf("未传 -o 时首个候选应为 ② EXE 目录，实际 %d", cands[0].Level)
	}

	// 纯函数契约：ResolveTargets 不得创建任何目录。
	probe := filepath.Join(t.TempDir(), "never-created", "reports")
	_ = ResolveTargets(probe)
	if _, err := os.Stat(probe); err == nil {
		t.Error("ResolveTargets 创建了目录 —— 它必须是纯函数，不做任何 IO")
	}
}

// TestWriteDegradesWhenExplicitDirNotWritable 覆盖「① 不可写 → 落到 ②」。
//
// 用「路径已存在但不是目录」制造失败：这在 Windows 上确定的失败方式，
// 且不像 ACL 那样依赖运行账户权限（CI 与开发机结果一致）。
func TestWriteDegradesWhenExplicitDirNotWritable(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备阻塞文件失败: %v", err)
	}
	fallback2 := filepath.Join(tmp, "fallback2")

	wr := newTestWriter()
	out, err := wr.Write(richSnapshot(), nil, []Candidate{
		{Level: LevelExplicit, Label: "命令行 -o 指定目录", Dir: blocker},
		{Level: LevelExeDir, Label: "EXE 所在目录", Dir: filepath.Join(blocker, "deeper")},
		{Level: LevelUserDir, Label: "用户报告目录", Dir: fallback2},
	})
	if err != nil {
		t.Fatalf("应降级成功，实际失败: %v", err)
	}
	if out.Level != LevelUserDir {
		t.Fatalf("最终应落在第 ③ 级，实际第 %d 级", out.Level)
	}
	if !out.Degraded {
		t.Error("Degraded 应为 true（更高级别确实尝试过且失败）")
	}
	if out.Level1Skipped {
		t.Error("Level1Skipped 应为 false：用户确实传了 -o")
	}
	if out.Reason == "" {
		t.Error("降级原因不能为空")
	}
	if !strings.Contains(out.Note, "已降级至") {
		t.Errorf("路径选择说明应包含降级表述，实际 %q", out.Note)
	}
	if _, err := os.Stat(out.Path); err != nil {
		t.Fatalf("报告未落盘: %v", err)
	}
	if len(out.Attempts) != 3 {
		t.Fatalf("应记录 3 次尝试，实际 %d", len(out.Attempts))
	}
	if out.Attempts[0].OK || out.Attempts[1].OK || !out.Attempts[2].OK {
		t.Errorf("尝试记录的成功/失败标记不符: %+v", out.Attempts)
	}
}

// TestWriteNoDegradeWithOnlyOneCandidate 验证「没有更高级别失败就不算降级」。
func TestWriteNoDegradeWithOnlyOneCandidate(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWriter()
	out, err := wr.Write(richSnapshot(), nil, []Candidate{
		{Level: LevelUserDir, Label: "用户报告目录", Dir: dir},
	})
	if err != nil {
		t.Fatalf("Write 失败: %v", err)
	}
	if out.Degraded {
		t.Error("只有一个候选且成功时，Degraded 必须为 false")
	}
	if !strings.Contains(out.Note, "未降级") {
		t.Errorf("路径选择说明应写明未降级，实际 %q", out.Note)
	}
}

// TestWriteReportsEveryLevelReasonWhenAllFail 是 REQ-F-507 的核心验证：
// 全部失败时错误信息必须逐级列出各级原因，供控制台直接打印后以退出码 2 结束。
func TestWriteReportsEveryLevelReasonWhenAllFail(t *testing.T) {
	tmp := t.TempDir()

	// 三级用三种不同的失败方式，便于逐级断言原因文本。
	asFile := filepath.Join(tmp, "file-blocker")
	if err := os.WriteFile(asFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备阻塞文件失败: %v", err)
	}
	// 上级是文件 → MkdirAll 必然失败。
	nestedUnderFile := filepath.Join(asFile, "reports")

	wr := newTestWriter()
	out, err := wr.Write(richSnapshot(), nil, []Candidate{
		{Level: LevelExplicit, Label: "命令行 -o 指定目录", Dir: asFile},
		{Level: LevelExeDir, Label: "EXE 所在目录", Dir: "", Reason: "无法获取可执行文件路径: 返回空路径"},
		{Level: LevelUserDir, Label: "用户报告目录", Dir: nestedUnderFile},
	})
	if err == nil {
		t.Fatal("全部候选失败时必须返回错误")
	}
	if out.Path != "" {
		t.Errorf("全部失败时 Outcome.Path 应为空，实际 %q", out.Path)
	}

	msg := err.Error()

	for _, level := range []int{LevelExplicit, LevelExeDir, LevelUserDir} {
		if !strings.Contains(msg, "["+itoa(level)+"/4]") {
			t.Errorf("错误信息缺少第 %d 级的记录行:\n%s", level, msg)
		}
	}
	// 每条具体原因都要出现。
	for _, want := range []string{
		"命令行 -o 指定目录",
		"路径已存在但不是目录",
		"EXE 所在目录",
		"无法获取可执行文件路径",
		"用户报告目录",
		"上级路径",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息缺少 %q:\n%s", want, msg)
		}
	}
	if !strings.Contains(msg, "均无法写入") {
		t.Errorf("错误信息缺少总述:\n%s", msg)
	}
	// 逐级原因必须都能从 Attempts 里取到（上层据此打印）。
	if len(out.Attempts) != 3 {
		t.Fatalf("应记录 3 次失败尝试，实际 %d", len(out.Attempts))
	}
	for i, a := range out.Attempts {
		if a.Reason == "" {
			t.Errorf("第 %d 条尝试缺少失败原因", i+1)
		}
		if a.OK {
			t.Errorf("第 %d 条尝试不应标记为成功", i+1)
		}
	}
}

// itoa 只在测试里用来拼 "[n/4]" 断言串。
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// 可写性探针不留垃圾

func TestEnsureWritableLeavesNoGarbage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "reports")

	if err := EnsureWritable(dir); err != nil {
		t.Fatalf("EnsureWritable 失败: %v", err)
	}
	left, err := TempProbeFiles(dir)
	if err != nil {
		t.Fatalf("扫描探针残留失败: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("可写性探测留下了垃圾文件: %v", left)
	}
	// 探测不应顺手写入报告文件。
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("列目录失败: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("探测后目录非空: %d 项", len(entries))
	}

	if err := EnsureWritable(""); err == nil {
		t.Error("空目录应返回错误")
	}
}

// 三层结构与空快照

// TestRenderContainsThreeLayers 验证 REQ-F-501 的三层结构与固定顺序。
func TestRenderContainsThreeLayers(t *testing.T) {
	text := RenderString(richSnapshot(), issueFixture(), newTestWriter().RenderContext)

	titles := []string{sectionTitleLayer1, sectionTitleLayer2, sectionTitleLayer3}
	last := -1
	for _, title := range titles {
		idx := strings.Index(text, title)
		if idx < 0 {
			t.Fatalf("报告中缺少分节标题 %q", title)
		}
		if idx <= last {
			t.Fatalf("分节标题顺序错误：%q 出现在错误位置", title)
		}
		last = idx
	}

	// 第二层的四个内置分节都要出现（注册机制生效）。
	for _, sub := range []string{layer2SubHost, layer2SubAdapters, layer2SubHealth, layer2SubConnect} {
		if !strings.Contains(text, " ▼ "+sub) {
			t.Errorf("第二层缺少分节 %q", sub)
		}
	}

	// 尾部只读声明（C-01/C-02）。
	if !strings.Contains(text, readOnlyFooter) {
		t.Errorf("报告尾部缺少只读声明")
	}
	// 原始数据附录必须标注来源。
	if !strings.Contains(text, "GetAdaptersAddresses") {
		t.Error("第三层缺少数据来源标注 GetAdaptersAddresses")
	}
	if !strings.Contains(text, `注册表 HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`) {
		t.Error("第三层缺少注册表来源标注")
	}
	if !strings.Contains(text, "GetSystemTimes") {
		t.Error("第三层缺少 GetSystemTimes 来源标注")
	}
}

// TestRenderEmptySnapshotDoesNotPanic 覆盖「全部采集失败」的极端输入。
func TestRenderEmptySnapshotDoesNotPanic(t *testing.T) {
	// 两个 nil 入参是最容易崩的路径：既空快照又空上下文。
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("渲染空快照时 panic: %v", r)
		}
	}()

	text := RenderString(nil, nil, RenderContext{})

	for _, title := range []string{sectionTitleLayer1, sectionTitleLayer2, sectionTitleLayer3} {
		if !strings.Contains(text, title) {
			t.Errorf("空快照下缺少分节标题 %q", title)
		}
	}
	if !strings.Contains(text, noIssueConclusion) {
		t.Error("空快照（无告警）时第一层应给出正向结论")
	}
	if !strings.Contains(text, notCollected) {
		t.Error("空快照时应出现「未采集」而不是空白")
	}
	if !strings.Contains(text, readOnlyFooter) {
		t.Error("空快照下尾部只读声明仍然必须存在")
	}

	// 第二层各节在无数据时也要给出可读文案，而不是空白行。
	for _, want := range []string{
		"本机未枚举到任何网络适配器",
		"采样失败",
		"未产生任何探测结果",
		"无法定位故障层级",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("空快照下第二层缺少兜底文案 %q", want)
		}
	}
	// 第三层为空时要有解释。
	if !strings.Contains(text, "第三层附录为空") && !strings.Contains(text, "未产生原始数据") {
		t.Error("空快照下第三层缺少解释文案")
	}

	// ReportSections 也必须能处理空快照。
	sections := ReportSections(nil, nil, RenderContext{})
	if len(sections) != 3 {
		t.Fatalf("ReportSections 应返回 3 层，实际 %d", len(sections))
	}
	for _, s := range sections {
		if len(s.Lines) == 0 {
			t.Errorf("分节 %q 的内容为空", s.Title)
		}
	}
}

// TestRenderNilWriterReturnsError 保证 nil writer 不会 panic 而是报错。
func TestRenderNilWriterReturnsError(t *testing.T) {
	if err := Render(nil, richSnapshot(), nil, RenderContext{}); err == nil {
		t.Fatal("nil writer 应返回错误")
	}
}

// TestRenderHealthyMachineGivesPositiveConclusion 覆盖 REQ-F-404：
// 健康机器在第一层必须有明确的正向结论，不能留空。
func TestRenderHealthyMachineGivesPositiveConclusion(t *testing.T) {
	sections := ReportSections(richSnapshot(), nil, newTestWriter().RenderContext)
	layer1 := strings.Join(sections[0].Lines, "\n")

	if !strings.Contains(layer1, "未发现异常项") {
		t.Fatalf("无告警时第一层缺少正向结论:\n%s", layer1)
	}
	if strings.Contains(layer1, "诊断不完整") {
		t.Errorf("无采集失败时不应出现「诊断不完整」:\n%s", layer1)
	}
	if strings.TrimSpace(layer1) == "" {
		t.Fatal("第一层为空")
	}
}

// 确定性（REQ-N-08）

// TestRenderIsDeterministic 是 REQ-N-08 的核心：同一份 Snapshot 渲染两次字节一致。
func TestRenderIsDeterministic(t *testing.T) {
	snap := richSnapshot()
	issues := issueFixture()
	ctx := newTestWriter().RenderContext

	first := RenderString(snap, issues, ctx)
	for i := 0; i < 5; i++ {
		if got := RenderString(snap, issues, ctx); got != first {
			t.Fatalf("第 %d 次渲染与首次不一致（存在 map 迭代顺序或时钟依赖）", i+2)
		}
	}
}

// TestRenderDeterministicAcrossInputOrder 验证输入顺序不影响输出：
// Raw 与 Layers 的切片顺序由采集器调度决定，不能被泄漏进报告。
func TestRenderDeterministicAcrossInputOrder(t *testing.T) {
	a := richSnapshot()
	b := richSnapshot()

	// 反转 Raw 与 Layers，并打乱同一分节内的行顺序（模拟不同采集顺序）。
	for i, j := 0, len(b.Raw)-1; i < j; i, j = i+1, j-1 {
		b.Raw[i], b.Raw[j] = b.Raw[j], b.Raw[i]
	}
	b.Layers = []model.LayerConclusion{}

	ctx := newTestWriter().RenderContext
	gotA := RenderString(a, issueFixture(), ctx)
	gotB := RenderString(b, issueFixture(), ctx)

	// 只比较第三层（Raw 排序的落点），Layers 已被清空，故单独比附录段。
	idxA := strings.Index(gotA, sectionTitleLayer3)
	idxB := strings.Index(gotB, sectionTitleLayer3)
	if idxA < 0 || idxB < 0 {
		t.Fatal("缺少第三层")
	}
	linesA := append([]string(nil), contentLines(gotA[idxA:])...)
	linesB := append([]string(nil), contentLines(gotB[idxB:])...)
	// 行序可以不同吗？—— 不可以：本节按 (Section, Source) 分组，组内行序保持采集顺序。
	// 但「分组顺序」必须一致，因此这里断言分组标题序列相同。
	groupsA := groupHeaders(linesA)
	groupsB := groupHeaders(linesB)
	if strings.Join(groupsA, "|") != strings.Join(groupsB, "|") {
		t.Fatalf("第三层分组顺序依赖于输入顺序:\nA=%v\nB=%v", groupsA, groupsB)
	}
}

// groupHeaders 摘出第三层里的分节/来源标题行。
func groupHeaders(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.Contains(l, "[分节]") || strings.Contains(l, "[来源]") {
			out = append(out, l)
		}
	}
	return out
}

// TestBuildReportContentDeterministic 验证完整落盘字节（含 BOM 与 CRLF）也确定。
func TestBuildReportContentDeterministic(t *testing.T) {
	wr := newTestWriter()
	first, err := wr.BuildReportContent(richSnapshot(), issueFixture())
	if err != nil {
		t.Fatalf("BuildReportContent 失败: %v", err)
	}
	second, err := wr.BuildReportContent(richSnapshot(), issueFixture())
	if err != nil {
		t.Fatalf("BuildReportContent 第二次失败: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("BuildReportContent 两次产出不一致")
	}
	if !strings.HasPrefix(string(first), BOM) {
		t.Fatal("BuildReportContent 缺少 BOM")
	}
}

// 分节注册机制（REQ-N-09）

// TestRegisterSectionOrder 验证排序完全由 (order, title) 决定，
// 与注册调用顺序无关。
func TestRegisterSectionOrder(t *testing.T) {
	const (
		tA = "测试分节-A"
		tB = "测试分节-B"
		tC = "测试分节-C"
	)
	// 故意按逆序注册。
	RegisterSection(tC, 25, func(w io.Writer, _ *model.Snapshot) { _, _ = w.Write(nil) })
	RegisterSection(tB, 15, func(w io.Writer, _ *model.Snapshot) { _, _ = w.Write(nil) })
	RegisterSection(tA, 15, func(w io.Writer, _ *model.Snapshot) { _, _ = w.Write(nil) })

	got := make([]string, 0)
	for _, s := range Sections() {
		switch s.Title {
		case tA, tB, tC:
			got = append(got, s.Title)
		}
	}
	// 同为 order=15 时按标题排序：A 在 B 之前。
	want := []string{tA, tB, tC}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("分节顺序错误：期望 %v，实际 %v", want, got)
	}
}

// TestRegisterSectionIgnoresInvalid 验证空标题与 nil 渲染器被忽略。
func TestRegisterSectionIgnoresInvalid(t *testing.T) {
	before := len(Sections())
	RegisterSection("", 99, func(_ io.Writer, _ *model.Snapshot) {})
	RegisterSection("   ", 99, func(_ io.Writer, _ *model.Snapshot) {})
	RegisterSection("测试分节-nil", 99, nil)
	if got := len(Sections()); got != before {
		t.Fatalf("非法注册改变了分节数量：%d → %d", before, got)
	}
}

// TestRegisterSectionOverridesSameTitle 验证同标题覆盖而不是重复两节。
func TestRegisterSectionOverridesSameTitle(t *testing.T) {
	const title = "测试分节-覆盖"
	first := func(_ io.Writer, _ *model.Snapshot) {}
	second := func(_ io.Writer, _ *model.Snapshot) {}

	RegisterSection(title, 50, first)
	RegisterSection(title, 50, second)

	count := 0
	var got SectionRenderer
	for _, s := range Sections() {
		if s.Title == title {
			count++
			got = s.Render
		}
	}
	if count != 1 {
		t.Fatalf("同标题注册应覆盖，实际出现 %d 次", count)
	}
	if got == nil {
		t.Fatal("覆盖后的渲染器为空")
	}
}

// TestSectionsReturnsCopy 验证 Sections 返回副本，调用方无法篡改注册表。
func TestSectionsReturnsCopy(t *testing.T) {
	before := Sections()
	if len(before) == 0 {
		t.Fatal("内置分节未注册")
	}
	before[0].Title = "被篡改的标题"
	before[0].Render = nil

	after := Sections()
	if after[0].Title == "被篡改的标题" || after[0].Render == nil {
		t.Fatal("Sections 返回了内部切片，注册表被外部篡改")
	}
}

// TestSectionRendererPanicIsContained 验证单个分节 panic 不会毁掉整份报告。
func TestSectionRendererPanicIsContained(t *testing.T) {
	const title = "测试分节-panic"
	RegisterSection(title, 999, func(_ io.Writer, _ *model.Snapshot) {
		panic("分节渲染器故意崩溃")
	})

	text := RenderString(richSnapshot(), nil, newTestWriter().RenderContext)
	if !strings.Contains(text, sectionTitleLayer3) {
		t.Fatal("分节 panic 导致报告后半部分丢失")
	}
	if !strings.Contains(text, "渲染失败") {
		t.Error("分节 panic 未在报告中留下可读提示")
	}
	if !strings.Contains(text, readOnlyFooter) {
		t.Error("分节 panic 导致报告尾部丢失")
	}
}

// 未采集 / 权限不足的表述（Q4 / REQ-F-703）

// TestRenderPermissionFailureLabeled 验证权限类失败使用逐字要求的文案。
func TestRenderPermissionFailureLabeled(t *testing.T) {
	snap := richSnapshot()
	snap.Failures = []model.CollectFailure{
		{Item: "网卡 DNS 配置", EnvVar: "Tcpip\\Interfaces", Reason: "拒绝访问 (Access is denied)"},
		{Item: "系统运行时长", Reason: "GetSystemTimes 调用失败", Partial: true},
		{Item: "已安装补丁列表", Reason: "查询超时"},
	}

	text := RenderString(snap, nil, newTestWriter().RenderContext)

	// 第一层必须有「诊断不完整」提示（REQ-F-405）。
	if !strings.Contains(text, incompleteConclusion) {
		t.Error("存在采集失败时缺少「诊断不完整」提示")
	}
	// 第二层必须逐条列出并标注（REQ-F-703）。
	if !strings.Contains(text, notCollectedPerm) {
		t.Errorf("权限类失败缺少 %q 标注", notCollectedPerm)
	}
	if !strings.Contains(text, "拒绝访问 (Access is denied)") {
		t.Error("权限类失败的原因原文丢失")
	}
	if !strings.Contains(text, notCollectedPartial) {
		t.Errorf("部分失败缺少 %q 标注", notCollectedPartial)
	}
	if !strings.Contains(text, notCollectedOther) {
		t.Errorf("非权限类失败缺少 %q 标注", notCollectedOther)
	}
	// 第一层要列出缺失项清单。
	if !strings.Contains(text, "3 项未采集") {
		t.Error("第一层缺少缺失项计数")
	}
}

// TestFailureLabelClassification 直接覆盖分类函数的三条分支。
func TestFailureLabelClassification(t *testing.T) {
	cases := []struct {
		name string
		in   model.CollectFailure
		want string
	}{
		{
			name: "权限不足",
			in:   model.CollectFailure{Item: "x", Reason: "Access is denied"},
			want: notCollectedPerm + permReasonPrefix + "Access is denied" + reasonSuffix,
		},
		{
			name: "中文权限",
			in:   model.CollectFailure{Item: "x", Reason: "拒绝访问。"},
			want: notCollectedPerm + permReasonPrefix + "拒绝访问。" + reasonSuffix,
		},
		{
			name: "部分未采集优先于通用",
			in:   model.CollectFailure{Item: "x", Reason: "超时", Partial: true},
			want: notCollectedPartial + permReasonPrefix + "超时" + reasonSuffix,
		},
		{
			name: "通用未采集且无原因",
			in:   model.CollectFailure{Item: "x"},
			want: notCollectedOther,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := failureLabel(c.in); got != c.want {
				t.Errorf("期望 %q，实际 %q", c.want, got)
			}
		})
	}
}

// TestFailureShortUsesItemOrEnvVarAndTruncates 覆盖第一层清单的短文案。
func TestFailureShortUsesItemOrEnvVarAndTruncates(t *testing.T) {
	if got := failureShort(model.CollectFailure{Item: "网卡 DNS", Reason: "拒绝访问"}); got != "网卡 DNS（原因: 拒绝访问）" {
		t.Errorf("Item 优先的短文案不符: %q", got)
	}
	if got := failureShort(model.CollectFailure{EnvVar: "USERPROFILE", Reason: "未设置"}); !strings.Contains(got, "USERPROFILE") {
		t.Errorf("Item 为空时应回落到 EnvVar: %q", got)
	}
	if got := failureShort(model.CollectFailure{Item: "x"}); !strings.Contains(got, "原因未知") {
		t.Errorf("无原因时应明说原因未知: %q", got)
	}

	long := strings.Repeat("原", 200)
	got := failureShort(model.CollectFailure{Item: "x", Reason: long})
	if !strings.HasSuffix(got, "…）") {
		t.Errorf("超长原因应截断并加省略号: %q", got)
	}
	if r := []rune(strings.TrimSuffix(strings.TrimPrefix(got, "x（原因: "), "）")); len(r) != 61 {
		t.Errorf("截断长度应为 60 个字符加省略号，实际 %d", len(r))
	}
}

// 报告头与路径说明

// TestReportHeaderIncludesActualPath 验证 REQ-F-506：
// 落盘后的报告头必须写明**实际**路径，且与实际文件位置一致。
func TestReportHeaderIncludesActualPath(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWriter()
	out, err := wr.Write(richSnapshot(), nil, []Candidate{
		{Level: LevelUserDir, Label: "用户报告目录", Dir: dir},
	})
	if err != nil {
		t.Fatalf("Write 失败: %v", err)
	}

	raw, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatalf("读回报告失败: %v", err)
	}
	text := string(raw)

	if !strings.Contains(text, out.Path) {
		t.Errorf("报告头未写出实际路径 %q", out.Path)
	}
	if !strings.Contains(text, "报告存放路径") {
		t.Error("报告头缺少「报告存放路径」字段")
	}
	if !strings.Contains(text, "路径选择说明") {
		t.Error("报告头缺少「路径选择说明」字段")
	}
	for _, want := range []string{"诊断时间", "计算机名", "运行账户", "操作系统", "系统运行时长", "诊断工具版本", "总耗时"} {
		if !strings.Contains(text, want) {
			t.Errorf("报告头缺少字段 %q", want)
		}
	}
	// 版本信息由调用方注入，必须出现在报告头。
	if !strings.Contains(text, "v0.1.0") || !strings.Contains(text, "abc1234") {
		t.Error("报告头缺少注入的版本/提交号")
	}
	if !strings.Contains(text, "8.4 秒") {
		t.Error("报告头缺少总耗时")
	}
}

// TestReportHeaderMentionsDegradation 验证降级时报告头带原因。
func TestReportHeaderMentionsDegradation(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备阻塞文件失败: %v", err)
	}
	fallback := filepath.Join(tmp, "fb")

	wr := newTestWriter()
	out, err := wr.Write(richSnapshot(), nil, []Candidate{
		{Level: LevelExplicit, Label: "命令行 -o 指定目录", Dir: blocker},
		{Level: LevelUserDir, Label: "用户报告目录", Dir: fallback},
	})
	if err != nil {
		t.Fatalf("Write 失败: %v", err)
	}

	raw, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatalf("读回报告失败: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "已降级至") {
		t.Error("降级时报告头应写明「已降级至」")
	}
	if !strings.Contains(text, "路径已存在但不是目录") {
		t.Error("降级原因应出现在报告头的路径选择说明里")
	}
}

// TestReportHeaderMarksUNCPath 覆盖 S-10：UNC 部署路径要在报告头注明。
func TestReportHeaderMarksUNCPath(t *testing.T) {
	ctx := newTestWriter().RenderContext
	ctx.ExePath = `\\fileserver\share\tools\desktop-diag.exe`
	ctx.ReportPath = `\\fileserver\share\tools\diag_20260930_012345.txt`

	text := RenderString(richSnapshot(), nil, ctx)
	if !strings.Contains(text, "UNC") {
		t.Error("UNC 部署路径未在报告头注明")
	}
	if !strings.Contains(text, `\\fileserver\share\tools`) {
		t.Error("UNC 提示未给出具体路径")
	}

	// 非 UNC 路径不应产生该提示。
	plain := RenderString(richSnapshot(), nil, newTestWriter().RenderContext)
	if strings.Contains(plain, "UNC") {
		t.Error("本地路径不应出现 UNC 提示")
	}
}

// TestIsUNCPath 覆盖路径判定的正反例。
func TestIsUNCPath(t *testing.T) {
	cases := map[string]bool{
		`\\server\share\x.txt`: true,
		`//server/share/x.txt`: true,
		`  \\server\share\x`:   true,
		`C:\reports\x.txt`:     false,
		`\reports\x.txt`:       false,
		`reports\x.txt`:        false,
		``:                     false,
		`\\`:                   true,
	}
	for in, want := range cases {
		if got := IsUNCPath(in); got != want {
			t.Errorf("IsUNCPath(%q) = %v，期望 %v", in, got, want)
		}
	}
}

// 第一层告警渲染

// TestLayer1RendersIssueFields 验证 REQ-F-402 的字段完整性。
func TestLayer1RendersIssueFields(t *testing.T) {
	sections := ReportSections(richSnapshot(), issueFixture(), newTestWriter().RenderContext)
	layer1 := strings.Join(sections[0].Lines, "\n")

	for _, want := range []string{
		"[严重] 网络 - DNS 服务器无响应",
		"[警告] 存储 - 系统盘剩余空间偏低",
		"规则: R-05",
		"建议: 检查 DNS 配置",
		"223.5.5.5 超时",
		"119.29.29.29 超时",
		"共 2 项：严重 1，警告 1",
		"无需处理（该项为提示性结论）",
	} {
		if !strings.Contains(layer1, want) {
			t.Errorf("第一层缺少 %q:\n%s", want, layer1)
		}
	}
	// NormalizeEvidence 应剔除纯空白证据，不留空证据行。
	for _, line := range sections[0].Lines {
		if line == indentEvidence {
			t.Errorf("第一层存在空的证据行（NormalizeEvidence 未生效）: %q", line)
		}
	}
}

// 第三层附录

// TestLayer3GroupsBySectionAndSource 验证按 Section 分组、标注来源。
func TestLayer3GroupsBySectionAndSource(t *testing.T) {
	snap := &model.Snapshot{}
	snap.AddRaw("网络适配器", "GetAdaptersAddresses", "line-1")
	snap.AddRaw("网络适配器", "GetAdaptersAddresses", "line-2")
	snap.AddRaw("网络适配器", "注册表", "line-3")
	snap.AddRaw("", "", "line-4")

	text := RenderString(snap, nil, newTestWriter().RenderContext)

	for _, want := range []string{
		"[分节] 网络适配器",
		"[来源] GetAdaptersAddresses",
		"[来源] 注册表",
		"line-1", "line-2", "line-3", "line-4",
		"[分节] " + layer3SubsectionRaw,
		"[来源] " + emptySource,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("第三层缺少 %q", want)
		}
	}
	// 分组顺序必须字典序：GetAdaptersAddresses 在「注册表」之前（ASCII 优先）。
	if strings.Index(text, "[来源] GetAdaptersAddresses") > strings.Index(text, "[来源] 注册表") {
		t.Error("同一分节内的来源未按字典序排序")
	}
}

// 枚举与数值格式化

func TestOperStatusLabel(t *testing.T) {
	cases := map[string]string{
		"Up":             "已连接",
		"Down":           "已断开",
		"Testing":        "正在测试",
		"Unknown":        "状态未知",
		"Dormant":        "休眠（等待链路事件）",
		"NotPresent":     "设备不存在",
		"LowerLayerDown": "下层链路断开",
		"":               notCollected,
		"SomethingWeird": "SomethingWeird",
	}
	for in, want := range cases {
		if got := operStatusLabel(in); got != want {
			t.Errorf("operStatusLabel(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestFormatBytesUsesGiB(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0.0 GB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{476 * 1024 * 1024 * 1024, "476.0 GB"},
		{1536 * 1024 * 1024, "1.5 GB"},
	}
	for _, c := range cases {
		if got := formatBytes(c.in); got != c.want {
			t.Errorf("formatBytes(%d) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"零值", 0, "0 分"},
		{"负数归零", -time.Hour, "0 分"},
		{"仅分钟", 7 * time.Minute, "7 分"},
		{"小时加分钟", 3*time.Hour + 7*time.Minute, "3 小时 7 分"},
		{"跨天", 3*24*time.Hour + 7*time.Hour + 5*time.Minute, "3 天 7 小时 5 分"},
		{"整一天", 24 * time.Hour, "1 天 0 小时 0 分"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatDuration(c.in); got != c.want {
				t.Errorf("formatDuration(%v) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

func TestFormatTimeZeroValue(t *testing.T) {
	if got := formatTime(time.Time{}); got != notCollected {
		t.Errorf("零值时间应显示「未采集」，实际 %q", got)
	}
	if got := formatTime(fixedTime); got != "2026-09-30 01:23:45" {
		t.Errorf("时间格式不符: %q", got)
	}
}

func TestFieldAndKVNeverLeaveBlankValue(t *testing.T) {
	if got := field("计算机名", ""); !strings.HasSuffix(got, ": "+notCollected) {
		t.Errorf("空值未回落到「未采集」: %q", got)
	}
	if got := kv("主机名", ""); !strings.HasSuffix(got, ": "+notCollected) {
		t.Errorf("kv 空值未回落到「未采集」: %q", got)
	}
	if got := field("诊断工具版本", "v1"); !strings.Contains(got, "诊断工具版本") || !strings.Contains(got, "v1") {
		t.Errorf("field 输出不完整: %q", got)
	}
	// 键超长时不得 panic、也不得把值挤掉。
	if got := field("一个非常非常非常长的键名超过补齐宽度", "v"); !strings.Contains(got, "v") {
		t.Errorf("超长键名时值丢失: %q", got)
	}
}

func TestSeverityTag(t *testing.T) {
	cases := []struct {
		in   model.Severity
		want string
	}{
		{model.SevSevere, "[严重]"},
		{model.SevWarning, "[警告]"},
		{model.SevOK, "[正常]"},
		{model.Severity(99), "[未知]"},
	}
	for _, c := range cases {
		if got := severityTag(c.in); got != c.want {
			t.Errorf("severityTag(%v) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestSortStringsDedupesAndSorts(t *testing.T) {
	got := sortStrings([]string{"b", "a", "b", "c", "a"})
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("期望 %v，实际 %v", want, got)
	}
	if got := sortStrings(nil); len(got) != 0 {
		t.Errorf("nil 输入应返回空切片，实际 %v", got)
	}
}

func TestRepeatRuneGuardsNonPositive(t *testing.T) {
	if got := repeatRune("=", 0); got != "" {
		t.Errorf("n=0 应返回空串，实际 %q", got)
	}
	if got := repeatRune("=", -5); got != "" {
		t.Errorf("n<0 应返回空串，实际 %q", got)
	}
	if got := repeatRune("", 10); got != "" {
		t.Errorf("空串应返回空串，实际 %q", got)
	}
	if got := repeatRune("=", 80); len(got) != 80 {
		t.Errorf("n=80 应返回 80 个字符，实际 %d", len(got))
	}
}

func TestSeparatorWidth(t *testing.T) {
	if got := len([]rune(separator())); got != lineWidth {
		t.Errorf("分隔线宽度应为 %d，实际 %d", lineWidth, got)
	}
	if got := len([]rune(rule())); got != lineWidth {
		t.Errorf("细分隔线宽度应为 %d，实际 %d", lineWidth, got)
	}
}

func TestLevelName(t *testing.T) {
	cases := map[int]string{
		LevelExplicit: "①命令行 -o",
		LevelExeDir:   "②EXE 所在目录",
		LevelUserDir:  "③用户目录",
		LevelTempDir:  "④系统临时目录",
		42:            "未知层级(42)",
	}
	for in, want := range cases {
		if got := LevelName(in); got != want {
			t.Errorf("LevelName(%d) = %q，期望 %q", in, got, want)
		}
	}
}

// 附加正文块（V1.1 扩展点）

// TestWriterExtraBlocksUseCRLF 验证额外正文块同样经过 CRLF 归一。
func TestWriterExtraBlocksUseCRLF(t *testing.T) {
	wr := newTestWriter()
	wr.Writers = []func(w io.Writer) error{
		func(w io.Writer) error {
			_, err := w.Write([]byte("\n[附加] 自定义块\n第二行\n"))
			return err
		},
	}

	content, err := wr.BuildReportContent(richSnapshot(), nil)
	if err != nil {
		t.Fatalf("BuildReportContent 失败: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "[附加] 自定义块") {
		t.Error("附加块未出现在报告中")
	}
	if strings.Contains(text, "[附加] 自定义块\n") {
		t.Error("附加块使用了裸 LF，未走 CRLF 归一")
	}
	if !strings.Contains(text, "[附加] 自定义块\r\n") {
		t.Error("附加块缺少 CRLF")
	}
}

// TestWriterNilExtraBlockIsSkipped 验证 nil 附加块被安全跳过。
func TestWriterNilExtraBlockIsSkipped(t *testing.T) {
	wr := newTestWriter()
	wr.Writers = []func(w io.Writer) error{nil, nil}
	if _, err := wr.BuildReportContent(richSnapshot(), nil); err != nil {
		t.Fatalf("nil 附加块导致失败: %v", err)
	}
}

// 断言辅助接口

// 目录冲突上限

// TestReserveReportFileGivesUpAfterMaxAttempts 验证撞名上限后返回错误，
// 而不是无限循环或覆盖已有文件。
func TestReserveReportFileGivesUpAfterMaxAttempts(t *testing.T) {
	if testing.Short() {
		t.Skip("该用例要建 1001 个文件，-short 下跳过")
	}
	dir := t.TempDir()
	base := "diag_20260930_012345.txt"

	// 用只读方式制造「已存在」：只需存在同名文件即可让 O_EXCL 失败。
	for i := 0; i <= maxCollisionAttempts; i++ {
		name := base
		if i > 0 {
			name = strings.TrimSuffix(base, fileNameExt) + "_" + itoa2(i) + fileNameExt
		}
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatalf("预置第 %d 个文件失败: %v", i, err)
		}
	}

	if _, _, err := reserveReportFile(dir, base); err == nil {
		t.Fatal("撞名达到上限时应返回错误")
	} else if !strings.Contains(err.Error(), "冲突过多") {
		t.Errorf("错误信息应说明冲突过多，实际: %v", err)
	}
}

// 探测结果渲染的分支覆盖

// TestRenderProbeBranches 覆盖 renderProbe 的「跳过」「失败」「成功」三类语义，
// 重点是「原因缺失时不能渲染出空洞」。
func TestRenderProbeBranches(t *testing.T) {
	cases := []struct {
		name string
		p    model.ProbeResult
		want []string
	}{
		{
			name: "跳过且原因缺失",
			p:    model.ProbeResult{Kind: model.ProbeTCP443, Skipped: true},
			want: []string{"已跳过（原因: 未说明原因）"},
		},
		{
			name: "跳过且有原因",
			p:    model.ProbeResult{Kind: model.ProbeTCP443, Skipped: true, SkipReason: "该网卡未连接"},
			want: []string{"已跳过（原因: 该网卡未连接）"},
		},
		{
			name: "失败且无错误文本",
			p:    model.ProbeResult{Kind: model.ProbeICMPGateway, Success: false},
			want: []string{"失败（未返回成功且未提供失败原因）"},
		},
		{
			name: "失败且带错误文本",
			p:    model.ProbeResult{Kind: model.ProbeICMPGateway, Success: false, Err: "IcmpSendEcho: 11010"},
			want: []string{"失败（IcmpSendEcho: 11010）"},
		},
		{
			name: "ICMP 成功",
			p: model.ProbeResult{
				Kind: model.ProbeICMPGateway, Success: true, Sent: 4, Recv: 3,
				LossPercent: 25, AvgRTT: 3 * time.Millisecond,
				MinRTT: 2 * time.Millisecond, MaxRTT: 5 * time.Millisecond,
			},
			want: []string{"丢包 25.0%", "共 3/4 包"},
		},
		{
			name: "DNS 成功但无解析结果",
			p: model.ProbeResult{
				Kind: model.ProbeDNSSystem, Success: true,
				Duration: 41 * time.Millisecond,
			},
			want: []string{"成功 → 无解析结果 (41 ms)"},
		},
		{
			name: "TCP 成功但目标缺失",
			p: model.ProbeResult{
				Kind: model.ProbeTCP443, Success: true,
				Duration: 12 * time.Millisecond,
			},
			want: []string{"未采集 可达 (12 ms)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			renderProbe(&sb, tc.p)
			got := sb.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("缺少 %q，实际: %s", want, got)
				}
			}
			// 无论哪个分支都不允许出现空值空洞。
			if strings.Contains(got, ": \r\n") || strings.HasSuffix(strings.TrimRight(got, "\r\n"), ":") {
				t.Errorf("渲染结果出现空值: %q", got)
			}
		})
	}
}

// TestRenderProbeSectionLayerOrdering 验证多条层级结论按 LevelOrder 排序，
// 且续行不会以 ":" 开头（多网卡场景）。
func TestRenderProbeSectionLayerOrdering(t *testing.T) {
	snap := richSnapshot()
	snap.Layers = []model.LayerConclusion{
		{Level: model.LevelOK, Summary: "正常", Severity: model.SevOK},
		{Level: model.LevelWANDNS, Summary: "DNS 异常", Severity: model.SevWarning},
		{Level: model.LevelLANDown, Summary: "局域网不通", Severity: model.SevSevere},
	}

	var sb strings.Builder
	renderProbeSection(&sb, snap)
	out := sb.String()

	iLAN := strings.Index(out, "局域网不通")
	iWAN := strings.Index(out, "DNS 异常")
	iOK := strings.Index(out, "正常")
	if !(iLAN >= 0 && iWAN >= 0 && iOK >= 0 && iLAN < iWAN && iWAN < iOK) {
		t.Errorf("层级结论未按 LevelOrder 排序（lan-down < wan-dns < ok）:\n%s", out)
	}
	if !strings.Contains(out, "[严重] 局域网不通") || !strings.Contains(out, "[警告] DNS 异常") {
		t.Errorf("层级结论缺少等级标记:\n%s", out)
	}
	// 层级结论必须落在统一的值列上：续行的键列用空格占满，冒号不能顶到行首。
	// 期望值直接用 kvWidth 算出，符合同一节内所有键值行共用一个格式化入口的约定。
	for _, want := range []string{
		kvWidth("故障层级判定", "[严重] 局域网不通", kvPad, indentInner),
		kvWidth(strings.Repeat(" ", len([]rune("故障层级判定"))), "[警告] DNS 异常", kvPad, indentInner),
		kvWidth(strings.Repeat(" ", len([]rune("故障层级判定"))), "正常", kvPad, indentInner),
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("层级结论行未对齐到值列，缺少:\n%q\n实际:\n%s", want, out)
		}
	}
	// 多网卡时必须带上 RK-07 的出口归属局限。
	if !strings.Contains(out, "注：ICMP 探测的实际出口") {
		t.Errorf("多网卡未标注出口归属局限:\n%s", out)
	}
}

// TestScopeLabelTable 覆盖 scopeLabel 的枚举分支与未知回落。
func TestScopeLabelTable(t *testing.T) {
	cases := map[string]string{
		model.ScopeGlobal:    "Global 全局",
		model.ScopeLinkLocal: "LinkLocal 链路本地",
		model.ScopeSiteLocal: "SiteLocal 站点本地",
		model.ScopeOther:     "Other",
		"":                   "",
		"Future":             "Future",
	}
	for in, want := range cases {
		if got := scopeLabel(in); got != want {
			t.Errorf("scopeLabel(%q) = %q，want %q", in, got, want)
		}
	}
}

// TestRetryHintAndFailureReasonWording 覆盖写入失败文案的归一化。
//
// 这些文案会直接打到控制台并决定用户下一步怎么做，属于对外契约。
func TestRetryHintAndFailureReasonWording(t *testing.T) {
	if got := writeFailureReason(nil); got != "" {
		t.Errorf("nil 错误应返回空串，实际 %q", got)
	}
	if got := writeFailureReason(os.ErrPermission); !strings.Contains(got, "权限不足") {
		t.Errorf("权限类错误未翻译成可读文案: %q", got)
	}
	diskFull := syscall.Errno(112) // ERROR_DISK_FULL
	if !isDiskFull(diskFull) {
		t.Error("isDiskFull 未识别 ERROR_DISK_FULL(112)")
	}
	if isDiskFull(os.ErrPermission) {
		t.Error("isDiskFull 误判了非磁盘满错误")
	}
	if got := writeFailureReason(diskFull); !strings.Contains(got, "磁盘空间不足") {
		t.Errorf("磁盘满错误未翻译成可读文案: %q", got)
	}
	other := errors.New("杀软拦截了文件写入")
	if got := writeFailureReason(other); !strings.Contains(got, "杀软拦截") {
		t.Errorf("未知错误应保留原文: %q", got)
	}

	msg := ErrorString([]Attempt{
		{Level: LevelExplicit, Label: "命令行 -o 指定目录", Dir: `C:\ro`, Reason: "目录不可写（权限不足）"},
		{Level: LevelExeDir, Label: "EXE 所在目录", Dir: "", Reason: "无法获取可执行文件路径"},
	})
	for _, want := range []string{"均无法写入", "[1/4]", "[2/4]", "(不可用)", "请用 -o 指定一个可写目录后重试"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrorString 缺少 %q:\n%s", want, msg)
		}
	}
}

// TestRenderSectionFailureCleansUpPlaceholder 验证渲染阶段失败时不留下空壳报告。
func TestRenderSectionFailureCleansUpPlaceholder(t *testing.T) {
	dir := t.TempDir()

	// 只注册会失败的附加块，让 BuildReportContent 报错。
	wr := newTestWriter()
	wr.Writers = []func(w io.Writer) error{
		func(w io.Writer) error { return errors.New("附加块渲染失败") },
	}

	_, err := wr.Write(richSnapshot(), nil, []Candidate{
		{Level: LevelExplicit, Label: "命令行 -o 指定目录", Dir: dir},
	})
	if err == nil {
		t.Fatal("附加块渲染失败时 Write 应返回错误")
	}

	// 关键：目录里不能残留任何东西 —— 半截报告比没有报告更危险。
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("渲染失败后残留了文件: %v", names)
	}
}

// TestFirstUNCPathPrefersFirstMatch 验证 EXE 路径优先于报告路径。
func TestFirstUNCPathPrefersFirstMatch(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
		want  string
	}{
		{name: "都没有", paths: []string{`C:\tools`, `D:\reports`}, want: ""},
		{name: "只有报告路径是 UNC", paths: []string{`C:\tools`, `\\srv\share\r`}, want: `\\srv\share\r`},
		{name: "EXE 路径优先", paths: []string{`\\srv\tools`, `\\srv\share\r`}, want: `\\srv\tools`},
		{name: "正斜杠形式", paths: []string{`C:\tools`, `//srv/share/r`}, want: `//srv/share/r`},
		{name: "空白填充被裁掉", paths: []string{"  "}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstUNCPath(tc.paths...); got != tc.want {
				t.Errorf("firstUNCPath(%v) = %q，want %q", tc.paths, got, tc.want)
			}
		})
	}
}

// TestContentLinesTrimsBlankEdges 覆盖空块与首尾空行裁剪。
func TestContentLinesTrimsBlankEdges(t *testing.T) {
	if got := contentLines("\n\n"); got != nil {
		t.Errorf("纯空行应返回 nil，实际 %v", got)
	}
	if got := contentLines(""); got != nil {
		t.Errorf("空串应返回 nil，实际 %v", got)
	}
	got := contentLines("\nA\nB\n\n")
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Errorf("首尾空行未被裁掉: %v", got)
	}
}

// itoa2 是测试用的十进制转换（不引入 strconv 以保持测试文件自足）。
func itoa2(n int) string {
	if n == 0 {
		return "0"
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
