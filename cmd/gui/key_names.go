// Package main — see main.go for overview.
// key_names.go is the trigger-key catalogue the Hotkeys dropdown offers. The
// daemon resolves a key name against its own key_def table at run time (that
// map lives in package main with the cgo/X11 code and is not importable here),
// and it silently drops a name it does not know. So the dropdown must list only
// canonical spellings, and it lists the keys a layout switcher can realistically
// bind: letters, digits, punctuation, navigation and F1-F24 -- not the ~600
// consumer-electronic codes key_def also carries.
//
// Every option reads "human name (canonical name)" and maps back to the
// canonical spelling, which is what [[Hotkeys]] "Keys" stores. Modifiers are NOT
// here: they are the Ctrl/Shift/Alt checkboxes, and key_def only accepts the
// L_/R_ forms of them, so a bare "CTRL" would be counted as a second trigger and
// the daemon's parser would refuse the combo.
package main

import (
	"fmt"
	"strings"
)

// keyTriggerOption is one dropdown entry: the canonical daemon name and the
// label shown to the user.
type keyTriggerOption struct {
	real    string
	display string
}

var keyTriggerList = buildKeyTriggerList()

// displayByReal/realByDisplay index the curated list both ways.
var (
	keyDisplayByReal = map[string]string{}
	keyRealByDisplay = map[string]string{}
)

func buildKeyTriggerList() []keyTriggerOption {
	var out []keyTriggerOption
	add := func(real, human string) {
		out = append(out, keyTriggerOption{real: real, display: human + " (" + real + ")"})
	}
	for r := 'A'; r <= 'Z'; r++ {
		c := string(r)
		add(c, c)
	}
	for d := '0'; d <= '9'; d++ {
		c := string(d)
		add(c, c)
	}
	// Canonical punctuation spellings from key_def; the single-character
	// aliases (MINUS, ";", "[", ...) are added to key_def only at daemon run
	// time by keys(), so the GUI names these by their canonical token.
	add("-", "Hyphen")
	add("=", "Equals")
	add("SEMICOLON", "Semicolon")
	add("APOSTROPHE", "Apostrophe")
	add("GRAVE", "Grave")
	add("BACKSLASH", "Backslash")
	add("COMMA", "Comma")
	add("DOT", "Dot")
	add("SLASH", "Slash")
	add("L_BRACE", "Left bracket")
	add("R_BRACE", "Right bracket")
	add("TAB", "Tab")
	add("ENTER", "Enter")
	add("BACKSPACE", "Backspace")
	add("SPACE", "Space")
	add("ESC", "Escape")
	add("UP", "Up")
	add("DOWN", "Down")
	add("LEFT", "Left")
	add("RIGHT", "Right")
	add("HOME", "Home")
	add("END", "End")
	add("INSERT", "Insert")
	add("DELETE", "Delete")
	add("PAGEUP", "Page up")
	add("PAGEDOWN", "Page down")
	for i := 1; i <= 24; i++ {
		f := fmt.Sprintf("F%d", i)
		add(f, f)
	}
	return out
}

func init() {
	for _, o := range keyTriggerList {
		keyDisplayByReal[o.real] = o.display
		keyRealByDisplay[o.display] = o.real
	}
}

// keyTriggerOptions returns the dropdown labels in catalogue order.
func keyTriggerOptions() []string {
	opts := make([]string, 0, len(keyTriggerList))
	for _, o := range keyTriggerList {
		opts = append(opts, o.display)
	}
	return opts
}

// keyLabelFor renders a canonical name as a dropdown label. A name outside the
// curated list (only reachable when loading a hand-written config) still shows
// as "NAME (NAME)" so the stored value survives an open/save round trip.
func keyLabelFor(real string) string {
	if d, ok := keyDisplayByReal[real]; ok {
		return d
	}
	return real + " (" + real + ")"
}

// keyRealFor turns a dropdown label back into the canonical daemon name: the
// curated lookup first, then the text inside the trailing parentheses, then the
// whole string upper-cased as a last resort.
func keyRealFor(display string) string {
	if r, ok := keyRealByDisplay[display]; ok {
		return r
	}
	if open := strings.LastIndexByte(display, '('); open >= 0 && strings.HasSuffix(display, ")") {
		if inner := strings.TrimSpace(display[open+1 : len(display)-1]); inner != "" {
			return inner
		}
	}
	return strings.ToUpper(strings.TrimSpace(display))
}

// keyHasReal reports whether the canonical name is in the curated catalogue.
func keyHasReal(real string) bool {
	_, ok := keyDisplayByReal[real]
	return ok
}
