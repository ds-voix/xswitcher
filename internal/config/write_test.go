package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConf(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xswitcher.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The GUI must never be able to put the daemon into a state where the config
// on disk no longer parses: WriteTOMLChecked validates the marshalled result
// with ParseTOML before replacing the file.
func TestWriteTOMLCheckedRejectsInvalidMods(t *testing.T) {
	const orig = "# keep me\n[ScanDevices]\nTest = \"/dev/input/event0\"\n\n[Keyboard]\nDelay = 5\n"
	path := writeConf(t, orig)

	err := WriteTOMLChecked(path, map[string]interface{}{"Keyboard.Delay": "five"})
	if err == nil {
		t.Fatal("a string Delay must not pass validation")
	}
	if !strings.Contains(err.Error(), "not written") {
		t.Errorf("error should state the file was kept: %v", err)
	}
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(data) != orig {
		t.Errorf("rejected modification changed the file:\n%s", data)
	}
}

func TestWriteTOMLCheckedAppliesValidModsAndKeepsComments(t *testing.T) {
	const orig = "# keep me\n[ScanDevices]\nTest = \"/dev/input/event0\"\n"
	path := writeConf(t, orig)

	if err := WriteTOMLChecked(path, map[string]interface{}{"Keyboard.Delay": 9}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# keep me") {
		t.Errorf("comment lost:\n%s", data)
	}
	if !strings.Contains(string(data), "Delay = 9") {
		t.Errorf("modification not applied:\n%s", data)
	}
	// The result must be loadable by the daemon parser.
	if _, perr := ParseTOML(data); perr != nil {
		t.Errorf("written file does not parse: %v", perr)
	}
}

// The shipped config is a wall of commented-out examples; the line editor
// must replace live keys, append missing ones, and never touch comments.
func TestWriteTOMLLineEditor(t *testing.T) {
	const cfg = `# header comment
[ScanDevices]
# Test = "commented example"
Search = "/dev/input/event*"

[Wayland]
Layout0 = ["A", "B"]
Delay = 50
`
	path := writeConf(t, cfg)
	mods := map[string]interface{}{
		"ScanDevices.Test": "event7",
		"Wayland.Delay":    60,
		"Wayland.Layout0":  []string{"X"},
		"Keyboard.Delay":   5,
	}
	if err := WriteTOMLChecked(path, mods); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	for _, want := range []string{
		"# header comment",
		`# Test = "commented example"`,
		`Test = "event7"`,
		"Delay = 60",
		`Layout0 = ["X"]`,
		"[Keyboard]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Delay = 50") {
		t.Errorf("old value survived:\n%s", got)
	}

	parsed, perr := ParseTOML(data)
	if perr != nil {
		t.Fatalf("result does not parse: %v", perr)
	}
	if parsed.Wayland.Delay != 60 || parsed.Keyboard.Delay != 5 {
		t.Errorf("parsed values wrong: %+v %+v", parsed.Wayland, parsed.Keyboard)
	}
}

// Dotted paths must address [Action.Name] tables, whose headers carry trailing
// comments in the shipped config, and must be able to create a fresh table.
func TestWriteTOMLDottedSection(t *testing.T) {
	const cfg = `# top
[Action.RetypeWord] # Switch layout, drop last word and type it again
Layout = -1
Action = ["Clean", "Retype"]

[Actions]
SeqLength = 8
`
	path := writeConf(t, cfg)
	mods := map[string]interface{}{
		"Action.RetypeWord.Layout": int64(1),
		"Action.NewAct.Exec":       "echo hi",
		// Numbers arrive as float64 through the JSON wire; integral floats
		// must be written as TOML integers.
		"Action.NewAct.Timeout": float64(150),
	}
	if err := WriteTOMLChecked(path, mods); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	wants := []string{
		"# top",
		"[Action.RetypeWord] # Switch layout, drop last word and type it again",
		"Layout = 1",
		`Action = ["Clean", "Retype"]`,
		"[Action.NewAct]",
		`Exec = "echo hi"`,
		"Timeout = 150",
		"[Actions]",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Layout = -1") {
		t.Errorf("old value survived:\n%s", got)
	}
	if _, perr := ParseTOML(data); perr != nil {
		t.Errorf("result does not parse: %v", perr)
	}
}

// A single-segment path with an array-of-maps value rewrites every
// [[WindowClasses]] block in place: entry count and contents follow the GUI,
// everything outside those blocks is untouched.
func TestWriteTOMLArrayTableRewrite(t *testing.T) {
	const cfg = `# header
[[WindowClasses]] # terminal behaviour
Regex = "xterm"
MouseClickDrops = true
Actions = "Actions"

[Actions]
SeqLength = 8

[[WindowClasses]]
Regex = ".*"
Actions = "Actions"
`
	path := writeConf(t, cfg)
	// The shape the GUI sends after a JSON round trip: []interface{} of
	// map[string]interface{}. The parser's gate only accepts Actions "" or
	// "Actions" (parse.go: "only one action set \"Actions\" is realised").
	entries := []interface{}{
		map[string]interface{}{"Regex": "kitty", "MouseClickDrops": false, "Actions": "Actions"},
		map[string]interface{}{"Regex": ".", "Actions": ""},
		map[string]interface{}{"Regex": "chrome", "Actions": "Actions"},
	}
	if err := WriteTOMLChecked(path, map[string]interface{}{"WindowClasses": entries}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	if n := strings.Count(got, "[[WindowClasses]]"); n != 3 {
		t.Errorf("got %d WindowClasses blocks, want 3:\n%s", n, got)
	}
	for _, want := range []string{
		"# header",
		"[Actions]",
		"SeqLength = 8",
		`Regex = "kitty"`,
		"MouseClickDrops = false",
		`Regex = "chrome"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, gone := range []string{`Regex = "xterm"`, `Regex = ".*"`} {
		if strings.Contains(got, gone) {
			t.Errorf("replaced block survived %q:\n%s", gone, got)
		}
	}
	if _, perr := ParseTOML(data); perr != nil {
		t.Errorf("result does not parse: %v", perr)
	}
	// The blocks stay where the first one was: before [Actions].
	if strings.Index(got, "[[WindowClasses]]") > strings.Index(got, "[Actions]") {
		t.Errorf("array-of-tables moved past [Actions]:\n%s", got)
	}
}

// A lone-segment path with a non-table value (a scalar or a string array) has
// no unambiguous meaning for the line editor and must be rejected, not
// silently dropped.
func TestWriteTOMLRejectsBareSectionPath(t *testing.T) {
	path := writeConf(t, "[Keyboard]\nDelay = 5\n")
	err := WriteTOMLChecked(path, map[string]interface{}{"Keyboard": []interface{}{"a", "b"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported key path") {
		t.Errorf("want unsupported-path error, got: %v", err)
	}
}
