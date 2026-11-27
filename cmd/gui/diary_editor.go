// Package main — see main.go for overview.
// diary_editor.go provides the Diary sub-tab: the [Diary] settings form
// (participates in the shared Apply flow) plus a read-only viewer fed by
// the get_diary IPC command, with a text filter, a date filter and an
// export button.
//
// The diary records every completed word the daemon processes. That is a
// keylogger capability: it is off by default, and the form labels it in
// plain words instead of euphemisms.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/ipc"
)

type diaryEditor struct {
	client      *ipc.Client
	enabled     *widget.Check
	pathEntry   *widget.Entry
	maxSize     *widget.Entry
	rotateCount *widget.Entry

	filterEntry *widget.Entry
	dateEntry   *widget.Entry
	view        *widget.Entry
	note        *widget.Label

	raw string // last fetched diary content, already capped by the daemon
}

func newDiaryEditor(client *ipc.Client) (fyne.CanvasObject, *diaryEditor) {
	e := &diaryEditor{client: client}

	e.enabled = widget.NewCheck("RECORD TYPED TEXT (keystrokes end up in a file)", nil)
	e.pathEntry = widget.NewEntry()
	e.pathEntry.SetPlaceHolder("/var/lib/xswitcher/diary.log")
	e.maxSize = widget.NewEntry()
	e.maxSize.SetPlaceHolder("10")
	e.rotateCount = widget.NewEntry()
	e.rotateCount.SetPlaceHolder("3")

	form := widget.NewForm(
		widget.NewFormItem("", e.enabled),
		widget.NewFormItem("Path", e.pathEntry),
		widget.NewFormItem("MaxSizeMB", e.maxSize),
		widget.NewFormItem("RotateCount", e.rotateCount),
	)

	e.filterEntry = widget.NewEntry()
	e.filterEntry.SetPlaceHolder("filter text...")
	e.filterEntry.OnChanged = func(string) { e.render() }
	e.dateEntry = widget.NewEntry()
	e.dateEntry.SetPlaceHolder("2006-01-02")
	e.dateEntry.OnChanged = func(string) { e.render() }

	fetchBtn := widget.NewButton("Fetch", e.fetch)
	exportBtn := widget.NewButton("Export", e.export)

	e.view = widget.NewMultiLineEntry()
	e.view.Wrapping = fyne.TextWrapOff
	e.view.Disable()

	e.note = widget.NewLabel("")

	top := container.NewVBox(
		widget.NewCard("[Diary] settings", "", form),
		container.NewHBox(fetchBtn, exportBtn, e.note),
		container.NewHBox(widget.NewLabel("text"), e.filterEntry, widget.NewLabel("date"), e.dateEntry),
	)
	return container.NewBorder(top, nil, nil, nil, container.NewVScroll(e.view)), e
}

// load fills the settings form from get_config data.
func (e *diaryEditor) load(data map[string]any) {
	diary := section(data, "Diary")
	e.enabled.SetChecked(mapBool(diary, "Enabled"))
	e.pathEntry.SetText(mapStr(diary, "Path"))
	e.maxSize.SetText(mapInt(diary, "MaxSizeMB"))
	e.rotateCount.SetText(mapInt(diary, "RotateCount"))
}

// mods returns the dotted [Diary] keys. Enabled is always sent (a bool
// has no "empty" state); the other fields only when non-empty.
func (e *diaryEditor) mods() map[string]any {
	mods := map[string]any{"Diary.Enabled": e.enabled.Checked}
	if s := strings.TrimSpace(e.pathEntry.Text); s != "" {
		mods["Diary.Path"] = s
	}
	if s := strings.TrimSpace(e.maxSize.Text); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			mods["Diary.MaxSizeMB"] = n
		}
	}
	if s := strings.TrimSpace(e.rotateCount.Text); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			mods["Diary.RotateCount"] = n
		}
	}
	return mods
}

// fetch pulls the diary tail through IPC — the file itself belongs to the
// daemon user and is not assumed readable by the GUI.
func (e *diaryEditor) fetch() {
	text, err := e.client.GetDiary()
	if err != nil {
		e.raw = ""
		e.note.SetText(fmt.Sprintf("%v", err))
		e.render()
		return
	}
	lines := strings.Count(text, "\n")
	e.note.SetText(fmt.Sprintf("%d entries", lines))
	e.raw = text
	e.render()
}

// render applies the two filters over the raw text without refetching.
func (e *diaryEditor) render() {
	var out []string
	needle := strings.ToLower(e.filterEntry.Text)
	date := strings.TrimSpace(e.dateEntry.Text)
	for _, line := range strings.Split(e.raw, "\n") {
		if line == "" {
			continue
		}
		if date != "" && !strings.HasPrefix(line, date) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(line), needle) {
			continue
		}
		out = append(out, line)
	}
	e.view.Enable()
	e.view.SetText(strings.Join(out, "\n"))
	e.view.Disable()
}

// export saves the filtered view through the standard Fyne save dialog.
func (e *diaryEditor) export() {
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			e.note.SetText("Export cancelled")
			return
		}
		defer writer.Close()
		if _, err := writer.Write([]byte(e.view.Text)); err != nil {
			e.note.SetText(fmt.Sprintf("Export failed: %v", err))
		}
	}, fyne.CurrentApp().Driver().AllWindows()[0])
}
