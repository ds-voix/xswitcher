// Package main — see main.go for overview.
// hotkeys_editor.go edits the [[Hotkeys]] array of tables. A combo is the
// daemon's shape: zero or more held modifiers plus exactly one trigger key. The
// modifier names the parser accepts are only the L_/R_ forms plus META and CAPS
// (see config.HotkeyModifiers), so a "Ctrl" checkbox writes BOTH L_CTRL and
// R_CTRL -- "either hand" -- rather than the bare "CTRL" the daemon would reject
// as a second trigger. The trigger key is a dropdown of canonical names.
package main

import (
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	cfg "xswitcher/internal/config"
)

// hkMods is the fixed set of modifier checkboxes. Each maps to the daemon's
// L_/R_ token pair; "any hand" is the checkbox's meaning, so it writes both.
var hkMods = []struct {
	base  string // matches the suffix of the daemon's L_<base>/R_<base> names
	left  string
	right string
	label string
}{
	{"CTRL", "L_CTRL", "R_CTRL", "Ctrl"},
	{"SHIFT", "L_SHIFT", "R_SHIFT", "Shift"},
	{"ALT", "L_ALT", "R_ALT", "Alt"},
}

type hkRow struct {
	trigger *widget.Select
	mods    []*widget.Check // parallel to hkMods
	action  *widget.Select

	origMods  map[string][]string // side-specific tokens the config already had
	origExtra []string            // modifier tokens with no checkbox (L_META/CAPS/...)
}

type hotkeysEditor struct {
	list        *fyne.Container
	rows        []*hkRow
	actionNames []string
}

func newHotkeysEditor() (fyne.CanvasObject, *hotkeysEditor) {
	e := &hotkeysEditor{list: container.NewVBox()}
	add := widget.NewButton("Add hotkey", func() { e.addRow(nil, "") })
	return container.NewBorder(add, nil, nil, nil, container.NewVScroll(e.list)), e
}

// modBase returns the hkMods base ("CTRL") for a daemon modifier token, or "" if
// the token is not a checkbox modifier (so it is either an extra modifier with no
// checkbox, or the trigger key).
func modBase(name string) string {
	for _, m := range hkMods {
		if name == m.left || name == m.right {
			return m.base
		}
	}
	return ""
}

func (e *hotkeysEditor) addRow(keys []string, action string) {
	triggerOpts := keyTriggerOptions()

	row := &hkRow{
		trigger:  widget.NewSelect(triggerOpts, nil),
		mods:     make([]*widget.Check, len(hkMods)),
		action:   widget.NewSelect(e.actionNames, nil),
		origMods: map[string][]string{},
	}
	for i, m := range hkMods {
		row.mods[i] = widget.NewCheck(m.label, nil)
	}

	// Split the stored combo into checkbox modifiers, extra modifiers and the
	// one trigger, keeping the exact tokens so a save that changes nothing
	// writes nothing back that differs.
	for _, k := range keys {
		k = strings.ToUpper(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if base := modBase(k); base != "" {
			row.origMods[base] = append(row.origMods[base], k)
		} else if cfg.HotkeyModifiers[k] {
			row.origExtra = append(row.origExtra, k)
		} else {
			if !keyHasReal(k) { // a hand-written name outside the catalogue
				row.trigger.Options = append(row.trigger.Options, keyLabelFor(k))
			}
			row.trigger.SetSelected(keyLabelFor(k))
		}
	}
	for i, m := range hkMods {
		row.mods[i].SetChecked(len(row.origMods[m.base]) > 0)
	}
	if action != "" {
		row.action.SetSelected(action)
	}
	e.rows = append(e.rows, row)

	var card *widget.Card // declared first: the remove closure below references it
	lay := newFieldLayout(0.20)
	lay.markFlex(row.trigger)
	lay.markFlex(row.action)
	remove := widget.NewButton("remove", func() {
		for i, r := range e.rows {
			if r == row {
				e.rows = append(e.rows[:i], e.rows[i+1:]...)
				break
			}
		}
		e.list.Remove(card)
	})
	objs := []fyne.CanvasObject{
		widget.NewLabel("Keys"), row.trigger, row.mods[0], row.mods[1], row.mods[2],
		widget.NewLabel("Action"), row.action, remove,
	}
	card = widget.NewCard("", "", container.New(lay, objs...))
	e.list.Add(card)
}

func (e *hotkeysEditor) load(data map[string]any) {
	e.actionNames = nil
	for name := range section(data, "Action") {
		e.actionNames = append(e.actionNames, name)
	}
	sort.Strings(e.actionNames)

	e.rows = nil
	e.list.Objects = nil
	for _, item := range asRecords(data["Hotkeys"]) {
		e.addRow(mapList2Slice(item["Keys"]), mapStr(item, "Action"))
	}
	e.list.Refresh()
}

func (e *hotkeysEditor) mods() map[string]any {
	if len(e.rows) == 0 {
		return nil
	}
	records := make([]any, 0, len(e.rows))
	for _, r := range e.rows {
		var keys []string
		for i, m := range hkMods {
			if !r.mods[i].Checked {
				continue
			}
			// Preserve the side-specific tokens the config already had; only a
			// freshly ticked box becomes "either hand".
			if toks := r.origMods[m.base]; len(toks) > 0 {
				keys = append(keys, toks...)
			} else {
				keys = append(keys, m.left, m.right)
			}
		}
		keys = append(keys, r.origExtra...) // meta/caps the form cannot edit
		if real := keyRealFor(r.trigger.Selected); real != "" {
			keys = append(keys, real)
		}
		records = append(records, map[string]any{
			"Keys":   keys,
			"Action": r.action.Selected,
		})
	}
	return map[string]any{"Hotkeys": records}
}

func mapList2Slice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
