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
