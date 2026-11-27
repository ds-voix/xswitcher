package main

import (
	"os/user"
	"strconv"
	"testing"
)

// Exec without a "run_as" UID used to be a silent no-op: user.Lookup("") fails, and the old code
// returned from Exec before starting the command. resolveRunAs must treat an empty UID as "current
// user" (0, 0, nil - the exec package only sets Credential when UID > 0) while still rejecting a
// genuinely unknown user, so the fix does not swallow every lookup error.
func TestResolveRunAs(t *testing.T) {
	uid, gid, err := resolveRunAs("", "")
	if err != nil {
		t.Fatalf("empty UID must not be an error, got %v", err)
	}
	if uid != 0 || gid != 0 {
		t.Errorf("empty UID must resolve to (0, 0) so exec runs as the current user, got (%d, %d)", uid, gid)
	}

	if _, _, err := resolveRunAs("no-such-user-zzz-xyz-does-not-exist", ""); err == nil {
		t.Error("a genuinely unknown user must still return an error")
	}

	// The positive path: a real account resolves to its numeric id, not to (0, 0).
	if cur, cerr := user.Current(); cerr == nil {
		if n, perr := strconv.ParseUint(cur.Uid, 10, 32); perr == nil && n > 0 {
			gotUID, _, gerr := resolveRunAs(cur.Username, "")
			if gerr != nil {
				t.Fatalf("lookup of current user %q failed: %v", cur.Username, gerr)
			}
			if gotUID != uint32(n) {
				t.Errorf("resolveRunAs(%q) uid = %d, want %d", cur.Username, gotUID, n)
			}
		}
	}
}
