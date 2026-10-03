// Package main — see main.go for overview.
// sounds_editor.go provides the [Sounds] sub-tab: the four fixed daemon
// event kinds, each with an audio file path and a Play button. The daemon
// pushes "sound" events over IPC; this tab binds them to files.
//
// Removal limit by design: mods only send non-empty paths (the parser
// rejects empty ones), so clearing a row keeps the stored value — delete
// the key in the raw config instead.
package main

import (
	"sort"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	cfg "xswitcher/internal/config"
	"xswitcher/internal/ipc"
	"xswitcher/internal/sounds"
)

// soundRouter holds the live event→file map for the IPC subscription
// callback. The callback runs on the client's reader goroutine while load()
// rewrites the map from the Fyne thread, hence the mutex.
type soundRouter struct {
	mu     sync.Mutex
	paths  map[string]string
	player *sounds.Player
}

func newSoundRouter(p *sounds.Player) *soundRouter {
	return &soundRouter{paths: map[string]string{}, player: p}
}

func (r *soundRouter) set(paths map[string]string) {
	r.mu.Lock()
	r.paths = paths
	r.mu.Unlock()
}

// handle plays the file bound to one pushed event; unknown or unmuted-by-
// config events are silent no-ops.
func (r *soundRouter) handle(ev ipc.Event) {
	if ev.Event != "sound" {
		return
	}
	r.mu.Lock()
	path := r.paths[ev.Name]
	r.mu.Unlock()
	r.player.Play(path)
}

type soundsEditor struct {
	rows map[string]*widget.Entry // event kind -> path entry
}

func newSoundsEditor(player *sounds.Player) (fyne.CanvasObject, *soundsEditor) {
	e := &soundsEditor{rows: make(map[string]*widget.Entry)}
	events := make([]string, 0, len(cfg.SoundEvents))
	for name := range cfg.SoundEvents {
		events = append(events, name)
	}
	sort.Strings(events)

	box := container.NewVBox(widget.NewLabel(
		"Sound files are played by the GUI process; leave a row empty for silence."))
	for _, ev := range events {
		entry := widget.NewEntry()
		entry.SetPlaceHolder("/usr/share/sounds/" + ev + ".wav")
		e.rows[ev] = entry
		card := widget.NewCard("", ev, container.NewBorder(nil, nil, nil,
			widget.NewButton("Play", func() { player.Play(entry.Text) }),
			container.NewHBox(widget.NewLabel("file"), container.NewGridWrap(
				fyne.NewSize(400, 36), entry)),
		))
		box.Add(card)
	}
	return container.NewVScroll(box), e
}

// load fills the rows from get_config data and refreshes the router map.
func (e *soundsEditor) load(data map[string]any, router *soundRouter) {
	sndSection := section(data, "Sounds")
	paths := make(map[string]string, len(e.rows))
	for ev, entry := range e.rows {
		path := mapStr(sndSection, ev)
		entry.SetText(path)
		if path != "" {
			paths[ev] = path
		}
	}
	if router != nil {
		router.set(paths)
	}
}

// mods sends every non-empty row as a dotted "Sounds.<event>" key; the
// line editor creates the [Sounds] section when the file lacks it.
func (e *soundsEditor) mods() map[string]any {
	mods := map[string]any{}
	for ev, entry := range e.rows {
		if s := strings.TrimSpace(entry.Text); s != "" {
			mods["Sounds."+ev] = s
		}
	}
	return mods
}
