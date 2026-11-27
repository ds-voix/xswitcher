package main

import "testing"

// matchAutoReplace resolves exact rules before partial ones regardless of
// config order, and partial rules match word endings only.
func TestMatchAutoReplacePriority(t *testing.T) {
	orig := AutoReplace
	defer func() { AutoReplace = orig; buildAutoReplace() }()

	AutoReplace = []TAutoReplace{
		{Trigger: "w", Replace: "double-u"}, // partial, would also eat "btw"
		{Trigger: "btw", Replace: "by the way", Exact: true},
		{Trigger: "teh", Replace: "the"},
	}
	buildAutoReplace()

	if got, ok := matchAutoReplace("btw"); !ok || got != "by the way" {
		t.Errorf("exact rule must win over the partial one: %q %v", got, ok)
	}
	if got, ok := matchAutoReplace("foo teh"); !ok || got != "the" {
		t.Errorf("partial suffix match failed: %q %v", got, ok)
	}
	if _, ok := matchAutoReplace("the"); ok {
		t.Error("partial rule must not match inside the word")
	}
	if _, ok := matchAutoReplace("zzz"); ok {
		t.Error("unrelated word must not match")
	}
}

// The guards must refuse before any keyboard I/O: without rules, with a
// compose in flight, with an empty completion slice, and when the completed
// text is not the head of what the application shows (BACKSPACE completion).
func TestTryAutoReplaceGuards(t *testing.T) {
	keys() // Inverts key_def[] into key_name[], which bufferToText reads
	origRules, origWord, origExtra, origCompose := AutoReplace, WORD, EXTRA, COMPOSE
	defer func() {
		AutoReplace, WORD, EXTRA, COMPOSE = origRules, origWord, origExtra, origCompose
		buildAutoReplace()
	}()

	AutoReplace = []TAutoReplace{{Trigger: "btw", Replace: "by the way", Exact: true}}
	buildAutoReplace()
	COMPOSE = 0
	EXTRA = 0
	WORD = nil
	if tryAutoReplace() {
		t.Error("empty WORD buffer replaced something")
	}

	AutoReplace = nil
	WORD = t_keys{{uint16(key_def["B"]), 1}, {uint16(key_def["B"]), 0}}
	if tryAutoReplace() {
		t.Error("replacement ran without rules")
	}

	AutoReplace = []TAutoReplace{{Trigger: "btw", Replace: "by the way", Exact: true}}
	buildAutoReplace()
	COMPOSE = 1
	WORD = nil
	if tryAutoReplace() {
		t.Error("replacement ran over an in-flight compose sequence")
	}
	COMPOSE = 0

	// The BACKSPACE completion: the buffer text now says "bt" after the user
	// typed "btw" and shrank it - "bt" must not match the "btw" rule, and the
	// head guard must not reject the normal path either.
	WORD = t_keys{
		{uint16(key_def["B"]), 1}, {uint16(key_def["B"]), 0},
		{uint16(key_def["T"]), 1}, {uint16(key_def["T"]), 0},
		{uint16(key_def["W"]), 1}, {uint16(key_def["W"]), 0},
		{uint16(key_def["BACKSPACE"]), 1}, {uint16(key_def["BACKSPACE"]), 0},
	}
	EXTRA = 0
	if got := bufferToText(WORD); got != "bt" {
		t.Fatalf("bufferToText lost the BACKSPACE semantics: %q", got)
	}
	if tryAutoReplace() {
		t.Error("replacement ran on a word a BACKSPACE already shrank")
	}
}
