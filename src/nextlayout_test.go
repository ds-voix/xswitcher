package main

import "testing"

// Switch() stepped through [Action.X] Layouts with `next = l + 1` and then indexed the list with
// that group value, so the cycle only worked for a list starting at group 0.
func TestNextLayoutUsesPositionsNotGroupValues(t *testing.T) {
	for _, tc := range []struct {
		ref  int
		list []int
		want int
		why  string
	}{
		{0, []int{0, 1}, 1, "the shipped list, from its first group"},
		{1, []int{0, 1}, 0, "the shipped list, wrapping back"},
		{7, []int{0, 1}, 0, "a group the list does not name starts it over"},
		{1, []int{1, 2}, 2, "the old arithmetic stayed on group 1 here"},
		{2, []int{1, 2}, 1, "and the last entry of such a list never wrapped"},
		{0, []int{1, 2}, 1, "an unlisted group takes the first entry"},
		{2, []int{0, 1, 2}, 0, "three layouts still wrap to the first"},
		{1, []int{1}, 1, "a single layout cannot move"},
	} {
		if got := nextLayout(tc.ref, tc.list); got != tc.want {
			t.Errorf("nextLayout(%d, %v) = %d, want %d: %s", tc.ref, tc.list, got, tc.want, tc.why)
		}
	}
}
