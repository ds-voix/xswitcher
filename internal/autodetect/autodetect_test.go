package autodetect

import (
	"strings"
	"testing"
	"time"
)

// shippedConfidence and shippedMinChars mirror the [AutoDetect] defaults the
// daemon ships with; the accuracy tests below measure the word lists against
// them, so changing one without the other is a lie in one of them.
const (
	shippedConfidence = 0.2
	shippedMinChars   = 5
)

// ruKeys maps a key of the US row onto the glyph the Russian layout puts in the
// same physical position (ЙЦУКЕН: q is й, a is ф, d is в). One word of a test
// case is enough to build both readings of the same keystrokes, so the table
// below holds real words in the language they were meant for and the other
// reading is derived here instead of being spelled by hand.
var ruKeys = map[rune]rune{
	'q': 'й', 'w': 'ц', 'e': 'у', 'r': 'к', 't': 'е', 'y': 'н', 'u': 'г', 'i': 'ш',
	'o': 'щ', 'p': 'з', '[': 'х', ']': 'ъ', '`': 'ё',
	'a': 'ф', 's': 'ы', 'd': 'в', 'f': 'а', 'g': 'п', 'h': 'р', 'j': 'о', 'k': 'л',
	'l': 'д', ';': 'ж', '\'': 'э',
	'z': 'я', 'x': 'ч', 'c': 'с', 'v': 'м', 'b': 'и', 'n': 'т', 'm': 'ь', ',': 'б',
	'.': 'ю',
}

var usKeys = func() map[rune]rune {
	m := make(map[rune]rune, len(ruKeys))
	for key, glyph := range ruKeys {
		m[glyph] = key
	}
	return m
}()

// ruOf renders latin keystrokes as the Russian layout spells the same positions.
func ruOf(latin string) string {
	return translate(latin, ruKeys)
}

// usOf renders a Russian word as the latin run of keystrokes that produced it.
func usOf(cyrillic string) string {
	return translate(cyrillic, usKeys)
}

func translate(text string, table map[rune]rune) string {
	out := make([]rune, 0, len(text))
	for _, r := range text {
		if c, ok := table[r]; ok {
			out = append(out, c)
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// readings builds the pair the daemon hands over for one keystroke run: what the
// typist sees, and what the other group would have spelled from the same keys.
func readings(latin string) map[string]string {
	return map[string]string{EN: latin, RU: ruOf(latin)}
}

// russianWords are Russian words typed with the layout left on US: the case
// Punto exists for, and the daemon sees only the latin keystroke run.
var russianWords = []string{
	"привет", "школа", "спасибо", "человек", "время", "вода", "сегодня",
	"работа", "город", "книга", "друг", "семья", "россия", "язык", "вопрос",
	"погода", "дерево", "мебель", "назад", "мать", "очень", "страна",
	"мужчина", "среда", "поехать", "улица", "площадь", "снег", "зима",
	"лето", "хлеб", "молоко", "дорога", "письмо", "номер", "окно",
}

// englishWords are latin keystroke runs a US-layout typist actually meant.
var englishWords = []string{
	"hello", "water", "people", "because", "should",
	"friend", "another", "language", "question", "morning",
	"evening", "history", "science", "market", "window",
	"bridge", "garden", "winter", "letter", "number",
	"street", "school", "money", "family", "answer",
}

// TestBestPrefersRussianReading and TestBestPrefersLatinReading split their
// word lists by whether the model saw the word during training: the corpora are
// small, so the words they contain prove only that the tables load, while the
// held-out ones are the generalisation the shipped threshold has to serve.
func classify(lang string, words []string) []outcome {
	out := make([]outcome, 0, len(words))
	for _, word := range words {
		latin, cyrillic := word, word
		if lang == RU {
			latin = usOf(word)
		} else {
			cyrillic = ruOf(word)
		}
		chosen, confidence, ok := Best(readings(latin))
		out = append(out, outcome{
			latin:      latin,
			glyphs:     cyrillic,
			want:       lang,
			got:        chosen,
			confidence: confidence,
			ok:         ok,
			heldOut:    !inCorpus(lang, word),
		})
	}
	return out
}

type outcome struct {
	latin, glyphs string
	want, got     string
	confidence    float64
	ok, heldOut   bool
}

// inCorpus reports whether the training text of a language contains a word.
func inCorpus(lang, word string) bool {
	data, err := corpusFS.ReadFile(corpusFiles[lang])
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(word))
}

func report(t *testing.T, runs []outcome) {
	t.Helper()
	missed, weak, heldWeak := 0, 0, 0
	for _, o := range runs {
		t.Logf("%-12s -> %-12s want %-2s got %-2s conf %.3f held-out %v",
			o.latin, o.glyphs, o.want, o.got, o.confidence, o.heldOut)
		if !o.ok || o.got != o.want {
			missed++
			continue
		}
		if o.confidence < shippedConfidence {
			weak++
			if o.heldOut {
				heldWeak++
			}
		}
	}
	t.Logf("verdict wrong or absent: %d; below the shipped %.2f: %d (held-out %d)",
		missed, shippedConfidence, weak, heldWeak)
	if missed > 0 {
		t.Errorf("%d of %d runs were not classified as %s", missed, len(runs), runs[0].want)
	}
}

func TestBestPrefersRussianReading(t *testing.T) {
	report(t, classify(RU, russianWords))
}

func TestBestPrefersLatinReading(t *testing.T) {
	report(t, classify(EN, englishWords))
}

// inconclusiveRuns are keystroke runs that spell no word in either language:
// keyboard patterns, held-down keys and finger slips. Confidence on these is
// what the config threshold has to reject, because switching on noise is the
// false positive a user feels as the daemon fighting their typing.
// TestInconclusiveRunsStayLow measures the other half of the operating point:
// keystroke runs that spell nothing in either language. Only runs of at least
// shippedMinChars are held to the bar, because that is the length the daemon
// evaluates at all - shorter noise ("длор", "цдщз") is exactly why the shipped
// MinChars default is 5, and a bigram model cannot tell those apart from a word
// without a dictionary behind it.
func TestInconclusiveRunsStayLow(t *testing.T) {
	noise := []string{"qwwz", "lkjh", "zxcv", "jjjj", "qqqq", "wlop", "hyuf", "kvqc",
		"poiuy", "mnbvc", "asdfg", "qwwzx", "hyufg", "wolqp", "kvqcp", "jkl;'", "xtenhf"}
	for _, latin := range noise {
		lang, confidence, ok := Best(readings(latin))
		if !ok {
			t.Logf("%-8s -> %-8s no verdict", latin, ruOf(latin))
			continue
		}
		t.Logf("%-8s -> %-8s chose %s at confidence %.3f", latin, ruOf(latin), lang, confidence)
		if len([]rune(ruOf(latin))) < shippedMinChars {
			continue
		}
		if confidence > shippedConfidence+0.05 {
			t.Errorf("%q (%q): noise reached confidence %.3f", latin, ruOf(latin), confidence)
		}
	}
}

func TestBestIgnoresCaseAndPunctuation(t *testing.T) {
	// Shift changes the glyph of both readings at once: a capital word is the
	// same keystroke run wearing different case in either language.
	lower, lowerConfidence, _ := Best(readings("ghbdtn"))
	caps, capsConfidence, _ := Best(map[string]string{EN: "Ghbdtn", RU: "Привет"})
	if lower != caps || lowerConfidence != capsConfidence {
		t.Errorf("case changed the verdict: %s %.3f vs %s %.3f",
			lower, lowerConfidence, caps, capsConfidence)
	}
	// A comma after the word is what the typist adds next, and one before it is
	// what the buffer keeps from the end of the previous sentence.
	marks, _, _ := Best(readings(",ghbdtn,"))
	if marks != lower {
		t.Errorf("punctuation changed the verdict: %q vs %q", marks, lower)
	}
}

func TestScoreOnlyCountsItsOwnAlphabet(t *testing.T) {
	// A Cyrillic run holds no English letters, so the English model has nothing
	// to say about it: that is what makes the two readings comparable at all.
	if _, ok := Score(EN, "привет"); ok {
		t.Error("Score(en, cyrillic) claimed to have scored no English letter")
	}
	if _, ok := Score(RU, "hello"); ok {
		t.Error("Score(ru, latin) claimed to have scored no Russian letter")
	}
	if _, ok := Score(RU, "а"); ok {
		t.Error("Score(ru, one letter) scored a run with no bigram")
	}
	cyrillic, ok := Score(RU, "привет")
	if !ok {
		t.Fatal("Score(ru, привет) failed")
	}
	garble, ok := Score(EN, "ghbdtn")
	if !ok {
		t.Fatal("Score(en, ghbdtn) failed")
	}
	if !(cyrillic > garble) {
		t.Errorf("real word %.3f should cost less than its garble %.3f", cyrillic, garble)
	}
}

func TestBestNeedsTwoReadings(t *testing.T) {
	if _, _, ok := Best(map[string]string{EN: "hello"}); ok {
		t.Error("Best accepted one reading: there was nothing to compare")
	}
	if _, _, ok := Best(map[string]string{}); ok {
		t.Error("Best accepted no readings")
	}
	// Neither side is scorable: digits and punctuation only.
	if _, _, ok := Best(map[string]string{EN: "12345", RU: "67890"}); ok {
		t.Error("Best returned a verdict for text with no letters")
	}
}

// TestUncrossedWordBoundaries guards the one asymmetry of a bigram model: pairs
// across a space are counted neither in the corpus nor when scoring, so a
// reading with punctuation must not be punished for it.
func TestUncrossedWordBoundaries(t *testing.T) {
	plain, _ := Score(RU, "приветмир")
	spaced, _ := Score(RU, "привет мир")
	// A joined run adds the pairs the language never uses across a boundary, so
	// its mean log probability must be the lower (worse) of the two.
	if !(plain < spaced) {
		t.Errorf("joining two words should cost more than separating them: %.3f vs %.3f", plain, spaced)
	}
}

// TestModelBuildCost measures the one-time cost of building both tables from
// the embedded corpora, which the daemon pays on the first completed word.
func TestModelBuildCost(t *testing.T) {
	start := time.Now()
	_, _, ok := Best(readings("ghbdtn"))
	elapsed := time.Since(start)
	if !ok {
		t.Fatal("no verdict")
	}
	t.Logf("model build + first score: %v", elapsed)
	if elapsed > 500*time.Millisecond {
		t.Errorf("model build took %v: too late to hide inside one key event", elapsed)
	}
}

func BenchmarkScore(b *testing.B) {
	latin := "ghbdtn"
	cyrillic := ruOf(latin)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Score(RU, cyrillic)
		_, _ = Score(EN, latin)
	}
}

func BenchmarkBest(b *testing.B) {
	pair := readings(usOf("человек"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = Best(pair)
	}
}

// TestCorpusShape keeps the corpora honest: an empty or single-alphabet file
// would silently make one language unscorable, and every test above would then
// pass on the other one.
func TestCorpusShape(t *testing.T) {
	modelOnce.Do(load)
	for _, lang := range []string{EN, RU} {
		m := models[lang]
		if m == nil {
			t.Fatalf("%s model missing: its corpus could not be read", lang)
		}
		if len(m.logp) < 100 {
			t.Errorf("%s model has %d bigrams: the corpus is too small to rank words", lang, len(m.logp))
		}
	}
	if got := ruOf("ghbdtn"); got != "привет" {
		t.Errorf("the ru keystroke map spells ghbdtn as %q, not привет", got)
	}
}
