// Package main is the xswitcher GUI entry point. It uses Fyne v2 to provide
// a configuration editor and About dialog, communicating with the daemon
// over a Unix domain socket.
//
// Build:
//
//	go build -ldflags "-X xswitcher/internal/about.Version=$(git describe --tags) \
//	  -X xswitcher/internal/about.Commit=$(git rev-parse --short HEAD)" \
//	  -o bin/xswitcher-gui ./cmd/gui/
package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/ipc"
)

func main() {
	a := app.New()
	w := a.NewWindow("xswitcher")
	w.Resize(fyne.NewSize(600, 500))

	client := ipc.NewClient(ipc.DefaultSocketPath)

	editor, loadConfig := newEditorPanel(client)

	tabs := container.NewAppTabs(
		container.NewTabItem("Config", editor),
		container.NewTabItem("About", newAboutPanel()),
	)
	// Re-read the live config every time the Config tab is opened.
	tabs.OnSelected = func(item *container.TabItem) {
		if item.Text == "Config" {
			loadConfig()
		}
	}

	statusBar := widget.NewLabel("Disconnected")
	connectBtn := widget.NewButton("Connect", func() {
		if err := client.Ping(); err != nil {
			statusBar.SetText(fmt.Sprintf("Error: %v", err))
		} else {
			statusBar.SetText("Connected to daemon")
			loadConfig()
		}
	})

	topBar := container.NewHBox(connectBtn, statusBar)
	content := container.NewBorder(topBar, nil, nil, nil, tabs)

	w.SetContent(content)
	w.ShowAndRun()
}
