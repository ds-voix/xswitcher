// Package main — see main.go for overview.
// autodetect_editor_test.go tests the preview's decision half: the report is what
// tells the user whether the thresholds on screen would let the detector act, so
// the guard order it prints has to be the daemon's, and the numbers below are the
// ones measured against the shipped models (Phase 5A/5B).
package main

import (
	"strings"
	"testing"
)

func TestPreviewReportGuardOrder(t *testing.T) {
	// "ghbdtn" is the Russian word привет typed on the latin group; the shipped
	// models read it as Russian at 0.379.
	in := previewInput{
		Buffer:    "ghbdtn",
		Readings:  map[string]string{"en": "ghbdtn", "ru": "привет"},
		Layout:    0,
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

	// MinChars is the brake that acts first: the same run at a threshold that
	// would let it through must still be refused for length.
	if got = previewReport(withMinChars(in, 7)); !strings.Contains(got, "Would NOT be corrected: 6 letters is below MinChars 7.") {
		t.Errorf("length gate did not speak first:\n%s", got)
	}
	if got = previewReport(withThreshold(in, 0.5)); !strings.Contains(got, "is below Confidence 0.500.") {
		t.Errorf("confidence gate did not speak:\n%s", got)
	}
	// On the group the model asks for there is nothing to correct, whatever the
	// confidence says.
	in.Layout = 1
	if got = previewReport(in); !strings.Contains(got, "Nothing to do: the winning group 1 is already the current layout.") {
		t.Errorf("a word already in the winning group was not left alone:\n%s", got)
	}
	// A winner the Languages list does not name cannot be switched to: the preview
	// says so instead of inventing a group.
	in.Groups = map[string]int{"en": 0}
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
	// two nonsense costs.
	got = previewReport(previewInput{
		Buffer: "123456", Readings: map[string]string{"en": "123456", "ru": "123456"},
		MinChars: 5, Threshold: 0.2, Groups: map[string]int{"en": 0, "ru": 1},
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

func withMinChars(in previewInput, n int) previewInput {
	in.MinChars = n
	return in
}

func withThreshold(in previewInput, f float64) previewInput {
	in.Threshold = f
	return in
}
