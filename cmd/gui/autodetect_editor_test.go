// Package main — see main.go for overview.
// autodetect_editor_test.go tests the preview's decision half: the report is what
// tells the user whether the thresholds on screen would let the detector act, so
// the guard order it prints has to be the daemon's, and the numbers below are the
// ones measured against the shipped models (Phase 5A/5B).
package main

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"xswitcher/internal/ipc"
)

func TestPreviewReportGuardOrder(t *testing.T) {
	// "ghbdtn" is the Russian word привет typed on the latin group; the shipped
	// models read it as Russian at 0.379.
	in := previewInput{
		Buffer:    "ghbdtn",
		Readings:  map[string]string{"en": "ghbdtn", "ru": "привет"},
		WordGroup: 0,
		MinChars:  5,
		Threshold: 0.2,
		Groups:    map[string]int{"en": 0, "ru": 1},
	}
	got := previewReport(in)
	for _, want := range []string{
		`Buffer: "ghbdtn" (6 letters)`,
		"  en   ghbdtn",
		"  ru   привет",
		"Best reading: ru at confidence 0.379",
		"WOULD be corrected to ru: group 1 instead of 0.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}

	// The daemon refuses a word typed in a group its Languages list does not name,
	// before it scores anything. Without this brake the preview promises a
	// correction the daemon will never make.
	for _, unnamed := range []int{3, -1} {
		got = previewReport(withWordGroup(in, unnamed))
		if !strings.Contains(got, "which Languages does not name") {
			t.Errorf("group %d was reported as correctable:\n%s", unnamed, got)
		}
		if strings.Contains(got, "Best reading") {
			t.Errorf("group %d reached the model, which the daemon never does:\n%s", unnamed, got)
		}
	}

	// MinChars is the brake that acts first among the ones that score: the same
	// run at a threshold that would let it through must still be refused for
	// length, and refused for length alone -- the daemon asks the model nothing
	// about a run this short, so neither may the preview name a winner.
	if got = previewReport(withMinChars(in, 7)); !strings.Contains(got, "Would NOT be corrected: 6 letters is below MinChars 7.") {
		t.Errorf("length gate did not speak first:\n%s", got)
	}
	if strings.Contains(got, "Best reading") {
		t.Errorf("a run refused for length still got a winner:\n%s", got)
	}
	if got = previewReport(withThreshold(in, 0.5)); !strings.Contains(got, "is below Confidence 0.500.") {
		t.Errorf("confidence gate did not speak:\n%s", got)
	}
	// On the group the model asks for there is nothing to correct, whatever the
	// confidence says.
	in.WordGroup = 1
	if got = previewReport(in); !strings.Contains(got, "Nothing to do: the winning group 1 is already the group the word was typed in.") {
		t.Errorf("a word already in the winning group was not left alone:\n%s", got)
	}
	// A winner the Languages list does not name cannot be switched to: the preview
	// says so instead of inventing a group.
	in.Groups = map[string]int{"en": 0, "de": 1}
	if got = previewReport(in); !strings.Contains(got, "The form names no group for ru") {
		t.Errorf("an unnamed winner was reported as switchable:\n%s", got)
	}
}

func TestPreviewReportRefusesToGuess(t *testing.T) {
	// One declared language: the daemon sends no readings at all, and the preview
	// has to say that rather than score a single string.
	got := previewReport(previewInput{Buffer: "ghbdtn", Readings: map[string]string{"ru": "привет"}})
	if !strings.Contains(got, "The daemon sent no readings") {
		t.Errorf("one reading was previewed anyway:\n%s", got)
	}
	// Two readings, neither of them language-like: no verdict, not the lesser of
	// two nonsense costs. MinChars is zero here because the brake that counts
	// letters is the daemon's first one and this case is about the model's answer.
	got = previewReport(previewInput{
		Buffer: "123456", Readings: map[string]string{"en": "123456", "ru": "123456"},
		MinChars: 0, Threshold: 0.2, Groups: map[string]int{"en": 0, "ru": 1},
	})
	if !strings.Contains(got, "No verdict") {
		t.Errorf("a run that fits nothing still got a winner:\n%s", got)
	}
}

func TestLanguageGroups(t *testing.T) {
	groups := languageGroups("0:en\n 1 : ru\nfirst:de\n-1:fr\nbroken")
	if len(groups) != 2 {
		t.Fatalf("usable entries: %v", groups)
	}
	if groups["en"] != 0 || groups["ru"] != 1 {
		t.Errorf("groups: %v", groups)
	}
}

func TestPreviewButtonDrivesTheReport(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	// A real server on a temp socket stands in for the daemon, so the whole
	// tap -> status -> render path runs without an X server. The group is a
	// variable because the report has to follow the group the daemon says the word
	// was typed in, and that number travels through the socket as JSON.
	sock := filepath.Join(t.TempDir(), "daemon.sock")
	wordGroup := 0
	srv := ipc.NewServer(sock, func() ipc.Response {
		return ipc.Response{
			Ok:         true,
			WordBuffer: "ghbdtn",
			Layout:     0,
			WordGroup:  wordGroup,
			Readings:   map[string]string{"en": "ghbdtn", "ru": "привет"},
		}
	}, nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("listen on %s: %v", sock, err)
	}
	defer srv.Stop()

	_, e := newAutodetectEditor(ipc.NewClient(sock))
	e.conf.SetText("0.2")
	e.languages.SetText("0:en\n1:ru")

	test.Tap(e.previewBtn)

	for _, want := range []string{
		`Buffer: "ghbdtn" (6 letters)`,
		"Best reading: ru at confidence 0.379",
		"WOULD be corrected to ru: group 1 instead of 0.",
	} {
		if !strings.Contains(e.preview.Text, want) {
			t.Errorf("the tap left the preview without %q:\n%s", want, e.preview.Text)
		}
	}

	// The group the status names is the one the daemon keys on: a word typed in a
	// group this form does not name must come back refused, not promised.
	wordGroup = 2
	test.Tap(e.previewBtn)
	if !strings.Contains(e.preview.Text, "the word was typed in group 2, which Languages does not name") {
		t.Errorf("an undeclared word group reached the verdict:\n%s", e.preview.Text)
	}

	if !e.preview.Disabled() {
		t.Error("the preview area is meant to be read-only, the button is the only writer")
	}
}

func TestPreviewButtonReportsADeadDaemon(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	// Nothing listens here, and the answer still has to reach the user: a button
	// that failed silently would be indistinguishable from an empty buffer.
	absent := filepath.Join(t.TempDir(), "absent.sock")
	_, e := newAutodetectEditor(ipc.NewClient(absent))
	test.Tap(e.previewBtn)

	if !strings.HasPrefix(e.preview.Text, "Status failed: ") {
		t.Errorf("a dead daemon produced no report:\n%q", e.preview.Text)
	}
	if !strings.Contains(e.preview.Text, absent) {
		t.Errorf("the report does not name the socket it could not reach:\n%q", e.preview.Text)
	}
}

func withMinChars(in previewInput, n int) previewInput {
	in.MinChars = n
	return in
}

func withWordGroup(in previewInput, n int) previewInput {
	in.WordGroup = n
	return in
}

func withThreshold(in previewInput, f float64) previewInput {
	in.Threshold = f
	return in
}
