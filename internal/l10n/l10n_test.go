package l10n

import (
	"os"
	"testing"
	"testing/fstest"
)

// TestMain pins a clean locale environment before Init so the default-locale
// assertions do not depend on the host's LANG, then loads the embedded
// catalogues once for the whole package.
func TestMain(m *testing.M) {
	for _, k := range []string{"XSWITCHER_LANG", "LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		os.Unsetenv(k)
	}
	if err := Init(); err != nil {
		panic("l10n Init: " + err.Error())
	}
	os.Exit(m.Run())
}

func TestAvailableSeedsEnglishAndRussian(t *testing.T) {
	got := map[string]Descriptor{}
	for _, d := range Available() {
		got[d.Code] = d
	}
	en, ok := got["en"]
	if !ok || en.Name != "English" {
		t.Fatalf("english descriptor: %+v (present %v)", en, ok)
	}
	ru, ok := got["ru"]
	if !ok {
		t.Fatal("russian catalogue was not embedded")
	}
	// The picker shows the endonym plus the English name; both come from the
	// catalogue, not a Go map, which is what keeps drop-in languages working.
	if ru.Name != "Русский" || ru.English != "Russian" {
		t.Errorf("russian descriptor from catalogue: %+v", ru)
	}
}

func TestUnknownKeyReturnsEnglishLiteral(t *testing.T) {
	restore := SetForTest("ru")
	defer restore()
	// A key with no entry in any catalogue must fall back to the call-site
	// literal, and must do so even while a non-English locale is active.
	if got := X("no.such.key", "Fallback literal"); got != "Fallback literal" {
		t.Errorf("unknown key resolved to %q, want the English literal", got)
	}
}

func TestActiveLocaleSelectsCatalogue(t *testing.T) {
	restore := SetForTest("en")
	defer restore()
	if got := X("gui.tab.config", "Config"); got != "Config" {
		t.Errorf("en: %q", got)
	}
	SetForTest("ru")
	if got := X("gui.tab.config", "Config"); got != "Настройки" {
		t.Errorf("ru: %q", got)
	}
}

func TestTemplateSubstitution(t *testing.T) {
	restore := SetForTest("ru")
	defer restore()
	data := map[string]any{"Buffer": "ghbdtn", "Len": 6}
	got := X("preview.letters", `Buffer: "{{.Buffer}}" ({{.Len}} letters)`, data)
	if want := `Буфер: "ghbdtn" (6 букв)`; got != want {
		t.Errorf("ru template:\n got %q\nwant %q", got, want)
	}
}

func TestRussianPluralCategories(t *testing.T) {
	restore := SetForTest("ru")
	defer restore()
	// The CLDR Russian set is one/few/many; these counts are the classic
	// discriminators (1 vs 21 against 2-4, and the 11 teens against 1).
	cases := []struct {
		count int
		want  string
	}{
		{1, "1 запись"},
		{2, "2 записи"},
		{5, "5 записей"},
		{11, "11 записей"},
		{21, "21 запись"},
	}
	for _, c := range cases {
		got := XN("diary.entries.count", "{{.Count}} entries", c.count)
		if got != c.want {
			t.Errorf("count %d: got %q want %q", c.count, got, c.want)
		}
	}
}

func TestSetRejectsUnknownCode(t *testing.T) {
	restore := SetForTest("en")
	defer restore()
	if err := Set("zz"); err == nil {
		t.Fatal("Set accepted a code with no catalogue")
	}
	if err := Set("ru_RU"); err != nil {
		t.Errorf("Set rejected a loose but known code: %v", err)
	}
	if Current() != "ru" {
		t.Errorf("ru_RU canonicalized to %q, want ru", Current())
	}
}

func TestDefaultFollowsEnvThenBase(t *testing.T) {
	os.Setenv("XSWITCHER_LANG", "ru")
	defer os.Unsetenv("XSWITCHER_LANG")
	if got := Default(); got != "ru" {
		t.Errorf("Default with XSWITCHER_LANG=ru: %q", got)
	}
	os.Unsetenv("XSWITCHER_LANG")
	if got := Default(); got != "en" {
		t.Errorf("Default with no env and no matching OS locale: %q", got)
	}
}

// TestOSLocalePrecedence pins the go-locale resolution the resolver must match,
// so our app and Fyne's built-in labels cannot diverge on the same machine. The
// anchor case is LC_ALL=C with LANGUAGE=ru: NLS is off, LANGUAGE is ignored, and
// the answer is English — the mixed-UI bug a naive "check LANGUAGE first" loop
// would produce.
func TestOSLocalePrecedence(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"C disables NLS and ignores LANGUAGE", map[string]string{"LC_ALL": "C", "LANGUAGE": "ru"}, "en"},
		{"C disables NLS and ignores LANG", map[string]string{"LC_ALL": "C", "LANG": "ru"}, "en"},
		{"LANGUAGE overrides the LC_ALL slot", map[string]string{"LC_ALL": "en_US.UTF-8", "LANGUAGE": "ru"}, "ru"},
		{"LC_ALL beats LANG", map[string]string{"LC_ALL": "ru_RU.UTF-8", "LANG": "en_US.UTF-8"}, "ru"},
		{"LANG used when LC_ALL and LC_MESSAGES empty", map[string]string{"LANG": "ru_RU.UTF-8"}, "ru"},
		{"unknown locale falls back to base", map[string]string{"LC_ALL": "fr_FR.UTF-8"}, "en"},
		{"LANGUAGE colon list picks the first known", map[string]string{"LANGUAGE": "fr:ru"}, "ru"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withLocaleEnv(t, c.env, Default); got != c.want {
				t.Errorf("env %v: Default()=%q want %q", c.env, got, c.want)
			}
		})
	}
}

// withLocaleEnv clears every locale-relevant variable, applies the case's env,
// runs f, then restores the process environment so the table cannot leak.
func withLocaleEnv(t *testing.T, env map[string]string, f func() string) string {
	t.Helper()
	keys := []string{"XSWITCHER_LANG", "LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"}
	orig := map[string]string{}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			orig[k] = v
		}
		os.Unsetenv(k)
	}
	for k, v := range env {
		os.Setenv(k, v)
	}
	defer func() {
		for _, k := range keys {
			os.Unsetenv(k)
			if v, ok := orig[k]; ok {
				os.Setenv(k, v)
			}
		}
	}()
	return f()
}

// TestDropInLoadsANewLanguage proves the "add a file, get a language" promise:
// a catalogue for a locale that is neither embedded nor named in any Go map
// becomes selectable after a single Load, with no code change. The code must be
// a real locale (go-i18n parses the file name into a language tag; a made-up
// code like "xx" is not a language and would resolve to "und"), which is exactly
// the contract the README's "adding a language" section documents.
func TestDropInLoadsANewLanguage(t *testing.T) {
	fsys := fstest.MapFS{
		"de.json": {Data: []byte(`{"language.name":"Deutsch","gui.tab.config":"Konfiguration"}`)},
	}
	if err := Load(fsys, "de.json"); err != nil {
		t.Fatalf("drop-in load: %v", err)
	}
	restore := SetForTest("en")
	defer restore()
	if err := Set("de"); err != nil {
		t.Fatalf("set drop-in: %v", err)
	}
	if Current() != "de" {
		t.Fatalf("current after drop-in set: %q", Current())
	}
	if got := X("gui.tab.config", "Config"); got != "Konfiguration" {
		t.Errorf("drop-in translation: %q", got)
	}
	found := false
	for _, d := range Available() {
		if d.Code == "de" {
			found = true
		}
	}
	if !found {
		t.Error("drop-in language not listed by Available()")
	}
}
