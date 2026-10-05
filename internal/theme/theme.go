// Package theme provides custom Fyne themes loaded from JSON files.
// User themes live under ~/.config/xswitcher/themes/<name>.json and override
// any subset of the 28 ColorName + 19 SizeName keys; missing keys fall back
// to Fyne's built-in dark or light palette depending on the "variant" field.
package theme

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	fynetheme "fyne.io/fyne/v2/theme"
)

type jsonTheme struct {
	Variant string             `json:"variant"`
	Name    string             `json:"name"`
	Colors  map[string]string  `json:"colors"`
	Sizes   map[string]float32 `json:"sizes"`
}

type customTheme struct {
	base   fyne.Theme
	name   string
	colors map[fyne.ThemeColorName]color.Color
	sizes  map[fyne.ThemeSizeName]float32
}

func (t *customTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if c, ok := t.colors[n]; ok {
		return c
	}
	return t.base.Color(n, v)
}

func (t *customTheme) Font(s fyne.TextStyle) fyne.Resource {
	return t.base.Font(s)
}

func (t *customTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(n)
}

func (t *customTheme) Size(n fyne.ThemeSizeName) float32 {
	if s, ok := t.sizes[n]; ok {
		return s
	}
	return t.base.Size(n)
}

func parseHexColor(hex string) (color.Color, error) {
	hex = strings.TrimPrefix(hex, "#")
	var r, g, b, a uint8
	a = 255
	switch len(hex) {
	case 6:
		_, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
		return color.NRGBA{R: r, G: g, B: b, A: a}, err
	case 8:
		_, err := fmt.Sscanf(hex, "%02x%02x%02x%02x", &r, &g, &b, &a)
		return color.NRGBA{R: r, G: g, B: b, A: a}, err
	default:
		return nil, fmt.Errorf("invalid hex color: #%s", hex)
	}
}

func Load(name string) (fyne.Theme, error) {
	if name == "" || name == "system" {
		return fynetheme.DefaultTheme(), nil
	}
	if name == "dark" {
		return fynetheme.DarkTheme(), nil
	}
	if name == "light" {
		return fynetheme.LightTheme(), nil
	}

	// A name may contain dots ("nord.v2" is a file the GUI itself lists), but it
	// must not navigate: separators and a parent reference are rejected outright.
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("theme %q: invalid name", name)
	}
	dir := userThemesDir()
	path := filepath.Join(dir, name+".json")
	if abs, err := filepath.Abs(path); err != nil || !strings.HasPrefix(abs, dir+string(os.PathSeparator)) {
		return nil, fmt.Errorf("theme %q: path escapes themes directory", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("theme %q: %w", name, err)
	}

	var jt jsonTheme
	if err := json.Unmarshal(data, &jt); err != nil {
		return nil, fmt.Errorf("theme %q: %w", name, err)
	}

	var base fyne.Theme
	switch jt.Variant {
	case "light":
		base = fynetheme.LightTheme()
	default:
		base = fynetheme.DarkTheme()
	}

	ct := &customTheme{
		base:   base,
		name:   jt.Name,
		colors: make(map[fyne.ThemeColorName]color.Color, len(jt.Colors)),
		sizes:  make(map[fyne.ThemeSizeName]float32, len(jt.Sizes)),
	}

	for k, v := range jt.Colors {
		c, err := parseHexColor(v)
		if err != nil {
			return nil, fmt.Errorf("theme %q: color %q: %w", name, k, err)
		}
		ct.colors[fyne.ThemeColorName(k)] = c
	}
	for k, v := range jt.Sizes {
		ct.sizes[fyne.ThemeSizeName(k)] = v
	}

	return ct, nil
}

func List() []string {
	names := []string{"system", "dark", "light"}
	dir := userThemesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return names
}

func userThemesDir() string {
	if dir := os.Getenv("XSWITCHER_THEMES_DIR"); dir != "" {
		return dir
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "themes")
	}
	return filepath.Join(configDir, "xswitcher", "themes")
}
