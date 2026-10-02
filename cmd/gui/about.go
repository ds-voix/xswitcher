// Package main — see main.go for overview.
// about.go provides the About dialog panel with build-time metadata,
// maintainer info, and project links.
package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/about"
)

// newAboutPanel creates the About tab content showing version, commit,
// maintainer, and project URLs.
func newAboutPanel() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("xswitcher GUI", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	version := widget.NewLabel(fmt.Sprintf("Version: %s", about.Version))
	commit := widget.NewLabel(fmt.Sprintf("Commit: %s", about.Commit))
	maintainer := widget.NewLabel(fmt.Sprintf("Maintainer: %s", about.Maintainer))
	forkLink := widget.NewHyperlink("Fork: "+about.ForkURL, nil)
	upstreamLink := widget.NewHyperlink("Upstream: "+about.UpstreamURL, nil)
	license := widget.NewLabel(fmt.Sprintf("License: %s", about.License))

	version.Alignment = fyne.TextAlignCenter
	commit.Alignment = fyne.TextAlignCenter
	maintainer.Alignment = fyne.TextAlignCenter
	license.Alignment = fyne.TextAlignCenter

	return container.NewVBox(
		container.NewPadded(title),
		version,
		commit,
		widget.NewSeparator(),
		maintainer,
		forkLink,
		upstreamLink,
		widget.NewSeparator(),
		license,
	)
}