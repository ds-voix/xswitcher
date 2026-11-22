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
	"xswitcher/internal/ipc"
	"xswitcher/internal/l10n"
	customtheme "xswitcher/internal/theme"
)

// newAboutPanel creates the About tab content showing version, commit,
// attribution, project URLs, the language selector and the theme picker.
// onLanguage applies a chosen catalogue code and returns a message to display.
func newAboutPanel(client *ipc.Client, onLanguage func(code string) string) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(l10n.X("about.title", "xswitcher GUI"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	version := widget.NewLabel(l10n.X("about.version", "Version: {{.Value}}", "Value", about.Version))
	commit := widget.NewLabel(l10n.X("about.commit", "Commit: {{.Value}}", "Value", about.Commit))
	changes := widget.NewLabel(l10n.X("about.changes", "Changes by: {{.Value}}", "Value", about.ChangesBy))
	upstreamLink := widget.NewHyperlink(l10n.X("about.upstream", "Upstream: {{.URL}}", "URL", about.UpstreamURL), nil)
	license := widget.NewLabel(l10n.X("about.license", "License: {{.Value}}", "Value", about.License))
	donateLink := widget.NewHyperlink(l10n.X("about.donate", "Donate: {{.URL}}", "URL", about.DonateURL), nil)

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

	themeFeedback := widget.NewLabel("")
	themeFeedback.Alignment = fyne.TextAlignCenter
	themeCard := widget.NewCard(
		l10n.X("about.theme_card", "Theme"),
		l10n.X("about.theme_note", "Select a built-in variant or a custom theme from ~/.config/xswitcher/themes/."),
		newThemePicker(client, feedback.SetText),
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
		donateLink,
		widget.NewSeparator(),
		languageCard,
		themeCard,
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

func newThemePicker(client *ipc.Client, showFeedback func(string)) *widget.Select {
	names := customtheme.List()
	picker := widget.NewSelect(names, nil)
	if cur := persistedTheme(client); cur != "" {
		for _, n := range names {
			if n == cur {
				picker.SetSelected(cur)
				break
			}
		}
	}
	picker.OnChanged = func(name string) {
		t, err := customtheme.Load(name)
		if err != nil {
			showFeedback(l10n.X("about.theme_rejected", "Not applied: {{.Err}}", "Err", err))
			return
		}
		fyne.CurrentApp().Settings().SetTheme(t)
		if err := client.SetConfig(map[string]any{"GUI.Theme": name}); err != nil {
			showFeedback(l10n.X("about.theme_not_saved", "Applied, but not saved: {{.Err}}", "Err", err))
			return
		}
		showFeedback(l10n.X("about.theme_applied", "Theme applied: {{.Name}}", "Name", name))
	}
	return picker
}
