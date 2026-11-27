// Package main — see main.go for overview.
// fyne_locale.go pins the environment that Fyne reads its own widget strings
// from. Fyne v2.6.3 exposes no locale setter — lang only offers SystemLocale()
// and AddTranslations*, so the process environment is the single lever for
// Fyne's built-in labels (Cancel/OK/…).
package main

import (
	"os"
)

// applyFyneLocale writes an explicit language choice into LC_ALL and LANGUAGE.
// An empty code means nobody chose a language: the inherited environment is left
// untouched so the desktop locale (and Fyne's translations for it) stays
// authoritative.
//
// The rule it has to satisfy is go-locale's getUnixLocales(), which l10n.osLocale
// reproduces and Fyne calls through lang.SystemLocale: the first non-empty of
// LC_ALL, LC_MESSAGES, LANG fills the slot; if that slot is "C" or "POSIX" NLS is
// off and LANGUAGE is not consulted at all; otherwise a non-empty LANGUAGE wins
// over the slot. LC_ALL therefore has to be set (it disables the "C" branch) and
// LANGUAGE has to be set with it (otherwise a stale LANGUAGE from the caller
// outranks us and the two layers disagree).
func applyFyneLocale(code string) {
	if code == "" {
		return
	}
	posix := posixLocale(code)
	os.Setenv("LC_ALL", posix)
	os.Setenv("LANGUAGE", code)
}

// posixLocale turns a catalogue code into a POSIX locale string. Fyne parses the
// slot itself, so the region and charset only have to look well-formed.
func posixLocale(code string) string {
	switch code {
	case "ru":
		return "ru_RU.UTF-8"
	case "en":
		return "en_US.UTF-8"
	}
	return code + "_UTF-8"
}
