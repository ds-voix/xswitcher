package theme

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	fynetheme "fyne.io/fyne/v2/theme"
)

// The built-in palette is only readable once a Fyne app exists: builtinTheme.Color
// asks the current app which colour category to serve, and panics when there is none.
func withApp(t *testing.T) {
	t.Helper()
	test.NewApp()
}

func writeTheme(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XSWITCHER_THEMES_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadBuiltIns(t *testing.T) {
	for _, name := range []string{"", "system", "dark", "light"} {
		th, err := Load(name)
		if err != nil {
			t.Fatalf("Load(%q): %v", name, err)
		}
		if th == nil {
			t.Fatalf("Load(%q) returned a nil theme", name)
		}
	}
}

func TestCustomThemeOverridesAndFallsBack(t *testing.T) {
	withApp(t)
	writeTheme(t, "neon", `{"variant":"light","name":"Neon",`+
		`"colors":{"background":"#10203040","primary":"#ff0000"},`+
		`"sizes":{"padding":11}}`)

	th, err := Load("neon")
	if err != nil {
		t.Fatal(err)
	}

	got := th.Color(fynetheme.ColorNameBackground, fynetheme.VariantLight)
	want := color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0x40}
	if got != want {
		t.Errorf("background = %v, want %v", got, want)
	}
	if c := th.Color(fynetheme.ColorNamePrimary, fynetheme.VariantLight); c != (color.NRGBA{R: 255, A: 255}) {
		t.Errorf("primary = %v, want opaque red", c)
	}

	base := fynetheme.LightTheme()
	// An unset colour and an unset size must come from the variant's base palette.
	customForeground := th.Color(fynetheme.ColorNameForeground, fynetheme.VariantLight)
	baseForeground := base.Color(fynetheme.ColorNameForeground, fynetheme.VariantLight)
	if customForeground != baseForeground {
		t.Errorf("foreground fallback = %v, want %v", customForeground, baseForeground)
	}
	if got := th.Size(fynetheme.SizeNamePadding); got != 11 {
		t.Errorf("padding = %v, want 11", got)
	}
	customText := th.Size(fynetheme.SizeNameText)
	baseText := base.Size(fynetheme.SizeNameText)
	if customText != baseText {
		t.Errorf("text size fallback = %v, want %v", customText, baseText)
	}
}

func TestCustomThemeVariantDefaultsToDark(t *testing.T) {
	withApp(t)
	writeTheme(t, "plain", `{"name":"Plain","colors":{"background":"#010203"}}`)
	th, err := Load("plain")
	if err != nil {
		t.Fatal(err)
	}
	dark := fynetheme.DarkTheme()
	customForeground := th.Color(fynetheme.ColorNameForeground, fynetheme.VariantDark)
	darkForeground := dark.Color(fynetheme.ColorNameForeground, fynetheme.VariantDark)
	if customForeground != darkForeground {
		t.Errorf("missing variant fell back to light: foreground = %v, want dark %v", customForeground, darkForeground)
	}
}

func TestLoadRejectsBadJSON(t *testing.T) {
	writeTheme(t, "badhex", `{"colors":{"background":"#zzz"}}`)
	if _, err := Load("badhex"); err == nil || !strings.Contains(err.Error(), "color") {
		t.Fatalf("bad hex: err = %v, want a color error", err)
	}

	writeTheme(t, "notjson", `{oops`)
	if _, err := Load("notjson"); err == nil {
		t.Fatal("malformed JSON loaded without an error")
	}

	writeTheme(t, "absent", `{}`)
	if _, err := Load("no-such-theme"); err == nil {
		t.Fatal("a missing file loaded without an error")
	}
}

func TestLoadRejectsNamesThatNavigate(t *testing.T) {
	dir := writeTheme(t, "keep", `{"colors":{"background":"#010203ff"}}`)
	if err := os.WriteFile(filepath.Dir(dir)+"/outside.json", []byte(`{"colors":{"background":"#ff0000ff"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Dir(dir) + "/outside.json")

	for _, name := range []string{"../outside", "..", "a/b", `a\b`, "../.."} {
		if _, err := Load(name); err == nil {
			t.Errorf("Load(%q) accepted a name that leaves the themes directory", name)
		}
	}

	// A dot inside a name is legal: List() reports "nord.v2" for nord.v2.json,
	// so rejecting dots would offer a picker entry that can never load.
	writeTheme(t, "nord.v2", `{"colors":{"background":"#010203ff"}}`)
	if _, err := Load("nord.v2"); err != nil {
		t.Errorf("Load(%q): %v", "nord.v2", err)
	}
}

func TestListIncludesBuiltInsAndFiles(t *testing.T) {
	dir := writeTheme(t, "neon", `{"name":"Neon"}`)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	names := List()
	want := map[string]bool{"system": true, "dark": true, "light": true, "neon": true}
	for _, n := range names {
		delete(want, n)
	}
	if len(want) != 0 {
		t.Errorf("List() = %v, missing %v", names, want)
	}
	for _, n := range names {
		if n == "notes" || n == "sub" {
			t.Errorf("List() offered the non-theme entry %q", n)
		}
	}
}
