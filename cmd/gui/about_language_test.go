// Package main — see main.go for overview.
// about_language_test.go pins the one place where building the interface can call
// back into rebuilding it: the language picker restores the active locale with
// SetSelected, and Fyne fires OnChanged from that call.
package main

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"xswitcher/internal/l10n"
)

// captionOf is the picker caption a catalogue code gets, resolved through the
// production helper so the test cannot drift from it.
func captionOf(code string) string {
	for _, d := range l10n.Available() {
		if d.Code == code {
			return languageLabel(d)
		}
	}
	return ""
}

func TestLanguagePickerDoesNotChooseForItself(t *testing.T) {
	loadCatalogues(t)
	app := test.NewApp()
	defer app.Quit()

	restore := l10n.SetForTest("en")
	defer restore()

	calls := 0
	feedback := ""
	picker := newLanguagePicker(
		func(code string) string { calls++; return "applied " + code },
		func(text string) { feedback = text },
	)
	// Restoring the active code must not reach onLanguage: onLanguage rebuilds the
	// window, the rebuild builds this picker again, and the loop ends only when the
	// stack runs out -- which is how the GUI shipped without a window at all.
	if calls != 0 {
		t.Errorf("building the picker called onLanguage %d time(s), want 0", calls)
	}
	if want := captionOf("en"); picker.Selected != want {
		t.Errorf("picker.Selected = %q, want the active language %q", picker.Selected, want)
	}
	// Re-choosing the active language is not a choice either: the rebuild would
	// only throw away the unsaved edits of the Config tab.
	picker.OnChanged(picker.Selected)
	if calls != 0 {
		t.Errorf("re-choosing the active language called onLanguage %d time(s), want 0", calls)
	}
	// A real switch still goes through, and its message reaches the label.
	other := captionOf("ru")
	if other == "" {
		t.Fatal("the ru catalogue is not available to the picker")
	}
	picker.OnChanged(other)
	if calls != 1 {
		t.Errorf("switching to ru called onLanguage %d time(s), want 1", calls)
	}
	if feedback != "applied ru" {
		t.Errorf("feedback = %q, want %q", feedback, "applied ru")
	}
	// An option that is not a catalogue code must be ignored, not panic.
	picker.OnChanged("Not a language")
	if calls != 1 {
		t.Errorf("an unknown caption called onLanguage (now %d times), want 1", calls)
	}
}
