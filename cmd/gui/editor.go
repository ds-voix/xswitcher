// Package main — see main.go for overview.
// editor.go provides the config editor panel with structured forms for
// ScanDevices, Keyboard, Wayland sections and a raw TOML text area for
// complex sections (Action.*, WindowClasses). MVP Phase 1.
package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/ipc"
)

// newEditorPanel creates the Config tab content with form widgets for each
// config section. The Apply button sends changes to the daemon via IPC reload.
func newEditorPanel(client *ipc.Client) fyne.CanvasObject {
	// ScanDevices section
	testEntry := widget.NewEntry()
	testEntry.SetPlaceHolder("/dev/input/event0")
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("/dev/input/event*")
	bypassEntry := widget.NewEntry()
	bypassEntry.SetPlaceHolder("(?i)Video|Camera")
	respawnEntry := widget.NewEntry()
	respawnEntry.SetPlaceHolder("30")

	scanDevicesForm := widget.NewForm(
		widget.NewFormItem("Test", testEntry),
		widget.NewFormItem("Search", searchEntry),
		widget.NewFormItem("Bypass", bypassEntry),
		widget.NewFormItem("Respawn", respawnEntry),
	)

	// Keyboard section
	delayEntry := widget.NewEntry()
	delayEntry.SetPlaceHolder("5")
	wipeCheck := widget.NewCheck("Wipe by selection", nil)

	keyboardForm := widget.NewForm(
		widget.NewFormItem("Delay", delayEntry),
		widget.NewFormItem("", wipeCheck),
	)

	// Wayland section
	bypassXCheck := widget.NewCheck("Bypass X", nil)
	waylandDelayEntry := widget.NewEntry()
	waylandDelayEntry.SetPlaceHolder("50")
	layout0Entry := widget.NewMultiLineEntry()
	layout0Entry.SetPlaceHolder("Layout 0 keys...")
	layout1Entry := widget.NewMultiLineEntry()
	layout1Entry.SetPlaceHolder("Layout 1 keys...")
	layout2Entry := widget.NewMultiLineEntry()
	layout2Entry.SetPlaceHolder("Layout 2 keys...")
	layout3Entry := widget.NewMultiLineEntry()
	layout3Entry.SetPlaceHolder("Layout 3 keys...")

	waylandForm := widget.NewForm(
		widget.NewFormItem("", bypassXCheck),
		widget.NewFormItem("Delay", waylandDelayEntry),
		widget.NewFormItem("Layout 0", layout0Entry),
		widget.NewFormItem("Layout 1", layout1Entry),
		widget.NewFormItem("Layout 2", layout2Entry),
		widget.NewFormItem("Layout 3", layout3Entry),
	)

	// Raw TOML area for complex sections (Action.*, WindowClasses)
	rawTOML := widget.NewMultiLineEntry()
	rawTOML.Wrapping = fyne.TextWrapWord
	rawTOML.SetPlaceHolder("# Raw TOML for [Action.*] and [[WindowClasses]]\n# Edit manually; validated on Apply.")

	statusLabel := widget.NewLabel("")
	applyBtn := widget.NewButton("Apply", func() {
		resp, err := client.Reload()
		if err != nil {
			statusLabel.SetText(fmt.Sprintf("Error: %v", err))
			return
		}
		if !resp.Ok {
			statusLabel.SetText(fmt.Sprintf("Reload failed: %s", resp.Error))
			return
		}
		statusLabel.SetText(fmt.Sprintf("Reloaded (PID %d)", resp.PID))
	})

	reloadBtn := widget.NewButton("Reload from disk", func() {
		resp, err := client.Reload()
		if err != nil {
			statusLabel.SetText(fmt.Sprintf("Error: %v", err))
			return
		}
		if !resp.Ok {
			statusLabel.SetText(fmt.Sprintf("Reload failed: %s", resp.Error))
			return
		}
		statusLabel.SetText(fmt.Sprintf("Reloaded from disk (PID %d)", resp.PID))
	})

	buttons := container.NewHBox(applyBtn, reloadBtn)

	scanGroup := widget.NewCard("ScanDevices", "", scanDevicesForm)
	keyboardGroup := widget.NewCard("Keyboard", "", keyboardForm)
	waylandGroup := widget.NewCard("Wayland", "", waylandForm)
	rawGroup := widget.NewCard("Advanced (raw TOML)", "", rawTOML)

	scroll := container.NewVScroll(
		container.NewVBox(
			scanGroup,
			keyboardGroup,
			waylandGroup,
			rawGroup,
			buttons,
			statusLabel,
		),
	)

	return scroll
}