package main

import "testing"

// EXTRA is how many trailing events a matched SEQ tail protects, and RetypeWord wipes
// len(WORD) - EXTRA characters. testAction assigns it on every true return, so no path reads a
// leftover today; the point of the clear in doWindowActions is that the number belongs to the
// trigger being handled, not to whatever matched earlier. WC == nil returns right after that clear,
// which is the earliest exit the call has.
func TestDoWindowActionsOwnsExtra(t *testing.T) {
	WC = nil
	EXTRA = 3

	doWindowActions()

	if EXTRA != 0 {
		t.Fatalf("EXTRA = %d after doWindowActions(), want 0 - a tail count must not outlive its trigger", EXTRA)
	}
}
