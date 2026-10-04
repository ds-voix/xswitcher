// Package main — see main.go for overview.
// about.go provides the About dialog panel with build-time metadata,
// attribution, project links and the language picker.
package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/about"
	"xswitcher/internal/l10n"
)

// newAboutPanel creates the About tab content showing version, commit,
// attribution, project URLs and the language selector. onLanguage applies a
// chosen catalogue code and returns a message to display (empty for nothing).
func newAboutPanel(onLanguage func(code string) string) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(l10n.X("about.title", "xswitcher GUI"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	version := widget.NewLabel(l10n.X("about.version", "Version: {{.Value}}", "Value", about.Version))
	commit := widget.NewLabel(l10n.X("about.commit", "Commit: {{.Value}}", "Value", about.Commit))
	changes := widget.NewLabel(l10n.X("about.changes", "Changes by: {{.Value}}", "Value", about.ChangesBy))
	upstreamLink := widget.NewHyperlink(l10n.X("about.upstream", "Upstream: {{.URL}}", "URL", about.UpstreamURL), nil)
	license := widget.NewLabel(l10n.X("about.license", "License: {{.Value}}", "Value", about.License))

	for _, l := range []*widget.Label{version, commit, changes, license} {
		l.Alignment = fyne.TextAlignCenter
	}

	feedback := widget.NewLabel("")
	feedback.Alignment = fyne.TextAlignCenter

	languageCard := widget.NewCard(
		l10n.X("about.language_card", "Language"),
		l10n.X("about.language_note", "Applies to this window at once. Fyne's own buttons (OK, Cancel, and the rest) follow the process locale."),
		container.NewVBox(newLanguagePicker(onLanguage, feedback.SetText), feedback),
	)

	return container.NewVBox(
		container.NewPadded(title),
		version,
		commit,
		widget.NewSeparator(),
		changes,
		upstreamLink,
		widget.NewSeparator(),
		license,
		widget.NewSeparator(),
		languageCard,
	)
}

// newLanguagePicker builds the dropdown over the catalogues l10n can speak.
// The callback goes on only after SetSelected has restored the active code,
// because Fyne fires OnChanged from SetSelected: a panel that reached onLanguage
// while being built would call setLanguage, which rebuilds the window, which
// builds this panel again -- the recursion that left the GUI without a window.
func newLanguagePicker(onLanguage func(code string) string, showFeedback func(string)) *widget.Select {
	languages := l10n.Available()
	labels := make([]string, 0, len(languages))
	codeByLabel := make(map[string]string, len(languages))
	labelByCode := make(map[string]string, len(languages))
	for _, d := range languages {
		label := languageLabel(d)
		labels = append(labels, label)
		codeByLabel[label] = d.Code
		labelByCode[d.Code] = label
	}

	picker := widget.NewSelect(labels, nil)
	if label, ok := labelByCode[l10n.Current()]; ok {
		picker.SetSelected(label)
	}
	picker.OnChanged = func(chosen string) {
		code, ok := codeByLabel[chosen]
		// Re-choosing the language that is already active is not a choice: the
		// rebuild it would trigger only throws away the edits of other tabs.
		if !ok || code == l10n.Current() {
			return
		}
		showFeedback(onLanguage(code))
	}
	return picker
}

// languageLabel is the caption a catalogue gets in the picker: its native name,
// with the English name appended when the two differ.
func languageLabel(d l10n.Descriptor) string {
	if d.Name == d.English {
		return d.Name
	}
	return fmt.Sprintf("%s (%s)", d.Name, d.English)
}
