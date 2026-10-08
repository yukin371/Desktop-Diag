//go:build windows

// Verifies offline HTML safety, completeness and exclusive file creation.
package report

import (
	"bytes"
	"errors"
	"github.com/yukin371/desktop-diag/internal/model"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHTMLReportEscapesCollectedContent covers host, findings, raw data and extensions.
func TestHTMLReportEscapesCollectedContent(t *testing.T) {
	snap := richSnapshot()
	payload := `<script>alert("host")</script><img src="https://example.test/leak">`
	snap.Host.ComputerName = payload
	snap.Raw = append(snap.Raw, model.RawLine{Section: "注入", Source: "test", Line: payload})
	wr := newTestWriter()
	wr.HTML = true
	wr.Writers = []func(io.Writer) error{func(w io.Writer) error { _, err := io.WriteString(w, payload); return err }}
	issues := []model.Issue{{RuleID: "R-X", Severity: model.SevSevere, Title: payload, Evidence: []string{payload}}}
	data, err := wr.BuildReportContent(snap, issues)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	if strings.Contains(html, payload) || strings.Contains(html, "<script>") || strings.Contains(html, "<img ") {
		t.Fatal("unescaped collected markup")
	}
	for _, want := range []string{"&lt;script&gt;", `lang="zh-CN"`, "Content-Security-Policy", "严重告警", "原始数据附录", "报告信息", "查看证据与排查建议", "存在严重告警"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !bytes.HasPrefix(data, []byte(BOM)) || strings.Contains(strings.ReplaceAll(html, "\r\n", ""), "\n") {
		t.Fatal("incorrect report encoding")
	}
	again, err := wr.BuildReportContent(snap, issues)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("HTML is not deterministic", err)
	}
}

// TestHTMLWriteCollisionAndFallback preserves completed reports and destination explanations.
func TestHTMLWriteCollisionAndFallback(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("occupied"), reportFileMode); err != nil {
		t.Fatal(err)
	}
	wr := newTestWriter()
	wr.HTML = true
	candidates := []Candidate{{Level: LevelExplicit, Label: "指定目录", Dir: blocked}, {Level: LevelExeDir, Label: "回退目录", Dir: dir}}
	first, err := wr.Write(richSnapshot(), nil, candidates)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := wr.Write(richSnapshot(), nil, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Degraded || filepath.Base(second.Path) != "diag_20260930_012345_1.html" {
		t.Fatalf("bad outcome %+v", second)
	}
	after, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !bytes.Contains(before, []byte(first.Path)) || !bytes.Contains(before, []byte("已降级")) {
		t.Fatal("collision or fallback contract broken")
	}
	if !bytes.HasSuffix(before, []byte("</html>")) {
		t.Fatal("content appended outside HTML document")
	}
}

// TestHTMLEmptyAndFailedExtensions verifies nil snapshots and cleanup on rendering errors.
func TestHTMLEmptyAndFailedExtensions(t *testing.T) {
	wr := newTestWriter()
	wr.HTML = true
	data, err := wr.BuildReportContent(nil, nil)
	if err != nil || !bytes.Contains(data, []byte("未采集")) {
		t.Fatal("nil snapshot", err)
	}
	expected := errors.New("extension failed")
	wr.Writers = []func(io.Writer) error{func(io.Writer) error { return expected }}
	dir := t.TempDir()
	if _, err := wr.Write(nil, nil, []Candidate{{Level: LevelExplicit, Dir: dir}}); !errors.Is(err, expected) {
		t.Fatalf("missing error chain %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("incomplete HTML remained", err)
	}
	if err := RenderHTML(nil, nil, nil, RenderContext{}, nil); err == nil {
		t.Fatal("nil writer accepted")
	}
}

// TestHTMLOverviewCountsAndTargets verifies unique findings, aggregate gaps and missing-data status.
func TestHTMLOverviewCountsAndTargets(t *testing.T) {
	snap := richSnapshot()
	snap.Failures = []model.CollectFailure{{EnvVar: "probe", Item: "网络", Reason: "拒绝"}, {EnvVar: "system", Item: "主机", Reason: "缺失"}}
	issues := []model.Issue{
		{RuleID: "R-01", Severity: model.SevSevere, Title: "IP 缺失"},
		{RuleID: "R-07", Severity: model.SevWarning, Title: "内存偏高"},
		{RuleID: "R-15", Severity: model.SevWarning, Title: "DNS 失败"},
		{RuleID: "R-19", Severity: model.SevWarning, Title: "诊断不完整"},
	}
	findings := []htmlFinding{{ID: "finding-0"}, {ID: "finding-1"}, {ID: "finding-2"}, {ID: "finding-3"}}
	rows := buildHTMLOverview(snap, issues, findings, []htmlSection{{ID: "network", Title: layer2SubAdapters}})
	if rows[1].Severe != 1 || rows[1].Target != "network" || rows[1].Links[0].ID != "finding-0" {
		t.Fatalf("network summary %+v", rows[1])
	}
	if rows[2].Warning != 1 || rows[3].Missing != 1 || rows[4].Missing != 2 || rows[4].Target != "completeness" || rows[0].Status != "数据不完整" {
		t.Fatalf("bad overview %+v", rows)
	}
	empty := buildHTMLOverview(&model.Snapshot{}, nil, nil, nil)
	for _, row := range empty[:4] {
		if row.Class == "normal" {
			t.Fatalf("empty domain labeled normal: %+v", row)
		}
	}
	var out bytes.Buffer
	if err := RenderHTML(&out, snap, issues, RenderContext{}, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="#finding-0"`, `id="finding-0"`, `<details open>`, `href="#overview"`, "各方面诊断总览"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
}
