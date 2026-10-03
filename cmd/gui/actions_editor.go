// Package main — see main.go for overview.
// actions_editor.go edits the [Action.<name>] tables: one card per named
// action, fields mirroring config.TAction. Modifications go back as dotted
// paths ("Action.RetTypeWord.Layout"); the line editor applies them inside
// the matching table and creates a fresh table for a new action name.
package main

import (
	"regexp"
	"sort"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Action names become part of a bare TOML table header ([Action.Name]), so
// only header-safe characters are accepted here.
var actionNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type actionRow struct {
	name *widget.Entry

	action      *widget.Entry // []string, one per line
	layouts     *widget.Entry // []int, one per line
	exec        *widget.Entry
	sendBuffer  *widget.Entry
	directory   *widget.Entry
	uid         *widget.Entry
	gid         *widget.Entry
	environment *widget.Entry // []string, one per line
	layout      *widget.Entry // int, -1 keeps the current layout
	timeout     *widget.Entry // int

	wait     *widget.Check
	useShell *widget.Check
	cleanEnv *widget.Check

	// Loaded (or default) checkbox states. UseShell/CleanEnv default to true
	// in the daemon, but the raw config tree omits absent keys, so an
	// untouched checkbox must not send a value — only changes are sent.
	initialWait, initialUseShell, initialCleanEnv bool
}

type actionsEditor struct {
	list *fyne.Container
	rows []*actionRow
}

func newActionsEditor() (fyne.CanvasObject, *actionsEditor) {
	e := &actionsEditor{list: container.NewVBox()}
	add := widget.NewButton("Add action", func() { e.addRow("", nil) })
	return container.NewBorder(add, nil, nil, nil, container.NewVScroll(e.list)), e
}

func (e *actionsEditor) addRow(name string, fields map[string]any) {
	r := &actionRow{
		name:        widget.NewEntry(),
		action:      widget.NewMultiLineEntry(),
		layouts:     widget.NewMultiLineEntry(),
		exec:        widget.NewEntry(),
		sendBuffer:  widget.NewEntry(),
		directory:   widget.NewEntry(),
		uid:         widget.NewEntry(),
		gid:         widget.NewEntry(),
		environment: widget.NewMultiLineEntry(),
		layout:      widget.NewEntry(),
		timeout:     widget.NewEntry(),
		wait:        widget.NewCheck("Wait", nil),
		useShell:    widget.NewCheck("Use shell", nil),
		cleanEnv:    widget.NewCheck("Clean environment", nil),
	}
	r.name.SetPlaceHolder("action name")
	r.name.SetText(name)

	if fields != nil {
		r.action.SetText(mapList(fields, "Action"))
		r.layout.SetText(mapInt(fields, "Layout"))
		r.layouts.SetText(mapList(fields, "Layouts"))
		r.exec.SetText(mapStr(fields, "Exec"))
		r.timeout.SetText(mapInt(fields, "Timeout"))
		r.sendBuffer.SetText(mapStr(fields, "SendBuffer"))
		r.directory.SetText(mapStr(fields, "Directory"))
		r.environment.SetText(mapList(fields, "Environment"))
		r.uid.SetText(mapStr(fields, "UID"))
		r.gid.SetText(mapStr(fields, "GID"))
		r.wait.SetChecked(mapBool(fields, "Wait"))
		r.useShell.SetChecked(mapBoolDefault(fields, "UseShell", true))
		r.cleanEnv.SetChecked(mapBoolDefault(fields, "CleanEnv", true))
	} else {
		r.useShell.SetChecked(true)
		r.cleanEnv.SetChecked(true)
	}
	r.initialWait = r.wait.Checked
	r.initialUseShell = r.useShell.Checked
	r.initialCleanEnv = r.cleanEnv.Checked

	form := widget.NewForm(
		widget.NewFormItem("Action", r.action),
		widget.NewFormItem("Layout", r.layout),
		widget.NewFormItem("Layouts", r.layouts),
		widget.NewFormItem("Exec", r.exec),
		widget.NewFormItem("Timeout", r.timeout),
		widget.NewFormItem("", r.wait),
		widget.NewFormItem("SendBuffer", r.sendBuffer),
		widget.NewFormItem("", r.useShell),
		widget.NewFormItem("Directory", r.directory),
		widget.NewFormItem("", r.cleanEnv),
		widget.NewFormItem("Environment", r.environment),
		widget.NewFormItem("UID", r.uid),
		widget.NewFormItem("GID", r.gid),
	)
	card := widget.NewCard("Action."+name, "", container.NewVBox(r.name, form))
	e.rows = append(e.rows, r)
	e.list.Add(card)
}

func (e *actionsEditor) load(data map[string]any) {
	e.rows = nil
	e.list.Objects = nil
	actions, _ := data["Action"].(map[string]any)
	names := make([]string, 0, len(actions))
	for n := range actions {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fields, _ := actions[n].(map[string]any)
		e.addRow(n, fields)
	}
	e.list.Refresh()
}

func (e *actionsEditor) mods() map[string]any {
	mods := map[string]any{}
	for _, r := range e.rows {
		name := r.name.Text
		if !actionNameRE.MatchString(name) {
			continue // no name or a name that cannot head a table: skip the row
		}
		base := "Action." + name + "."
		setStr := func(key, val string) {
			if val != "" {
				mods[base+key] = val
			}
		}
		setInt := func(key, val string) {
			if n, err := strconv.Atoi(val); err == nil {
				mods[base+key] = int64(n)
			}
		}
		setStrList := func(key string, ent *widget.Entry) {
			if lines := splitNonEmpty(ent.Text); len(lines) > 0 {
				mods[base+key] = lines
			}
		}

		setStrList("Action", r.action)
		setInt("Layout", r.layout.Text)
		setStr("Exec", r.exec.Text)
		setInt("Timeout", r.timeout.Text)
		setStr("SendBuffer", r.sendBuffer.Text)
		setStr("Directory", r.directory.Text)
		setStrList("Environment", r.environment)
		setStr("UID", r.uid.Text)
		setStr("GID", r.gid.Text)

		// Layouts is []int: send it only when every line parses, otherwise
		// the string form would make the daemon gate reject the file.
		if lines := splitNonEmpty(r.layouts.Text); len(lines) > 0 {
			nums := make([]int64, 0, len(lines))
			ok := true
			for _, l := range lines {
				n, err := strconv.ParseInt(l, 10, 64)
				if err != nil {
					ok = false
					break
				}
				nums = append(nums, n)
			}
			if ok {
				mods[base+"Layouts"] = nums
			}
		}

		if r.wait.Checked != r.initialWait {
			mods[base+"Wait"] = r.wait.Checked
		}
		if r.useShell.Checked != r.initialUseShell {
			mods[base+"UseShell"] = r.useShell.Checked
		}
		if r.cleanEnv.Checked != r.initialCleanEnv {
			mods[base+"CleanEnv"] = r.cleanEnv.Checked
		}
	}
	return mods
}

// mapBoolDefault mirrors mapBool but reports the daemon-side default for an
// absent key instead of false.
func mapBoolDefault(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}
