package exec

import (
	"bytes"
	"testing"
	"time"
)

// Command.max_reply is unexported and the only caller never sets it, so the old code ran
// io.CopyN(dst, src, 0): zero bytes were ever captured and every Exec action reported empty output.
// ExecCommand must now fall back to defaultMaxReply and actually return the child's stdout/stderr.
func TestExecCommandCapturesOutput(t *testing.T) {
	r := ExecCommand(&Command{Command: "echo hello-stdout; echo hello-stderr 1>&2", UseShell: true, Timeout: 10})
	if !bytes.Contains(r.StdOut, []byte("hello-stdout")) {
		t.Errorf("stdout not captured: %q", r.StdOut)
	}
	if !bytes.Contains(r.StdErr, []byte("hello-stderr")) {
		t.Errorf("stderr not captured: %q", r.StdErr)
	}
}

// A reply larger than the cap must be truncated to it, not grown without bound and not hang the pipe.
func TestExecCommandCapsOutput(t *testing.T) {
	// ~588 KB of digits, far above defaultMaxReply.
	r := ExecCommand(&Command{Command: "seq 1 100000", UseShell: true, Timeout: 10})
	if int64(len(r.StdOut)) != defaultMaxReply {
		t.Errorf("stdout length = %d, want cap %d", len(r.StdOut), defaultMaxReply)
	}
}

// A command past its deadline must be terminated and reaped without the old data race: the killer
// used to poll cmd.ProcessState while cmd.Wait() wrote it. Run under `go test -race`; the rewrite
// reads only the pid and a done channel, so there is nothing left to race on. Timeout is seconds, so
// Timeout:1 kills `sleep 30` in about a second instead of waiting 30s.
func TestExecCommandTimesOutAndKills(t *testing.T) {
	start := time.Now()
	r := ExecCommand(&Command{Command: "sleep 30", UseShell: true, Timeout: 1})
	elapsed := time.Since(start)

	if elapsed > 10*time.Second {
		t.Errorf("timeout did not cut the command short; it took %v", elapsed)
	}
	if r.Status == 0 {
		t.Errorf("a killed command must not report success (status %d)", r.Status)
	}
}
