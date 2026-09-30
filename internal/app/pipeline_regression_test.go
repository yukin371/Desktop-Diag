//go:build windows

// This file verifies application contracts with local, deterministic dependencies.
package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yukin371/desktop-diag/internal/cli"
	"github.com/yukin371/desktop-diag/internal/collect"
	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/report"
)

// fixtureCollector runs a local fixture without altering the registration table.
type fixtureCollector struct {
	name string
	fn   func(context.Context, *model.Snapshot) error
}

func (c fixtureCollector) Name() string                                         { return c.name }
func (c fixtureCollector) Collect(ctx context.Context, s *model.Snapshot) error { return c.fn(ctx, s) }

// TestRealtimeProgressAndVerbose checks output before a later collector finishes.
func TestRealtimeProgressAndVerbose(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		var out, errOut bytes.Buffer
		list := []collect.Collector{
			fixtureCollector{"first", func(_ context.Context, s *model.Snapshot) error {
				s.AddRaw("fixture", "source-api", "raw-value=42")
				s.AddFailure("first", "fixture", "optional field missing", true)
				return nil
			}},
			fixtureCollector{"second", func(_ context.Context, _ *model.Snapshot) error {
				text := out.String()
				if !strings.Contains(text, "first") || !strings.Contains(text, "部分采集") || !strings.Contains(text, "second ... 检测中") {
					t.Fatalf("progress not emitted before collection: %s", text)
				}
				return nil
			}},
		}
		args := []string{}
		if verbose {
			args = append(args, "-v")
		}
		write := func(cli.Options, *model.Snapshot, []model.Issue, time.Duration, func() time.Time) (report.Outcome, error) {
			return report.Outcome{Path: "fixture.txt"}, nil
		}
		if code := runWith(args, &out, &errOut, fixedNow, list, write); code != ExitOK {
			t.Fatalf("code=%d stderr=%s", code, errOut.String())
		}
		if strings.Contains(out.String(), "raw-value=42") != verbose || strings.Contains(out.String(), "source-api") != verbose {
			t.Fatalf("verbose=%v output=%s", verbose, out.String())
		}
	}
}

// TestHelpVersionDoNotCollectOrWrite rejects hidden side effects in early exits.
func TestHelpVersionDoNotCollectOrWrite(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-version", "-unknown"} {
		var out, errOut bytes.Buffer
		list := []collect.Collector{fixtureCollector{"forbidden", func(context.Context, *model.Snapshot) error { t.Fatal("early exit collected"); return nil }}}
		write := func(cli.Options, *model.Snapshot, []model.Issue, time.Duration, func() time.Time) (report.Outcome, error) {
			t.Fatal("early exit wrote report")
			return report.Outcome{}, nil
		}
		want := ExitOK
		if arg == "-unknown" {
			want = ExitError
		}
		if got := runWith([]string{arg}, &out, &errOut, fixedNow, list, write); got != want {
			t.Fatalf("%s: %d", arg, got)
		}
	}
}

// TestExitCodesAndElapsedIncludesWrite fixes the three exit states and post-write clock boundary.
func TestExitCodesAndElapsedIncludesWrite(t *testing.T) {
	for _, tc := range []struct {
		name               string
		severe, writeError bool
		want               int
	}{{"ok", false, false, ExitOK}, {"severe", true, false, ExitSevere}, {"error", false, true, ExitError}} {
		t.Run(tc.name, func(t *testing.T) {
			clock := fixedNow()
			var out, errOut bytes.Buffer
			list := []collect.Collector{fixtureCollector{"health", func(ctx context.Context, s *model.Snapshot) error {
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 26*time.Second {
					t.Fatal("collection budget missing")
				}
				s.Health.CPUKnown = true
				if tc.severe {
					s.Health.CPUPercent = 100
				}
				clock = clock.Add(time.Second)
				return nil
			}}}
			write := func(_ cli.Options, _ *model.Snapshot, _ []model.Issue, elapsed time.Duration, _ func() time.Time) (report.Outcome, error) {
				if elapsed != time.Second {
					t.Fatalf("pre-write duration=%s", elapsed)
				}
				clock = clock.Add(2 * time.Second)
				if tc.writeError {
					return report.Outcome{}, errors.New("fixture write failure")
				}
				return report.Outcome{Path: "fixture.txt"}, nil
			}
			if code := runWith(nil, &out, &errOut, func() time.Time { return clock }, list, write); code != tc.want {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
			if !tc.writeError && !strings.Contains(out.String(), "总耗时：3s") {
				t.Fatalf("write omitted from elapsed: %s", out.String())
			}
		})
	}
}

// failingOutput supplies a broken pipe or silent short write.
type failingOutput struct{ short bool }

func (w failingOutput) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, io.ErrClosedPipe
}

// TestOutputErrorsAbortBeforeCollection checks header and early-exit output failures.
func TestOutputErrorsAbortBeforeCollection(t *testing.T) {
	for _, short := range []bool{false, true} {
		for _, args := range [][]string{nil, {"-h"}, {"-version"}} {
			var errOut bytes.Buffer
			list := []collect.Collector{fixtureCollector{"forbidden", func(context.Context, *model.Snapshot) error { t.Fatal("broken output still collected"); return nil }}}
			if code := runWith(args, failingOutput{short}, &errOut, fixedNow, list, nil); code != ExitError {
				t.Fatalf("short=%v args=%v code=%d", short, args, code)
			}
		}
	}
}
