package config

import (
	"strings"
	"testing"
)

func TestParseAutoReplaceAndHotkeys(t *testing.T) {
	const cfg = `
[[AutoReplace]]
Trigger = "btw"
Replace = "by the way"
Exact = true

[[AutoReplace]]
Trigger = "tehn"
Replace = "tech"

[[Hotkeys]]
Keys = ["L_CTRL", "L_ALT", "F1"]
Action = "Respawn"

[Action.Respawn]
Action = [ "Respawn" ]
`
	parsed, err := ParseTOML([]byte(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.AutoReplace) != 2 {
		t.Fatalf("AutoReplace rules: %d", len(parsed.AutoReplace))
	}
	if parsed.AutoReplace[0].Trigger != "btw" || !parsed.AutoReplace[0].Exact || parsed.AutoReplace[0].Replace != "by the way" {
		t.Errorf("first rule wrong: %+v", parsed.AutoReplace[0])
	}
	if parsed.AutoReplace[1].Exact {
		t.Errorf("missing Exact must default to false: %+v", parsed.AutoReplace[1])
	}
	if len(parsed.Hotkeys) != 1 {
		t.Fatalf("Hotkeys rules: %d", len(parsed.Hotkeys))
	}
	if parsed.Hotkeys[0].Action != "Respawn" || len(parsed.Hotkeys[0].Keys) != 3 {
		t.Errorf("hotkey wrong: %+v", parsed.Hotkeys[0])
	}
}

func TestParseAutoReplaceAndHotkeysRejectInvalid(t *testing.T) {
	const head = "[Action.Respawn]\nAction = [\"Respawn\"]\n\n"
	cases := []struct {
		name   string
		body   string
		wanted string
	}{
		{"empty trigger", "[[AutoReplace]]\nReplace = \"x\"\n", `non-empty "Trigger"`},
		{"empty replacement", "[[AutoReplace]]\nTrigger = \"x\"\n", `non-empty "Trigger" and "Replace"`},
		{"two triggers", "[[Hotkeys]]\nKeys = [\"F1\", \"F2\"]\nAction = \"Respawn\"\n", "exactly one non-modifier"},
		{"no trigger", "[[Hotkeys]]\nKeys = [\"L_CTRL\", \"CAPS\"]\nAction = \"Respawn\"\n", "exactly one non-modifier"},
		{"empty Action", "[[Hotkeys]]\nKeys = [\"F1\"]\n", `must name an [Action.*] section`},
		{"unknown Action", "[[Hotkeys]]\nKeys = [\"F1\"]\nAction = \"NoSuch\"\n", "action definition not found"},
		{"single table AutoReplace", "[AutoReplace]\nTrigger = \"x\"\nReplace = \"y\"\n", "must be a slice of sections"},
		{"single table Hotkeys", "[Hotkeys]\nKeys = [\"F1\"]\nAction = \"Respawn\"\n", "must be a slice of sections"},
	}
	for _, tc := range cases {
		_, err := ParseTOML([]byte(head + tc.body))
		if err == nil {
			t.Errorf("%s: accepted invalid config", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.wanted) {
			t.Errorf("%s: error %q does not mention %q", tc.name, err.Error(), tc.wanted)
		}
	}
}

func TestParseSoundsAndDiary(t *testing.T) {
	const cfg = `
[Sounds]
layout_switch = "/usr/share/sounds/sw.wav"
retype = "/usr/share/sounds/rt.wav"

[Diary]
Enabled = true
Path = "/var/log/xswitcher/diary.log"
`
	parsed, err := ParseTOML([]byte(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Sounds) != 2 || parsed.Sounds["layout_switch"] != "/usr/share/sounds/sw.wav" {
		t.Errorf("Sounds wrong: %+v", parsed.Sounds)
	}
	if !parsed.Diary.Enabled || parsed.Diary.Path != "/var/log/xswitcher/diary.log" {
		t.Errorf("Diary wrong: %+v", parsed.Diary)
	}
	if parsed.Diary.MaxSizeMB != 10 || parsed.Diary.RotateCount != 3 {
		t.Errorf("Diary defaults missing: %+v", parsed.Diary)
	}
}

func TestParseSoundsAndDiaryRejectInvalid(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		wanted string
	}{
		{"unknown sound event", "[Sounds]\nclicking = \"/x.wav\"\n", `unknown event "clicking"`},
		{"sound path not a string", "[Sounds]\nretype = 5\n", "must be a file path string"},
		{"sound empty path", "[Sounds]\nretype = \"\"\n", "must not be empty"},
		{"diary enabled without path", "[Diary]\nEnabled = true\n", "Path\" is required"},
		{"diary relative path", "[Diary]\nEnabled = true\nPath = \"diary.log\"\n", "must be absolute"},
		{"diary bad size", "[Diary]\nPath = \"/d\"\nMaxSizeMB = 0\n", "at least 1"},
		{"diary negative rotation", "[Diary]\nPath = \"/d\"\nRotateCount = -1\n", "must not be negative"},
	}
	for _, tc := range cases {
		_, err := ParseTOML([]byte(tc.body))
		if err == nil {
			t.Errorf("%s: accepted invalid config", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.wanted) {
			t.Errorf("%s: error %q does not mention %q", tc.name, err.Error(), tc.wanted)
		}
	}
}

func TestParseAutoDetect(t *testing.T) {
	parsed, err := ParseTOML([]byte("[AutoDetect]\nEnabled = true\nLanguages = [\"0:en\", \"1:ru\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	ad := parsed.AutoDetect
	if !ad.Enabled || len(ad.Languages) != 2 {
		t.Errorf("section not read: %+v", ad)
	}
	// The thresholds are the shipped brakes, so an absent key must not arrive as
	// Go's zero: 0 would detect every two-key run and accept any margin.
	if ad.MinChars != 5 {
		t.Errorf("MinChars default: %d", ad.MinChars)
	}
	if ad.Confidence != 0.2 {
		t.Errorf("Confidence default: %v", ad.Confidence)
	}

	parsed, err = ParseTOML([]byte("[AutoDetect]\nMinChars = 7\nConfidence = 0.45\nLanguages = [\"2:ru\", \"3:en\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.AutoDetect.MinChars != 7 || parsed.AutoDetect.Confidence != 0.45 {
		t.Errorf("explicit thresholds not kept: %+v", parsed.AutoDetect)
	}
	if parsed.AutoDetect.Enabled {
		t.Errorf("Enabled must stay off unless the config turns it on")
	}
	// Rules may be listed while the feature is off: that is how a config is
	// prepared before it is switched on, and nothing acts on them.
	parsed, err = ParseTOML([]byte("[AutoDetect]\nLanguages = [\"0:en\"]\n"))
	if err != nil {
		t.Fatalf("one entry with Enabled off: %v", err)
	}
	if len(parsed.AutoDetect.Languages) != 1 {
		t.Errorf("Languages not read: %+v", parsed.AutoDetect)
	}
}

func TestParseAutoDetectRejectsInvalid(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		wanted string
	}{
		{"entry without a group", "[AutoDetect]\nLanguages = [\"en\"]\n", `must be "<group>:<language>"`},
		{"group not an integer", "[AutoDetect]\nLanguages = [\"first:en\"]\n", "non-negative integer"},
		{"negative group", "[AutoDetect]\nLanguages = [\"-1:en\"]\n", "non-negative integer"},
		{"language with no model", "[AutoDetect]\nLanguages = [\"0:de\"]\n", `no model for "de"`},
		{"group named twice", "[AutoDetect]\nLanguages = [\"0:en\", \"0:ru\"]\n", "group 0 is named twice"},
		{"language named twice", "[AutoDetect]\nLanguages = [\"0:en\", \"1:en\"]\n", "language \"en\" is named twice"},
		{"enabled with one entry", "[AutoDetect]\nEnabled = true\nLanguages = [\"0:en\"]\n", "at least two Languages"},
		{"MinChars below the signal", "[AutoDetect]\nMinChars = 1\n", "at least 2"},
		{"Confidence zero", "[AutoDetect]\nConfidence = 0\n", "above 0"},
		{"Confidence above one", "[AutoDetect]\nConfidence = 1.5\n", "at most 1"},
		{"wayland bypass", "[AutoDetect]\nEnabled = true\nLanguages = [\"0:en\", \"1:ru\"]\n\n[Wayland]\nBypassX = true\n", "not observable"},
	}
	for _, tc := range cases {
		_, err := ParseTOML([]byte(tc.body))
		if err == nil {
			t.Errorf("%s: accepted invalid config", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.wanted) {
			t.Errorf("%s: error %q does not mention %q", tc.name, err.Error(), tc.wanted)
		}
	}
}

// TestDefaultAutoDetect pins the pair the GUI preview judges a word by, and it
// is worth two assertions because the mechanism has a trap measured during this
// phase: a document with no [AutoDetect] section at all parses to the zero
// struct, so an implementation that reached for ParseTOML(nil) would hand out
// MinChars 0 and Confidence 0 -- both brakes off -- and nothing else would fail.
func TestDefaultAutoDetect(t *testing.T) {
	def := DefaultAutoDetect()
	if def.MinChars != 5 || def.Confidence != 0.2 {
		t.Errorf("defaults: got MinChars %d Confidence %v, want 5 and 0.2", def.MinChars, def.Confidence)
	}
	// The same pair is what the daemon runs a present-but-keyless section with.
	parsed, err := ParseTOML([]byte("[AutoDetect]\nLanguages = [\"0:en\", \"1:ru\"]\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.AutoDetect.MinChars != def.MinChars || parsed.AutoDetect.Confidence != def.Confidence {
		t.Errorf("the section path gives MinChars %d Confidence %v, the tags give %d/%v",
			parsed.AutoDetect.MinChars, parsed.AutoDetect.Confidence, def.MinChars, def.Confidence)
	}
}

// A self-referential [Action.*] graph is valid TOML, so it used to pass the parser and
// kill the daemon later: doAction() expands "Action.xxx" with no depth limit, so the
// cycle overflows the stack on the first keystroke matching the rule -- far from boot and
// far from the config line that caused it. Rejecting it here rather than only inside the
// daemon's own boot is what lets all three users of this parser refuse the same file: the
// respawn gate (src configKillsChild), the GUI's validate command, and the config writer
// the GUI's Save goes through. Before this, a GUI user could write a cycle, be told the
// reload succeeded, and have no daemon at all a moment later.
func TestParseRejectsActionCycles(t *testing.T) {
	for _, tc := range []struct{ what, cfg, want string }{
		{"a section pointing at itself", `
[Action.Looping]
Action = [ "Action.Looping" ]
`, `"Action.Looping" refers to itself: Looping -> Looping`},
		{"two sections pointing at each other", `
[Action.A]
Action = [ "Action.B" ]

[Action.B]
Action = [ "Action.A" ]
`, `"Action.A" refers to itself: A -> B -> A`},
		{"a three-section ring", `
[Action.A]
Action = [ "Action.B" ]

[Action.B]
Action = [ "Action.C" ]

[Action.C]
Action = [ "Action.A" ]
`, `"Action.A" refers to itself: A -> B -> C -> A`},
	} {
		_, err := ParseTOML([]byte(tc.cfg))
		if err == nil {
			t.Errorf("%s: the cycle was accepted", tc.what)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q does not name the chain %q", tc.what, err.Error(), tc.want)
		}
	}
}

// The reported chain is only a contract if it is stable. Walking the map in Go's
// randomized order reported the same ring in one of its rotations, so the assertion could
// not decide what the daemon prints -- and neither could a user reading the log. Twenty
// repetitions are what a single run cannot show.
func TestActionCyclePathIsTheSameEveryRun(t *testing.T) {
	const ring = `
[Action.A]
Action = [ "Action.B" ]

[Action.B]
Action = [ "Action.C" ]

[Action.C]
Action = [ "Action.A" ]
`
	first := ""
	const attempts = 20 // 20 rotations all matching is p ~ 3e-10 for an unsorted walk
	for i := 0; i < attempts; i++ {
		_, err := ParseTOML([]byte(ring))
		if err == nil {
			t.Fatalf("run %d accepted the ring", i)
		}
		if first == "" {
			first = err.Error()
		} else if err.Error() != first {
			t.Fatalf("run %d reported %q, run 0 reported %q", i, err.Error(), first)
		}
	}
}

// The negative half: a diamond reaches one section by two paths and is not a cycle, and a
// chain naming a leaf action function ("RetypeWord") must not be mistaken for a section
// reference just because a section shares the name. Without these the cycle gate could
// look satisfied by a check that rejects every graph.
func TestParseAcceptsAcyclicActionGraphs(t *testing.T) {
	for _, cfg := range []string{
		``,
		`
[Action.RetypeWord]
Action = [ "Action.CyclicSwitch", "RetypeWord" ]

[Action.CyclicSwitch]
Action = [ "Switch" ]
`,
		`
[Action.A]
Action = [ "Action.B", "Action.C" ]

[Action.B]
Action = [ "Action.D" ]

[Action.C]
Action = [ "Action.D" ]

[Action.D]
Action = [ "Layout" ]
`,
	} {
		if _, err := ParseTOML([]byte(cfg)); err != nil {
			t.Errorf("acyclic graph rejected: %v", err)
		}
	}
}
