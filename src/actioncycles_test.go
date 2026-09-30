package main

import (
	"fmt"
	"strings"
	"testing"
)

func actionSets(chains map[string][]string) map[string]TAction {
	set := make(map[string]TAction, len(chains))
	for name, actions := range chains {
		set[name] = TAction{Action: actions}
	}
	return set
}

// recoverMessage runs f and returns the panic text, or "" when f returned normally.
func recoverMessage(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprintf("%v", r)
		}
	}()
	f()
	return ""
}

// config() used to accept any chain of [Action.*] sections, while doAction() expanded
// them by unbounded recursion. "Action.Looping" pointing at itself was a valid config
// that killed the daemon with a stack overflow on the first keystroke matching its rule.
func TestCheckActionCyclesRejectsSelfReference(t *testing.T) {
	old := ActionSet
	defer func() { ActionSet = old }()
	ActionSet = actionSets(map[string][]string{"Looping": {"Action.Looping"}})

	msg := recoverMessage(checkActionCycles)
	if !strings.Contains(msg, "refers to itself") {
		t.Fatalf("self-reference accepted: %q", msg)
	}
	if !strings.Contains(msg, "Looping -> Looping") {
		t.Errorf("the cycle path is missing from the message: %q", msg)
	}
}

// Two sections pointing at each other are just as fatal as a direct self-reference, and
// the reported path has to name both of them.
func TestCheckActionCyclesRejectsMutualReference(t *testing.T) {
	old := ActionSet
	defer func() { ActionSet = old }()
	ActionSet = actionSets(map[string][]string{
		"A": {"Action.B"},
		"B": {"Action.C"},
		"C": {"Action.A"},
	})

	msg := recoverMessage(checkActionCycles)
	if !strings.Contains(msg, "A -> B -> C -> A") {
		t.Fatalf("mutual reference not reported with its path: %q", msg)
	}
}

// The path above is only a contract if it is stable. checkActionCycles() used to start its
// depth-first walk from an arbitrary section, because Go randomizes map iteration, so the
// same 3-cycle was reported in one of three rotations and the assertion could not decide
// what the daemon prints. Repetitions are what a single run cannot show.
func TestCheckActionCyclesReportsTheSamePathEveryRun(t *testing.T) {
	old := ActionSet
	defer func() { ActionSet = old }()
	ActionSet = actionSets(map[string][]string{
		"A": {"Action.B"},
		"B": {"Action.C"},
		"C": {"Action.A"},
	})

	const attempts = 20 // 20 random rotations all matching is p ~ 3e-10
	first := ""
	for i := 0; i < attempts; i++ {
		msg := recoverMessage(checkActionCycles)
		if first == "" {
			first = msg
		} else if msg != first {
			t.Fatalf("run %d reported %q, run 0 reported %q", i, msg, first)
		}
	}
}

// A chain, a diamond and the shape the shipped config actually uses must all pass:
// "RetypeWord" here is a leaf action function, not a section reference, and must not be
// mistaken for one just because it is mentioned by name.
func TestCheckActionCyclesAcceptsAcyclicConfigs(t *testing.T) {
	old := ActionSet
	defer func() { ActionSet = old }()

	for _, chains := range []map[string][]string{
		{"RetypeWord": {"Action.CyclicSwitch", "RetypeWord"}, "CyclicSwitch": {"Switch"}},
		{"A": {"Action.B", "Action.C"}, "B": {"Action.D"}, "C": {"Action.D"}, "D": {"Layout"}},
		{},
	} {
		ActionSet = actionSets(chains)
		if msg := recoverMessage(checkActionCycles); msg != "" {
			t.Errorf("acyclic %v rejected: %q", chains, msg)
		}
	}
}
