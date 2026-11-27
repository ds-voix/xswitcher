package diary

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfg "xswitcher/internal/config"
)

func TestWriteAndRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "diary.log")
	w := New(cfg.TDiary{Enabled: true, Path: path, MaxSizeMB: 1, RotateCount: 2})
	w.maxSize = 40 // shrink the threshold: the MB default is fine, rotation wants testing cheaply
	for i := 0; i < 8; i++ {
		if err := w.Write(fmt.Sprintf("word-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("first rotation missing: %v", err)
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Errorf("second rotation missing: %v", err)
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Errorf("RotateCount=2 must keep only .1 and .2")
	}
	last, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	// Each line is 27 bytes, so the 8th write crosses the 40-byte
	// threshold and rotates: the newest entry must land in .1.
	if !strings.Contains(string(last), "word-7") {
		t.Errorf("newest entry missing from the first rotation: %q", last)
	}
}

func TestReadTailCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.log")
	var body strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&body, "line %d payload payload payload\n", i)
	}
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	tail, err := Read(path, 300)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(tail)) > 300 {
		t.Errorf("tail longer than the cap: %d", len(tail))
	}
	if !strings.HasSuffix(tail, "199 payload payload payload\n") || !strings.HasPrefix(tail, "line ") {
		t.Errorf("tail must end with the last full line and start at a line boundary: %q", tail[:min(40, len(tail))])
	}
	missing, err := Read(filepath.Join(dir, "nope.log"), 100)
	if err != nil || missing != "" {
		t.Errorf("missing file must read as empty diary: %q %v", missing, err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
