//go:build windows

package report

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yukin371/desktop-diag/internal/model"
)

const (
	// fileNameLayout 是 REQ-F-502 规定的文件名格式（本地时间）。
	fileNameLayout = "20060102_150405"
	// fileNamePrefix / fileNameExt 组成 diag_YYYYMMDD_HHMMSS.txt。
	fileNamePrefix = "diag_"
	fileNameExt    = ".txt"

	// collisionSuffixFormat 是撞名时的追加序号（_1、_2…）。
	collisionSuffixFormat = "_%d"

	// maxCollisionAttempts 给后缀递增设上限，避免在被塞满垃圾文件的目录里近乎死循环。
	maxCollisionAttempts = 1000

	// reportFileMode 是报告文件的权限位（设计文档 8.1）。
	reportFileMode = 0o644

	// probeTempPattern 用点前缀隐藏探针文件，且不会与 diag_*.txt 混淆。
	probeTempPattern = ".desktop-diag-write-probe-*"
)

// ReportFileName 返回给定时刻对应的报告文件名（未做撞名检查）。
func ReportFileName(t time.Time) string {
	return fileNamePrefix + t.Format(fileNameLayout) + fileNameExt
}

// 候选层级编号，与设计文档 8.3 的 ①②③④ 一一对应。
const (
	LevelExplicit = 1 // ① -o 指定目录
	LevelExeDir   = 2 // ② EXE 所在目录
	LevelUserDir  = 3 // ③ %USERPROFILE%\Desktop-Diag\reports
	LevelTempDir  = 4 // ④ %TEMP%\Desktop-Diag\reports
)

// Candidate 是写入链上的一个候选目录。
type Candidate struct {
	// Level 是 ①~④ 的序号，用于「最终用了第几级」的表达。
	Level int
	// Label 是该级的人类可读名称，直接进报告头与错误信息。
	Label string
	// Dir 是候选目录（未做 Clean/可写性检查，检查在 Write 中进行）。
	Dir string
	// Reason 在 Dir 为空时说明这一级为什么不可用（如环境变量缺失）。
	Reason string
}

// ResolveTargets 构造四级降级候选链（顺序即优先级）。
//
// 纯函数：不建目录、不探测可写性 —— 真实可写性由 Write 里那次 O_EXCL 创建说了算。
// explicit 为空表示用户没有传 -o，此时跳过第 ① 级。
func ResolveTargets(explicit string) []Candidate {
	out := make([]Candidate, 0, 4)

	if dir := strings.TrimSpace(explicit); dir != "" {
		out = append(out, Candidate{Level: LevelExplicit, Label: "命令行 -o 指定目录", Dir: dir})
	}

	exeDir, exeReason := executableDir()
	out = append(out, Candidate{Level: LevelExeDir, Label: "EXE 所在目录", Dir: exeDir, Reason: exeReason})

	userDir, userReason := userReportDir()
	out = append(out, Candidate{Level: LevelUserDir, Label: `用户目录 %USERPROFILE%\Desktop-Diag\reports`, Dir: userDir, Reason: userReason})

	tempDir, tempReason := tempReportDir()
	out = append(out, Candidate{Level: LevelTempDir, Label: `系统临时目录 %TEMP%\Desktop-Diag\reports`, Dir: tempDir, Reason: tempReason})

	return out
}

// executableDir 返回 EXE 所在目录；os.Executable 失败时给出可读原因。
func executableDir() (string, string) {
	exe, err := os.Executable()
	if err != nil {
		return "", "无法获取可执行文件路径: " + err.Error()
	}
	if exe == "" {
		return "", "无法获取可执行文件路径: 返回空路径"
	}
	return filepath.Dir(exe), ""
}

// userReportDir 返回 %USERPROFILE%\Desktop-Diag\reports。
//
// UserHomeDir 在 Windows 上走 USERPROFILE；失败时退回同名环境变量。
func userReportDir() (string, string) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = os.Getenv("USERPROFILE")
	}
	if strings.TrimSpace(home) == "" {
		return "", "环境变量 USERPROFILE 不可用（os.UserHomeDir 亦失败）"
	}
	return filepath.Join(home, "Desktop-Diag", "reports"), ""
}

// tempReportDir 返回 %TEMP%\Desktop-Diag\reports。
func tempReportDir() (string, string) {
	tmp := strings.TrimSpace(os.TempDir())
	if tmp == "" {
		return "", "环境变量 TEMP/TMP 不可用"
	}
	return filepath.Join(tmp, "Desktop-Diag", "reports"), ""
}

// 写入结果

// Outcome 描述一次报告写入的最终结果。
type Outcome struct {
	// Path 是报告实际落盘路径；全部失败时为空。
	Path string
	// Level 是最终使用的候选层级（1~4）。
	Level int
	// Label 是最终使用层级的名称。
	Label string
	// Degraded 表示发生了降级（即更高级别的候选确实尝试过且失败）。
	Degraded bool
	// Level1Skipped 表示用户未传 -o（第 ① 级本就不在候选链上）。
	// 必须与「试过但失败了」区分开，否则默认路径下正常运行也会显示「已降级」。
	Level1Skipped bool
	// Reason 是降级原因（未降级时为空）。
	Reason string
	// Note 是直接写进报告头「路径选择说明」的一行文案。
	Note string
	// Attempts 是逐级尝试记录，无论成功与否都保留，供上层打印与排查。
	Attempts []Attempt
}

// Attempt 是一级候选的尝试结果。
type Attempt struct {
	Level  int
	Label  string
	Dir    string
	OK     bool
	Reason string
}

// ErrorString 把逐级失败原因拼成一段可直接打印的错误文本（REQ-F-507）。
func ErrorString(attempts []Attempt) string {
	var sb strings.Builder
	sb.WriteString("报告写入失败：已依次尝试全部候选目录，均无法写入。\n")
	for _, a := range attempts {
		dir := a.Dir
		if strings.TrimSpace(dir) == "" {
			dir = "(不可用)"
		}
		fmt.Fprintf(&sb, "  [%d/4] %s (%s): %s\n", a.Level, a.Label, dir, a.Reason)
	}
	sb.WriteString("  提示: 请用 -o 指定一个可写目录后重试。")
	return sb.String()
}

// 渲染与写入

// Writer 是报告的渲染 + 落盘入口。
type Writer struct {
	// RenderContext 提供版本与耗时等报告环境信息；
	// 其中 ReportPath 与 PathNote 由 Write 覆盖，调用方无需填。
	RenderContext RenderContext
	// Writers 是可选的附加正文块，追加在第三层之后（V1.1 扩展点）。
	Writers []func(w io.Writer) error
	// Now 是文件名时间源，零值时使用 time.Now()。测试可注入固定时刻。
	Now func() time.Time
}

// now 返回文件名使用的时间。
func (wr Writer) now() time.Time {
	if wr.Now != nil {
		return wr.Now()
	}
	return time.Now()
}

// BuildReportContent 生成完整报告字节（含 UTF-8 BOM 与 CRLF 换行）。
func (wr Writer) BuildReportContent(snap *model.Snapshot, issues []model.Issue) ([]byte, error) {
	if snap == nil {
		snap = &model.Snapshot{}
	}

	var buf bytes.Buffer
	buf.WriteString(BOM)

	// 报告头里必须有实际路径；调用方若没填（正常流程都由 Write 填），
	// 这里补一个明确的占位，避免报告头出现空洞。
	ctx := wr.RenderContext
	if strings.TrimSpace(ctx.ReportPath) == "" {
		ctx.ReportPath = "(尚未落盘)"
	}

	if err := Render(&buf, snap, issues, ctx); err != nil {
		return nil, err
	}
	for _, extra := range wr.Writers {
		if extra == nil {
			continue
		}
		// 附加块同样要走 CRLF 归一：直接 Write 会把 \n 混进 CRLF 文件里。
		text, err := captureSection(extra)
		if err != nil {
			return nil, err
		}
		if _, err := WriteCRLF(&buf, text); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// captureSection 执行一个附加渲染块并把输出取回为字符串。
func captureSection(fn func(w io.Writer) error) (string, error) {
	var sb strings.Builder
	if err := fn(&sb); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// Write 依次尝试候选目录，把报告写入第一个成功的目录。
//
// 成功时 Outcome 携带实际路径与降级说明；全部失败时 error 非 nil，
// 且 Outcome.Attempts 保留逐级原因供上层打印后以退出码 2 结束。
//
// 单条候选只尝试一次：候选目录是「位置」而不是「重试渠道」，改名重试不会改变结论。
func (wr Writer) Write(snap *model.Snapshot, issues []model.Issue, cands []Candidate) (Outcome, error) {
	snap = snapsOf(snap)

	attempts := make([]Attempt, 0, len(cands))
	// 只有「更高级别的候选确实尝试过且失败」才叫降级。
	sawHigherFailure := false
	// level1Present 记录「用户是否传了 -o」：没传时第 ① 级根本不在候选链上。
	level1Present := false
	for _, c := range cands {
		if c.Level == LevelExplicit {
			level1Present = true
			break
		}
	}

	for _, c := range cands {
		if reason, ok := candidateUnusable(c); !ok {
			attempts = append(attempts, Attempt{Level: c.Level, Label: c.Label, Dir: c.Dir, Reason: reason})
			sawHigherFailure = true
			continue
		}

		// 只有确定要往这一级写，才创建目录 —— 「探测不留垃圾」的原则同样适用于
		// 「不要在不打算用的地方留下空目录」。
		if err := os.MkdirAll(c.Dir, 0o755); err != nil {
			attempts = append(attempts, Attempt{
				Level: c.Level, Label: c.Label, Dir: c.Dir,
				Reason: "创建目录失败: " + err.Error(),
			})
			sawHigherFailure = true
			continue
		}

		// 降级说明在渲染前就要定下来，因为它要写进报告头（REQ-F-506）。
		// 此刻已能确定「更高级别都失败了」，而本级必然会成功或转入下一级。
		degraded := sawHigherFailure
		reason := ""
		if degraded {
			reason = degradeReason(attempts)
		}

		path, err := wr.writeToDir(c, snap, issues, degraded, reason)
		if err != nil {
			attempts = append(attempts, Attempt{
				Level: c.Level, Label: c.Label, Dir: c.Dir,
				Reason: writeFailureReason(err),
			})
			sawHigherFailure = true
			continue
		}

		attempts = append(attempts, Attempt{Level: c.Level, Label: c.Label, Dir: c.Dir, OK: true})

		out := Outcome{
			Path:          path,
			Level:         c.Level,
			Label:         c.Label,
			Degraded:      degraded,
			Level1Skipped: !level1Present,
			Reason:        reason,
			Attempts:      attempts,
		}
		if degraded {
			out.Note = fmt.Sprintf("已降级至 %s（原因: %s）；实际路径: %s", c.Label, reason, path)
		} else {
			out.Note = fmt.Sprintf("未降级，使用 %s；实际路径: %s", c.Label, path)
		}
		return out, nil
	}

	return Outcome{Attempts: attempts}, errors.New(ErrorString(attempts))
}

// snapsOf 让 nil 快照在调用链上尽早归一，避免每处都写一遍判空。
func snapsOf(snap *model.Snapshot) *model.Snapshot {
	if snap == nil {
		return &model.Snapshot{}
	}
	return snap
}

// candidateUnusable 判断一个候选是否在本轮就不可用，并给出原因。
func candidateUnusable(c Candidate) (string, bool) {
	if strings.TrimSpace(c.Dir) == "" {
		if strings.TrimSpace(c.Reason) != "" {
			return c.Reason, false
		}
		return "候选目录为空", false
	}
	if fi, err := os.Stat(c.Dir); err == nil && !fi.IsDir() {
		// 路径存在但不是目录（例如用户把 -o 指到了某个文件）：明确报错，
		// 不能盲目 MkdirAll —— 那会返回意义含混的 "file exists"。
		return "路径已存在但不是目录", false
	}
	// 父目录已存在但不是目录时，MkdirAll 会失败；这里提前给出更清楚的原因，
	// 也避免在候选链上白跑一轮。
	if parent := filepath.Dir(c.Dir); parent != c.Dir && parent != "." {
		if fi, err := os.Stat(parent); err == nil && !fi.IsDir() {
			return fmt.Sprintf("上级路径 %s 已存在但不是目录", parent), false
		}
	}
	return "", true
}

// writeToDir 在 dir 中创建一份报告：先占位、再渲染、后填充。
//
// 必须先占位：报告头要写实际落盘路径（REQ-F-506），而文件名只有 O_EXCL
// 成功后才确定。任何一步失败都删除占位文件 —— 半截报告会被当成「机器没问题」。
func (wr Writer) writeToDir(c Candidate, snap *model.Snapshot, issues []model.Issue, degraded bool, reason string) (string, error) {
	f, full, err := reserveReportFile(c.Dir, ReportFileName(wr.now()))
	if err != nil {
		return "", err
	}
	// 报告会被复制、转发、归档，相对路径对读者没有意义，一律记绝对路径。
	if abs, aerr := filepath.Abs(full); aerr == nil {
		full = abs
	}

	// 清理失败不改变对外结果，只记录原始失败原因。
	fail := func(cause error) (string, error) {
		if cerr := f.Close(); cerr != nil {
			cause = errors.Join(cause, fmt.Errorf("关闭占位文件失败: %w", cerr))
		}
		if rerr := os.Remove(full); rerr != nil {
			cause = errors.Join(cause, fmt.Errorf("删除占位文件 %s 失败: %w", full, rerr))
		}
		return "", cause
	}

	ctx := wr.RenderContext
	ctx.ReportPath = full
	ctx.PathNote = describePath(c, degraded, reason)
	wr.RenderContext = ctx

	content, err := wr.BuildReportContent(snap, issues)
	if err != nil {
		return fail(fmt.Errorf("渲染报告内容失败: %w", err))
	}

	if _, err := f.Write(content); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(full)
		return "", err
	}
	return full, nil
}

// reserveReportFile 以 O_CREATE|O_EXCL 创建报告文件，撞名时追加 _1、_2 重试。
//
// O_EXCL 让「覆盖上一份报告」在内核层面不可能发生，而不是靠有竞态的 Stat+Create。
func reserveReportFile(dir, baseName string) (*os.File, string, error) {
	for attempt := 0; attempt <= maxCollisionAttempts; attempt++ {
		name := baseName
		if attempt > 0 {
			name = strings.TrimSuffix(baseName, fileNameExt) + fmt.Sprintf(collisionSuffixFormat, attempt) + fileNameExt
		}
		full := filepath.Join(dir, name)

		f, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, reportFileMode)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				// 同秒内已有一份报告：绝不覆盖，递增后缀重试（REQ-F-502）。
				continue
			}
			return nil, "", err
		}
		return f, full, nil
	}

	return nil, "", fmt.Errorf("同一秒内文件名冲突过多（已尝试 %d 个后缀），放弃当前目录", maxCollisionAttempts)
}

// writeFailureReason 把写入错误翻译成可直接给用户看的文案。
func writeFailureReason(err error) string {
	if err == nil {
		return ""
	}
	if os.IsPermission(err) {
		return "目录不可写（权限不足）: " + err.Error()
	}
	if isDiskFull(err) {
		return "磁盘空间不足: " + err.Error()
	}
	return err.Error()
}

// errDiskFull 对应 Win32 的 ERROR_DISK_FULL。
const errDiskFull = 112

// isDiskFull 判断错误是否为磁盘写满。
func isDiskFull(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return uintptr(errno) == errDiskFull
	}
	return false
}

// describePath 生成报告头的「路径选择说明」。
//
// 未传 -o 时第 ① 级根本不在候选链上，绝不能写成「已降级」——那是误报。
// 因此用 LevelExplicit 区分「用户指定了目录」与「走了默认位置」。
func describePath(c Candidate, degraded bool, reason string) string {
	switch {
	case degraded:
		return fmt.Sprintf("第 ① 级（命令行 -o 指定目录）不可用，已降级至 %s（原因: %s）", c.Label, reason)
	case c.Level == LevelExplicit:
		return "由命令行 -o 指定目录（未降级）"
	default:
		return fmt.Sprintf("未传 -o，改用 %s（未降级）", c.Label)
	}
}

// degradeReason 把更高级别的失败原因压成一句可放进报告头的话。
//
// 取最后一条失败记录：④ 级成功时，「③ 级为什么也不行」比「② 级也不行」有用。
func degradeReason(attempts []Attempt) string {
	failed := make([]Attempt, 0, len(attempts))
	for _, a := range attempts {
		if !a.OK {
			failed = append(failed, a)
		}
	}
	if len(failed) == 0 {
		return "未说明"
	}
	last := failed[len(failed)-1]
	return fmt.Sprintf("%s 不可用（%s）", last.Label, last.Reason)
}

// EnsureWritable 探测 dir 是否可写，必要时创建它。
//
// 只认「真的建出一个文件」：Windows 的目录只读位不决定能否创建文件，不可靠。
// 探针文件在返回前必定删除；正式写入路径不依赖本函数。
func EnsureWritable(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("目录为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, probeTempPattern)
	if err != nil {
		return err
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr
	}
	if removeErr != nil {
		return fmt.Errorf("探针文件创建成功但无法删除（目录可能只允许创建）: %w", removeErr)
	}
	return nil
}

// TempProbeFiles 返回 dir 中残留的探针文件（正常情况下应为空），供测试断言。
func TempProbeFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(probeTempPattern, "*")
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// LevelName 返回候选层级的稳定名称，便于日志与测试断言。
func LevelName(level int) string {
	switch level {
	case LevelExplicit:
		return "①命令行 -o"
	case LevelExeDir:
		return "②EXE 所在目录"
	case LevelUserDir:
		return "③用户目录"
	case LevelTempDir:
		return "④系统临时目录"
	default:
		return "未知层级(" + strconv.Itoa(level) + ")"
	}
}
