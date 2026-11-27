package main

import "testing"

// keys() joins a second time as soon as any earlier test called it. The
// alias names it adds to key_def share codes with canonical names, so an
// unrestricted re-inversion used to let the map order decide whether
// key_name[51] reads "COMMA" or "," - and the templates and usGlyphs only
// know the canonical spelling. Repeated calls must keep it canonical.
func TestKeysStaysCanonicalWhenCalledTwice(t *testing.T) {
	keys()
	keys()
	for code, want := range map[uint]string{
		12: "-", 13: "=", 26: "L_BRACE", 27: "R_BRACE", 39: "SEMICOLON",
		40: "APOSTROPHE", 41: "GRAVE", 43: "BACKSLASH", 51: "COMMA", 52: "DOT", 53: "SLASH",
	} {
		if got := key_name[code]; got != want {
			t.Errorf("key_name[%d] = %q, want canonical %q", code, got, want)
		}
	}
	// The aliases still reach key_def, which is what lets a config name keys
	// by the character they type.
	for _, alias := range []string{"MINUS", "[", ";", ","} {
		if _, ok := key_def[alias]; !ok {
			t.Errorf("alias %q missing from key_def after keys()", alias)
		}
	}
}
