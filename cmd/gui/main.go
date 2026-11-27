// Package main is the xswitcher GUI entry point. It uses Fyne v2 to provide
// a configuration editor and About dialog, communicating with the daemon
// over a Unix domain socket.
//
// Localization: every user-visible string is resolved through internal/l10n at
// widget-creation time, so changing the language rebuilds the window content
// (buildRoot). Fyne's own built-in strings are driven by the process locale and
// are set once at startup through applyFyneLocale.
//
// Build:
//
//	go build -ldflags "-X xswitcher/internal/about.Version=$(git describe --tags) \
//	  -X xswitcher/internal/about.Commit=$(git rev-parse --short HEAD)" \
//	  -o bin/xswitcher-gui ./cmd/gui/
package main

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/ipc"
	"xswitcher/internal/l10n"
	"xswitcher/internal/sounds"
	customtheme "xswitcher/internal/theme"
)

func main() {
	if err := l10n.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "l10n: %v\n", err)
	}

	client := ipc.NewClient(ipc.SocketPath())

	// The daemon owns the persisted choice and the IPC client needs no Fyne, so it
	// is read before the first widget exists: Fyne takes its own locale from the
	// process environment the first time it is asked and offers no setter, so a
	// later change cannot reach its built-in strings.
	code := ""
	if os.Getenv("XSWITCHER_LANG") != "" {
		code = l10n.Current()
	}
	if persisted := persistedLanguage(client); persisted != "" {
		if err := l10n.Set(persisted); err == nil {
			code = l10n.Current()
		} else {
			fmt.Fprintf(os.Stderr, "l10n: %v\n", err)
		}
	}
	applyFyneLocale(code)

	a := app.NewWithID("com.github.xswitcher.gui")
	if persistedTheme := persistedTheme(client); persistedTheme != "" {
		if t, err := customtheme.Load(persistedTheme); err == nil {
			a.Settings().SetTheme(t)
		} else {
			fmt.Fprintf(os.Stderr, "theme: %v\n", err)
		}
	}
	w := a.NewWindow("xswitcher")
	w.Resize(fyne.NewSize(600, 500))

	router := newSoundRouter(sounds.New())

	// Keep a live event subscription: every daemon respawn closes the old
	// connection, and the loop re-subscribes two seconds later. Events only
	// start a playback process, so the reader goroutine never touches the
	// Fyne widgets and needs no fyne.Do marshalling.
	go func() {
		for {
			done, err := client.Subscribe(router.handle)
			if err == nil {
				<-done
			}
			time.Sleep(2 * time.Second)
		}
	}()

	var rebuild func()
	// Remembered across a language rebuild by tab index, never by label: the
	// labels are the very thing that changes language.
	state := new(uiState)
	rebuild = func() { w.SetContent(buildRoot(w, client, router, state, rebuild)) }
	rebuild()

	w.ShowAndRun()
}

// uiState is where the open tabs live while the window content is thrown away and
// built again.
type uiState struct {
	tab, sub int
}

// buildRoot creates the entire window content for the active locale. Fyne
// widgets capture their text when they are constructed, so a language change is
// applied by throwing this tree away and building it again; that is why
// rebuilding also re-reads the live config and status (unsaved edits in the
// Config tab are dropped by design).
func buildRoot(w fyne.Window, client *ipc.Client, router *soundRouter, state *uiState, rebuild func()) fyne.CanvasObject {
	editor, loadConfig := newEditorPanel(client, router, state)
	statusPanel, refreshStatus := newStatusPanel(client)
	aboutPanel := newAboutPanel(client, func(code string) string { return setLanguage(client, code, rebuild) })

	tabConfig := container.NewTabItem(l10n.X("gui.tab.config", "Config"), editor)
	tabStatus := container.NewTabItem(l10n.X("gui.tab.status", "Status"), statusPanel)
	tabAbout := container.NewTabItem(l10n.X("gui.tab.about", "About"), aboutPanel)

	tabs := container.NewAppTabs(tabConfig, tabStatus, tabAbout)
	// Re-read the live daemon state every time a tab is opened. Match the tab by
	// identity, not by its label: the label is localized and changes language.
	tabs.OnSelected = func(item *container.TabItem) {
		state.tab = tabs.SelectedIndex()
		switch item {
		case tabConfig:
			loadConfig()
		case tabStatus:
			refreshStatus()
		}
	}
	// AppTabs starts on the first item, so only a departure from it needs a call;
	// selecting Config would also re-read the daemon for nothing.
	if state.tab > 0 {
		tabs.SelectIndex(state.tab)
	}

	statusBar := widget.NewLabel(l10n.X("gui.state.disconnected", "Disconnected"))
	setStatus := func(text string) { statusBar.SetText(text) }

	connect := func() {
		if err := client.Ping(); err != nil {
			statusBar.SetText(l10n.X("gui.state.error", "Error: {{.Err}}", "Err", err))
			return
		}
		statusBar.SetText(l10n.X("gui.state.connected", "Connected to daemon"))
		loadConfig()
		refreshStatus()
	}

	connectBtn := widget.NewButton(l10n.X("gui.btn.connect", "Connect"), connect)

	setupTray(w, client, setStatus)

	topBar := container.NewHBox(connectBtn, statusBar)
	connect() // auto-connect on start (Punto parity Phase 1)

	return container.NewBorder(topBar, nil, nil, nil, tabs)
}

// persistedLanguage reads [GUI] Language from the live daemon. It returns "" when
// the daemon is unreachable or the key is unset, which means "follow the OS".
func persistedLanguage(client *ipc.Client) string {
	data, err := client.GetConfig()
	if err != nil {
		return ""
	}
	return mapStr(section(data, "GUI"), "Language")
}

func persistedTheme(client *ipc.Client) string {
	data, err := client.GetConfig()
	if err != nil {
		return ""
	}
	return mapStr(section(data, "GUI"), "Theme")
}

// setLanguage applies a language choice: it switches the resolver, persists the
// preference in [GUI] Language through the daemon (no respawn — the preference is
// not something the daemon hot-reloads), rebuilds the window so every widget
// re-captures its text, and returns the message the About tab should show.
func setLanguage(client *ipc.Client, code string, rebuild func()) string {
	if err := l10n.Set(code); err != nil {
		return l10n.X("about.language_rejected", "Not applied: {{.Err}}", "Err", err)
	}
	applyFyneLocale(code)
	rebuild()
	if err := client.SetConfig(map[string]any{"GUI.Language": code}); err != nil {
		return l10n.X("about.language_not_saved", "Applied, but not saved: {{.Err}}", "Err", err)
	}
	return l10n.X("about.language_saved", "Saved", "Code", code)
}
