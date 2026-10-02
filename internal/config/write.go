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
// applied. Only two-level keys ("Section.Key") are supported: the GUI edits
// whole sections, and deeper keys belong to action definitions edited as raw
// TOML in a later phase.
func applyModsLines(path string, mods map[string]interface{}) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	lines := strings.SplitAfter(string(data), "\n")
	for key, value := range mods {
		parts := strings.Split(key, ".")
		if len(parts) != 2 {
			return nil, fmt.Errorf("unsupported key path %q: only Section.Key is editable", key)
		}
		if value == nil {
			continue // a JSON-explicit null must not create an invalid `Key = ` line
		}
		formatted, err := formatValue(value)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
		lines, err = setLine(lines, parts[0], parts[1], formatted)
		if err != nil {
			return nil, err
		}
	}
	return []byte(strings.Join(lines, "")), nil
}

var (
	sectionRE = regexp.MustCompile(`^\s*\[([^\[\]]+)\]\s*$`)
	// A key line: optional indent, bare or quoted key, '='. Comments (`# key
	// = ...`) never match because the line must start with the key.
	keyLineRE = regexp.MustCompile(`^([ \t]*)([A-Za-z0-9_]+|"([^"]+)")[ \t]*=`)
)

// setLine applies "Section.Key = value" to the line slice.
func setLine(lines []string, section, key, value string) ([]string, error) {
	inSection := -1 // index of the [section] header
	sectionEnd := len(lines)

	for i, line := range lines {
		if m := sectionRE.FindStringSubmatch(strings.TrimRight(line, "\r\n")); m != nil {
			if inSection >= 0 {
				sectionEnd = i
				break
			}
			if strings.TrimSpace(m[1]) == section {
				inSection = i
			}
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
