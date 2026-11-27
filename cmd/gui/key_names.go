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
	"sync/atomic"

	"xswitcher/internal/l10n"
)

// keyTriggerOption is one dropdown entry: the canonical daemon name and the
// label shown to the user.
type keyTriggerOption struct {
	real    string
	display string
}

// keyNameTable is the catalogue for one locale, indexed both ways.
type keyNameTable struct {
	code          string
	options       []keyTriggerOption
	displayByReal map[string]string
	realByDisplay map[string]string
}

// keyNames holds the table for the locale it was built under. Package init runs
// before main() calls l10n.Init(), so a list built once would be frozen in
// English and the Hotkeys dropdown would stay English in a Russian window; the
// table is rebuilt whenever the active locale differs from the cached one.
var keyNames atomic.Pointer[keyNameTable]

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
	add("-", l10n.X("keyname.hyphen", "Hyphen"))
	add("=", l10n.X("keyname.equals", "Equals"))
	add("SEMICOLON", l10n.X("keyname.semicolon", "Semicolon"))
	add("APOSTROPHE", l10n.X("keyname.apostrophe", "Apostrophe"))
	add("GRAVE", l10n.X("keyname.grave", "Grave"))
	add("BACKSLASH", l10n.X("keyname.backslash", "Backslash"))
	add("COMMA", l10n.X("keyname.comma", "Comma"))
	add("DOT", l10n.X("keyname.dot", "Dot"))
	add("SLASH", l10n.X("keyname.slash", "Slash"))
	add("L_BRACE", l10n.X("keyname.left.bracket", "Left bracket"))
	add("R_BRACE", l10n.X("keyname.right.bracket", "Right bracket"))
	add("TAB", l10n.X("keyname.tab", "Tab"))
	add("ENTER", l10n.X("keyname.enter", "Enter"))
	add("BACKSPACE", l10n.X("keyname.backspace", "Backspace"))
	add("SPACE", l10n.X("keyname.space", "Space"))
	add("ESC", l10n.X("keyname.escape", "Escape"))
	add("UP", l10n.X("keyname.up", "Up"))
	add("DOWN", l10n.X("keyname.down", "Down"))
	add("LEFT", l10n.X("keyname.left", "Left"))
	add("RIGHT", l10n.X("keyname.right", "Right"))
	add("HOME", l10n.X("keyname.home", "Home"))
	add("END", l10n.X("keyname.end", "End"))
	add("INSERT", l10n.X("keyname.insert", "Insert"))
	add("DELETE", l10n.X("keyname.delete", "Delete"))
	add("PAGEUP", l10n.X("keyname.page.up", "Page up"))
	add("PAGEDOWN", l10n.X("keyname.page.down", "Page down"))
	for i := 1; i <= 24; i++ {
		f := fmt.Sprintf("F%d", i)
		add(f, l10n.X("keyname.fn", "F{{.N}}", "N", i))
	}
	return out
}

func keyTable() *keyNameTable {
	code := l10n.Current()
	if t := keyNames.Load(); t != nil && t.code == code {
		return t
	}
	options := buildKeyTriggerList()
	t := &keyNameTable{
		code:          code,
		options:       options,
		displayByReal: map[string]string{},
		realByDisplay: map[string]string{},
	}
	for _, o := range options {
		t.displayByReal[o.real] = o.display
		t.realByDisplay[o.display] = o.real
	}
	keyNames.Store(t)
	return t
}

// keyTriggerOptions returns the dropdown labels in catalogue order.
func keyTriggerOptions() []string {
	options := keyTable().options
	opts := make([]string, 0, len(options))
	for _, o := range options {
		opts = append(opts, o.display)
	}
	return opts
}

// keyLabelFor renders a canonical name as a dropdown label. A name outside the
// curated list (only reachable when loading a hand-written config) still shows
// as "NAME (NAME)" so the stored value survives an open/save round trip.
func keyLabelFor(real string) string {
	if d, ok := keyTable().displayByReal[real]; ok {
		return d
	}
	return l10n.X("keyname.label.unknown", "{{.Name}} ({{.Name}})", "Name", real)
}

// keyRealFor turns a dropdown label back into the canonical daemon name: the
// curated lookup first, then the text inside the trailing parentheses, then the
// whole string upper-cased as a last resort.
func keyRealFor(display string) string {
	if r, ok := keyTable().realByDisplay[display]; ok {
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
	_, ok := keyTable().displayByReal[real]
	return ok
}
