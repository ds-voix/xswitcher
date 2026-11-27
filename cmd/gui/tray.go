// Package main — see main.go for overview.
// tray.go installs the system tray menu (Punto parity Phase 1): show/hide,
// reload daemon, quit. Closing the main window hides it to the tray instead
// of quitting, Punto-style.
package main

import (
	_ "embed"
	"fmt"

	"fyne.io/fyne/v2"

	"xswitcher/internal/ipc"
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
		setStatus("System tray is not available on this driver")
		return
	}

	visible := true
	showHide := fyne.NewMenuItem("Show/Hide window", nil)
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

	reload := fyne.NewMenuItem("Reload daemon", func() {
		resp, err := client.Reload()
		switch {
		case err != nil:
			setStatus(fmt.Sprintf("Reload failed: %v", err))
		case !resp.Ok:
			setStatus(fmt.Sprintf("Reload failed: %s", resp.Error))
		default:
			setStatus(fmt.Sprintf("Daemon respawned (PID %d)", resp.PID))
		}
	})

	menu := fyne.NewMenu("xswitcher",
		showHide,
		reload,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() { fyne.CurrentApp().Quit() }),
	)
	d.SetSystemTrayMenu(menu)
	d.SetSystemTrayIcon(fyne.NewStaticResource("tray.png", trayIconPNG))

	w.SetCloseIntercept(func() {
		w.Hide()
		visible = false
	})
}
