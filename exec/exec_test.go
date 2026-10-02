package exec

import (
	"bytes"
	"testing"
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
