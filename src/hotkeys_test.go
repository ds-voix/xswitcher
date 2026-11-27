package main

import "testing"

// A hotkey fires only on a fresh press of its trigger key, with every
// modifier of the combo held; other events must stay untouched.
func TestCheckHotkeys(t *testing.T) {
	keys()

	origHotkeys, origActionSet, origCTRL, origExtra := Hotkeys, ActionSet, copyCTRL(CTRL), EXTRA
	defer func() {
		Hotkeys, ActionSet, EXTRA = origHotkeys, origActionSet, origExtra
		CTRL = origCTRL
		delete(ACTIONS, "TestHotkeyMark")
	}()

	fired := 0
	ACTIONS["TestHotkeyMark"] = func(*TAction) { fired++ }
	ActionSet = map[string]TAction{"Combo": {Action: []string{"TestHotkeyMark"}}}
	Hotkeys = []THotkey{{Keys: []string{"L_CTRL", "L_ALT", "F1"}, Action: "Combo"}}
	buildHotkeys()

	f1 := t_key{uint16(key_def["F1"]), 1}

	CTRL = map[string]bool{"L_CTRL": true}
	EXTRA = 7
	checkHotkeys(f1) // L_ALT missing
	if fired != 0 {
		t.Error("combo fired without all modifiers held")
	}

	CTRL = map[string]bool{"L_CTRL": true, "L_ALT": true}
	checkHotkeys(f1)
	if fired != 1 || EXTRA != 0 {
		t.Errorf("combo did not fire cleanly: fired=%d EXTRA=%d", fired, EXTRA)
	}

	checkHotkeys(t_key{f1.code, 0}) // release
	checkHotkeys(t_key{f1.code, 2}) // autorepeat
	if fired != 1 {
		t.Errorf("release/repeat re-fired the combo: fired=%d", fired)
	}

	// An unknown trigger name is indexed (and warned about) but no physical
	// event can ever carry it, so nothing fires.
	Hotkeys = []THotkey{{Keys: []string{"NOTAKEY"}, Action: "Combo"}}
	buildHotkeys()
	checkHotkeys(f1)
	if fired != 1 {
		t.Errorf("fired after reindex without the rule: %d", fired)
	}
}
