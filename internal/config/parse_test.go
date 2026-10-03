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
