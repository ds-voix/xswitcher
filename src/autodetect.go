// Package main — see xswitcher.go for overview.
// autodetect.go runs the [AutoDetect] correction on a completed word: the same
// physical keys are spelled out in every language the config declares,
// internal/autodetect says which spelling the run fits best, and when that is
// not the layout the word was typed in, the daemon switches and retypes it.
package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"

	"xswitcher/internal/autodetect"
	"xswitcher/internal/l10n"
)

// ruGlyphs spells the canonical key positions of src/keys.go in Cyrillic, on the
// ЙЦУКЕН ordering the shipped Russian layout uses: the position keys.go calls Q
// types й, the one it calls W types ц, and so on. Index 0 is the unshifted glyph
// and index 1 the shifted one, like usGlyphs.
//
// Only the letters are listed. autodetect scores letters and treats anything
// else as a word boundary, so the punctuation rows cannot move a verdict, and
// leaving them out keeps this table from claiming to know how a Russian layout
// spells its symbols -- which it does not; there are several conventions and the
// daemon reads none of them.
var ruGlyphs = map[string]string{
	"Q": "йЙ", "W": "цЦ", "E": "уУ", "R": "кК", "T": "еЕ", "Y": "нН",
	"U": "гГ", "I": "шШ", "O": "щЩ", "P": "зЗ", "L_BRACE": "хХ", "R_BRACE": "ъЪ",
	"GRAVE": "ёЁ",
	"A":     "фФ", "S": "ыЫ", "D": "вВ", "F": "аА", "G": "пП", "H": "рР",
	"J": "оО", "K": "лЛ", "L": "дД", "SEMICOLON": "жЖ", "APOSTROPHE": "эЭ",
	"Z": "яЯ", "X": "чЧ", "C": "сС", "V": "мМ", "B": "иИ", "N": "тТ",
	"M": "ьЬ", "COMMA": "бБ", "DOT": "юЮ",
}

// ruGlyph is renderBuffer's Cyrillic table. A key this map does not name types
// no letter in this script, which is what the scoring needs to know: the digit
// and punctuation positions stay unrendered rather than borrowing the latin
// glyphs, so a reading never mixes two alphabets.
func ruGlyph(name string, shift, caps bool) (rune, bool) {
	glyph, ok := ruGlyphs[name]
	if !ok {
		return 0, false
	}
	runes := []rune(glyph)
	// As in the latin table, a letter is uppercase when exactly one of Shift and
	// CapsLock is in effect.
	if shift != caps {
		return runes[1], true
	}
	return runes[0], true
}

// detectSpellings is the rendering each language code is scored with. It has to
// cover the codes cfg.AutoDetectLanguages allows -- and only those, because a
// language with no model in internal/autodetect would score as nothing.
var detectSpellings = map[string]glyphFunc{
	autodetect.EN: usGlyph,
	autodetect.RU: ruGlyph,
}

var (
	// detectGroupOf and detectLangOf are [AutoDetect] Languages read two ways: the
	// word's own group has to be named to be compared against, and the language
	// that wins has to be turned back into a group to switch to.
	detectGroupOf map[string]int
	detectLangOf  map[int]string
	// autoDetectSkip is the one-word grace a manual switch gets, see
	// noteManualSwitch.
	autoDetectSkip bool
	// autoDetectFixed counts the corrections since the daemon started; the IPC
	// status reports it, and the key path writes it from the event goroutine, so
	// it is atomic.
	autoDetectFixed int64
)

// buildAutoDetect indexes the Languages entries for the word-completion path.
// It runs on every config load, so the hot path only reads maps.
func buildAutoDetect() {
	detectGroupOf = make(map[string]int, len(AutoDetect.Languages))
	detectLangOf = make(map[int]string, len(AutoDetect.Languages))
	if !AutoDetect.Enabled {
		return
	}
	for _, item := range AutoDetect.Languages {
		sep := strings.Index(item, ":")
		if sep < 0 {
			fmt.Printf("%s\n", l10n.X("daemon.autodetect_bad_entry", "AutoDetect warning: entry {{.Item}} is not \"<group>:<language>\"", "Item", item))
			continue
		}
		group, err := strconv.Atoi(strings.TrimSpace(item[:sep]))
		if err != nil {
			fmt.Printf("%s\n", l10n.X("daemon.autodetect_bad_group", "AutoDetect warning: entry {{.Item}} names no usable group", "Item", item))
			continue
		}
		lang := strings.TrimSpace(item[sep+1:])
		if _, ok := detectSpellings[lang]; !ok {
			fmt.Printf("%s\n", l10n.X("daemon.autodetect_no_glyph_table", "AutoDetect warning: entry {{.Item}} names a language with no glyph table", "Item", item))
			continue
		}
		detectGroupOf[lang] = group
		detectLangOf[group] = lang
	}
}

// detectReadings spells the buffer the user is filling right now in every
// language [AutoDetect] declares, for the GUI's preview. It is the same
// renderBuffer call autoDetectWord makes on a completed word, run over the
// whole buffer instead because a preview wants the word in progress rather than
// the last finished one.
//
// The status snapshot calls it on the key path, so the guard matters: a
// detection the config switched off must not cost a buffer walk per keystroke.
// The two readings of the same keys are not a second copy of what the status
// already publishes either -- WordBuffer is one rendering of the same physical
// presses, and this names what the other renderings look like.
func detectReadings() map[string]string {
	if !AutoDetect.Enabled || len(detectGroupOf) < 2 {
		return nil
	}
	readings := make(map[string]string, len(detectGroupOf))
	for lang := range detectGroupOf {
		readings[lang] = renderBuffer(WORD, detectSpellings[lang])
	}
	return readings
}

// noteManualSwitch arms the grace a hand-driven layout change gets. Switch and
// Layout are what the user's own keys and the config's SEQ rules run when they
// take the layout into their own hands; the word typed right after that is what
// they asked for, so detection skips it and judges the next one again. The
// correction itself calls Language() directly and is not a manual switch.
func noteManualSwitch() {
	if AutoDetect.Enabled {
		autoDetectSkip = true
	}
}

// autoDetectVerdict is the decision taken about one completed word: the group it
// should be retyped in, plus the readings the decision came from, so that the
// verbose log can say what the daemon saw.
type autoDetectVerdict struct {
	group      int
	lang       string
	confidence float64
	latin      string // the word as the latin positions spell it
	text       string // the winning reading: what the replay should produce
}

// autoDetectWord runs the guards and the scoring for the word WORD has just
// completed. tryAutoReplace's neighbour: it reads the same completion, while WORD
// still holds the finished word and the tail of the next one.
//
// The decision is kept apart from the keys it eventually moves because that is
// the half a test can drive: the same buffer state the live run reaches has no
// X server here, and a feature that can take the keyboard over has to have its
// refusals checked, not only its corrections.
//
// Two things keep it out of the way of a fired [[AutoReplace]] rule and of the
// rules it shares the buffer with: COMPOSE counts an unfinished compose sequence,
// and EXTRA < 0 or a completion slice shorter than one event means there is no
// completed text to score at all. A replacement that already ran leaves WORD
// empty, so this returns false on the length guard without needing to know why.
func autoDetectWord() (autoDetectVerdict, bool) {
	if !AutoDetect.Enabled || COMPOSE > 0 || EXTRA < 0 || len(WORD)-EXTRA < 1 {
		return autoDetectVerdict{}, false
	}
	if len(detectGroupOf) < 2 {
		return autoDetectVerdict{}, false // nothing to correct to: one declared language scores alone
	}
	if autoDetectSkip {
		autoDetectSkip = false
		return autoDetectVerdict{}, false
	}
	// wordLayout is the group the last word character was produced in, and
	// detectLangOf only names the groups the config declared. -1 (no character
	// recorded yet) and a group the config never named both fall out here: without
	// a declared language for the word there is no baseline to compare against.
	if _, ok := detectLangOf[wordLayout]; !ok {
		return autoDetectVerdict{}, false
	}

	completed := WORD[:len(WORD)-EXTRA]
	latin := bufferToText(completed)
	letters := 0
	for _, r := range latin {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	// MinChars counts the completed word, not the whole buffer, because it is the
	// only brake that separates a short run from a real word: measured on the
	// shipped models, the four keys "lkjh" spell "длор" at confidence 0.377 while
	// "ghbdtn" spells "привет" at 0.379, so the margin says nothing about length.
	if letters < AutoDetect.MinChars {
		return autoDetectVerdict{}, false
	}

	readings := make(map[string]string, len(detectGroupOf))
	for lang := range detectGroupOf {
		readings[lang] = renderBuffer(completed, detectSpellings[lang])
	}
	lang, confidence, ok := autodetect.Best(readings)
	if !ok || confidence < AutoDetect.Confidence {
		return autoDetectVerdict{}, false
	}
	group := detectGroupOf[lang]
	if group == wordLayout {
		// The word is already in the group the model asks for: nothing to correct.
		return autoDetectVerdict{}, false
	}
	return autoDetectVerdict{
		group: group, lang: lang, confidence: confidence, latin: latin, text: readings[lang],
	}, true
}

// tryAutoDetect applies the correction autoDetectWord decided on. The replay
// sends the scancodes the buffer already holds; the server translates them with
// the group just selected, which is what puts the word on screen in the script
// the model picked.
func tryAutoDetect() bool {
	verdict, ok := autoDetectWord()
	if !ok {
		return false
	}
	from := wordLayout
	Language(verdict.group)
	RetypeWord(&TAction{})
	atomic.AddInt64(&autoDetectFixed, 1)
	if *VERBOSE || *DEBUG {
		fmt.Printf("AutoDetect: %q typed in group %d >> %q in %s(%d), confidence %.2f\n",
			verdict.latin, from, verdict.text, verdict.lang, verdict.group, verdict.confidence)
	}
	return true
}
