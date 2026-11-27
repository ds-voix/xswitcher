package main

import (
	"reflect"
	"testing"
)

// The ActSeq firing loop mutates shared state (WORD/EXTRA/COMPOSE) once per matching rule and does
// not break, so the order rules fire in is observable. Range-over-map order is randomized per run;
// sortedSeqNames must return keys in a stable order so the same keystroke always produces the same
// response.
func TestSortedSeqNamesIsDeterministic(t *testing.T) {
	seq := map[string]TSequences{
		"Zulu":  nil,
		"Alpha": nil,
		"Mike":  nil,
		"bravo": nil,
	}
	want := []string{"Alpha", "Mike", "Zulu", "bravo"} // byte order: uppercase before lowercase
	if got := sortedSeqNames(seq); !reflect.DeepEqual(got, want) {
		t.Errorf("sortedSeqNames = %v, want %v", got, want)
	}

	if got := sortedSeqNames(map[string]TSequences{}); len(got) != 0 {
		t.Errorf("empty map should yield no names, got %v", got)
	}
}
