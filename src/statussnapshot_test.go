package main

import (
	"sort"
	"testing"
)

// The GUI status tab reads daemon state through the atomic snapshots only; they
// must mirror the serve-loop globals and the attached-device map.
func TestStatusSnapshotMirrorsServeLoopState(t *testing.T) {
	oldLang, oldClass := LANG, ActiveWindowClass
	oldAttached := attached
	attached = make(map[string]bool)
	defer func() { LANG, ActiveWindowClass, attached = oldLang, oldClass, oldAttached }()

	LANG = 1
	ActiveWindowClass = "xterm"
	attached["/dev/input/event42"] = true
	updateStatusSnapshot()

	core, devices := currentStatus()
	if core.Lang != 1 || core.Window != "xterm" {
		t.Errorf("core = %+v, want Lang 1 and Window xterm", core)
	}
	if devices == nil {
		t.Fatal("devices must be a non-nil slice: the GUI joins it without checks")
	}
	if len(devices) != 1 || devices[0] != "/dev/input/event42" {
		t.Errorf("devices = %v, want [/dev/input/event42]", devices)
	}
}

// A device detaching after the last key event must still reach the GUI: the
// devices snapshot is refreshed on detach, not only when serve() runs.
func TestDevicesSnapshotReflectsDetach(t *testing.T) {
	oldAttached := attached
	attached = map[string]bool{"/dev/input/event1": true, "/dev/input/event2": true}
	defer func() { attached = oldAttached }()

	updateDevicesSnapshot()
	_, devices := currentStatus()
	want := []string{"/dev/input/event1", "/dev/input/event2"}
	sort.Strings(devices)
	if len(devices) != 2 || devices[0] != want[0] || devices[1] != want[1] {
		t.Fatalf("devices = %v, want %v", devices, want)
	}

	attachedMu.Lock()
	delete(attached, "/dev/input/event1")
	attachedMu.Unlock()
	updateDevicesSnapshot()

	_, devices = currentStatus()
	if len(devices) != 1 || devices[0] != "/dev/input/event2" {
		t.Errorf("devices after detach = %v, want [/dev/input/event2]", devices)
	}
}
