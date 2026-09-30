//go:build windows

// This file verifies report errors retain cleanup failures and silent short writes.
package report

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/yukin371/desktop-diag/internal/model"
)

// faultyReport simulates file write and close failures without modifying the filesystem.
type faultyReport struct {
	writeErr, closeErr error
	short              bool
	closed             int
}

func (f *faultyReport) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.short {
		return len(p) - 1, nil
	}
	return len(p), nil
}
func (f *faultyReport) Close() error { f.closed++; return f.closeErr }

// TestWriteFailureRetainsAllCauses checks both Write and Close branches and failed removal.
func TestWriteFailureRetainsAllCauses(t *testing.T) {
	writeErr, closeErr, removeErr := errors.New("write fixture"), errors.New("close fixture"), errors.New("remove fixture")
	for _, tc := range []struct {
		name  string
		write error
		short bool
		want  error
	}{{"write", writeErr, false, writeErr}, {"close", nil, false, closeErr}, {"short", nil, true, io.ErrShortWrite}} {
		t.Run(tc.name, func(t *testing.T) {
			file := &faultyReport{writeErr: tc.write, closeErr: closeErr, short: tc.short}
			removed := 0
			reserved := 0
			wr := Writer{reserve: func(dir, name string) (io.WriteCloser, string, error) {
				reserved++
				return file, filepath.Join(dir, name), nil
			}, remove: func(string) error { removed++; return removeErr }}
			out, err := wr.Write(&model.Snapshot{}, nil, []Candidate{{Level: LevelExplicit, Label: "fixture", Dir: t.TempDir()}, {Level: LevelTempDir, Label: "must not hide cleanup failure", Dir: t.TempDir()}})
			if !errors.Is(err, tc.want) || !errors.Is(err, closeErr) || !errors.Is(err, removeErr) || file.closed != 1 || removed != 1 || reserved != 1 || out.Path != "" {
				t.Fatalf("out=%+v err=%v closed=%d removed=%d", out, err, file.closed, removed)
			}
		})
	}
	if err := Render(&faultyReport{short: true}, nil, nil, RenderContext{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("render short write lost: %v", err)
	}
}
