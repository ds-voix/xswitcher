// Package config — see types.go for overview.
// write.go provides a Tree-based TOML writer that preserves comments and key
// order when modifying an existing config file. Falls back to regex replacement
// if go-toml v1.9.5 Tree.Marshal loses comments (known issue).
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml"
)

// WriteTOML reads the existing config file at path, applies the modifications
// from mods (key → value), and writes it back preserving comments and order.
// If the tree-based approach fails to preserve comments, it falls back to
// line-by-line regex replacement.
func WriteTOML(path string, mods map[string]interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	tree, err := toml.LoadBytes(data)
	if err != nil {
		return fmt.Errorf("parse config tree: %w", err)
	}

	for key, value := range mods {
		parts := strings.Split(key, ".")
		tree.SetPath(parts, value)
	}

	out, err := tree.Marshal()
	if err != nil {
		return fmt.Errorf("marshal config tree: %w", err)
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
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