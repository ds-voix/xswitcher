// Package l10n is the application-level localization resolver. It owns the
// message catalogues and the active locale and deliberately does NOT import
// Fyne, so the daemon (src, internal) can localize its own messages without
// pulling fyne.io into the root binary (see the Phase 7 guard
// `go list -deps ./src | grep -c fyne.io == 0`). Fyne's own built-in widget
// strings (Cancel/OK/…) are localized separately, in cmd/gui, through
// fyne.io/fyne/v2/lang — that layer is unrelated to this package.
//
// Catalogues are go-i18n JSON files embedded under locales/. A message is looked
// up by key; when a key is missing from every catalogue in the fallback chain the
// English literal passed at the call site is returned, so adding a translation is
// never required for the program to run and editing an English string never
// silently breaks another language.
package l10n

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localesFS embed.FS

// baseCode is the fallback locale and the language every DefaultMessage literal
// is written in.
const baseCode = "en"

// Descriptor names one available language. Code is the stable key stored in
// config and matched against the catalogue file name. Name is the endonym shown
// in the language picker and English is the same language named in English, for
// the "(English)" hint beside a foreign endonym. Both are read from the
// catalogue (language.name / language.english), not from a Go map, so "drop a
// JSON file, get a language" keeps working without touching code.
type Descriptor struct {
	Code    string
	Name    string
	English string
}

var (
	mu        sync.RWMutex
	bundle    = i18n.NewBundle(language.English)
	localizer = i18n.NewLocalizer(bundle, baseCode)
	current   = baseCode // resolved active locale; never empty after Init
	catalogs  = map[string]Descriptor{}
)

// Init loads every embedded locales/*.json, records each language's descriptor,
// and resolves the starting locale (XSWITCHER_LANG, else the OS locale, else
// en). Callers that persist a [GUI] Language choice apply it afterwards with
// Set; l10n stays independent of the config parser on purpose.
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	matches, err := fs.Glob(localesFS, "locales/*.json")
	if err != nil {
		return err
	}
	for _, path := range matches {
		if err := loadFS(localesFS, path); err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
	}
	if _, ok := catalogs[baseCode]; !ok {
		return fmt.Errorf("base catalogue %q is not embedded", baseCode)
	}
	applyLocked(resolve(""))
	return nil
}

// Load adds a single catalogue file from an arbitrary fs.FS at runtime. Init
// uses it for the embedded locales; tests and any future on-disk drop use it
// too. It does not switch the active locale — call Set for that.
func Load(fsys fs.FS, path string) error {
	mu.Lock()
	defer mu.Unlock()
	return loadFS(fsys, path)
}

func loadFS(fsys fs.FS, path string) error {
	mf, err := bundle.LoadMessageFileFS(fsys, path)
	if err != nil {
		return err
	}
	base, _ := mf.Tag.Base()
	code := base.String()
	desc := Descriptor{Code: code, Name: code, English: code}
	loc := i18n.NewLocalizer(bundle, code)
	if s, err := loc.Localize(&i18n.LocalizeConfig{MessageID: "language.name"}); err == nil {
		desc.Name = s
	}
	if s, err := loc.Localize(&i18n.LocalizeConfig{MessageID: "language.english"}); err == nil {
		desc.English = s
	}
	catalogs[code] = desc
	return nil
}

// Available returns every loaded language, sorted by code.
func Available() []Descriptor {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Descriptor, 0, len(catalogs))
	for _, d := range catalogs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Current returns the resolved active locale code.
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set switches the active locale. An empty code means "follow the OS locale";
// any other code must name a loaded catalogue, otherwise it is rejected so a
// typo in [GUI] Language cannot silently pin the UI to the wrong file.
func Set(code string) error {
	mu.Lock()
	defer mu.Unlock()
	if code != "" && !known(code) {
		return fmt.Errorf("unknown language %q", code)
	}
	applyLocked(resolve(code))
	return nil
}

// Default reports the locale the resolver would pick with no explicit choice:
// XSWITCHER_LANG, else the OS locale, else en.
func Default() string {
	mu.RLock()
	defer mu.RUnlock()
	return resolve("")
}

// X localizes key, substituting {{.Field}} templates from data (a
// map[string]any or alternating "name", value pairs). The English literal
// fallback is returned when the key is not in any catalogue.
func X(key, fallback string, data ...any) string {
	mu.RLock()
	l := localizer
	mu.RUnlock()
	s, err := l.Localize(&i18n.LocalizeConfig{
		MessageID:      key,
		TemplateData:   templateData(data),
		DefaultMessage: &i18n.Message{ID: key, Other: fallback},
	})
	if err != nil {
		return fallback
	}
	return s
}

// XN localizes a plural key. fallback is the English "other" form; count selects
// the CLDR plural category for the active locale (Russian gets one/few/many for
// free via x/text/plural). data carries the template values; "Count" is filled
// with count when the caller does not supply it.
func XN(key, fallback string, count int, data ...any) string {
	mu.RLock()
	l := localizer
	mu.RUnlock()
	td := templateData(data)
	m, ok := td.(map[string]any)
	if !ok {
		m = map[string]any{}
	}
	if _, has := m["Count"]; !has {
		m["Count"] = count
	}
	s, err := l.Localize(&i18n.LocalizeConfig{
		MessageID:      key,
		PluralCount:    count,
		TemplateData:   m,
		DefaultMessage: &i18n.Message{ID: key, One: fallback, Other: fallback},
	})
	if err != nil {
		return fallback
	}
	return s
}

// Errf builds an error whose message is localized like X. The daemon uses it for
// user-visible failures that still surface as plain error strings.
func Errf(key, formatFallback string, data ...any) error {
	return fmt.Errorf("%s", X(key, formatFallback, data...))
}

// MustInitFromEnv initializes l10n for a non-GUI process, degrading to the
// English literals (never aborting the daemon) if the catalogues cannot load.
func MustInitFromEnv() {
	if err := Init(); err != nil {
		fmt.Fprintf(os.Stderr, "l10n: %v\n", err)
	}
}

// SetForTest switches the locale and returns a closure that restores the
// previous one, so tests that pin a locale do not leak into their neighbors.
func SetForTest(code string) func() {
	mu.Lock()
	prev := current
	applyLocked(resolve(code))
	mu.Unlock()
	return func() {
		mu.Lock()
		applyLocked(prev)
		mu.Unlock()
	}
}

// applyLocked rebuilds the localizer for an explicit resolved code. Callers hold
// the write lock. The chain is [code, en] so a key missing from the active
// catalogue falls through to English before the call-site literal.
func applyLocked(code string) {
	current = code
	tags := []string{code}
	if code != baseCode {
		tags = append(tags, baseCode)
	}
	localizer = i18n.NewLocalizer(bundle, tags...)
}

// resolve turns a requested code into an effective one: an explicit non-empty
// code wins, otherwise XSWITCHER_LANG, otherwise the OS locale, otherwise en.
func resolve(code string) string {
	if code != "" {
		return canonical(code)
	}
	if env := os.Getenv("XSWITCHER_LANG"); env != "" {
		if c := canonical(env); c != "" {
			return c
		}
	}
	if c := osLocale(); c != "" {
		return c
	}
	return baseCode
}

func known(code string) bool {
	return canonical(code) != ""
}

// canonical maps a loose code (ru, RU, ru_RU, ru.UTF-8) to a loaded catalogue
// code, or "" when nothing matches.
func canonical(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if i := strings.IndexAny(raw, ".@"); i >= 0 {
		raw = raw[:i]
	}
	hyphen := strings.ReplaceAll(raw, "_", "-")
	base := strings.SplitN(hyphen, "-", 2)[0]
	for code := range catalogs {
		if code == raw || code == hyphen || code == base {
			return code
		}
	}
	return ""
}

// osLocale mirrors jeandeaual/go-locale's getUnixLocales() exactly, because Fyne
// resolves its own built-in strings through that same function (lang/lang.go ->
// locale.GetLocales): any divergence makes our app and Fyne's Cancel/OK labels
// disagree on the same machine. The rule is: the first non-empty of LC_ALL,
// LC_MESSAGES, LANG fills the slot; if that slot is "C"/"POSIX" NLS is off and
// LANGUAGE is NOT consulted at all; otherwise a non-empty LANGUAGE overrides the
// slot with a colon-separated priority list. LANGUAGE is therefore the strongest
// lever, not the weakest.
func osLocale() string {
	slot := ""
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			slot = v
			break
		}
	}
	if slot == "C" || slot == "POSIX" {
		return ""
	}
	locale := slot
	if lang := os.Getenv("LANGUAGE"); lang != "" {
		locale = lang
	}
	if locale == "" {
		return ""
	}
	for _, part := range strings.Split(locale, ":") {
		if c := canonical(part); c != "" {
			return c
		}
	}
	return ""
}

func templateData(data []any) any {
	if len(data) == 1 {
		if m, ok := data[0].(map[string]any); ok {
			return m
		}
	}
	m := map[string]any{}
	for i := 0; i+1 < len(data); i += 2 {
		k, _ := data[i].(string)
		m[k] = data[i+1]
	}
	return m
}
