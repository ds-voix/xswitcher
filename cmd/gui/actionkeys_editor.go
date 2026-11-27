// Package main — see main.go for overview.
// actionkeys_editor.go edits [ActionKeys]: the key-name lists that trigger
// layout switching and word actions. Each list is one array-valued mod line.
package main

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type actionKeysEditor struct {
	layouts *widget.Entry
	lists   [4]*widget.Entry
}

var akListKeys = []string{"Add", "Drop", "Test", "StateKeys"}

func newActionKeysEditor() (fyne.CanvasObject, *actionKeysEditor) {
	e := &actionKeysEditor{layouts: widget.NewEntry()}
	e.layouts.SetPlaceHolder("0,1")
	rows := []*widget.FormItem{widget.NewFormItem("Layouts", e.layouts)}
	for i, key := range akListKeys {
		e.lists[i] = widget.NewMultiLineEntry()
		e.lists[i].SetPlaceHolder(fmt.Sprintf("%s key patterns, one per line", key))
		rows = append(rows, widget.NewFormItem(key, e.lists[i]))
	}
	form := widget.NewForm(rows...)
	return container.NewVScroll(widget.NewCard("ActionKeys", "Keys that trigger switching and word actions (one pattern per line)", form)), e
}

func (e *actionKeysEditor) load(data map[string]any) {
	ak := section(data, "ActionKeys")
	e.layouts.SetText(mapList(ak, "Layouts"))
	for i, key := range akListKeys {
		e.lists[i].SetText(mapList(ak, key))
	}
}

func (e *actionKeysEditor) mods() map[string]any {
	mods := map[string]any{}
	// Layouts is []int in the config type; sending it as strings would make
	// the daemon's parse gate reject the whole file.
	var nums []int64
	for _, line := range splitNonEmpty(e.layouts.Text) {
		n, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			return map[string]any{} // bad input: send nothing, keep the file valid
		}
		nums = append(nums, n)
	}
	if len(nums) > 0 {
		mods["ActionKeys.Layouts"] = nums
	}
	for i, key := range akListKeys {
		if lines := splitNonEmpty(e.lists[i].Text); len(lines) > 0 {
			mods["ActionKeys."+key] = lines
		}
	}
	return mods
}
