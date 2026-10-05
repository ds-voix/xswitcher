// Package main -- see xswitcher.go for the overview.
//
// handover.go is the proof that a respawn actually happened. forkChild() can only report
// that a child was CREATED, and every fatal state the shared parser cannot see -- an
// invalid WordChars/WordHead/OFF/ON/SEQ regex, a duplicated OFF/ON/SEQ declaration, an
// unresolvable key name, a [ScanDevices] Test device that is not there -- kills that child
// AFTER the parent has already let go. main() recovers from each of those panics and
// returns, so the child exits successfully and the keyboard is simply gone while the GUI
// has been told it respawned.
//
// The proof the parent waits for is the child taking over the IPC socket, and the reason
// that is decidable is where serve() binds it: listenIPC() is the last step of the boot
// sequence, so a process answering on that path has already gotten through config(),
// sequences(), keys(), the device scan, the virtual keyboard, inotify and X11. A child
// that dies therefore dies with the parent's socket untouched, which is what lets the
// parent refuse the handover and keep serving.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"xswitcher/internal/ipc"
	"xswitcher/internal/l10n"
)

const (
	// handoverWindow bounds how long a live child may take to reach listenIPC(). Every
	// state that can stop it panics within tens of milliseconds of the fork, so this is
	// slack for a slow machine, not a timeout waiting on known work.
	handoverWindow = 3 * time.Second
	// handoverDeadGrace is how long the parent keeps watching after the child is gone.
	// A child that re-execs itself -- the startup DE-wait does exactly that -- exits
	// while a grandchild comes up, and the handover is real if that one takes the
	// socket. Without the grace the parent would refuse, keep serving and leave two
	// readers on the same keyboards.
	handoverDeadGrace = time.Second
	// handoverStep is the poll interval. One round trip is a unix-socket dial and a
	// status line, and the parent only ever polls its own server.
	handoverStep = 50 * time.Millisecond
	// handoverQuery bounds one status round trip: a poll that hangs would stall the
	// watch past its own deadline.
	handoverQuery = 200 * time.Millisecond
)

// listenIPC binds the GUI socket. Its position at the END of the boot sequence is the
// contract the handover proof rests on: a daemon that owns the path is a daemon that
// finished booting. A deployment where the bind fails (an unwritable /run, for one) loses
// the GUI and nothing else, so this reports and carries on instead of panicking.
func listenIPC() {
	if ipcSrv == nil {
		return
	}
	if err := ipcSrv.Start(); err != nil {
		fmt.Printf("%s\n", l10n.X("daemon.ipc_start_failed", "IPC server failed to start: {{.Err}}", "Err", err.Error()))
		return
	}
	if *DEBUG || *VERBOSE {
		fmt.Printf("%s\n", l10n.X("daemon.ipc_listening", "IPC server listening on {{.Path}}", "Path", ipc.SocketPath()))
	}
}

// socketOwnerPid asks the IPC socket who is answering, and reports false when nothing is.
// The parent recognises itself in the reply: its own pid means the child has not taken the
// socket yet, any other pid is the proof of a completed handover.
func socketOwnerPid() (int, bool) {
	conn, err := net.DialTimeout("unix", ipc.SocketPath(), handoverQuery)
	if err != nil {
		return 0, false
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(handoverQuery)); err != nil {
		return 0, false
	}
	data, err := json.Marshal(ipc.Request{Cmd: "status"})
	if err != nil {
		return 0, false
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return 0, false
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return 0, false
	}
	var resp ipc.Response
	if err := json.Unmarshal(line, &resp); err != nil || !resp.Ok {
		return 0, false
	}
	return resp.PID, true
}

// handoverWatch is one waited-out respawn, with the signals injected so the decision is
// testable without forking a daemon: owner says who holds the socket, exited delivers the
// reason the child stopped, kill stops a child that never got there.
type handoverWatch struct {
	own       int
	owner     func() (int, bool)
	exited    <-chan string
	kill      func()
	window    time.Duration
	deadGrace time.Duration
	step      time.Duration
}

// run returns nil when the keyboard is provably in new hands. It returns a reason when the
// child is gone and nothing took its place, or when a child that could have proved itself
// never did -- in which case the child is stopped, because a second reader of the same
// keyboards loses events for both. When nothing ever answered the socket the watch has no
// evidence either way and hands over on the child staying alive, which is the best a
// socketless deployment can do and what it did before.
func (w handoverWatch) run() error {
	started := time.Now()
	answered := false
	var died string
	deadAt := time.Time{}

	for {
		if pid, ok := w.owner(); ok {
			answered = true
			if pid != w.own {
				return nil
			}
		}
		now := time.Now()
		if died != "" {
			if now.Sub(deadAt) >= w.deadGrace {
				break
			}
		} else if now.Sub(started) >= w.window {
			break
		}
		select {
		case reason := <-w.exited:
			died, deadAt = reason, time.Now()
		case <-time.After(w.step):
		}
	}

	if died != "" {
		return fmt.Errorf("%s", l10n.X("daemon.handover_child_exited", "the respawned child exited instead of coming up ({{.Reason}})", "Reason", died))
	}
	if answered {
		w.kill()
		return fmt.Errorf("%s", l10n.X("daemon.handover_child_timeout", "the respawned child never took over the keyboard within {{.Window}}", "Window", w.window))
	}
	return nil
}

// handover waits for p to prove it booted. The exit reason comes from p.Wait in a
// goroutine: a process that was never started, or already reaped, reports that as its own
// error rather than as a clean exit.
func handover(p *os.Process) error {
	exited := make(chan string, 1)
	go func() {
		state, err := p.Wait()
		switch {
		case err != nil:
			exited <- err.Error()
		case !state.Exited():
			exited <- fmt.Sprintf("signal %v", state.Sys())
		default:
			exited <- fmt.Sprintf("exit status %d", state.ExitCode())
		}
	}()
	return handoverWatch{
		own:       os.Getpid(),
		owner:     socketOwnerPid,
		exited:    exited,
		kill:      func() { _ = p.Kill() },
		window:    handoverWindow,
		deadGrace: handoverDeadGrace,
		step:      handoverStep,
	}.run()
}
