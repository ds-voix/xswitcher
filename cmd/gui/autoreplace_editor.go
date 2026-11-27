// Package main — see main.go for overview.
// autoreplace_editor.go edits the [[AutoReplace]] array of tables. The whole
// array goes back as one set_config mod, exactly like WindowClasses; the
// daemon parser rejects empty Trigger/Replace, so invalid rows surface as a
// Set failure instead of corrupting the config file.
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type arRow struct {
	trigger *widget.Entry
	replace *widget.Entry
	exact   *widget.Check
}

type autoReplaceEditor struct {
	list *fyne.Container
	rows []*arRow
}

func newAutoReplaceEditor() (fyne.CanvasObject, *autoReplaceEditor) {
	e := &autoReplaceEditor{list: container.NewVBox()}
	add := widget.NewButton("Add rule", func() { e.addRow("", "", false) })
	return container.NewBorder(add, nil, nil, nil, container.NewVScroll(e.list)), e
}

func (e *autoReplaceEditor) addRow(trigger, replace string, exact bool) {
	row := &arRow{
		trigger: widget.NewEntry(),
		replace: widget.NewEntry(),
		exact:   widget.NewCheck("whole word only", nil),
	}
	row.trigger.SetPlaceHolder("trigger, e.g. btw")
	row.replace.SetPlaceHolder("replacement, e.g. by the way")
	row.trigger.SetText(trigger)
	row.replace.SetText(replace)
	row.exact.SetChecked(exact)
	e.rows = append(e.rows, row)

	var card *widget.Card // declared first: the remove closure below references it
	remove := widget.NewButton("remove", func() {
		for i, r := range e.rows {
			if r == row {
				e.rows = append(e.rows[:i], e.rows[i+1:]...)
				break
			}
		}
		e.list.Remove(card)
	})
	lay := newFieldLayout(0.20)
	lay.markFlex(row.trigger)
	lay.markFlex(row.replace)
	card = widget.NewCard("", "", container.New(lay,
		widget.NewLabel("Trigger"), row.trigger,
		widget.NewLabel("Replace"), row.replace,
		row.exact,
		remove,
	))
	e.list.Add(card)
}

func (e *autoReplaceEditor) load(data map[string]any) {
	e.rows = nil
	e.list.Objects = nil
	for _, item := range asRecords(data["AutoReplace"]) {
		e.addRow(mapStr(item, "Trigger"), mapStr(item, "Replace"), mapBool(item, "Exact"))
	}
	e.list.Refresh()
}

func (e *autoReplaceEditor) mods() map[string]any {
	if len(e.rows) == 0 {
		return nil
	}
	records := make([]any, 0, len(e.rows))
	for _, r := range e.rows {
		records = append(records, map[string]any{
			"Trigger": r.trigger.Text,
			"Replace": r.replace.Text,
			"Exact":   r.exact.Checked,
		})
	}
	return map[string]any{"AutoReplace": records}
}
