package main

import (
	"sort"
	"testing"
)

// The GUI status tab reads daemon state through the atomic snapshots only; they
// must mirror the serve-loop globals and the attached-device map.
func TestStatusSnapshotMirrorsServeLoopState(t *testing.T) {
	oldLang, oldClass := LANG, ActiveWindowClass
	oldWord, oldAttached := wordLayout, attached
	attached = make(map[string]bool)
	defer func() {
		LANG, ActiveWindowClass, wordLayout, attached = oldLang, oldClass, oldWord, oldAttached
	}()

	LANG = 1
	wordLayout = 2
	ActiveWindowClass = "xterm"
	attached["/dev/input/event42"] = true
	updateStatusSnapshot()

	core, devices := currentStatus()
	if core.Lang != 1 || core.Window != "xterm" {
		t.Errorf("core = %+v, want Lang 1 and Window xterm", core)
	}
	// The GUI's [AutoDetect] preview answers about the group the word was typed
	// in, which is a different number from the layout the daemon sits on now: 1
	// here, 2 there. If the snapshot ever collapses the two, the preview promises
	// corrections the daemon will not make.
	if core.WordGroup != 2 {
		t.Errorf("WordGroup = %d, want the 2 the word was typed in, not the current layout %d", core.WordGroup, core.Lang)
	}
	wordLayout = -1 // no character of the word recorded yet
	updateStatusSnapshot()
	if core, _ = currentStatus(); core.WordGroup != -1 {
		t.Errorf("WordGroup = %d, want the -1 the daemon means by it", core.WordGroup)
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
