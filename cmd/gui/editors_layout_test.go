// Package main — see main.go for overview.
// editors_layout_test.go pins the two behaviours the Hotkeys/AutoReplace editors
// gained: a config field is given a fraction of the row width (so it is readable,
// and a single-line Entry scrolls the rest itself), and a hotkey combo is
// reassembled from the dropdown plus the modifier checkboxes without quietly
// rewriting a config the user did not edit.
package main

import (
	"math"
	"sort"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestFieldLayoutGivesFractionToFlexObjects(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	entry := widget.NewEntry()
	lay := newFieldLayout(0.20)
	lay.markFlex(entry)
	label := widget.NewLabel("Trigger")
	row := container.New(lay, label, entry)

	// 20% of a 1000-wide row is 200 -- reachable, because the label and gaps
	// leave far more than that over.
	row.Resize(fyne.NewSize(1000, 40))
	if want := float32(200); int(math.Abs(float64(entry.Size().Width)-float64(want))) > 1 {
		t.Errorf("entry width %v, want %v (20%% of 1000)", entry.Size().Width, want)
	}

	// A cramped row clamps the field instead of running past the right edge.
	row.Resize(fyne.NewSize(label.MinSize().Width+lay.gap+10, 40))
	if entry.Size().Width > 200 {
		t.Errorf("a cramped row still gave the entry %v wide", entry.Size().Width)
	}
	used := label.Size().Width + lay.gap + entry.Size().Width
	if used > label.MinSize().Width+lay.gap+11 {
		t.Errorf("cramped row overflows its width: used %v", used)
	}
}

func TestAutoReplaceRowGivesBothFieldsTheirWidth(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	_, e := newAutoReplaceEditor()
	e.addRow("btw", "by the way", false)
	row := e.rows[0]
	card := e.list.Objects[0].(*widget.Card)
	card.Content.Resize(fyne.NewSize(1000, 40))

	// Both entries take the fraction (20% of 1000 = 200), not the few pixels the
	// old HBox left them; the label/check/button keep their own widths.
	for name, ent := range map[string]*widget.Entry{"trigger": row.trigger, "replace": row.replace} {
		if d := math.Abs(float64(ent.Size().Width) - 200); d > 1 {
			t.Errorf("%s entry width %v, want ~200 (20%% of 1000)", name, ent.Size().Width)
		}
	}
}

func hotkeyKeys(mods map[string]any, i int) []string {
	records, ok := mods["Hotkeys"].([]any)
	if !ok || i >= len(records) {
		return nil
	}
	rec := records[i].(map[string]any)
	keys, _ := rec["Keys"].([]string)
	return keys
}

func sameSet(got []string, want ...string) bool {
	a := append([]string{}, got...)
	b := append([]string{}, want...)
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHotkeysFreshCheckboxWritesBothHands(t *testing.T) {
	_, e := newHotkeysEditor()
	e.actionNames = []string{"Toggle"}
	e.addRow(nil, "Toggle")
	row := e.rows[0]
	row.mods[0].SetChecked(true) // Ctrl
	row.trigger.SetSelected(keyLabelFor("F1"))

	keys := hotkeyKeys(e.mods(), 0)
	if !sameSet(keys, "L_CTRL", "R_CTRL", "F1") {
		t.Errorf("a freshly ticked Ctrl wrote %v, want either hand L_CTRL+R_CTRL plus F1", keys)
	}
}

func TestHotkeysLoadPreservesSingleSideAndExtraMods(t *testing.T) {
	// A config that named only the left Ctrl and the left Alt plus a Meta must
	// survive an open/save unchanged -- otherwise touching an unrelated hotkey
	// would silently widen it to "either hand" and drop Meta.
	_, e := newHotkeysEditor()
	e.actionNames = []string{"Toggle"}
	e.addRow([]string{"L_META", "L_CTRL", "L_ALT", "F1"}, "Toggle")
	row := e.rows[0]

	if !row.mods[0].Checked || !row.mods[2].Checked || row.mods[1].Checked {
		t.Errorf("checkbox states did not follow the config: ctrl=%v shift=%v alt=%v",
			row.mods[0].Checked, row.mods[1].Checked, row.mods[2].Checked)
	}
	keys := hotkeyKeys(e.mods(), 0)
	if !sameSet(keys, "L_CTRL", "L_ALT", "L_META", "F1") {
		t.Errorf("round trip changed the modifiers: %v", keys)
	}
}

func TestHotkeysLoadNamesTriggerFromDropdown(t *testing.T) {
	_, e := newHotkeysEditor()
	e.actionNames = []string{"Toggle"}
	e.addRow([]string{"SPACE"}, "Toggle")
	if got := e.rows[0].trigger.Selected; got != "Space (SPACE)" {
		t.Errorf("trigger shows %q, want the human-readable label", got)
	}
	if !sameSet(hotkeyKeys(e.mods(), 0), "SPACE") {
		t.Errorf("trigger round trip lost its name: %v", hotkeyKeys(e.mods(), 0))
	}

	// A hand-written trigger outside the catalogue is added as its own option so
	// it neither vanishes from the dropdown nor mutates on save.
	_, e = newHotkeysEditor()
	e.actionNames = []string{"Toggle"}
	e.addRow([]string{"COMPOSE"}, "Toggle")
	if got := e.rows[0].trigger.Selected; got != "COMPOSE (COMPOSE)" {
		t.Errorf("an off-catalogue trigger shows %q", got)
	}
	if !sameSet(hotkeyKeys(e.mods(), 0), "COMPOSE") {
		t.Errorf("off-catalogue trigger mutated: %v", hotkeyKeys(e.mods(), 0))
	}
}
