package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forkChildTestChild marks a process that forkChild() really did start. It exists for the
// negative control of this file: with the gate removed forkChild() reaches os.StartProcess
// and execs another copy of the test binary, and without a marker that copy would run these
// tests again and fork again. forkChild passes os.Environ() to the child, so the marker
// reaches it and the recursion stops at depth one.
const forkChildTestChild = "XSWITCHER_TEST_FORK_CHILD"

func useConfigPath(t *testing.T, path string) {
	t.Helper()
	old := CONFIG_PATH
	CONFIG_PATH = path
	t.Cleanup(func() { CONFIG_PATH = old })
}

func writeConf(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Every respawn funnels through forkChild: the IPC reload command, the Respawn action a
// user binds to a hotkey or an [[Actions]] entry, and the re-exec that waits for the DE at
// startup. Only the first answers to anything from the outside, so the gate has to sit in
// the shared point and report in the error itself.
func TestForkChildRefusesAConfigTheChildCannotParse(t *testing.T) {
	if os.Getenv(forkChildTestChild) != "" {
		t.Skip("started by forkChild, not by go test")
	}
	t.Setenv(forkChildTestChild, "1")

	path := writeConf(t, "xswitcher.conf", "this is not = valid = toml\n")
	useConfigPath(t, path)

	p, err := forkChild()
	if err == nil {
		p.Kill()
		t.Fatalf("forkChild started a child over an unparsable %s", path)
	}
	if p != nil {
		t.Errorf("forkChild returned a process and an error: %v", p)
	}
	// An error that does not name the file is not this gate: os.Executable and
	// os.StartProcess fail with their own texts, and either would let an unrelated
	// environment problem pass as proof.
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error does not name %s: %v", path, err)
	}
}

// The other half: a file the child cannot even open is not fatal, because config() falls
// back to the embedded defaults -- the same way the running parent booted. Blocking it
// would take away the startup DE-wait re-exec from a user who has no config file at all.
func TestUnreadableConfigDoesNotBlockRespawn(t *testing.T) {
	useConfigPath(t, filepath.Join(t.TempDir(), "absent.conf"))
	if err := configKillsChild(); err != nil {
		t.Errorf("an unreadable config blocked the respawn: %v", err)
	}
	// The GUI still has to hear about it, though: there the question is whether the
	// settings the user edits are the ones on disk.
	if err := validateConfig(); err == nil {
		t.Error("validateConfig accepted a config file that is not there")
	}
}

// Without this the test above could pass on a predicate that always errors.
func TestValidateConfigAcceptsTheShippedConf(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "xswitcher.conf"))
	if err != nil {
		t.Fatal(err)
	}
	useConfigPath(t, path)
	if err := validateConfig(); err != nil {
		t.Errorf("the shipped config does not parse: %v", err)
	}
	if err := configKillsChild(); err != nil {
		t.Errorf("the shipped config would kill a respawned child: %v", err)
	}
}
