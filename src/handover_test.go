package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xswitcher/internal/ipc"
)

// The decision the watch makes is the whole safety of a respawn, and it may be wrong in
// exactly one direction: hand over only on proof. A fake owner stands in for the socket, a
// buffered channel for the child's exit, so every branch is reached on its own.
type watchProbe struct {
	mu    sync.Mutex
	calls int
	kills int
}

func fakeWatch(own int, owner func(int) (int, bool), exited chan string, window, grace, step time.Duration) (handoverWatch, *watchProbe) {
	probe := &watchProbe{}
	return handoverWatch{
		own: own,
		owner: func() (int, bool) {
			probe.mu.Lock()
			probe.calls++
			n := probe.calls
			probe.mu.Unlock()
			return owner(n)
		},
		exited:    exited,
		kill:      func() { probe.mu.Lock(); probe.kills++; probe.mu.Unlock() },
		window:    window,
		deadGrace: grace,
		step:      step,
	}, probe
}

// The proof is a foreign pid on the socket, and nothing less: the same process answering is
// the parent itself, which says only that no child got that far.
func TestHandoverWaitsForTheChildToTakeTheSocket(t *testing.T) {
	exited := make(chan string, 1)
	w, probe := fakeWatch(100, func(n int) (int, bool) {
		if n < 3 {
			return 100, true // still us
		}
		return 200, true // the child owns it: it booted
	}, exited, 2*time.Second, time.Second, time.Millisecond)

	if err := w.run(); err != nil {
		t.Errorf("a child that took the socket was refused: %v", err)
	}
	if probe.kills != 0 {
		t.Errorf("a successful handover killed %d processes, want none", probe.kills)
	}
}

// Every config state the parser cannot name arrives here as an exit -- main() recovers from
// the panic and returns -- and a dead child is not a reload.
func TestHandoverRefusesADeadChild(t *testing.T) {
	exited := make(chan string, 1)
	w, probe := fakeWatch(100, func(int) (int, bool) { return 100, true }, exited, 2*time.Second, 40*time.Millisecond, time.Millisecond)
	exited <- "exit status 0"

	err := w.run()
	if err == nil {
		t.Fatal("a child that exited was accepted")
	}
	if !strings.Contains(err.Error(), "exited instead of coming up (exit status 0)") {
		t.Errorf("wrong reason: %v", err)
	}
	if probe.kills != 0 {
		t.Errorf("a dead child was killed %d times, want none", probe.kills)
	}
}

// The startup DE-wait makes a respawned child re-exec, so the parent legitimately sees its
// child die while a grandchild is still coming up. Refusing that would leave this process
// serving next to the grandchild -- two readers of one keyboard.
func TestHandoverFollowsAChildThatReExecs(t *testing.T) {
	exited := make(chan string, 1)
	w, probe := fakeWatch(100, func(n int) (int, bool) {
		if n < 4 {
			return 100, true
		}
		return 300, true // the grandchild took over
	}, exited, 2*time.Second, 200*time.Millisecond, time.Millisecond)
	exited <- "exit status 0"

	if err := w.run(); err != nil {
		t.Errorf("the re-exec chain was refused instead of followed: %v", err)
	}
	if probe.kills != 0 {
		t.Errorf("a successful handover killed %d processes, want none", probe.kills)
	}
}

// Alive at the deadline on a socket that answered all along means a child stuck before the
// bind. It must not be left running: it would take the socket and the keyboards later.
func TestHandoverStopsAChildThatNeverTakesOver(t *testing.T) {
	exited := make(chan string, 1)
	w, probe := fakeWatch(100, func(int) (int, bool) { return 100, true }, exited, 60*time.Millisecond, time.Second, time.Millisecond)

	err := w.run()
	if err == nil || !strings.Contains(err.Error(), "never took over") {
		t.Fatalf("a silent child was accepted: %v", err)
	}
	if probe.kills != 1 {
		t.Errorf("a silent child was killed %d times, want exactly once", probe.kills)
	}
}

// A deployment with no working socket has no proof available -- and no reason to lose its
// keyboard either. A live child is handed over there, as it was before the watch existed.
func TestHandoverWithoutASocketFallsBackToStayingAlive(t *testing.T) {
	exited := make(chan string, 1)
	w, probe := fakeWatch(100, func(int) (int, bool) { return 0, false }, exited, 60*time.Millisecond, time.Second, time.Millisecond)

	if err := w.run(); err != nil {
		t.Errorf("a live child was refused for want of a socket: %v", err)
	}
	if probe.kills != 0 {
		t.Errorf("the fallback killed a live child %d times, want none", probe.kills)
	}
}

// socketOwnerPid is the proof read straight off the wire: it has to find the pid the daemon
// answers with, and report nothing at all when no daemon is there.
func TestSocketOwnerPidReadsTheLiveServer(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "handover.sock")
	t.Setenv("XSWITCHER_SOCK", sock)

	if _, ok := socketOwnerPid(); ok {
		t.Error("an absent socket reported an owner")
	}

	srv := ipc.NewServer(sock, func() ipc.Response {
		return ipc.Response{Ok: true, PID: os.Getpid()}
	}, nil)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	pid, ok := socketOwnerPid()
	if !ok {
		t.Fatal("a live server on the socket was not reported")
	}
	if pid != os.Getpid() {
		t.Errorf("the socket answered as %d, want this process %d", pid, os.Getpid())
	}
}

// The wiring on the other side of the seam: a real process, really waited on. A child that
// exits has to be reported dead, and reported quickly.
func TestHandoverSeesARealProcessDie(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "silent.sock")
	t.Setenv("XSWITCHER_SOCK", sock) // nothing listens, so only the death can end the watch

	cmd := exec.Command("/bin/sh", "-c", "exit 3")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start a child process here: %v", err)
	}
	start := time.Now()
	err := handover(cmd.Process)
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("a child that exited 3 was not refused: %v", err)
	}
	if elapsed > handoverDeadGrace+500*time.Millisecond {
		t.Errorf("the death took %v to notice, want no more than the grace %v", elapsed, handoverDeadGrace)
	}
}
