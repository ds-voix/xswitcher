package main

import (
	"strings"
	"testing"
	"xswitcher/internal/about"
)

// The release pipeline proves that a shipped asset was built from the tag it is published under by
// running `--version` on it as an ordinary user, so the line has exactly one shape: one line, the
// binary name, the version, the commit in parentheses. A build that was never stamped must stay
// obviously unstamped ("dev (unknown)") rather than looking like a release.
func TestVersionStamp(t *testing.T) {
	savedVersion, savedCommit := about.Version, about.Commit
	defer func() { about.Version, about.Commit = savedVersion, savedCommit }()

	about.Version, about.Commit = "v1.1.0", "2d66d12"
	line := versionStamp()

	if want := "xswitcher v1.1.0 (2d66d12)"; line != want {
		t.Errorf("stamp line must be %q, got %q", want, line)
	}
	if strings.ContainsAny(line, "\r\n") {
		t.Errorf("the stamp must be a single line the CI can diff, got %q", line)
	}

	about.Version, about.Commit = "dev", "unknown"
	if unstamped := versionStamp(); unstamped != "xswitcher dev (unknown)" {
		t.Errorf("an unstamped build must say so verbatim, got %q", unstamped)
	}
}
