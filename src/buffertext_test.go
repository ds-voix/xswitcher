package main

import "testing"

// bufferToText is what an external command receives through [Action.X] SendBuffer = "WORD", so the
// characters it spells out are the interface: the test drives it with the same event pairs the
// daemon collects.
func TestBufferToText(t *testing.T) {
	keys() // Inverts key_def[] into key_name[], which the renderer reads
	old := CTRL
	defer func() { CTRL = old }()

	// tap appends the press and the release of a key, the way evdev delivers a typed key.
	tap := func(buf t_keys, names ...string) t_keys {
		for _, n := range names {
			buf = append(buf, t_key{uint16(key_def[n]), 1}, t_key{uint16(key_def[n]), 0})
		}
		return buf
	}

	hello := tap(nil, "H", "E", "L", "L", "O")

	t.Run("plain letters are the unshifted glyphs", func(t *testing.T) {
		CTRL = map[string]bool{}
		if got := bufferToText(hello); got != "hello" {
			t.Errorf("bufferToText(HELLO) = %q, want %q", got, "hello")
		}
	})

	t.Run("a held Shift capitalizes", func(t *testing.T) {
		CTRL = map[string]bool{}
		buf := tap(t_keyset("L_SHIFT", 1), "H")
		buf = append(buf, t_key{uint16(key_def["L_SHIFT"]), 0})
		buf = tap(buf, "E", "L", "L", "O")
		if got := bufferToText(buf); got != "Hello" {
			t.Errorf("bufferToText(shifted HELLO) = %q, want %q", got, "Hello")
		}
	})

	t.Run("CapsLock state comes from the machine and from the buffer", func(t *testing.T) {
		buf := tap(nil, "CAPS")
		if got := bufferToText(buf); got != "" {
			t.Errorf("a CapsLock press produced %q, want no character", got)
		}
		CTRL = map[string]bool{}
		if got := bufferToText(tap(buf, "H", "I")); got != "HI" {
			t.Errorf("CapsLock toggled inside the buffer gave %q, want %q", got, "HI")
		}
		CTRL = map[string]bool{"CAPS": true}
		if got := bufferToText(hello); got != "HELLO" {
			t.Errorf("CapsLock held before the buffer gave %q, want %q", got, "HELLO")
		}
		// Shift over a locked CapsLock is how the layout itself types: lowercase.
		CTRL = map[string]bool{"CAPS": true}
		buf = append(t_keyset("L_SHIFT", 1), tap(nil, "H")...)
		buf = append(buf, t_key{uint16(key_def["L_SHIFT"]), 0})
		if got := bufferToText(buf); got != "h" {
			t.Errorf("Shift+CapsLock gave %q, want %q", got, "h")
		}
	})

	t.Run("digits and their shifted glyphs", func(t *testing.T) {
		CTRL = map[string]bool{}
		if got := bufferToText(tap(nil, "1", "2", "3")); got != "123" {
			t.Errorf("digits gave %q, want %q", got, "123")
		}
		buf := append(t_keyset("R_SHIFT", 1), tap(nil, "1", "9", "0")...)
		buf = append(buf, t_key{uint16(key_def["R_SHIFT"]), 0})
		if got := bufferToText(buf); got != "!()" { // "1"->"!", "9"->"(", "0"->")"
			t.Errorf("shifted digits gave %q, want %q", got, "!()")
		}
	})

	t.Run("punctuation follows the US rows", func(t *testing.T) {
		CTRL = map[string]bool{}
		buf := tap(nil, "-", "=", "L_BRACE", "SEMICOLON", "COMMA", "BACKSLASH")
		if got := bufferToText(buf); got != "-=[;,\\" {
			t.Errorf("unshifted punctuation gave %q, want %q", got, "-=[;,\\")
		}
		buf = append(t_keyset("L_SHIFT", 1), buf...)
		buf = append(buf, t_key{uint16(key_def["L_SHIFT"]), 0})
		if got := bufferToText(buf); got != "_+{:<|" {
			t.Errorf("shifted punctuation gave %q, want %q", got, "_+{:<|")
		}
		if got := bufferToText(tap(nil, "KP5", "KP0")); got != "50" {
			t.Errorf("keypad digits gave %q, want %q", got, "50")
		}
	})

	t.Run("space tab enter and backspace edit the text", func(t *testing.T) {
		CTRL = map[string]bool{}
		buf := tap(nil, "H", "I", "SPACE", "T", "TAB", "X", "ENTER")
		if got := bufferToText(buf); got != "hi t\tx\n" {
			t.Errorf("whitespace gave %q, want %q", got, "hi t\tx\n")
		}
		// A BackSpace inside a word is what the user typed, so the render deletes the character.
		buf = tap(nil, "H", "E", "L", "BACKSPACE", "L", "O")
		if got := bufferToText(buf); got != "helo" {
			t.Errorf("backspace gave %q, want %q", got, "helo")
		}
		buf = tap(nil, "BACKSPACE")
		if got := bufferToText(buf); got != "" {
			t.Errorf("a backspace on empty text gave %q, want it to stay empty", got)
		}
	})

	t.Run("keys that type nothing are skipped", func(t *testing.T) {
		CTRL = map[string]bool{}
		buf := tap(nil, "F1", "L_ALT", "L_CTRL", "PAUSE", "UP", "H", "ESC")
		if got := bufferToText(buf); got != "h" {
			t.Errorf("non-printing keys gave %q, want %q", got, "h")
		}
		// A repeated press (value 2) is one character, not a stream of them.
		buf = t_keys{{uint16(key_def["A"]), 1}, {uint16(key_def["A"]), 2}, {uint16(key_def["A"]), 2},
			{uint16(key_def["A"]), 0}}
		if got := bufferToText(buf); got != "a" {
			t.Errorf("a held key gave %q, want %q", got, "a")
		}
	})
}

// t_keyset builds the event of one key in a given state.
func t_keyset(name string, value int32) t_keys {
	return t_keys{t_key{uint16(key_def[name]), value}}
}
