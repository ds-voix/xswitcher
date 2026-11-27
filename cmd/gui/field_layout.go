// Package main — see main.go for overview.
// field_layout.go lays an editor row out left to right, giving every object its
// natural width except the ones marked flexible: those get a fixed fraction of
// the row's width so a config field is wide enough to read, and a single-line
// Entry scrolls its own text once the text outruns that width. The fraction is
// applied against the width the container is actually given, so the fields track
// the window as it is resized, and they shrink below the fraction only when the
// row would otherwise run past the right edge.
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type fieldLayout struct {
	frac float32
	gap  float32
	flex map[fyne.CanvasObject]bool
}

func newFieldLayout(frac float32) *fieldLayout {
	return &fieldLayout{frac: frac, gap: theme.Padding(), flex: map[fyne.CanvasObject]bool{}}
}

// markFlex makes o take the layout fraction instead of its own minimum width.
func (l *fieldLayout) markFlex(o fyne.CanvasObject) { l.flex[o] = true }

func (l *fieldLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) == 0 {
		return
	}
	height := float32(0)
	fixed := float32(0)
	nflex := 0
	for _, o := range objs {
		min := o.MinSize()
		if min.Height > height {
			height = min.Height
		}
		if l.flex[o] {
			nflex++
		} else {
			fixed += min.Width
		}
	}
	gaps := l.gap * float32(len(objs)-1)

	// The desired field width is the fraction of the row; whatever the rest of
	// the row does not leave over is the ceiling, so the row never overflows.
	per := size.Width * l.frac
	if room := (size.Width - gaps - fixed) / float32(nflex); nflex > 0 && room < per {
		if room < 0 {
			room = 0
		}
		per = room
	}

	x := float32(0)
	for _, o := range objs {
		w := o.MinSize().Width
		if l.flex[o] {
			w = per
		}
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(w, height))
		x += w + l.gap
	}
}

func (l *fieldLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	width := float32(0)
	height := float32(0)
	for _, o := range objs {
		min := o.MinSize()
		width += min.Width
		if min.Height > height {
			height = min.Height
		}
	}
	if len(objs) > 1 {
		width += l.gap * float32(len(objs)-1)
	}
	return fyne.NewSize(width, height)
}
