package main

import "testing"

// RetypeWord() takes EXTRA -- how many trailing events belong to the shortcut itself -- from
// the length of the matched SEQ tail, so a tail that can match a varying number of key events
// silently changes how much of the word gets wiped.
func TestHasFreeQuantifier(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		want    bool
	}{
		{"(PAUSE:1,PAUSE:0)", false},                             // the shipped rule
		{"([LR]_CTRL:1,L_ALT:0,[0-9]:1)", false},                 // a class is not a repetition
		{"(?:PAUSE:1)", false},                                   // a group flag carries no length
		{"(.*PAUSE:1,PAUSE:0)", true},                            // eats the whole SeqLength window
		{"(,(@WORD@|@SEPARATOR@):[012])*PAUSE:1,PAUSE:0)", true}, // the word part is optional
		{"(PAUSE:1,PAUSE:0)?", true},
		{"(PAUSE:1{2},PAUSE:0)", true},
	} {
		if got := hasFreeQuantifier(tc.pattern); got != tc.want {
			t.Errorf("hasFreeQuantifier(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

// Only the chains that really run RetypeWord may be warned about. NewWord, Compose and
// TypeClipboard use quantifiers on purpose, and their rules must stay silent.
func TestChainHasAction(t *testing.T) {
	old := ActionSet
	defer func() { ActionSet = old }()
	ActionSet = actionSets(map[string][]string{
		"RetypeWord":   {"Action.CyclicSwitch", "RetypeWord"},
		"CyclicSwitch": {"Switch"},
		"Nested":       {"Action.RetypeWord"},
		"Plain":        {"Action.CyclicSwitch", "Layout"},
	})

	for name, want := range map[string]bool{
		"RetypeWord":   true,
		"Nested":       true, // reached through a reference, exactly as doAction() expands it
		"CyclicSwitch": false,
		"Plain":        false,
	} {
		if got := chainHasAction(name, "RetypeWord"); got != want {
			t.Errorf("chainHasAction(%q) = %v, want %v", name, got, want)
		}
	}
}
