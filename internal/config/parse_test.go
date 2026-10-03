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
