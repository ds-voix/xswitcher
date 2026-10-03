// Package main — see xswitcher.go for overview.
// autoreplace.go implements Punto-style auto text replacement: when a
// completed word matches a [[AutoReplace]] trigger, the text the application
// received is wiped and the replacement is typed in its place.
package main

import (
	"fmt"
	"strings"

	"github.com/holoplot/go-evdev"

	"xswitcher/scancodes"
)

var (
	autoExactRules   []TAutoReplace
	autoPartialRules []TAutoReplace
)

// buildAutoReplace splits the rules by matching mode once per config load, so
// the per-key path only walks the slice it needs.
func buildAutoReplace() {
	autoExactRules = nil
	autoPartialRules = nil
	for _, r := range AutoReplace {
		if r.Exact {
			autoExactRules = append(autoExactRules, r)
		} else {
			autoPartialRules = append(autoPartialRules, r)
		}
	}
}

// matchAutoReplace resolves a completed word to a replacement. Exact rules
// win over partial ones; within a mode the config order decides.
func matchAutoReplace(word string) (string, bool) {
	for _, r := range autoExactRules {
		if r.Trigger == word {
			return r.Replace, true
		}
	}
	for _, r := range autoPartialRules {
		if strings.HasSuffix(word, r.Trigger) {
			return r.Replace, true
		}
	}
	return "", false
}

// wipeText removes exactly the characters the application displays for the
// text, so the alignment never depends on which releases WordChars counts.
func wipeText(text string) {
	n := len([]rune(text))
	if n < 1 {
		return
	}
	if Keyboard.WipeBySelection {
		sendKey(t_key{uint16(key_def["L_SHIFT"]), 1})
		for i := 0; i < n; i++ {
			pressKey(evdev.KEY_LEFT)
		}
		sendKey(t_key{uint16(key_def["L_SHIFT"]), 0})
		pressKey(evdev.KEY_BACKSPACE)
	} else {
		for i := 0; i < n; i++ {
			pressKey(evdev.KEY_BACKSPACE)
		}
	}
}

// typeText injects arbitrary text through the virtual keyboard, the same
// scancode path the clipboard typing uses. Characters the keymap does not
// name are sent as KEY_RESERVED and land nowhere, like in typeClipboard.
func typeText(text string) {
	if text == "" {
		return
	}
	for _, cc := range strings.Split(scancodes.SequenceForString(text), " ") {
		c := strings.Split(cc, "+")
		if len(c) > 1 {
			sendKey(t_key{uint16(key_def["L_SHIFT"]), 1})
			sendKey(t_key{uint16(key_def[c[1]]), 1})
			sendKey(t_key{uint16(key_def[c[1]]), 0})
			sendKey(t_key{uint16(key_def["L_SHIFT"]), 0})
		} else {
			if cc == "Enter" {
				cc = "ENTER"
			}
			if cc == "Space" {
				cc = "SPACE"
			}
			sendKey(t_key{uint16(key_def[cc]), 1})
			sendKey(t_key{uint16(key_def[cc]), 0})
		}
	}
}

// tryAutoReplace runs on the NewWord completion, while WORD still holds the
// finished word, its separator and the first key of the next word (the
// matched EXTRA tail, already visible in the application). The whole visible
// text is wiped and replacement+tail is typed back as one piece.
func tryAutoReplace() bool {
	if len(AutoReplace) == 0 || COMPOSE > 0 || EXTRA < 0 || len(WORD)-EXTRA < 1 {
		return false
	}
	completed := bufferToText(WORD[:len(WORD)-EXTRA])
	visible := bufferToText(WORD)
	// The NewWord rules do not all cut at a word end (a BACKSPACE completion
	// already edited the text it reports). Replace only when the completed
	// text really is the head of what the application shows.
	if completed == "" || !strings.HasPrefix(visible, completed) {
		return false
	}
	replacement, ok := matchAutoReplace(completed)
	if !ok {
		return false
	}
	wipeText(visible)
	typeText(replacement + visible[len(completed):])
	WORD = nil
	sendSound("auto_replace")
	if *VERBOSE || *DEBUG {
		fmt.Printf("AutoReplace: %q >> %q\n", completed, replacement)
	}
	return true
}
