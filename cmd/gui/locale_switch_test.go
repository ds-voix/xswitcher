// Package main — see main.go for overview.
// locale_switch_test.go pins what the language switch actually reaches. Two
// things had no test and both broke silently: a form caption that is also a TOML
// key name (it must localize as a caption and stay raw as a key), and the
// Hotkeys dropdown built by a package-level cache (built in init() it froze in
// English, because main() only loads the catalogues after package init).
package main

import (
	"sync"
	"testing"

	"fyne.io/fyne/v2/test"

	"xswitcher/internal/l10n"
)

var initCatalogues sync.Once

// loadCatalogues fills the bundle from the embedded files: without it every
// lookup falls back to the call-site literal and a locale test proves nothing.
func loadCatalogues(t *testing.T) {
	t.Helper()
	initCatalogues.Do(func() {
		if err := l10n.Init(); err != nil {
			t.Fatalf("l10n.Init: %v", err)
		}
	})
}

func TestConfigKeyLabelFollowsLocale(t *testing.T) {
	loadCatalogues(t)

	restore := l10n.SetForTest("en")
	if got := configKeyLabel("SendBuffer"); got != "SendBuffer" {
		t.Errorf("en caption = %q, want the raw field name", got)
	}
	restore()

	restore = l10n.SetForTest("ru")
	defer restore()
	if got, want := configKeyLabel("SendBuffer"), "Отправляемый буфер"; got != want {
		t.Errorf("ru caption = %q, want %q", got, want)
	}
	// A field with no entry keeps its machine name on screen rather than
	// inventing a translation of it.
	if got := configKeyLabel("NoSuchField"); got != "NoSuchField" {
		t.Errorf("unknown field caption = %q, want the raw field name", got)
	}
}

func TestKeyNamesFollowLocale(t *testing.T) {
	loadCatalogues(t)
	// The table is cached per locale, so the same call must answer twice in one
	// locale and change only when the locale changes.
	restore := l10n.SetForTest("en")
	first := keyLabelFor("-")
	if second := keyLabelFor("-"); second != first {
		t.Fatalf("en label changed under one locale: %q then %q", first, second)
	}
	if first != "Hyphen (-)" {
		t.Errorf("en label = %q, want %q", first, "Hyphen (-)")
	}
	restore()

	restore = l10n.SetForTest("ru")
	defer restore()
	got := keyLabelFor("-")
	if want := "Дефис (-)"; got != want {
		t.Errorf("ru label = %q, want %q", got, want)
	}
	// The canonical name in parentheses is what the config stores, and the
	// reverse lookup must still find it through the rebuilt table.
	if real := keyRealFor(got); real != "-" {
		t.Errorf("keyRealFor(%q) = %q, want %q", got, real, "-")
	}
	if !keyHasReal("-") {
		t.Error("keyHasReal(\"-\") = false under ru, the curated list was lost")
	}
}

func TestHotkeysDropdownOptionsAreLocalized(t *testing.T) {
	loadCatalogues(t)
	app := test.NewApp()
	defer app.Quit()

	restore := l10n.SetForTest("ru")
	defer restore()

	_, e := newHotkeysEditor()
	e.addRow([]string{"L_CTRL", "PAGEUP"}, "")
	if len(e.rows) != 1 {
		t.Fatalf("addRow made %d rows, want 1", len(e.rows))
	}
	row := e.rows[0]
	if row.trigger.Selected != "Стр. вверх (PAGEUP)" {
		t.Errorf("selected = %q, want the Russian caption of PAGEUP", row.trigger.Selected)
	}
	// Saving must write the canonical daemon name, not the caption.
	mods := e.mods()
	recs, _ := mods["Hotkeys"].([]any)
	if len(recs) != 1 {
		t.Fatalf("mods Hotkeys = %v, want one record", mods["Hotkeys"])
	}
	keys, _ := recs[0].(map[string]any)["Keys"].([]string)
	found := false
	for _, k := range keys {
		if k == "PAGEUP" {
			found = true
		}
	}
	if !found {
		t.Errorf("Keys = %v, want the canonical PAGEUP back in the config", keys)
	}
}
