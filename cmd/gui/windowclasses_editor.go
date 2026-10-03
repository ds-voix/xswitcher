// Package main — see main.go for overview.
// windowclasses_editor.go edits the [[WindowClasses]] array of tables. The
// whole array goes back as one set_config mod; the line editor regenerates
// the blocks in place (comments inside replaced blocks are lost, everything
// else in the file is untouched).
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// The parser's gate (internal/config/parse.go) only realises the action set
// named "Actions": any other non-empty value makes the whole config fail to
// load, so the editor offers exactly "" and "Actions".
const (
	wcNoneLabel = "(none)"  // written back as Actions = ""
	wcMainLabel = "Actions" // the only realised action set
)

func wcLabelFor(value string) string {
	if value == "" {
		return wcNoneLabel
	}
	return value
}

func wcValueFor(label string) string {
	if label == wcNoneLabel {
		return ""
	}
	return label
}

type wcRow struct {
	regex   *widget.Entry
	drops   *widget.Check
	actions *widget.Select
}

type windowClassesEditor struct {
	list *fyne.Container
	rows []*wcRow
}

func newWindowClassesEditor() (fyne.CanvasObject, *windowClassesEditor) {
	e := &windowClassesEditor{list: container.NewVBox()}
	add := widget.NewButton("Add window class", func() { e.addRow("", false, "Actions") })
	// The add button lives outside the list: load() replaces every row and
	// must not have to re-add the button.
	return container.NewBorder(add, nil, nil, nil, container.NewVScroll(e.list)), e
}

func (e *windowClassesEditor) addRow(regex string, drops bool, actions string) {
	row := &wcRow{
		regex:   widget.NewEntry(),
		drops:   widget.NewCheck("clicks drop", nil),
		actions: widget.NewSelect([]string{wcNoneLabel, wcMainLabel}, nil),
	}
	row.regex.SetPlaceHolder("window class regex")
	row.regex.SetText(regex)
	row.drops.SetChecked(drops)
	row.actions.SetSelected(wcLabelFor(actions))
	e.rows = append(e.rows, row)

	var card *widget.Card // declared first: the remove closure below references it
	card = widget.NewCard("", "", container.NewHBox(
		widget.NewLabel("Regex"), row.regex,
		widget.NewLabel("Mouse"), row.drops,
		widget.NewLabel("Actions"), row.actions,
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

// load rebuilds the rows from get_config data; called on every tab open, so
// rows added locally since the last load are replaced by the daemon state.
func (e *windowClassesEditor) load(data map[string]any) {
	e.rows = nil
	e.list.Objects = nil
	for _, item := range asRecords(data["WindowClasses"]) {
		e.addRow(mapStr(item, "Regex"), mapBool(item, "MouseClickDrops"), mapStr(item, "Actions"))
	}
	e.list.Refresh()
}

// mods returns the whole array as one mod. With zero rows nothing is sent:
// an empty [[WindowClasses]] array has no writable representation anyway.
func (e *windowClassesEditor) mods() map[string]any {
	if len(e.rows) == 0 {
		return nil
	}
	records := make([]any, 0, len(e.rows))
	for _, r := range e.rows {
		records = append(records, map[string]any{
			"Regex":           r.regex.Text,
			"MouseClickDrops": r.drops.Checked,
			"Actions":         wcValueFor(r.actions.Selected),
		})
	}
	return map[string]any{"WindowClasses": records}
}

// asRecords normalises the JSON-decoded shape of an array of tables.
func asRecords(v any) []map[string]any {
	switch items := v.(type) {
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case []map[string]any:
		return items
	}
	return nil
}
