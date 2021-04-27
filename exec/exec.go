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

type Command struct {
	id string        // Sequence ID may be set. Othewise, it is auto-generated.
	no_exec bool
	uid, gid uint32  // "run_as" linux representation
	UseShell bool
	no_wait bool     // https://golang.org/pkg/os/exec/#Cmd.Start vs "Run()" or m.b. "Wait()"
	timeout float64
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
	if c.uid > 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: c.uid, Gid: c.gid}
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
  	if ! c.no_wait { // Otherwise, just fork process with /dev/null at stdout&stderr.
		stdoutIn, _ = cmd.StdoutPipe()
		stderrIn, _ = cmd.StderrPipe()
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

	if c.no_wait {
		return r
	}

	// Wait for the process to finish or kill it after a timeout (whichever happens first):
	if c.timeout > 0 {
		go func(cmd *exec.Cmd, pid int) { // !!! panic: runtime error: invalid memory address or nil pointer dereference
			for i := 0; i < int(c.timeout * 10); i++ {
				time.Sleep(time.Second / 10)
				if cmd == nil { return }
				if cmd.ProcessState != nil {
					break
				}
			}

			if cmd.ProcessState == nil {
//				cmd.Process.Signal(syscall.SIGTERM) // No, such a method terminates only this PID, leaving orphans!
				if cmd == nil { return }
				syscall.Kill(-pid, syscall.SIGTERM) // !!! [signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x7c1ef0]
				time.Sleep(time.Second / 2)
				if cmd == nil { return }
				syscall.Kill(-pid, syscall.SIGKILL)
			}
		} (cmd, cmd.Process.Pid)
	} else if c.timeout == 0 { // Wait for interrupt
		go func(cmd *exec.Cmd, pid int) {
//			defer waitWorkers.Done()
			for {
				time.Sleep(time.Second / 50)
				if cmd == nil { return }
				if cmd.ProcessState != nil {
					break
				}
/*
				if interrupt {
					break
				}
*/
			}

			if cmd.ProcessState == nil {
//				cmd.Process.Signal(syscall.SIGTERM) // No, such a method terminates only this PID, leaving orphans!
				if cmd == nil { return }
				syscall.Kill(-pid, syscall.SIGTERM) // !!! [signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x7c1ef0]
				time.Sleep(time.Second / 2)
				if cmd == nil { return }
				syscall.Kill(-pid, syscall.SIGKILL)
			}
		} (cmd, cmd.Process.Pid)
	}

	// https://blog.kowalczyk.info/article/wOYk/advanced-command-execution-in-go-with-osexec.html
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_, errStdout = io.CopyN(stdout, stdoutIn, c.max_reply)
		if errStdout == io.EOF {
			errStdout = nil
		} else {
			io.Copy(ioutil.Discard, stdoutIn) // Discard all the rest of stdout
		}
		wg.Done()
	} ()
	_, errStderr = io.CopyN(stderr, stderrIn, c.max_reply)
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
