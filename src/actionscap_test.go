package main

import (
	"testing"

	cfg "xswitcher/internal/config"
)

// The [Actions] capacity hint subtracts the 4 preset keys from the number of keys present. A config
// with fewer than 4 keys made that negative and `make` panicked ("len out of range") on what is
// otherwise a legal, if unusual, config. The hint must never go below zero, and must still account
// for the presets once there are enough keys.
func TestActionsCustomHintNeverNegative(t *testing.T) {
	cases := []struct{ n, want int }{
		{0, 0}, {1, 0}, {3, 0}, // fewer keys than presets: no negative cap
		{4, 0},          // exactly the presets: nothing custom to reserve for
		{5, 1}, {10, 6}, // custom keys beyond the four presets
	}
	for _, c := range cases {
		if got := cfg.ActionsCustomHint(c.n); got != c.want {
			t.Errorf("ActionsCustomHint(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}
