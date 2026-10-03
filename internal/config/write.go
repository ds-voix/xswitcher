// Package config — see types.go for overview.
// write.go modifies an existing config file while preserving its comments and
// key order. go-toml v1.9.5 Tree.Marshal drops every comment (measured), so
// modifications are applied line-by-line instead: replace the `Key = value`
// line inside its section, append to the section when the key is absent, and
// append a fresh section when that is absent too.
package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml"
)

// WriteTOML reads the existing config file at path, applies the modifications
// from mods (dotted "Section.Key" → value) preserving comments and order.
func WriteTOML(path string, mods map[string]interface{}) error {
	out, err := applyModsLines(path, mods)
	if err != nil {
		return err
	}
	return writeFile(path, out)
}

// WriteTOMLChecked is WriteTOML with a safety gate: the result is parsed by
// ParseTOML first, and the file is only replaced if it is valid. A rejected
// modification leaves the original config untouched.
func WriteTOMLChecked(path string, mods map[string]interface{}) error {
	out, err := applyModsLines(path, mods)
	if err != nil {
		return err
	}
	if _, err := ParseTOML(out); err != nil {
		return fmt.Errorf("resulting config is invalid, file not written: %w", err)
	}
	return writeFile(path, out)
}

// ReadTOMLTree loads a TOML file into a go-toml Tree for inspection or
// modification without losing comments.
func ReadTOMLTree(path string) (*toml.Tree, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	tree, err := toml.LoadBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parse config tree: %w", err)
	}
	return tree, nil
}

func writeFile(path string, out []byte) error {
	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// applyModsLines returns the file content with every dotted key in mods
// applied. Supported paths:
//
//   - "Section.Key" (and deeper, e.g. "Action.RetTypeWord.Layout" for the
//     [Action.RetTypeWord] table): one scalar/array value per line.
//   - "Section" with an array-of-maps value: the whole [[Section]] array of
//     tables is regenerated in place. Comments inside replaced blocks are
//     lost; everything else in the file is untouched.
func applyModsLines(path string, mods map[string]interface{}) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	lines := strings.SplitAfter(string(data), "\n")
	for key, value := range mods {
		parts := strings.Split(key, ".")
		if value == nil {
			continue // a JSON-explicit null must not create an invalid `Key = ` line
		}
		var err error
		if len(parts) == 1 {
			entries, ok := arrayOfTables(value)
			if !ok {
				return nil, fmt.Errorf("unsupported key path %q: expected Section.Key, or a table name with an array-of-tables value", key)
			}
			lines, err = setArrayTable(lines, parts[0], entries)
		} else {
			var formatted string
			formatted, err = formatValue(value)
			if err == nil {
				section := strings.Join(parts[:len(parts)-1], ".")
				lines, err = setLine(lines, section, parts[len(parts)-1], formatted)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
	}
	return []byte(strings.Join(lines, "")), nil
}

// arrayOfTables reports whether a JSON-decoded value is a non-empty array
// whose every element is a table — the shape of a [[Section]] rewrite.
func arrayOfTables(value interface{}) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if !ok || len(items) == 0 {
		return nil, false
	}
	for _, item := range items {
		if _, ok := item.(map[string]interface{}); !ok {
			return nil, false
		}
	}
	return items, true
}

var (
	// Trailing comments are allowed on every header form: the shipped config
	// writes `[[WindowClasses]] # Default behaviour: ...` and
	// `[Action.RetypeWord] # Switch layout, ...`.
	tableHeaderRE = regexp.MustCompile(`^\s*\[([^\[\]]+)\](\s*#.*)?$`)
	arrayHeaderRE = regexp.MustCompile(`^\s*\[\[([^\[\]]+)\]\](\s*#.*)?$`)
	// A key line: optional indent, bare or quoted key, '='. Comments (`# key
	// = ...`) never match because the line must start with the key.
	keyLineRE = regexp.MustCompile(`^([ \t]*)([A-Za-z0-9_]+|"([^"]+)")[ \t]*=`)
)

// headerKind classifies a line as 0 = not a header, 1 = [table],
// 2 = [[array-of-tables]], and returns the trimmed section name.
func headerKind(line string) (int, string) {
	trimmed := strings.TrimRight(line, "\r\n")
	if m := arrayHeaderRE.FindStringSubmatch(trimmed); m != nil {
		return 2, strings.TrimSpace(m[1])
	}
	if m := tableHeaderRE.FindStringSubmatch(trimmed); m != nil {
		return 1, strings.TrimSpace(m[1])
	}
	return 0, ""
}

// setArrayTable replaces every [[name]] block (header line up to the next
// header) with the generated entries, keeping the position of the first
// block; with no existing blocks the new ones are appended at the end.
func setArrayTable(lines []string, name string, entries []interface{}) ([]string, error) {
	var kept []string
	insertAt := -1
	inBlock := false
	for _, line := range lines {
		kind, hname := headerKind(line)
		if kind == 2 && hname == name {
			if insertAt < 0 {
				insertAt = len(kept)
			}
			inBlock = true
			continue
		}
		if kind != 0 {
			inBlock = false
		}
		if !inBlock {
			kept = append(kept, line)
		}
	}

	var blocks []string
	if insertAt < 0 {
		insertAt = len(kept)
		if len(kept) > 0 {
			last := kept[len(kept)-1]
			if strings.TrimRight(last, "\r\n") != "" {
				blocks = append(blocks, "\n")
			}
		}
	} else if insertAt > 0 {
		prev := strings.TrimRight(kept[insertAt-1], "\r\n")
		kind, _ := headerKind(prev)
		if prev != "" && kind == 0 { // separate a preceding key/comment line from the new header
			blocks = append(blocks, "\n")
		}
	}
	if n := len(kept); n > 0 && insertAt == n && !strings.HasSuffix(kept[n-1], "\n") {
		kept[n-1] = kept[n-1] + "\n"
	}
	for _, entry := range entries {
		table := entry.(map[string]interface{})
		blocks = append(blocks, "[["+name+"]]\n")
		keys := make([]string, 0, len(table))
		for k := range table {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if table[k] == nil {
				continue
			}
			s, err := formatValue(table[k])
			if err != nil {
				return nil, fmt.Errorf("value of %s.%s: %w", name, k, err)
			}
			blocks = append(blocks, k+" = "+s+"\n")
		}
		blocks = append(blocks, "\n")
	}

	out := make([]string, 0, len(kept)+len(blocks))
	out = append(out, kept[:insertAt]...)
	out = append(out, blocks...)
	out = append(out, kept[insertAt:]...)
	return out, nil
}

// setLine applies "Section.Key = value" to the line slice. Section may be a
// dotted table name ("Action.RetTypeWord"); array-of-tables headers never
// match, they are rewritten whole via setArrayTable.
func setLine(lines []string, section, key, value string) ([]string, error) {
	inSection := -1 // index of the [section] header
	sectionEnd := len(lines)

	for i, line := range lines {
		kind, name := headerKind(line)
		if kind == 0 {
			continue
		}
		if inSection >= 0 {
			sectionEnd = i
			break
		}
		if kind == 1 && name == section {
			inSection = i
		}
	}
	if inSection < 0 {
		// No such section: append it (and the key) at the end of the file.
		appended := lines
		if len(appended) > 0 && !strings.HasSuffix(appended[len(appended)-1], "\n") {
			appended[len(appended)-1] = appended[len(appended)-1] + "\n"
		}
		appended = append(appended, "\n["+section+"]\n", key+" = "+value+"\n")
		return appended, nil
	}

	// Replace an existing key line inside the section.
	for i := inSection + 1; i < sectionEnd; i++ {
		if m := keyLineRE.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n")); m != nil {
			bare := m[2]
			if m[3] != "" {
				bare = m[3]
			}
			if bare == key {
				trimmed := strings.TrimRight(lines[i], "\r\n")
				eol := lines[i][len(trimmed):]
				lines[i] = m[1] + key + " = " + value + eol
				return lines, nil
			}
		}
	}

	// Key absent: insert after the last non-empty line of the section.
	insertAt := sectionEnd
	for i := sectionEnd - 1; i > inSection; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			insertAt = i + 1
			break
		}
	}
	if !strings.HasSuffix(lines[insertAt-1], "\n") {
		lines[insertAt-1] = lines[insertAt-1] + "\n"
	}
	fresh := key + " = " + value + "\n"
	lines = append(lines[:insertAt], append([]string{fresh}, lines[insertAt:]...)...)
	return lines, nil
}

// formatValue renders a Go value as a TOML literal.
func formatValue(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v), nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case []string:
		items := make([]interface{}, len(v))
		for i, s := range v {
			items[i] = s
		}
		return formatArray(items)
	case []interface{}:
		return formatArray(v)
	default:
		return "", fmt.Errorf("unsupported value type %T", value)
	}
}

func formatArray(items []interface{}) (string, error) {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		s, err := formatValue(item)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}
