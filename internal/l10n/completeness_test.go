package l10n

// completeness_test.go is the drift guard for the catalogues: it re-derives the
// set of message ids from the call sites in this repository and refuses to pass
// when a key is missing from a catalogue, when the English catalogue and the code
// literal disagree, or when a catalogue entry nothing asks for. The GUI prints the
// literal when a key is absent, so a missing ru entry is a UI that is quietly
// half in English; a mismatched en entry is worse, because the two languages then
// describe the same action in different words.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// goString matches one Go string literal, quoted or raw. A negated byte class
// already spans newlines, so a call wrapped over lines needs no extra flag.
var goString = "\"(?:[^\"\\\\]|\\\\.)*\"|" + "`[^`]*`"

// callRe matches X/XN/Errf with either the package qualifier (callers) or without
// it (this package's own helpers). The literal argument is one Go string, possibly
// built from concatenated parts, and the call may wrap over several lines.
var callRe = regexp.MustCompile(
	"(?:^|[^A-Za-z0-9_])(?:l10n\\.)?(?:X|XN|Errf)\\(\\s*\"(?P<key>[^\"]+)\"\\s*,\\s*" +
		"(?P<lit>(?:" + goString + ")(?:\\s*\\+\\s*(?:" + goString + "))*)",
)

// partRe pulls the pieces of a possibly concatenated Go string literal.
var partRe = regexp.MustCompile("\"((?:[^\"\\\\]|\\\\.)*)\"|`([^`]*)`")

// configLabelRe matches the helper that turns a TOML field name into a caption:
// its key is derived at run time, so the audit has to derive it the same way.
var configLabelRe = regexp.MustCompile("configKeyLabel\\(\\s*\"([^\"]+)\"\\s*\\)")

// configLabelDynamicRe matches the same helper called with a variable (the
// ActionKeys captions), which makes the whole namespace reachable.
var configLabelDynamicRe = regexp.MustCompile("configKeyLabel\\(\\s*[^\"\\)\\s]\\w*\\s*\\)")

var placeholderRe = regexp.MustCompile("\\{\\{\\.(\\w+)\\}\\}")

// skipDirs are trees that are not source of this repository: the module cache is
// mounted inside the working copy on developer machines, and the stand's scratch
// trees appear and disappear.
var skipDirs = map[string]bool{".git": true, ".gomodcache": true, "bin": true, "tmpprobe": true}

type site struct {
	file    string
	literal string
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("no caller information; cannot locate the repository root")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("the derived root %q is not the module root: %v", root, err)
	}
	return root
}

// goStringLiteral decodes the concatenated Go literal back to the bytes the
// program sees.
func goStringLiteral(raw string) string {
	var b strings.Builder
	for _, m := range partRe.FindAllStringSubmatch(raw, -1) {
		text := m[1]
		if text == "" {
			text = m[2]
		} else {
			text = strings.NewReplacer("\\\"", "\"", "\\\\", "\\", "\\t", "\t", "\\r", "\r", "\\n", "\n").Replace(text)
		}
		b.WriteString(text)
	}
	return b.String()
}

// forms returns the strings a catalogue entry can produce: one for a plain
// message, one per plural category for a pluralised one.
func forms(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case map[string]any:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		sort.Strings(out)
		return out
	default:
		return nil
	}
}

func readCatalogue(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var entries map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return entries
}

func collectCallSites(t *testing.T, root string) (map[string][]site, map[string]bool) {
	t.Helper()
	sites := map[string][]site{}
	dynamic := map[string]bool{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, m := range callRe.FindAllStringSubmatch(string(text), -1) {
			key := m[callRe.SubexpIndex("key")]
			lit := goStringLiteral(m[callRe.SubexpIndex("lit")])
			sites[key] = append(sites[key], site{file: rel, literal: lit})
		}
		for _, m := range configLabelRe.FindAllStringSubmatch(string(text), -1) {
			key := "config.key." + strings.ToLower(m[1])
			sites[key] = append(sites[key], site{file: rel, literal: m[1]})
		}
		if configLabelDynamicRe.MatchString(string(text)) {
			dynamic["config.key."] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return sites, dynamic
}

func TestCataloguesCoverEveryCallSite(t *testing.T) {
	root := repoRoot(t)
	en := readCatalogue(t, filepath.Join(root, "internal", "l10n", "locales", "en.json"))
	ru := readCatalogue(t, filepath.Join(root, "internal", "l10n", "locales", "ru.json"))
	sites, dynamic := collectCallSites(t, root)
	if len(sites) == 0 {
		t.Fatal("the scan found no l10n call sites; the audit is measuring nothing")
	}

	problems := []string{}
	keys := make([]string, 0, len(sites))
	for key := range sites {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		literals := map[string]string{}
		for _, s := range sites[key] {
			literals[s.literal] = s.file
		}
		if len(literals) > 1 {
			names := make([]string, 0, len(literals))
			for lit := range literals {
				names = append(names, lit)
			}
			sort.Strings(names)
			problems = append(problems, key+": one key called with different literals "+strings.Join(names, " | "))
			continue
		}
		var literal string
		for lit := range literals {
			literal = lit
		}
		enValue, ok := en[key]
		if !ok {
			problems = append(problems, key+": call site in "+sites[key][0].file+" but en.json has no entry")
		} else if !contains(forms(enValue), literal) {
			problems = append(problems, key+": en.json "+strings.Join(forms(enValue), "|")+" is not the literal "+literal)
		}
		ruValue, okRU := ru[key]
		if !okRU {
			problems = append(problems, key+": ru.json has no entry, the Russian UI falls back to English here")
			continue
		}
		if ok {
			enPlace := placeholders(forms(enValue))
			ruPlace := placeholders(forms(ruValue))
			if strings.Join(enPlace, ",") != strings.Join(ruPlace, ",") {
				problems = append(problems, key+": ru placeholders "+strings.Join(ruPlace, ",")+" differ from en "+strings.Join(enPlace, ","))
			}
		}
	}

	used := map[string]bool{}
	for key := range sites {
		used[key] = true
	}
	dead := []string{}
	for key := range en {
		if used[key] || strings.HasPrefix(key, "language.") {
			continue
		}
		reachable := false
		for prefix := range dynamic {
			if strings.HasPrefix(key, prefix) {
				reachable = true
			}
		}
		if !reachable {
			dead = append(dead, key)
		}
	}
	sort.Strings(dead)
	if len(dead) > 0 {
		problems = append(problems, "en entries with no call site: "+strings.Join(dead, ", "))
	}

	missingRU := []string{}
	for key := range en {
		if _, ok := ru[key]; !ok {
			missingRU = append(missingRU, key)
		}
	}
	sort.Strings(missingRU)
	if len(missingRU) > 0 {
		problems = append(problems, "keys in en.json but not in ru.json: "+strings.Join(missingRU, ", "))
	}
	missingEN := []string{}
	for key := range ru {
		if _, ok := en[key]; !ok {
			missingEN = append(missingEN, key)
		}
	}
	sort.Strings(missingEN)
	if len(missingEN) > 0 {
		problems = append(problems, "keys in ru.json but not in en.json: "+strings.Join(missingEN, ", "))
	}

	t.Logf("call sites: %d keys, en: %d entries, ru: %d entries", len(sites), len(en), len(ru))
	if len(problems) > 0 {
		for _, p := range problems {
			t.Error(p)
		}
	}
}

func placeholders(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		for _, m := range placeholderRe.FindAllStringSubmatch(value, -1) {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}