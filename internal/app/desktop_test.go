//go:build windows

// Checks launch guidance without invoking a real report viewer.
package app

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestDesktopGuideCancellationAndErrors ensures no diagnostic begins before consent.
func TestDesktopGuideCancellationAndErrors(t *testing.T) {
	for _, test := range []struct {
		input string
		code  int
	}{{"q\n", ExitOK}, {"wrong\nQ\n", ExitOK}, {"", ExitError}} {
		var out, errOut bytes.Buffer
		code := runDesktop(nil, &out, &errOut, strings.NewReader(test.input), true, time.Now)
		if code != test.code || !strings.Contains(out.String(), "按 Enter") || strings.Contains(out.String(), "[1/4]") {
			t.Fatalf("guide result %d %s", code, out.String())
		}
	}
	var out, errOut bytes.Buffer
	if code := runDesktop([]string{"-h"}, &out, &errOut, strings.NewReader(""), true, time.Now); code != ExitOK || strings.Contains(out.String(), "按 Enter") {
		t.Fatal("help waited for input")
	}
}

// TestDesktopGuideStartsAndFinishes covers the Enter path while disabling viewer side effects.
func TestDesktopGuideStartsAndFinishes(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runDesktop([]string{"-no-open", "-o", t.TempDir()}, &out, &errOut, strings.NewReader("\n\n"), true, time.Now)
	if code > ExitSevere || !strings.Contains(out.String(), "报告路径") || !strings.Contains(out.String(), "按 Enter 关闭窗口") {
		t.Fatalf("guide diagnosis failed %d %s %s", code, out.String(), errOut.String())
	}
}
