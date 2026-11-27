package main

import (
	"testing"

	"xswitcher/internal/autodetect"
	cfg "xswitcher/internal/config"
)

// t_tap appends the press and the release of each named key, the way evdev
// delivers a typed key.
func t_tap(names ...string) t_keys {
	var buf t_keys
	for _, n := range names {
		code := uint16(key_def[n])
		buf = append(buf, t_key{code, 1}, t_key{code, 0})
	}
	return buf
}

// t_held puts a modifier down for the whole buffer and lifts it at the end, which
// is how a word typed with Shift reads off evdev.
func t_held(mod string, buf t_keys) t_keys {
	code := uint16(key_def[mod])
	held := make(t_keys, 0, len(buf)+2)
	held = append(held, t_key{code, 1})
	held = append(held, buf...)
	return append(held, t_key{code, 0})
}

// The Cyrillic reading is what the detector scores, so the position table is the
// interface: the canonical mistake (the word "привет" struck while the machine
// was still latin) has to come out of the six keys G H B D T N.
func TestRuGlyphSpellsTheCyrillicRow(t *testing.T) {
	keys() // Inverts key_def[] into key_name[], which renderBuffer reads
	old := CTRL
	defer func() { CTRL = old }()
	CTRL = map[string]bool{}

	word := t_tap("G", "H", "B", "D", "T", "N")
	if got := renderBuffer(word, ruGlyph); got != "привет" {
		t.Errorf("ruGlyph read %q, want %q", got, "привет")
	}
	if got := renderBuffer(word, usGlyph); got != "ghbdtn" {
		t.Errorf("usGlyph read the same keys as %q, want %q", got, "ghbdtn")
	}

	// Shift capitalizes and CapsLock does the same, exactly as in the latin row;
	// Shift over a locked CapsLock is how the layout itself types lowercase.
	if got := renderBuffer(t_held("L_SHIFT", t_tap("G")), ruGlyph); got != "П" {
		t.Errorf("shifted G gave %q, want %q", got, "П")
	}
	CTRL = map[string]bool{"CAPS": true}
	if got := renderBuffer(t_tap("G"), ruGlyph); got != "П" {
		t.Errorf("CapsLock G gave %q, want %q", got, "П")
	}
	if got := renderBuffer(t_held("R_SHIFT", t_tap("G")), ruGlyph); got != "п" {
		t.Errorf("Shift+CapsLock G gave %q, want %q", got, "п")
	}
	CTRL = map[string]bool{}

	// The symbol positions a Russian layout spells by convention are not rendered
	// here: a reading that mixed two alphabets would score the punctuation of one
	// language against the letters of the other. (COMMA and DOT are NOT symbols on
	// ЙЦУКЕН -- they are б and ю, and the table above renders them.)
	if got := renderBuffer(t_tap("1", "KP5", "-", "=", "A"), ruGlyph); got != "ф" {
		t.Errorf("ruGlyph read 1 KP5 - = A as %q, want only %q", got, "ф")
	}
	if got := renderBuffer(t_tap("1", "A"), usGlyph); got != "1a" {
		t.Errorf("usGlyph read 1 A as %q, want %q", got, "1a")
	}
	// The two positions the latin row has no letter for still read in Cyrillic:
	// GRAVE is ё and R_BRACE is ъ on ЙЦУКЕН.
	if got := renderBuffer(t_tap("GRAVE", "R_BRACE"), ruGlyph); got != "ёъ" {
		t.Errorf("GRAVE R_BRACE gave %q, want %q", got, "ёъ")
	}
}

// A language the parser accepts has to be one the daemon can both render and
// score, or buildAutoDetect would drop it and the config would silently manage
// fewer groups than it names.
func TestDetectSpellingsCoverConfiguredLanguages(t *testing.T) {
	for lang := range cfg.AutoDetectLanguages {
		glyph, ok := detectSpellings[lang]
		if !ok {
			t.Errorf("[AutoDetect] accepts %q but the daemon has no glyph table for it", lang)
			continue
		}
		r, spelled := glyph("A", false, false)
		if !spelled {
			t.Errorf("the table for %q spells no letter on a letter key", lang)
			continue
		}
		if _, ok := autodetect.Score(lang, string([]rune{r, r})); !ok {
			t.Errorf("%q has a glyph table but no model in internal/autodetect", lang)
		}
	}
}

// buildAutoDetect indexes the "<group>:<language>" entries in both directions and
// refuses an entry it cannot use instead of pointing a group at the wrong
// language. The parser rejects all of these shapes; the guard stays because a
// skipped entry only loses a group while a wrong one moves the keyboard.
func TestBuildAutoDetectIndexes(t *testing.T) {
	old := AutoDetect
	defer func() { AutoDetect = old; buildAutoDetect() }()

	AutoDetect = TAutoDetect{Enabled: true, Languages: []string{"1:ru", " 0 : en "}}
	buildAutoDetect()
	if got := detectGroupOf["ru"]; got != 1 {
		t.Errorf("detectGroupOf[ru] = %d, want 1", got)
	}
	if got := detectGroupOf["en"]; got != 0 {
		t.Errorf("detectGroupOf[en] = %d, want 0", got)
	}
	if got := detectLangOf[1]; got != "ru" {
		t.Errorf("detectLangOf[1] = %q, want ru", got)
	}
	if got := detectLangOf[0]; got != "en" {
		t.Errorf("detectLangOf[0] = %q, want en", got)
	}
	if len(detectGroupOf) != 2 || len(detectLangOf) != 2 {
		t.Errorf("indexed %v / %v, want the two declared groups both ways", detectGroupOf, detectLangOf)
	}

	AutoDetect = TAutoDetect{Enabled: true, Languages: []string{
		"ru",   // no group at all
		"x:en", // a group that is not a number
		"2:de", // a language with neither a model nor a glyph table
		"3:RU", // codes are case-sensitive: "RU" is not a known language
		"4:ru", // the only usable entry
	}}
	buildAutoDetect()
	if len(detectGroupOf) != 1 || detectGroupOf["ru"] != 4 {
		t.Errorf("usable entries = %v, want only ru in group 4", detectGroupOf)
	}
	if len(detectLangOf) != 1 || detectLangOf[4] != "ru" {
		t.Errorf("groups = %v, want only group 4 named", detectLangOf)
	}

	// Disabled means the daemon must not even consider the groups: the maps stay
	// empty so the word path has nothing to look up.
	AutoDetect = TAutoDetect{Enabled: false, Languages: []string{"0:en", "1:ru"}}
	buildAutoDetect()
	if len(detectGroupOf) != 0 || len(detectLangOf) != 0 {
		t.Errorf("a disabled [AutoDetect] still indexed %v", detectLangOf)
	}
}

// noteManualSwitch arms the grace only while the feature is on; spending it on
// one word is TestAutoDetectGraceCoversOneWord.
func TestNoteManualSwitch(t *testing.T) {
	oldDetect, oldSkip := AutoDetect, autoDetectSkip
	defer func() { AutoDetect, autoDetectSkip = oldDetect, oldSkip }()

	AutoDetect = TAutoDetect{Enabled: false}
	noteManualSwitch()
	if autoDetectSkip {
		t.Error("a disabled detector armed the grace")
	}

	AutoDetect = TAutoDetect{Enabled: true}
	noteManualSwitch()
	if !autoDetectSkip {
		t.Fatal("a manual switch did not arm the grace")
	}
}

// autoDetectWord is the whole decision, so the guards have to be observable
// without a keyboard: the reference case below has to ask for a correction first,
// or every refusal under it proves nothing.
func TestAutoDetectWordRefusals(t *testing.T) {
	keys()
	oldDetect, oldWord, oldExtra := AutoDetect, WORD, EXTRA
	oldCompose, oldLayout, oldSkip := COMPOSE, wordLayout, autoDetectSkip
	oldCTRL := CTRL
	defer func() {
		AutoDetect, WORD, EXTRA, COMPOSE, wordLayout, autoDetectSkip, CTRL =
			oldDetect, oldWord, oldExtra, oldCompose, oldLayout, oldSkip, oldCTRL
		buildAutoDetect()
	}()
	CTRL = map[string]bool{}
	autoDetectSkip = false
	COMPOSE = 0
	EXTRA = 0
	wordLayout = 0

	// "ghbdtn" is "привет" on the same keys: typed in group 0, it belongs to
	// group 1, so the setup itself is the correction case.
	AutoDetect = TAutoDetect{Enabled: true, MinChars: 5, Confidence: 0.2, Languages: []string{"0:en", "1:ru"}}
	buildAutoDetect()
	WORD = t_tap("G", "H", "B", "D", "T", "N")

	v, ok := autoDetectWord()
	if !ok {
		t.Fatal("the reference word did not ask for a correction: the refusals below would be vacuous")
	}
	if v.group != 1 || v.lang != "ru" || v.text != "привет" {
		t.Errorf("verdict = %+v, want group 1 / ru / %q", v, "привет")
	}
	if v.latin != "ghbdtn" {
		t.Errorf("the verdict kept latin = %q, want %q", v.latin, "ghbdtn")
	}
	if v.confidence <= AutoDetect.Confidence {
		t.Errorf("confidence %.3f did not clear the threshold", v.confidence)
	}

	refusals := []struct {
		name  string
		setup func()
	}{
		{"disabled", func() { AutoDetect.Enabled = false }},
		{"one language", func() {
			AutoDetect.Languages = []string{"1:ru"}
			buildAutoDetect()
		}},
		{"a group the config never named", func() { wordLayout = 3 }},
		{"no recorded group", func() { wordLayout = -1 }},
		{"compose in flight", func() { COMPOSE = 1 }},
		{"no completed text", func() { EXTRA = -1 }},
		{"empty buffer", func() { WORD = nil }},
	}
	for _, tc := range refusals {
		AutoDetect = TAutoDetect{Enabled: true, MinChars: 5, Confidence: 0.2, Languages: []string{"0:en", "1:ru"}}
		buildAutoDetect()
		COMPOSE, EXTRA, WORD, wordLayout, autoDetectSkip = 0, 0, t_tap("G", "H", "B", "D", "T", "N"), 0, false
		tc.setup()
		if _, ok := autoDetectWord(); ok {
			t.Errorf("%s: the guard did not refuse", tc.name)
		}
	}

	// The word is already in the language the model asks for: the group the
	// reading wins is the group the user is on, so there is nothing to switch to.
	AutoDetect = TAutoDetect{Enabled: true, MinChars: 5, Confidence: 0.2, Languages: []string{"0:en", "1:ru"}}
	buildAutoDetect()
	wordLayout = 1 // the same six keys typed on the russian group are the word they spell
	if _, ok := autoDetectWord(); ok {
		t.Error("a correctly typed russian word was called a mistake")
	}

	// The two brakes on false positives, each measured on the shipped models.
	// Confidence: "asdfg" spells "фывап", which the models do prefer over the
	// latin run, but only by 0.10.
	WORD = t_tap("A", "S", "D", "F", "G")
	wordLayout = 0
	if _, ok := autoDetectWord(); ok {
		t.Error("a 0.10-margin run cleared the shipped Confidence")
	}
	// MinChars: the four keys "lkjh" spell "длор" at 0.377 -- the same margin as
	// "привет" -- so only the length gate can tell the noise from the word.
	WORD = t_tap("L", "K", "J", "H")
	if _, ok := autoDetectWord(); ok {
		t.Error("a four-key run cleared the shipped MinChars")
	}
	AutoDetect.MinChars = 1
	if _, ok := autoDetectWord(); !ok {
		t.Error("MinChars is not what refused the four-key run: the pair above proves nothing")
	}
	AutoDetect.MinChars = 5

	// The reverse direction has to work as well: a latin word struck while the
	// machine was on the russian group ("hello" spells "руддощ" there).
	AutoDetect = TAutoDetect{Enabled: true, MinChars: 5, Confidence: 0.2, Languages: []string{"0:en", "1:ru"}}
	buildAutoDetect()
	WORD = t_tap("H", "E", "L", "L", "O")
	wordLayout = 1
	v, ok = autoDetectWord()
	if !ok {
		t.Fatal("a latin word typed on the russian group was left alone")
	}
	if v.group != 0 || v.lang != "en" || v.text != "hello" {
		t.Errorf("verdict = %+v, want group 0 / en / %q", v, "hello")
	}
}

// The grace flag is consumed by the first decision, not by the correction, so a
// skipped word still reports the refusal and the next one is judged again.
func TestAutoDetectGraceCoversOneWord(t *testing.T) {
	keys()
	oldDetect, oldWord := AutoDetect, WORD
	oldExtra, oldLayout := EXTRA, wordLayout
	oldSkip, oldCTRL := autoDetectSkip, CTRL
	defer func() {
		AutoDetect, WORD, EXTRA, wordLayout, autoDetectSkip, CTRL =
			oldDetect, oldWord, oldExtra, oldLayout, oldSkip, oldCTRL
		buildAutoDetect()
	}()
	CTRL = map[string]bool{}
	AutoDetect = TAutoDetect{Enabled: true, MinChars: 5, Confidence: 0.2, Languages: []string{"0:en", "1:ru"}}
	buildAutoDetect()
	WORD, EXTRA, COMPOSE, wordLayout = t_tap("G", "H", "B", "D", "T", "N"), 0, 0, 0

	autoDetectSkip = true
	if _, ok := autoDetectWord(); ok {
		t.Error("the word right after a manual switch was corrected anyway")
	}
	if _, ok := autoDetectWord(); !ok {
		t.Error("the second word was still under the grace")
	}
}
