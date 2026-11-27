// Package config — see types.go for overview.
// roundtrip_test.go is the byte-level half of the config writer, contributed by
// the peer review of Phases 3-5 (2026-10-03): the tests that shipped with the
// writer checked that an edit landed and that the result still parsed, which is
// not the promise the GUI relies on. The GUI reads the daemon's file as a tree,
// sends back the keys the user touched, and the writer must leave every other
// byte alone -- comments, unknown keys, array tables, the commented-out examples
// that document a section. Only a byte comparison says that.
//
// These run against the config that ships at the module root, so they fail if the
// shipped file gains a shape the writer reflows.
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shippedConf reads the real daemon config that ships at the repo root.
// The package dir is internal/config, so ../../ lands on the module root.
func shippedConf(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "xswitcher.conf"))
	if err != nil {
		t.Fatalf("read shipped conf: %v", err)
	}
	return b
}

func confCopy(t *testing.T, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "xswitcher.conf")
	if err := os.WriteFile(p, content, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustConf(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A write with nothing to change has to give the file back exactly as it was.
func TestRoundTripNoOpIsByteIdentical(t *testing.T) {
	orig := shippedConf(t)
	path := confCopy(t, orig)
	if err := WriteTOML(path, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if got := mustConf(t, path); string(got) != string(orig) {
		t.Errorf("no-op round-trip changed bytes: orig=%d got=%d", len(orig), len(got))
	}
}

// After one real edit, a second no-op pass must be a fixed point: the writer's
// own output has to be something it reads back unchanged.
func TestRoundTripIsAFixedPoint(t *testing.T) {
	path := confCopy(t, shippedConf(t))
	if err := WriteTOML(path, map[string]any{"Keyboard.Delay": 7}); err != nil {
		t.Fatal(err)
	}
	first := mustConf(t, path)
	if err := WriteTOML(path, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if second := mustConf(t, path); string(first) != string(second) {
		t.Errorf("second no-op pass changed bytes")
	}
}

// One scalar edit changes one line and nothing else -- the line count and every
// comment and array table around it stay where they were.
func TestRoundTripScalarEditsOneLine(t *testing.T) {
	origStr := string(shippedConf(t))
	orig := strings.SplitAfter(origStr, "\n")
	path := confCopy(t, []byte(origStr))
	if err := WriteTOMLChecked(path, map[string]any{"Keyboard.Delay": 7}); err != nil {
		t.Fatal(err)
	}
	gotStr := string(mustConf(t, path))
	got := strings.SplitAfter(gotStr, "\n")
	if len(got) != len(orig) {
		t.Fatalf("line count changed: orig=%d got=%d", len(orig), len(got))
	}
	changed := 0
	for i := range orig {
		if orig[i] == got[i] {
			continue
		}
		changed++
		if !strings.HasPrefix(strings.TrimSpace(got[i]), "Delay = 7") {
			t.Errorf("line %d changed unexpectedly: %q -> %q", i, orig[i], got[i])
		}
	}
	if changed != 1 {
		t.Errorf("expected exactly one changed line, got %d", changed)
	}
	for _, want := range []string{"# xswitcher config file.", "[[WindowClasses]]", "# Search mask", "# [[AutoReplace]]"} {
		if !strings.Contains(gotStr, want) {
			t.Errorf("verbatim preservation lost for %q", want)
		}
	}
}

// Editing a key whose section the shipped file only shows as a commented example
// appends a real section; the bytes already there must survive as an exact prefix,
// the commented example included. This also exercises the defaults path: the
// parser has to accept a [AutoDetect] that names only one of its keys.
func TestRoundTripAppendsMissingSectionKeepsPrefix(t *testing.T) {
	before := string(shippedConf(t))
	path := confCopy(t, []byte(before))
	if err := WriteTOMLChecked(path, map[string]any{"AutoDetect.MinChars": 6}); err != nil {
		t.Fatalf("append [AutoDetect] rejected: %v", err)
	}
	after := string(mustConf(t, path))
	if !strings.HasPrefix(after, strings.TrimRight(before, "\n")) {
		t.Errorf("append disturbed existing bytes; original is no longer a prefix")
	}
	if !strings.Contains(after, "MinChars = 6") {
		t.Errorf("appended key missing")
	}
	if !strings.Contains(after, "# [[AutoReplace]]") {
		t.Errorf("commented AutoReplace example was disturbed")
	}
}

// The [AutoDetect] numbers come from the struct tags on the parse path the daemon
// and the GUI's preview share: a present section that omits MinChars and
// Confidence must still land on 5 / 0.2, not on the zero values -- a preview built
// on 0/0 would say "would be corrected" about any keystroke run.
func TestRoundTripAutoDetectTagDefaultsOnSectionParse(t *testing.T) {
	p, err := ParseTOML([]byte("[AutoDetect]\nEnabled = true\nLanguages = [\"0:en\", \"1:ru\"]\n"))
	if err != nil {
		t.Fatalf("partial [AutoDetect] without MinChars/Confidence rejected: %v", err)
	}
	if p.AutoDetect.MinChars != 5 || p.AutoDetect.Confidence != 0.2 {
		t.Errorf("tag defaults not applied: MinChars=%d Confidence=%v (want 5 / 0.2)",
			p.AutoDetect.MinChars, p.AutoDetect.Confidence)
	}
}

// A file with no [AutoDetect] at all parses to the zero struct, which is harmless
// only while Enabled stays false. This pins that: the writer does not inject
// defaults and the daemon's gates must keep reading a missing section as off.
func TestRoundTripAbsentAutoDetectSectionIsDisabledZero(t *testing.T) {
	p, err := ParseTOML([]byte("[Keyboard]\nDelay = 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.AutoDetect.Enabled {
		t.Errorf("absent [AutoDetect] must stay disabled")
	}
	if def := DefaultAutoDetect(); def.MinChars != 5 || def.Confidence != 0.2 {
		t.Errorf("DefaultAutoDetect() drifted from the tags: %+v", def)
	}
}
