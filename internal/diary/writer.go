// Package diary implements the opt-in typed-text diary: the daemon
// appends one line per completed word and rotates the file by size.
// Recording typed text is a keylogger capability — the writer only ever
// exists when [Diary] Enabled = true in the config.
package diary

import (
	"fmt"
	"os"
	"time"

	cfg "xswitcher/internal/config"
)

// Writer appends timestamped lines to a file and rotates it when the
// size crosses MaxSizeMB, keeping RotateCount numbered older copies
// (path.1 is the newest). Not goroutine-safe by design: the daemon calls
// Write from its single key-event path.
type Writer struct {
	path    string
	maxSize int64
	keep    int
}

// New builds a writer from a validated [Diary] section. Rotation settings
// are floored defensively even though the parser gates them.
func New(d cfg.TDiary) *Writer {
	max := int64(d.MaxSizeMB)
	if max < 1 {
		max = 1
	}
	keep := d.RotateCount
	if keep < 0 {
		keep = 0
	}
	return &Writer{path: d.Path, maxSize: max << 20, keep: keep}
}

// Write appends one diary line. The file is created 0600: it belongs to
// the daemon user only; the GUI reads it through the get_diary command.
func (w *Writer) Write(text string) error {
	line := fmt.Sprintf("%s\t%s\n", time.Now().Format("2006-01-02 15:04:05"), text)
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(line); err != nil {
		f.Close()
		return err
	}
	size, err := f.Seek(0, os.SEEK_END)
	f.Close()
	if err != nil {
		return err
	}
	if size >= w.maxSize {
		return w.rotate()
	}
	return nil
}

// rotate shifts path.N-1 -> path.N ... path -> path.1 and drops the
// oldest copy beyond the keep limit.
func (w *Writer) rotate() error {
	os.Remove(fmt.Sprintf("%s.%d", w.path, w.keep))
	for i := w.keep - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", w.path, i)
		to := fmt.Sprintf("%s.%d", w.path, i+1)
		if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(w.path, w.path+".1")
}

// Read returns the diary content, capped to the last maxBytes so a huge
// file never crosses the whole socket at once. A missing file is an
// empty diary, not an error.
func Read(path string, maxBytes int64) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if int64(len(data)) > maxBytes {
		trimmed := data[int64(len(data))-maxBytes:]
		// Drop the partial first line a tail-cut can leave behind.
		for i, b := range trimmed {
			if b == '\n' {
				trimmed = trimmed[i+1:]
				break
			}
		}
		data = trimmed
	}
	return string(data), nil
}
