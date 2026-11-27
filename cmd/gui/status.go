// Package main — see main.go for overview.
// status.go provides the live daemon status panel (Punto parity Phase 1):
// pid, uptime, config path, current layout, active window class, the word
// buffer preview and attached devices, refreshed over IPC.
package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/ipc"
)

type statusPanel struct {
	client *ipc.Client

	pidLabel, uptimeLabel, configLabel  *widget.Label
	layoutLabel, windowLabel, wordLabel *widget.Label
	devicesLabel, statusLabel           *widget.Label
}

// newStatusPanel builds the Status tab and returns a refresh function that
// main.go calls when the tab is selected.
func newStatusPanel(client *ipc.Client) (fyne.CanvasObject, func()) {
	p := &statusPanel{client: client}
	p.pidLabel = widget.NewLabel("-")
	p.uptimeLabel = widget.NewLabel("-")
	p.configLabel = widget.NewLabel("-")
	p.layoutLabel = widget.NewLabel("-")
	p.windowLabel = widget.NewLabel("-")
	p.wordLabel = widget.NewLabel("-")
	p.devicesLabel = widget.NewLabel("-")
	p.statusLabel = widget.NewLabel("")

	form := widget.NewForm(
		widget.NewFormItem("PID", p.pidLabel),
		widget.NewFormItem("Uptime", p.uptimeLabel),
		widget.NewFormItem("Config", p.configLabel),
		widget.NewFormItem("Layout", p.layoutLabel),
		widget.NewFormItem("Active window", p.windowLabel),
		widget.NewFormItem("Word buffer", p.wordLabel),
		widget.NewFormItem("Devices", p.devicesLabel),
	)

	refreshBtn := widget.NewButton("Refresh", p.refresh)

	panel := container.NewVScroll(container.NewVBox(
		widget.NewCard("Daemon status", "", form),
		container.NewHBox(refreshBtn),
		p.statusLabel,
	))

	return panel, p.refresh
}

func (p *statusPanel) refresh() {
	resp, err := p.client.Status()
	if err != nil {
		p.statusLabel.SetText(fmt.Sprintf("Status failed: %v", err))
		return
	}
	if !resp.Ok {
		p.statusLabel.SetText(fmt.Sprintf("Status failed: %s", resp.Error))
		return
	}
	p.pidLabel.SetText(fmt.Sprintf("%d", resp.PID))
	p.uptimeLabel.SetText(resp.Uptime)
	p.configLabel.SetText(resp.Config)
	p.layoutLabel.SetText(fmt.Sprintf("%d", resp.Layout))
	p.windowLabel.SetText(resp.ActiveWindow)
	// The word buffer preview travels only over the root:input 0660 socket and
	// is never logged.
	p.wordLabel.SetText(resp.WordBuffer)
	p.devicesLabel.SetText(strings.Join(resp.Devices, "\n"))
	p.statusLabel.SetText("Status refreshed")
}
