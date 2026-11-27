package main

import (
	"regexp"
	"testing"
)

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

// The flag has to survive parsing: only a RetypeWord chain with a variable-length tail is
// disabled, while the same tail in a chain that never retypes stays usable.
func TestUnprovableTailIsMarkedWhileParsing(t *testing.T) {
	oldActions, oldSet, oldSeq := Actions, ActionSet, ActSeq
	defer func() { Actions, ActionSet, ActSeq = oldActions, oldSet, oldSeq }()

	ActionSet = actionSets(map[string][]string{
		"RetypeWord": {"Action.CyclicSwitch", "RetypeWord"},
		"Plain":      {"Layout"},
	})
	Actions.Custom = map[string][]string{
		"RetypeWord": {"SEQ:(.*PAUSE:1,PAUSE:0)", "SEQ:(PAUSE:1,PAUSE:0)"},
		"Plain":      {"SEQ:(.*PAUSE:1,PAUSE:0)"},
	}
	sequences()

	for _, tc := range []struct {
		what string
		got  bool
		want bool
	}{
		{"RetypeWord rule with a free tail", ActSeq["RetypeWord"][0].tailUnprovable, true},
		{"RetypeWord rule with an exact tail", ActSeq["RetypeWord"][1].tailUnprovable, false},
		{"non-retyping rule with a free tail", ActSeq["Plain"][0].tailUnprovable, false},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: tailUnprovable = %v, want %v", tc.what, tc.got, tc.want)
		}
	}
}

// The guard testAction acts on. "H,E,O,H,L,PAUSE:1,PAUSE:0" is what the stand feeds through;
// every one of those keys is in Add[], so nothing subtracts itself from the match length and
// EXTRA really is the number of matched events.
func TestUnprovableTailNeverFires(t *testing.T) {
	oldTest, oldAdd, oldExtra := TEST, ADD, EXTRA
	defer func() { TEST, ADD, EXTRA = oldTest, oldAdd, oldExtra }()

	const stream = ",H:1,E:1,O:1,H:1,L:1,PAUSE:1,PAUSE:0"
	events := []t_key{{35, 1}, {18, 1}, {24, 1}, {35, 1}, {38, 1}, {119, 1}, {119, 0}} // evdev codes of the stream
	TEST = nil
	ADD = make(map[uint16]bool, len(events))
	for _, event := range events {
		TEST = append(TEST, event)
		ADD[event.code] = true
	}

	free := regexp.MustCompile("(.*PAUSE:1,PAUSE:0)$")
	exact := regexp.MustCompile("(PAUSE:1,PAUSE:0)$")

	run := func(seq TSequence) (bool, int) {
		TAIL.Reset()
		TAIL.WriteString(stream)
		EXTRA = -1
		fired := testAction(&TSequences{seq})
		return fired, EXTRA
	}

	// What the unfixed guard did: read seven events off the match and wipe the word minus them.
	if fired, extra := run(TSequence{SEQ: free}); !fired || extra != 7 {
		t.Errorf("an unmarked free tail: fired = %v EXTRA = %d, want true and 7", fired, extra)
	}
	if fired, extra := run(TSequence{SEQ: exact}); !fired || extra != 2 {
		t.Errorf("the shipped exact tail: fired = %v EXTRA = %d, want true and 2", fired, extra)
	}
	if fired, extra := run(TSequence{SEQ: free, tailUnprovable: true}); fired || extra != -1 {
		t.Errorf("a marked free tail: fired = %v EXTRA = %d, want false and EXTRA left alone", fired, extra)
	}
}
