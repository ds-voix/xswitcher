// Package main — see main.go for overview.
// hotkeys_editor.go edits the [[Hotkeys]] array of tables. Keys are entered
// as a separator-split list of daemon key names (the same spellings the
// ActionKeys lists use); the daemon parser is the gate that rejects a combo
// without exactly one trigger key or with an unknown action.
package main

import (
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	cfg "xswitcher/internal/config"
)

type hkRow struct {
	keys   *widget.Entry
	action *widget.Select
}

type hotkeysEditor struct {
	list        *fyne.Container
	rows        []*hkRow
	actionNames []string
}

func newHotkeysEditor() (fyne.CanvasObject, *hotkeysEditor) {
	e := &hotkeysEditor{list: container.NewVBox()}
	add := widget.NewButton("Add hotkey", func() { e.addRow("", "") })
	return container.NewBorder(add, nil, nil, nil, container.NewVScroll(e.list)), e
}

func (e *hotkeysEditor) addRow(keys, action string) {
	row := &hkRow{
		keys:   widget.NewEntry(),
		action: widget.NewSelect(e.actionNames, nil),
	}
	row.keys.SetPlaceHolder("L_CTRL, L_ALT, F1  (modifiers: " + strings.Join(modifierNames(), ", ") + ")")
	row.keys.SetText(keys)
	if action != "" {
		row.action.SetSelected(action)
	}
	e.rows = append(e.rows, row)

	var card *widget.Card // declared first: the remove closure below references it
	card = widget.NewCard("", "", container.NewHBox(
		widget.NewLabel("Keys"), row.keys,
		widget.NewLabel("Action"), row.action,
		widget.NewButton("remove", func() {
			for i, r := range e.rows {
				if r == row {
					e.rows = append(e.rows[:i], e.rows[i+1:]...)
					break
				}
			}
			e.list.Remove(card)
		}),
	))
	e.list.Add(card)
}

func modifierNames() []string {
	names := make([]string, 0, len(cfg.HotkeyModifiers))
	for n := range cfg.HotkeyModifiers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// splitKeys accepts commas, spaces and '+' as separators between key names.
func splitKeys(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '+' || r == ' ' || r == '\t'
	})
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
		keys := strings.Join(mapList2Slice(item["Keys"]), ", ")
		e.addRow(keys, mapStr(item, "Action"))
	}
	e.list.Refresh()
}

func (e *hotkeysEditor) mods() map[string]any {
	if len(e.rows) == 0 {
		return nil
	}
	records := make([]any, 0, len(e.rows))
	for _, r := range e.rows {
		keys := splitKeys(strings.ToUpper(r.keys.Text))
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
