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
	"xswitcher/internal/l10n"
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
		widget.NewFormItem(l10n.X("status.field.pid", "PID"), p.pidLabel),
		widget.NewFormItem(l10n.X("status.field.uptime", "Uptime"), p.uptimeLabel),
		widget.NewFormItem(l10n.X("status.field.config", "Config"), p.configLabel),
		widget.NewFormItem(l10n.X("status.field.layout", "Layout"), p.layoutLabel),
		widget.NewFormItem(l10n.X("status.field.active_window", "Active window"), p.windowLabel),
		widget.NewFormItem(l10n.X("status.field.word_buffer", "Word buffer"), p.wordLabel),
		widget.NewFormItem(l10n.X("status.field.devices", "Devices"), p.devicesLabel),
	)

	refreshBtn := widget.NewButton(l10n.X("status.btn.refresh", "Refresh"), p.refresh)

	panel := container.NewVScroll(container.NewVBox(
		widget.NewCard(l10n.X("status.card.daemon_status", "Daemon status"), "", form),
		container.NewHBox(refreshBtn),
		p.statusLabel,
	))

	return panel, p.refresh
}

func (p *statusPanel) refresh() {
	resp, err := p.client.Status()
	if err != nil {
		p.statusLabel.SetText(l10n.X("status.failed_v", "Status failed: {{.Err}}", "Err", err))
		return
	}
	if !resp.Ok {
		p.statusLabel.SetText(l10n.X("status.failed_s", "Status failed: {{.Error}}", "Error", resp.Error))
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
	p.statusLabel.SetText(l10n.X("status.refreshed", "Status refreshed"))
}
