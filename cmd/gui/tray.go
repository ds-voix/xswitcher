// Package main — see main.go for overview.
// tray.go installs the system tray menu (Punto parity Phase 1): show/hide,
// reload daemon, quit. Closing the main window hides it to the tray instead
// of quitting, Punto-style.
package main

import (
	_ "embed"

	"fyne.io/fyne/v2"

	"xswitcher/internal/ipc"
	"xswitcher/internal/l10n"
)

//go:embed assets/tray.png
var trayIconPNG []byte

// systrayDriver is the marker interface Fyne 2.4 drivers implement for tray
// support; the public desktop API exposes it via a type assertion on the
// current driver.
type systrayDriver interface {
	SetSystemTrayMenu(*fyne.Menu)
	SetSystemTrayIcon(fyne.Resource)
}

func setupTray(w fyne.Window, client *ipc.Client, setStatus func(string)) {
	d, ok := fyne.CurrentApp().Driver().(systrayDriver)
	if !ok {
		setStatus(l10n.X("tray.not_available", "System tray is not available on this driver"))
		return
	}

	visible := true
	showHide := fyne.NewMenuItem(l10n.X("tray.menu.show_hide", "Show/Hide window"), nil)
	showHide.Action = func() {
		if visible {
			w.Hide()
			visible = false
		} else {
			w.Show()
			w.RequestFocus()
			visible = true
		}
	}

	reload := fyne.NewMenuItem(l10n.X("tray.menu.reload_daemon", "Reload daemon"), func() {
		resp, err := client.Reload()
		switch {
		case err != nil:
			setStatus(l10n.X("tray.reload_failed_v", "Reload failed: {{.Err}}", "Err", err))
		case !resp.Ok:
			setStatus(l10n.X("tray.reload_failed_s", "Reload failed: {{.Error}}", "Error", resp.Error))
		default:
			setStatus(l10n.X("tray.respawned", "Daemon respawned (PID {{.PID}})", "PID", resp.PID))
		}
	})

	menu := fyne.NewMenu("xswitcher",
		showHide,
		reload,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(l10n.X("tray.menu.quit", "Quit"), func() { fyne.CurrentApp().Quit() }),
	)
	d.SetSystemTrayMenu(menu)
	d.SetSystemTrayIcon(fyne.NewStaticResource("tray.png", trayIconPNG))

	w.SetCloseIntercept(func() {
		w.Hide()
		visible = false
	})
}
