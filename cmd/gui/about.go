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

	languages := l10n.Available()
	labels := make([]string, 0, len(languages))
	codeByLabel := make(map[string]string, len(languages))
	labelByCode := make(map[string]string, len(languages))
	for _, d := range languages {
		label := d.Name
		if d.Name != d.English {
			label = fmt.Sprintf("%s (%s)", d.Name, d.English)
		}
		labels = append(labels, label)
		codeByLabel[label] = d.Code
		labelByCode[d.Code] = label
	}

	feedback := widget.NewLabel("")
	feedback.Alignment = fyne.TextAlignCenter

	current := l10n.Current()
	picker := widget.NewSelect(labels, func(chosen string) {
		code, ok := codeByLabel[chosen]
		if !ok {
			return
		}
		feedback.SetText(onLanguage(code))
	})
	if label, ok := labelByCode[current]; ok {
		picker.SetSelected(label)
	}

	languageCard := widget.NewCard(
		l10n.X("about.language_card", "Language"),
		l10n.X("about.language_note", "Applies to this window at once. Fyne's own buttons (OK, Cancel, and the rest) follow the process locale."),
		container.NewVBox(picker, feedback),
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
