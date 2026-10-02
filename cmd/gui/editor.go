// Package main — see main.go for overview.
// editor.go provides the config editor panel with structured forms for
// ScanDevices, Keyboard, Wayland sections and a raw TOML text area for
// complex sections (Action.*, WindowClasses). Phase 0: forms are bound to
// the daemon config via IPC get_config/set_config.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/pelletier/go-toml"

	"xswitcher/internal/ipc"
)

// configPanel bundles the editor widgets with their load/apply logic.
type configPanel struct {
	client *ipc.Client

	testEntry, searchEntry, bypassEntry *widget.Entry
	respawnEntry                        *widget.Entry
	delayEntry                          *widget.Entry
	wipeCheck                           *widget.Check
	bypassXCheck                        *widget.Check
	waylandDelayEntry                   *widget.Entry
	layoutEntries                       [4]*widget.Entry
	rawTOML                             *widget.Entry
	statusLabel                         *widget.Label
}

// newEditorPanel creates the Config tab content. It also returns a load
// function that fetches the live config from the daemon and populates the
// widgets; main.go calls it when the Config tab is selected.
func newEditorPanel(client *ipc.Client) (fyne.CanvasObject, func()) {
	p := &configPanel{client: client}

	// ScanDevices section
	p.testEntry = widget.NewEntry()
	p.testEntry.SetPlaceHolder("/dev/input/event0")
	p.searchEntry = widget.NewEntry()
	p.searchEntry.SetPlaceHolder("/dev/input/event*")
	p.bypassEntry = widget.NewEntry()
	p.bypassEntry.SetPlaceHolder("(?i)Video|Camera")
	p.respawnEntry = widget.NewEntry()
	p.respawnEntry.SetPlaceHolder("30")

	scanDevicesForm := widget.NewForm(
		widget.NewFormItem("Test", p.testEntry),
		widget.NewFormItem("Search", p.searchEntry),
		widget.NewFormItem("Bypass", p.bypassEntry),
		widget.NewFormItem("Respawn", p.respawnEntry),
	)

	// Keyboard section
	p.delayEntry = widget.NewEntry()
	p.delayEntry.SetPlaceHolder("5")
	p.wipeCheck = widget.NewCheck("Wipe by selection", nil)

	keyboardForm := widget.NewForm(
		widget.NewFormItem("Delay", p.delayEntry),
		widget.NewFormItem("", p.wipeCheck),
	)

	// Wayland section
	p.bypassXCheck = widget.NewCheck("Bypass X", nil)
	p.waylandDelayEntry = widget.NewEntry()
	p.waylandDelayEntry.SetPlaceHolder("50")
	for i := range p.layoutEntries {
		p.layoutEntries[i] = widget.NewMultiLineEntry()
		p.layoutEntries[i].SetPlaceHolder(fmt.Sprintf("Layout %d keys...", i))
	}

	waylandRows := []*widget.FormItem{
		widget.NewFormItem("", p.bypassXCheck),
		widget.NewFormItem("Delay", p.waylandDelayEntry),
	}
	for i, e := range p.layoutEntries {
		waylandRows = append(waylandRows, widget.NewFormItem(fmt.Sprintf("Layout %d", i), e))
	}
	waylandForm := widget.NewForm(waylandRows...)

	// Raw TOML area for complex sections (Action.*, WindowClasses)
	p.rawTOML = widget.NewMultiLineEntry()
	p.rawTOML.Wrapping = fyne.TextWrapWord
	p.rawTOML.SetPlaceHolder("# [Action.*] and [[WindowClasses]] sections (read-only until Phase 2)")

	p.statusLabel = widget.NewLabel("")

	loadBtn := widget.NewButton("Load", p.load)
	applyBtn := widget.NewButton("Apply", p.apply)
	reloadBtn := widget.NewButton("Reload daemon", p.reload)

	buttons := container.NewHBox(loadBtn, applyBtn, reloadBtn)

	scanGroup := widget.NewCard("ScanDevices", "", scanDevicesForm)
	keyboardGroup := widget.NewCard("Keyboard", "", keyboardForm)
	waylandGroup := widget.NewCard("Wayland", "", waylandForm)
	rawGroup := widget.NewCard("Advanced (raw TOML)", "", p.rawTOML)

	scroll := container.NewVScroll(
		container.NewVBox(
			scanGroup,
			keyboardGroup,
			waylandGroup,
			rawGroup,
			buttons,
			p.statusLabel,
		),
	)

	return scroll, p.load
}

// --- value extraction from the nested map returned by get_config ------------

func section(data map[string]any, name string) map[string]any {
	if s, ok := data[name].(map[string]any); ok {
		return s
	}
	return nil
}

func mapStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func mapInt(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	}
	return ""
}

func mapBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, _ := m[key].(bool)
	return v
}

func mapList(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	items, ok := m[key].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, fmt.Sprint(it))
	}
	return strings.Join(parts, "\n")
}

// --- panel operations --------------------------------------------------------

// load fetches the live config from the daemon and fills every widget.
func (p *configPanel) load() {
	data, err := p.client.GetConfig()
	if err != nil {
		p.statusLabel.SetText(fmt.Sprintf("Load failed: %v", err))
		return
	}

	scan := section(data, "ScanDevices")
	p.testEntry.SetText(mapStr(scan, "Test"))
	p.searchEntry.SetText(mapStr(scan, "Search"))
	p.bypassEntry.SetText(mapStr(scan, "Bypass"))
	p.respawnEntry.SetText(mapInt(scan, "Respawn"))

	keyboard := section(data, "Keyboard")
	p.delayEntry.SetText(mapInt(keyboard, "Delay"))
	p.wipeCheck.SetChecked(mapBool(keyboard, "WipeBySelection"))

	wayland := section(data, "Wayland")
	p.bypassXCheck.SetChecked(mapBool(wayland, "BypassX"))
	p.waylandDelayEntry.SetText(mapInt(wayland, "Delay"))
	for i, e := range p.layoutEntries {
		e.SetText(mapList(wayland, fmt.Sprintf("Layout%d", i)))
	}

	p.rawTOML.SetText(renderRawSections(data))
	p.statusLabel.SetText("Config loaded from daemon")
}

// modsFromWidgets collects dotted-key modifications from non-empty widgets.
func (p *configPanel) modsFromWidgets() map[string]any {
	mods := map[string]any{}
	setStr := func(key, val string) {
		if val != "" {
			mods[key] = val
		}
	}
	setInt := func(key, val string) {
		if val == "" {
			return
		}
		if n, err := strconv.Atoi(val); err == nil {
			mods[key] = n
		}
	}

	setStr("ScanDevices.Test", p.testEntry.Text)
	setStr("ScanDevices.Search", p.searchEntry.Text)
	setStr("ScanDevices.Bypass", p.bypassEntry.Text)
	setInt("ScanDevices.Respawn", p.respawnEntry.Text)

	setInt("Keyboard.Delay", p.delayEntry.Text)
	mods["Keyboard.WipeBySelection"] = p.wipeCheck.Checked

	mods["Wayland.BypassX"] = p.bypassXCheck.Checked
	setInt("Wayland.Delay", p.waylandDelayEntry.Text)
	for i, e := range p.layoutEntries {
		lines := splitNonEmpty(e.Text)
		if len(lines) > 0 {
			mods[fmt.Sprintf("Wayland.Layout%d", i)] = lines
		}
	}
	return mods
}

func splitNonEmpty(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// apply sends the modifications to the daemon (validated before the file is
// written) and then triggers a respawn so the new config takes effect.
func (p *configPanel) apply() {
	mods := p.modsFromWidgets()
	if len(mods) == 0 {
		p.statusLabel.SetText("Nothing to apply")
		return
	}
	if err := p.client.SetConfig(mods); err != nil {
		p.statusLabel.SetText(fmt.Sprintf("Set failed: %v", err))
		return
	}
	p.reload()
}

// reload asks the daemon to respawn with the config currently on disk.
func (p *configPanel) reload() {
	resp, err := p.client.Reload()
	if err != nil {
		p.statusLabel.SetText(fmt.Sprintf("Reload failed: %v (config saved)", err))
		return
	}
	if !resp.Ok {
		p.statusLabel.SetText(fmt.Sprintf("Reload failed: %s", resp.Error))
		return
	}
	p.statusLabel.SetText(fmt.Sprintf("Applied, daemon respawned (PID %d)", resp.PID))
}

// renderRawSections dumps every config section not covered by the structured
// forms so the user can at least inspect Actions, WindowClasses, etc.
// Display-only: JSON turns TOML ints into float64, so integral floats are
// restored to ints before marshalling to avoid "1.0" in the output.
func renderRawSections(data map[string]any) string {
	filtered := make(map[string]any)
	for key, value := range data {
		switch key {
		case "ScanDevices", "Keyboard", "Wayland":
			continue
		}
		filtered[key] = normalizeFloats(value)
	}
	if len(filtered) == 0 {
		return ""
	}
	out, err := toml.Marshal(filtered)
	if err != nil {
		return fmt.Sprintf("# (cannot render: %v)", err)
	}
	return string(out)
}

func normalizeFloats(value any) any {
	switch v := value.(type) {
	case float64:
		if i := int64(v); float64(i) == v {
			return i // go-toml's Marshal refuses plain int
		}
		return v
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeFloats(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = normalizeFloats(item)
		}
		return out
	default:
		return value
	}
}
