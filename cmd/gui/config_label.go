// Package main — see main.go for overview.
// config_label.go renders a TOML field name as a form caption. In these editors
// one word plays two roles — the caption the user reads and the key the editor
// reads from and writes to the config — so only the caption may go through the
// catalogue: every mods["..."] write and every map lookup keeps its raw
// literal, and a language with no entry for the field shows the field name
// verbatim rather than a mistranslation of it.
package main

import (
	"strings"

	"xswitcher/internal/l10n"
)

// configKeyLabel localizes the caption of one TOML field. The catalogue key is
// the lower-cased field name, so "SendBuffer" is config.key.sendbuffer.
func configKeyLabel(field string) string {
	return l10n.X("config.key."+strings.ToLower(field), field)
}
