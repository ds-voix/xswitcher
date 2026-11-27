// Package main — see main.go for overview.
// autodetect_editor.go edits [AutoDetect]: the detector that retypes a word
// finished in the wrong layout. The scoring lives in internal/autodetect and the
// decision in the daemon (src/autodetect.go); this form writes the four settings
// the daemon reads through the same dotted-key mods the Apply button sends to the
// daemon's validated writer, and previews a verdict.
package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"xswitcher/internal/autodetect"
	cfg "xswitcher/internal/config"
	"xswitcher/internal/ipc"
	"xswitcher/internal/l10n"
)

type autodetectEditor struct {
	client     *ipc.Client
	enabled    *widget.Check
	minChars   *widget.Entry
	conf       *widget.Entry
	languages  *widget.Entry
	note       *widget.Label
	preview    *widget.Entry
	previewBtn *widget.Button
}

func newAutodetectEditor(client *ipc.Client) (fyne.CanvasObject, *autodetectEditor) {
	e := &autodetectEditor{client: client}
	e.enabled = widget.NewCheck(l10n.X("autodetect.enabled_label", "Retype a word that was finished in the wrong layout"), nil)
	e.minChars = widget.NewEntry()
	e.minChars.SetPlaceHolder("5")
	e.conf = widget.NewEntry()
	e.conf.SetPlaceHolder("0.2")
	e.languages = widget.NewMultiLineEntry()
	e.languages.SetPlaceHolder("0:en\n1:ru")
	e.note = widget.NewLabel("")
	e.note.Wrapping = fyne.TextWrapWord

	form := widget.NewForm(
		widget.NewFormItem("", e.enabled),
		widget.NewFormItem(configKeyLabel("MinChars"), e.minChars),
		widget.NewFormItem(configKeyLabel("Confidence"), e.conf),
		widget.NewFormItem(configKeyLabel("Languages"), e.languages),
	)

	codes := make([]string, 0, len(cfg.AutoDetectLanguages))
	for code := range cfg.AutoDetectLanguages {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	hint := widget.NewLabel(l10n.X("autodetect.languages_hint",
		`Languages holds one entry per line as "<group>:<language>", where group is the 0-based XKB group to switch to and language is one of: {{.Value}}. MinChars is the shortest run of letters worth judging and Confidence the margin between the two best readings the daemon must see before it touches the keyboard.`,
		"Value", strings.Join(codes, ", ")))
	hint.Wrapping = fyne.TextWrapWord

	// The preview asks the daemon for the buffer it is holding now and scores the
	// readings against the numbers in this form, so it answers "what would
	// happen if I applied this" before anything is written.
	e.preview = widget.NewMultiLineEntry()
	e.preview.Wrapping = fyne.TextWrapWord
	e.preview.Disable()
	e.previewBtn = widget.NewButton(l10n.X("autodetect.preview_button", "Preview current buffer"), e.previewNow)
	previewHint := widget.NewLabel(l10n.X("autodetect.preview_hint",
		"The daemon judges a word when it is completed; the preview judges the whole buffer as it stands, so a buffer that already holds the first keys of the next word is scored one word longer here. The daemon also refuses a word while a compose sequence is in progress and skips the one word right after a layout you switched by hand; neither state is in the status, so the preview cannot show those refusals."))
	previewHint.Wrapping = fyne.TextWrapWord

	return container.NewVScroll(container.NewVBox(
		widget.NewCard(l10n.X("autodetect.settings_card", "[AutoDetect] settings"), "", form),
		hint,
		e.note,
		widget.NewCard(l10n.X("autodetect.preview_card", "Preview"), "", container.NewVBox(e.previewBtn, previewHint, e.preview)),
	)), e
}

// load fills the form from get_config data. The [Wayland] BypassX flag is read
// here because the parser refuses Enabled together with it: the daemon cannot
// observe a layout it did not change itself, so detecting from its own cache
// would double the switch the desktop already made.
func (e *autodetectEditor) load(data map[string]any) {
	ad := section(data, "AutoDetect")
	e.enabled.SetChecked(mapBool(ad, "Enabled"))
	e.minChars.SetText(mapNum(ad, "MinChars"))
	e.conf.SetText(mapNum(ad, "Confidence"))
	e.languages.SetText(mapList(ad, "Languages"))

	e.note.SetText("")
	if mapBool(section(data, "Wayland"), "BypassX") {
		e.note.SetText(l10n.X("autodetect.bypassx_note",
			"[Wayland] BypassX is on: the daemon cannot see a layout change it did not make itself, so the parser refuses Enabled = true while that flag stays on. Clear it on the Basics tab first."))
	}
}

// mods returns the dotted [AutoDetect] keys. Enabled is always sent (a check
// has no empty state); a number that does not parse is sent as text so the
// daemon answers with its own gate message instead of the form dropping the
// edit silently. WriteTOMLChecked validates before it replaces the file, so a
// bad value never reaches the configuration.
func (e *autodetectEditor) mods() map[string]any {
	mods := map[string]any{"AutoDetect.Enabled": e.enabled.Checked}
	setNum := func(key, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			mods[key] = f
			return
		}
		mods[key] = text
	}
	setNum("AutoDetect.MinChars", e.minChars.Text)
	setNum("AutoDetect.Confidence", e.conf.Text)
	if lines := splitNonEmpty(e.languages.Text); len(lines) > 0 {
		mods["AutoDetect.Languages"] = lines
	}
	return mods
}

// thresholds reads the two brakes off the form, falling back to the config
// defaults where a field is blank. The defaults come from the struct tags
// through cfg.DefaultAutoDetect rather than from numbers copied here: the file
// the daemon runs may name no [AutoDetect] keys at all, and a preview built on
// 0/0 would say "would be corrected" about any keystroke run.
func (e *autodetectEditor) thresholds() (int, float64) {
	def := cfg.DefaultAutoDetect()
	minChars, confidence := def.MinChars, def.Confidence
	if n, err := strconv.Atoi(strings.TrimSpace(e.minChars.Text)); err == nil {
		minChars = n
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(e.conf.Text), 64); err == nil {
		confidence = f
	}
	return minChars, confidence
}

// languageGroups indexes the Languages list as the form holds it: which XKB group
// a winning language would switch to. An entry the parser would refuse -- no
// colon, a group that is not a number -- is skipped rather than guessed at, which
// is what the daemon's own buildAutoDetect does with the same text.
func languageGroups(text string) map[string]int {
	groups := map[string]int{}
	for _, line := range splitNonEmpty(text) {
		group, lang, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(group))
		if err != nil || n < 0 {
			continue // the parser refuses a negative group; a preview must not invent one
		}
		groups[strings.TrimSpace(lang)] = n
	}
	return groups
}

// previewInput is everything the report needs: the live state from the daemon and
// the numbers as the form holds them, which is what lets the preview answer about
// an edit the user has not applied yet.
type previewInput struct {
	Buffer    string            // the US rendering of the buffer, as the status reports it
	Readings  map[string]string // the same keys spelled in each declared language
	WordGroup int               // the group the word was typed in, as the daemon reports it; -1 for none
	MinChars  int
	Threshold float64
	Groups    map[string]int
}

// previewReport lists the readings, the model's verdict over them and which guard
// the case falls to. It is a plain function of plain values because it mirrors
// src/autodetect.go's decision -- the same brakes, in the same order, each one
// answering about the same quantity the daemon reads -- and that order is the part
// worth pinning down without an X server, a daemon or a widget in sight.
//
// Two differences are structural and stay in the hint the user reads: the daemon
// judges a completed word while the preview judges the whole live buffer, and the
// daemon's transient gates (a compose sequence in progress, the one-word grace
// after a hand-driven switch) are not states the status publishes.
func previewReport(in previewInput) string {
	if len(in.Readings) < 2 {
		return l10n.X("autodetect.no_readings",
			"Buffer: \"{{.Buffer}}\"\n\nThe daemon sent no readings: [AutoDetect] is switched off in the running config or names one language. Enable it, Apply, then preview again.",
			"Buffer", in.Buffer)
	}

	// Letters are counted over the buffer the daemon renders, which is the same
	// physical presses the daemon measures; the daemon measures them on the
	// finished word rather than on the tail still being typed.
	letters := 0
	for _, r := range in.Buffer {
		if unicode.IsLetter(r) {
			letters++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", l10n.X("preview.letters", `Buffer: "{{.Buffer}}" ({{.Len}} letters)`, "Buffer", in.Buffer, "Len", letters))
	codes := make([]string, 0, len(in.Readings))
	for code := range in.Readings {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		fmt.Fprintf(&b, "  %-4s %s\n", code, in.Readings[code])
	}

	// First of the daemon's own refusals: a word typed in a group the Languages
	// list does not name has no declared script to be read in, so nothing is ever
	// scored, whatever the readings look like.
	if !namesGroup(in.Groups, in.WordGroup) {
		fmt.Fprintf(&b, "%s\n", l10n.X("autodetect.not_corrected_group",
			"Would NOT be corrected: the word was typed in group {{.N}}, which Languages does not name.",
			"N", in.WordGroup))
		return b.String()
	}

	// Length is the brake the daemon applies before it asks the model anything, so
	// a run too short to judge never reports a winner here either.
	if letters < in.MinChars {
		fmt.Fprintf(&b, "%s\n", l10n.X("autodetect.not_corrected_min_chars",
			"Would NOT be corrected: {{.Count}} letters is below MinChars {{.MinChars}}.",
			"Count", letters, "MinChars", in.MinChars))
		return b.String()
	}

	lang, confidence, ok := autodetect.Best(in.Readings)
	if !ok {
		b.WriteString(l10n.X("autodetect.no_verdict",
			"No verdict: this run fits neither reading, so the daemon leaves it alone.") + "\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%s\n", l10n.X("autodetect.best_reading",
		"Best reading: {{.Layout}} at confidence {{.Confidence}} (threshold {{.Threshold}}, MinChars {{.MinChars}})",
		"Layout", lang,
		"Confidence", fmt.Sprintf("%.3f", confidence),
		"Threshold", fmt.Sprintf("%.3f", in.Threshold),
		"MinChars", in.MinChars))
	if confidence < in.Threshold {
		fmt.Fprintf(&b, "%s\n", l10n.X("autodetect.not_corrected_confidence",
			"Would NOT be corrected: {{.Confidence}} is below Confidence {{.Threshold}}.",
			"Confidence", fmt.Sprintf("%.3f", confidence),
			"Threshold", fmt.Sprintf("%.3f", in.Threshold)))
		return b.String()
	}
	group, named := in.Groups[lang]
	switch {
	case !named:
		fmt.Fprintf(&b, "%s\n", l10n.X("autodetect.no_group_named",
			"The form names no group for {{.Layout}}: add it to Languages before applying.",
			"Layout", lang))
	case group == in.WordGroup:
		fmt.Fprintf(&b, "%s\n", l10n.X("autodetect.nothing_to_do",
			"Nothing to do: the winning group {{.N}} is already the group the word was typed in.",
			"N", group))
	default:
		fmt.Fprintf(&b, "%s\n", l10n.X("autodetect.would_correct",
			"WOULD be corrected to {{.Layout}}: group {{.Group}} instead of {{.WordGroup}}.",
			"Layout", lang, "Group", group, "WordGroup", in.WordGroup))
	}
	return b.String()
}

// namesGroup reports whether a Languages list names the given XKB group at all.
// A negative group is the daemon's "no character recorded yet" and is named by nobody.
func namesGroup(groups map[string]int, want int) bool {
	if want < 0 {
		return false
	}
	for _, group := range groups {
		if group == want {
			return true
		}
	}
	return false
}

// previewNow asks the daemon for the live buffer and says what the detector would
// do with it under the numbers in this form. The daemon does the rendering (it
// owns the glyph tables and the key buffer) and the GUI does the scoring, with the
// same pure package, so both halves judge the same strings.
func (e *autodetectEditor) previewNow() {
	resp, err := e.client.Status()
	if err != nil {
		e.show(l10n.X("autodetect.status_failed", "Status failed: {{.Err}}", "Err", err.Error()))
		return
	}
	minChars, threshold := e.thresholds()
	e.show(previewReport(previewInput{
		Buffer:    resp.WordBuffer,
		Readings:  resp.Readings,
		WordGroup: resp.WordGroup,
		MinChars:  minChars,
		Threshold: threshold,
		Groups:    languageGroups(e.languages.Text),
	}))
}

func (e *autodetectEditor) show(text string) {
	e.preview.Enable()
	e.preview.SetText(text)
	e.preview.Disable()
}
