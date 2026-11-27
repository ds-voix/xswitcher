// Package autodetect decides which language a run of keystrokes was meant to
// spell. It holds two letter-bigram models built at first use from the small
// corpora under corpus/ and scores alternative readings of the same physical
// keys against them.
//
// Nothing here knows about evdev codes, XKB groups or layouts: the daemon
// renders a buffer once per candidate group and hands the package the strings.
// That keeps the statistical model independent of the keyboard mapping, which
// is a daemon-side concern with its own shipped assumptions.
package autodetect

import (
	"embed"
	"math"
	"strings"
	"sync"
)

//go:embed corpus
var corpusFS embed.FS

// Language keys double as the corpus file names under corpus/.
const (
	EN = "en"
	RU = "ru"
)

// smoothing keeps a bigram the corpus never showed at a low but finite cost, so
// an unseen pair cannot make a reading infinitely worse than a seen one.
const smoothing = 0.5

// plausibilitySlack is how far below its own entropy a reading may cost and
// still count as language-like, in nats per bigram. The margin between two
// readings says which of them fits better; it cannot say whether either fits at
// all, and a run of keystrokes that is nonsense in both languages still has a
// better half. Without this veto a threshold would switch on that half.
const plausibilitySlack = 3.0

var corpusFiles = map[string]string{
	EN: "corpus/en.txt",
	RU: "corpus/ru.txt",
}

// alphabetSize is the number of letters each model can produce. It sizes the
// smoothing denominator: a smaller alphabet must not get a cheaper floor.
var alphabetSize = map[string]int{EN: 26, RU: 33}

// isLetter reports whether a lowercased r belongs to the language's alphabet.
// Russian counts ё as a letter of its own because the keyboard has a key for it,
// so a glyph mapping can produce it.
func isLetter(lang string, r rune) bool {
	switch lang {
	case EN:
		return r >= 'a' && r <= 'z'
	case RU:
		return r >= 'а' && r <= 'я' || r == 'ё'
	}
	return false
}

type bigram struct{ a, b rune }

type model struct {
	logp   map[bigram]float64
	unseen float64 // log probability of any pair the corpus never contained
	base   float64 // entropy of the model: what a fresh word of this language costs
}

// languageLike reports whether a reading costs about as much as this language's
// own text does. A keystroke run is not: it is far below, on both sides.
func (m *model) languageLike(cost float64) bool {
	return cost > m.base-plausibilitySlack
}

var (
	modelOnce sync.Once
	models    map[string]*model
)

// get returns the model for a language, building both on first call.
func get(lang string) *model {
	modelOnce.Do(load)
	return models[lang]
}

func load() {
	models = make(map[string]*model, len(corpusFiles))
	for lang, file := range corpusFiles {
		data, err := corpusFS.ReadFile(file)
		if err != nil {
			// An unreadable embedded corpus is a bug in the binary, not a
			// runtime condition: leaving the model out makes that language
			// unscorable, so no verdict is ever guessed from nothing.
			continue
		}
		models[lang] = build(lang, string(data))
	}
}

// build counts bigrams inside words of one alphabet. Any other character is a
// word boundary: pairs across a space are never counted, because the daemon
// scores runs of letters the same way.
func build(lang, text string) *model {
	counts := map[bigram]int{}
	pairs := 0
	var prev rune
	for _, r := range strings.ToLower(text) {
		if !isLetter(lang, r) {
			prev = 0
			continue
		}
		if prev != 0 {
			counts[bigram{prev, r}]++
			pairs++
		}
		prev = r
	}

	v := alphabetSize[lang] * alphabetSize[lang]
	denom := float64(pairs) + smoothing*float64(v)
	logp := make(map[bigram]float64, len(counts))
	for b, c := range counts {
		logp[b] = math.Log((float64(c) + smoothing) / denom)
	}
	unseen := math.Log(smoothing / denom)

	// base is the entropy of the model itself: the cost a fresh word of this
	// language is expected to carry. It is the reference a reading is measured
	// against when the package decides whether either side is language-like at
	// all, which is why it counts the smoothing mass of every empty bin too.
	entropy := 0.0
	for _, c := range counts {
		p := (float64(c) + smoothing) / denom
		entropy -= p * math.Log(p)
	}
	empty := float64(v-len(counts)) * (smoothing / denom)
	entropy -= empty * unseen

	return &model{logp: logp, unseen: unseen, base: -entropy}
}

// Score reports the mean log probability per bigram of text under one model:
// negative, and closer to zero the better the text fits. ok is false when the
// language's alphabet finds fewer than two letters to score, or when the model
// is missing.
func Score(lang, text string) (float64, bool) {
	m := get(lang)
	if m == nil {
		return 0, false
	}
	sum, scored := 0.0, 0
	var prev rune
	for _, r := range strings.ToLower(text) {
		if !isLetter(lang, r) {
			prev = 0
			continue
		}
		if prev != 0 {
			if p, ok := m.logp[bigram{prev, r}]; ok {
				sum += p
			} else {
				sum += m.unseen
			}
			scored++
		}
		prev = r
	}
	if scored < 1 {
		return 0, false
	}
	return sum / float64(scored), true
}

// Best scores every reading of the same keystrokes and returns the language of
// the strongest one with the confidence of that choice.
//
// Confidence is the relative drop in cost between the winner and the runner-up,
// (best - second) / -second, in [0,1). It reaches 1 when only one reading is
// scorable and 0 when the two read alike, which a threshold gate can use
// directly. ok is false when there is nothing to compare and when the winner
// itself costs far more than this language's own text does: a run that is
// nonsense in every reading gets no verdict instead of the lesser of two
// nonsense scores.
func Best(readings map[string]string) (string, float64, bool) {
	type scored struct {
		lang  string
		value float64
		ok    bool
	}
	list := make([]scored, 0, len(readings))
	for lang, text := range readings {
		v, ok := Score(lang, text)
		list = append(list, scored{lang, v, ok})
	}
	if len(list) < 2 {
		return "", 0, false
	}

	best, second := -1, -1
	for i, s := range list {
		if !s.ok {
			continue
		}
		switch {
		case best == -1 || s.value > list[best].value:
			second, best = best, i
		case second == -1 || s.value > list[second].value:
			second = i
		}
	}
	if best == -1 || !get(list[best].lang).languageLike(list[best].value) {
		return "", 0, false
	}
	if second == -1 {
		return list[best].lang, 1, true
	}
	confidence := (list[best].value - list[second].value) / -list[second].value
	if confidence < 0 {
		confidence = 0
	}
	return list[best].lang, confidence, true
}
