package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Both halves of the device watcher are gated here: a CREATE still has to reach the
// callback, and closing the watcher has to end the loop instead of spinning on the
// closed channels.
func TestWatchInputDevicesForwardsCreateAndStopsOnClose(t *testing.T) {
	dir := t.TempDir()
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		t.Fatalf(`Add("%s"): %v`, dir, err)
	}

	created := make(chan string, 8)
	stopped := make(chan struct{})
	go func() {
		watchInputDevices(w, func(name string) { created <- name })
		close(stopped)
	}()

	path := filepath.Join(dir, "event9")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	select {
	case got := <-created:
		if got != path {
			t.Errorf("CREATE forwarded %q, want %q", got, path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CREATE never reached the callback")
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("watchInputDevices kept reading after the watcher was closed")
	}
}
