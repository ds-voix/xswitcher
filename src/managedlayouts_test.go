package main

import "testing"

// The gate that discards keys of an "extra language" has to know the groups the configuration can
// switch into, not only the global [ActionKeys] Layouts. It may only trust the layout fields of a
// section that names the leaf reading them: nothing in the parser applies the `default:"-1"` tag
// (no reflect anywhere in src/), so an unset Layout is Go's zero 0 and would otherwise hand group
// 0 to xswitcher in every config.
func TestCollectManagedLayouts(t *testing.T) {
	oldKeys, oldSet, oldManaged := ActionKeys, ActionSet, managedLayouts
	defer func() { ActionKeys, ActionSet, managedLayouts = oldKeys, oldSet, oldManaged }()

	ActionKeys.Layouts = []int{1}
	ActionSet = map[string]TAction{
		"CyclicSwitch": {Action: []string{"Switch"}, Layouts: []int{1, 2}}, // 2 comes only from here
		"Layout5":      {Action: []string{"Layout"}, Layout: 5},            // 5 only from here
		"Nested":       {Action: []string{"Action.Layout5"}},               // Layout unset -> 0
		"Respawn":      {Action: []string{"Respawn"}},                      // Layout unset -> 0
	}
	collectManagedLayouts()

	for _, tc := range []struct {
		group int
		want  bool
	}{
		{1, true},  // global list and both actions
		{2, true},  // named only by an action's Layouts
		{5, true},  // named only by an action's Layout
		{0, false}, // the zero value of an unset Layout in a section that never selects a layout
		{3, false}, // named nowhere: the "extra language (e.g., Chinese)" case
		{4, false},
	} {
		if got := managedLayouts[tc.group]; got != tc.want {
			t.Errorf("managedLayouts[%d] = %v, want %v", tc.group, got, tc.want)
		}
	}
	if len(managedLayouts) != 3 {
		t.Errorf("managedLayouts = %v, want exactly the three named groups", managedLayouts)
	}
}
