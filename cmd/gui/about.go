// Package main — see main.go for overview.
// about.go provides the About dialog panel with build-time metadata,
// attribution and project links.
package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/about"
)

// newAboutPanel creates the About tab content showing version, commit,
// attribution and project URLs.
func newAboutPanel() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("xswitcher GUI", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	version := widget.NewLabel(fmt.Sprintf("Version: %s", about.Version))
	commit := widget.NewLabel(fmt.Sprintf("Commit: %s", about.Commit))
	changes := widget.NewLabel(fmt.Sprintf("Changes by: %s", about.ChangesBy))
	upstreamLink := widget.NewHyperlink("Upstream: "+about.UpstreamURL, nil)
	license := widget.NewLabel(fmt.Sprintf("License: %s", about.License))

	version.Alignment = fyne.TextAlignCenter
	commit.Alignment = fyne.TextAlignCenter
	changes.Alignment = fyne.TextAlignCenter
	license.Alignment = fyne.TextAlignCenter

	return container.NewVBox(
		container.NewPadded(title),
		version,
		commit,
		widget.NewSeparator(),
		changes,
		upstreamLink,
		widget.NewSeparator(),
		license,
	)
}
