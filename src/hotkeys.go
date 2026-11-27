// Package main — see xswitcher.go for overview.
// hotkeys.go evaluates the [[Hotkeys]] combos on every key press, straight
// from the CTRL modifier map and independent of the managed layout, so a
// combo works even where the SEQ machinery is switched off.
package main

import (
	"fmt"

	cfg "xswitcher/internal/config"
)

var hotkeyByTrigger = map[string][]THotkey{}

// buildHotkeys indexes the rules by their single trigger key and warns about
// names the daemon's key table does not know: such a rule can never fire.
func buildHotkeys() {
	hotkeyByTrigger = make(map[string][]THotkey, len(Hotkeys))
	for _, hk := range Hotkeys {
		for _, k := range hk.Keys {
			if !cfg.HotkeyModifiers[k] {
				hotkeyByTrigger[k] = append(hotkeyByTrigger[k], hk)
			}
		}
	}
	for name := range hotkeyByTrigger {
		if _, ok := key_def[name]; !ok || key_def[name] == 0 {
			fmt.Printf("Hotkey warning: unknown key %q can never fire\n", name)
		}
	}
}

// checkHotkeys is called from checkAppend right after the CTRL map was
// updated, so the modifier state it reads is the state of this very event.
// Only a fresh press (value 1) fires: repeats cannot trigger a combo twice.
func checkHotkeys(event t_key) {
	if event.value != 1 || len(Hotkeys) == 0 {
		return
	}
	name := key_name[event.code]
	rules, ok := hotkeyByTrigger[name]
	if !ok {
		return
	}
	for _, hk := range rules {
		held := true
		for _, k := range hk.Keys {
			if k == name {
				continue
			}
			if !CTRL[k] {
				held = false
				break
			}
		}
		if !held {
			continue
		}
		if *VERBOSE || *DEBUG {
			fmt.Printf("Hotkey: %v >> %s\n", hk.Keys, hk.Action)
		}
		// The chain must not protect keys with an EXTRA left over from an
		// unrelated SEQ match: a hotkey fires for itself, tail included.
		EXTRA = 0
		action := hk.Action
		doAction(&action)
		return // one action per press, however many rules matched
	}
}
