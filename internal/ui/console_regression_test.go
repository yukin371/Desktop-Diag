//go:build windows

// This file tests console modes and shared-state restoration with no real console changes.
package ui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/yukin371/desktop-diag/internal/model"
	"github.com/yukin371/desktop-diag/internal/winapi"
)

// TestConsoleSetupModesAndRestore exercises both handles sharing a code page in LIFO order.
func TestConsoleSetupModesAndRestore(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		cp                      uint32
		cpFail, vtFail, noColor bool
		mode                    Mode
	}{{"color", 936, false, false, false, ModeColor}, {"vt-failure", 936, false, true, false, ModePlain}, {"no-color", 936, false, false, true, ModePlain}, {"cp-failure", 936, true, false, false, ModeASCII}, {"cp-unavailable", 0, false, false, false, ModeASCII}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")
			if tc.noColor {
				t.Setenv("NO_COLOR", "1")
			}
			cp := tc.cp
			var setCP []uint32
			var setMode []uint32
			api := consoleAPI{getMode: func(uintptr) (uint32, error) { return 1, nil }, getCP: func() uint32 { return cp }, setCP: func(value uint32) error {
				if tc.cpFail {
					return errors.New("cp fixture")
				}
				setCP = append(setCP, value)
				cp = value
				return nil
			}, setMode: func(_ uintptr, value uint32) error {
				if tc.vtFail {
					return errors.New("vt fixture")
				}
				setMode = append(setMode, value)
				return nil
			}}
			first, second := setup(os.Stdout, api), setup(os.Stderr, api)
			if first.Mode() != tc.mode || second.Mode() != tc.mode {
				t.Fatalf("got %v/%v", first.Mode(), second.Mode())
			}
			if tc.vtFail && !strings.Contains(first.notice, "已使用中文纯文本输出") {
				t.Fatalf("颜色降级提示未中文化: %s", first.notice)
			}
			if err := second.Close(); err != nil {
				t.Fatal(err)
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			if cp != tc.cp {
				t.Fatalf("shared cp changed: %d", cp)
			}
			if !tc.cpFail && tc.cp != 0 && !reflect.DeepEqual(setCP, []uint32{winapi.CPUTF8, winapi.CPUTF8, winapi.CPUTF8, tc.cp}) {
				t.Fatalf("restore order=%v", setCP)
			}
			if tc.mode == ModeColor && !reflect.DeepEqual(setMode, []uint32{winapi.VTOutputMode(1), winapi.VTOutputMode(1), 1, 1}) {
				t.Fatalf("mode restore=%v", setMode)
			}
			before := len(setCP)
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			if len(setCP) != before {
				t.Fatal("Close not idempotent")
			}
		})
	}
}

// TestConsoleRestoreRetainsBothErrors verifies a failed mode restore still restores the code page.
func TestConsoleRestoreRetainsBothErrors(t *testing.T) {
	modeErr, cpErr := errors.New("mode restore"), errors.New("cp restore")
	api := consoleAPI{getMode: func(uintptr) (uint32, error) { return 1, nil }, getCP: func() uint32 { return 936 }, setMode: func(_ uintptr, v uint32) error {
		if v == 1 {
			return modeErr
		}
		return nil
	}, setCP: func(v uint32) error {
		if v == 936 {
			return cpErr
		}
		return nil
	}}
	t.Setenv("NO_COLOR", "")
	c := setup(os.Stdout, api)
	if err := c.Close(); !errors.Is(err, modeErr) || !errors.Is(err, cpErr) {
		t.Fatalf("restore errors lost: %v", err)
	}
	api.getMode = func(uintptr) (uint32, error) { return 0, errors.New("not console") }
	api.setCP = func(uint32) error { t.Fatal("redirected output changed console"); return nil }
	if c := setup(os.Stdout, api); c.Mode() != ModePlain {
		t.Fatal("nonconsole not plain")
	}
}

// shortOutput returns no error but fewer bytes than requested.
type shortOutput struct{}

func (shortOutput) Write(p []byte) (int, error) { return len(p) - 1, nil }

// TestASCIIAndWriteErrors checks the complete fallback output rather than just its mode enum.
func TestASCIIAndWriteErrors(t *testing.T) {
	var out bytes.Buffer
	c := New(&out, ModeASCII)
	c.Header("test")
	c.Step(1, 4, "网络适配器信息", "部分采集", model.SevWarning)
	c.Issue(model.Issue{RuleID: "R-19", Severity: model.SevWarning, Title: "采集不完整", Suggestion: "查看报告"})
	c.Summary(0, 1, "C:\\中文\\diag.txt", 0)
	for _, b := range out.Bytes() {
		if b > 127 {
			t.Fatalf("fallback contains non-ASCII: %q", out.String())
		}
	}
	for _, word := range []string{"Network adapters", "Partial", "WARNING", "Report path", "\\u"} {
		if !strings.Contains(out.String(), word) {
			t.Errorf("fallback missing %q: %s", word, out.String())
		}
	}
	c = New(shortOutput{}, ModePlain)
	c.Line("hello")
	if !errors.Is(c.Err(), io.ErrShortWrite) {
		t.Fatalf("short write swallowed: %v", c.Err())
	}
}
