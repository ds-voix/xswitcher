package exec

/*
  Fold external command execution.
*/

import (
	"bytes"
	"io"
	"io/ioutil"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// defaultMaxReply bounds how many bytes of a command's stdout/stderr we keep. The Command.max_reply
// field is unexported and the only caller never assigns it, so without a fallback io.CopyN(dst, src, 0)
// copied zero bytes and every Exec action reported empty output. 64 KiB is enough to show a useful
// reply while a chatty child cannot grow our buffers without limit.
const defaultMaxReply = 1 << 16

type Command struct {
	id string        // Sequence ID may be set. Othewise, it is auto-generated.
	no_exec bool
	UID, GID uint32  // "run_as" linux representation
	UseShell bool
	No_wait bool     // https://golang.org/pkg/os/exec/#Cmd.Start vs "Run()" or m.b. "Wait()"
	Timeout float64
	max_reply int64
// https://golang.org/pkg/os/exec/#Cmd
	Set_env []string
	Set_dir string
	Command string
	Args []string
	StdIn []byte
}

type Result struct {
	ID string             `json:"id"`
	Processed bool        `json:"processed"` // Was this command ever been processed?
	Command string        `json:"command"`
	Args []string         `json:"args,omitempty"`
	Status int            `json:"status"`
	StdOut []byte         `json:"stdout,omitempty"`
	StdErr []byte         `json:"stderr,omitempty"`
}

// https://golang.org/pkg/os/exec/#Cmd
func ExecCommand(c *Command) (*Result) {
//	var waitWorkers *sync.WaitGroup

	r := &Result{ID: c.id,
					Command: c.Command,
					Args: c.Args}

	cmd := exec.Command(c.Command)
	if c.UseShell { // Pack into bash environment (In fact, bash is *sh's mainstream. And I unwill to deep into specifics)
		cmd = exec.Command("/bin/bash")
		// -c If the -c option is present, then commands are read from the first non-option argument command_string.  If there are arguments after the command_string, the first argu-
		//    ment is assigned to $0 and any remaining arguments are assigned to the positional parameters.  The assignment to $0 sets the name of the shell, which is used in warning
		//    and error messages.
		cmd.Args = append(cmd.Args, "-c")
		cmd.Args = append(cmd.Args, c.Command)
	}
	// https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Args = append(cmd.Args, c.Args...)
	cmd.Stdin = bytes.NewReader(c.StdIn)

	// https://stackoverflow.com/questions/21705950/running-external-commands-through-os-exec-under-another-user
	if c.UID > 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: c.UID, Gid: c.GID}
	}
	if len(c.Set_dir) > 0 {
		cmd.Dir = c.Set_dir
	}
	if len(c.Set_env) > 0 {
		cmd.Env = append(cmd.Env, c.Set_env...)
	}

	// http://www.agardner.me/golang/garbage/collection/gc/escape/analysis/2015/10/18/go-escape-analysis.html
	// https://habr.com/ru/company/intel/blog/422447/
	var _stdout, _stderr     bytes.Buffer
	var stdoutIn, stderrIn   io.ReadCloser
	var errStdout, errStderr error

	stdout := io.Writer(&_stdout)
	stderr := io.Writer(&_stderr)
  	if ! c.No_wait { // Otherwise, just fork process with /dev/null at stdout&stderr.
		var perr error
		if stdoutIn, perr = cmd.StdoutPipe(); perr != nil {
			r.Status = -1
			r.StdErr = []byte("stdout pipe: " + perr.Error())
			return r
		}
		if stderrIn, perr = cmd.StderrPipe(); perr != nil {
			r.Status = -1
			r.StdErr = []byte("stderr pipe: " + perr.Error())
			return r
		}
	}

	r.Status = 0
	if c.no_exec {
		r.StdErr = []byte("no_exec")
		return r
	}

	err := cmd.Start()
	r.Processed = true
	if err != nil {
		r.StdErr= []byte(err.Error())
		if exitErr, ok := err.(*exec.ExitError); ok {
			if _status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				r.Status = _status.ExitStatus()
			}
		} else {
			r.Status = -1 // No such command at all?
		}

		return r
	}
//	fmt.Printf("pid = %d\n", cmd.Process.Pid)

	if c.No_wait {
		cmd.Process.Release() // Drop PPID to avoid zombies
		return r
	}

	// Terminate the child - and its whole process group, thanks to Setpgid - if it overruns the
	// timeout. The previous version polled cmd.ProcessState in a goroutine while cmd.Wait() wrote it
	// (a data race that `go test -race` flags), and every "if cmd == nil" guard was dead code because
	// cmd is a non-nil local. A done channel closed on return replaces both: the killer only reads the
	// pid (an int copy) and done, so it never touches the reaped cmd or the reused pgid after Wait.
	pid := cmd.Process.Pid
	done := make(chan struct{})
	defer close(done)
	if c.Timeout > 0 {
		go func() {
			select {
			case <-done:
				return
			case <-time.After(time.Duration(c.Timeout * float64(time.Second))):
			}
			syscall.Kill(-pid, syscall.SIGTERM)
			select {
			case <-done: // the SIGTERM already let cmd.Wait() reap it
			case <-time.After(time.Second / 2):
				syscall.Kill(-pid, syscall.SIGKILL)
			}
		}()
	}
	// Timeout == 0 means no deadline: cmd.Wait() below simply blocks until the child exits on its own.
	// The old zero-timeout goroutine only busy-polled the racy ProcessState and its kill path was
	// unreachable (guarded by ProcessState == nil after Wait already set it), so it is dropped.

	// https://blog.kowalczyk.info/article/wOYk/advanced-command-execution-in-go-with-osexec.html
	maxReply := c.max_reply
	if maxReply <= 0 {
		maxReply = defaultMaxReply
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_, errStdout = io.CopyN(stdout, stdoutIn, maxReply)
		if errStdout == io.EOF {
			errStdout = nil
		} else {
			io.Copy(ioutil.Discard, stdoutIn) // Discard all the rest of stdout
		}
		wg.Done()
	} ()
	_, errStderr = io.CopyN(stderr, stderrIn, maxReply)
	if errStderr == io.EOF {
		errStderr = nil
	} else {
		io.Copy(ioutil.Discard, stderrIn) // Discard all the rest of stderr
	}
	wg.Wait()

	err = cmd.Wait()
	if err != nil {
		r.StdErr = []byte(err.Error())
		if exitErr, ok := err.(*exec.ExitError); ok {
			if _status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				r.Status = _status.ExitStatus()
			}
		} else {
			r.Status = -1 // No such command at all?
		}

	}

	if errStdout != nil {
		r.StdOut = []byte(errStdout.Error())
		r.Status = -254
	}
	if errStderr != nil {
		r.StdErr = []byte(errStderr.Error())
		r.Status = -255
	}

	r.StdOut = _stdout.Bytes()
	r.StdErr = _stderr.Bytes()
	return r
}
