// Package config — see types.go for overview.
// gui_test.go covers the [GUI] section: the Language key validated against the
// catalogues l10n embeds, and the writer's round-trip that appends the section
// to a file which does not have it yet.
package config

import (
	"strings"
	"testing"

	"xswitcher/internal/l10n"
)

// initL10n loads the embedded catalogues so the Language validation has
// something to resolve against. Without it l10n.Available() is empty and every
// non-empty Language would be rejected -- which is exactly what the daemon and
// the GUI avoid by calling Init (or MustInitFromEnv) before parsing.
func initL10n(t *testing.T) {
	t.Helper()
	if err := l10n.Init(); err != nil {
		t.Fatalf("l10n.Init: %v", err)
	}
	if len(l10n.Available()) < 2 {
		t.Fatalf("catalogues not loaded: %+v", l10n.Available())
	}
}

func TestParseGUILanguage(t *testing.T) {
	initL10n(t)
	parsed, err := ParseTOML([]byte("[GUI]\nLanguage = \"ru\"\n"))
	if err != nil {
		t.Fatalf("valid Language rejected: %v", err)
	}
	if parsed.GUI.Language != "ru" {
		t.Errorf("Language not read: %+v", parsed.GUI)
	}
	// The same base code written loosely must pass the same check canonical()
	// applies inside l10n, so a user cannot be refused a language they can set.
	for _, raw := range []string{"en", "RU", "ru_RU", "ru-UA", "ru.UTF-8", "ru@euro"} {
		if _, err := ParseTOML([]byte("[GUI]\nLanguage = \"" + raw + "\"\n")); err != nil {
			t.Errorf("loose spelling %q rejected: %v", raw, err)
		}
	}
}

func TestParseGUIAllowsEmptyOrAbsentLanguage(t *testing.T) {
	initL10n(t)
	// An empty key means "follow the OS locale" -- the default that keeps the
	// shipped config free of a [GUI] section nobody asked for.
	parsed, err := ParseTOML([]byte("[GUI]\nLanguage = \"\"\n"))
	if err != nil {
		t.Fatalf("empty Language rejected: %v", err)
	}
	if parsed.GUI.Language != "" {
		t.Errorf("empty Language not kept: %q", parsed.GUI.Language)
	}
	parsed, err = ParseTOML([]byte("[Keyboard]\nDelay = 5\n"))
	if err != nil {
		t.Fatalf("config without [GUI] rejected: %v", err)
	}
	if parsed.GUI.Language != "" {
		t.Errorf("absent [GUI] must stay the zero value, got %q", parsed.GUI.Language)
	}
}

func TestParseGUIRejectsUnknownLanguage(t *testing.T) {
	initL10n(t)
	_, err := ParseTOML([]byte("[GUI]\nLanguage = \"de\"\n"))
	if err == nil {
		t.Fatal("unknown Language accepted")
	}
	if !strings.Contains(err.Error(), `[GUI] unknown Language "de"`) {
		t.Errorf("error %q does not name the key", err.Error())
	}
	// The message has to list what is loadable, or the user cannot fix the typo.
	for _, want := range []string{"en", "ru", "available:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// The writer half: dotted key "GUI.Language" against a shipped config that has
// no [GUI] section at all. setLine must create the section, and the result must
// survive the ParseTOML gate of WriteTOMLChecked and parse back with the value.
func TestRoundTripGUILanguageAppendsSection(t *testing.T) {
	initL10n(t)
	before := string(shippedConf(t))
	path := confCopy(t, []byte(before))
	if err := WriteTOMLChecked(path, map[string]any{"GUI.Language": "ru"}); err != nil {
		t.Fatalf("append [GUI] rejected: %v", err)
	}
	after := string(mustConf(t, path))
	if !strings.HasPrefix(after, strings.TrimRight(before, "\n")) {
		t.Errorf("append disturbed existing bytes; original is no longer a prefix")
	}
	if !strings.Contains(after, "[GUI]") || !strings.Contains(after, "Language = \"ru\"") {
		t.Errorf("appended section missing from:\n%s", after)
	}
	parsed, err := ParseTOML([]byte(after))
	if err != nil {
		t.Fatalf("written config does not parse back: %v", err)
	}
	if parsed.GUI.Language != "ru" {
		t.Errorf("round-trip lost the value: %+v", parsed.GUI)
	}
	// Fixed point: the same key again must replace the line, not append a
	// second section or key.
	if err := WriteTOMLChecked(path, map[string]any{"GUI.Language": "en"}); err != nil {
		t.Fatalf("second [GUI] write rejected: %v", err)
	}
	second := string(mustConf(t, path))
	if n := strings.Count(second, "[GUI]"); n != 1 {
		t.Errorf("[GUI] section written twice (%d), setLine did not find the appended one", n)
	}
	if !strings.Contains(second, "Language = \"en\"") || strings.Contains(second, "Language = \"ru\"") {
		t.Errorf("the key line was not replaced in place:\n%s", second)
	}
}
