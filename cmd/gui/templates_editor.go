// Package main — see main.go for overview.
// templates_editor.go edits [Templates]: a flat key = "value" table used by
// the parser's "@name@" substitution. Every row is sent as one dotted mod
// line; removing a template is not offered because the line editor cannot
// delete keys.
package main

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/l10n"
)

type templateRow struct {
	key *widget.Entry
	val *widget.Entry
}

type templatesEditor struct {
	list *fyne.Container
	rows []*templateRow
}

func newTemplatesEditor() (fyne.CanvasObject, *templatesEditor) {
	e := &templatesEditor{list: container.NewVBox()}
	add := widget.NewButton(l10n.X("templates.btn.add", "Add template"), func() { e.addRow("", "") })
	return container.NewBorder(add, nil, nil, nil, container.NewVScroll(e.list)), e
}

func (e *templatesEditor) addRow(key, val string) {
	row := &templateRow{key: widget.NewEntry(), val: widget.NewMultiLineEntry()}
	row.key.SetPlaceHolder("WordChars")
	row.key.SetText(key)
	row.val.SetText(val)
	e.rows = append(e.rows, row)

	form := widget.NewForm(
		widget.NewFormItem(l10n.X("templates.form.key", "Key"), row.key),
		widget.NewFormItem(l10n.X("templates.form.value", "Value"), row.val),
	)
	e.list.Add(widget.NewCard("", "", form))
}

func (e *templatesEditor) load(data map[string]any) {
	e.rows = nil
	e.list.Objects = nil
	tmpl := section(data, "Templates")
	keys := make([]string, 0, len(tmpl))
	for k := range tmpl {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e.addRow(k, mapStr(tmpl, k))
	}
	e.list.Refresh()
}

func (e *templatesEditor) mods() map[string]any {
	mods := map[string]any{}
	for _, r := range e.rows {
		if r.key.Text != "" {
			mods["Templates."+r.key.Text] = r.val.Text
		}
	}
	return mods
}
