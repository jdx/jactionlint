//go:build linux || darwin

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// IO connects the command to the caller.
type IO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// SetupError means the policy could not be applied, so the command did not run.
type SetupError struct{ Reason string }

func (e *SetupError) Error() string { return "sandbox: " + e.Reason }

// launch is a prepared start of the command inside the sandbox (see prepare in the file of each OS).
type launch struct {
	cmd *exec.Cmd
	// afterStart closes the parent's copies of the descriptors given to the child.
	afterStart func()
	// status blocks until the child has applied the sandbox and started the command, and returns why it could not.
	status  func() string
	cleanup func()
}

// Run starts the command of the policy inside the sandbox and waits for it. It returns the exit status of the command
// (128+n when a signal n ended it). An error that is a *SetupError means the sandbox could not be applied and nothing ran.
func Run(ctx context.Context, p *Policy, io IO) (int, error) {
	if p.Version == 0 {
		p.Version = PolicyVersion
	}
	if err := p.Validate(); err != nil {
		return 0, &SetupError{err.Error()}
	}
	// The kernel ties PR_SET_PDEATHSIG to the thread that started the child: keep this goroutine on one thread
	// until the child has exited.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	l, err := prepare(p)
	if err != nil {
		return 0, &SetupError{err.Error()}
	}
	defer l.cleanup()
	cmd := l.cmd
	cmd.Stdin, cmd.Stdout, cmd.Stderr = io.Stdin, io.Stdout, io.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return 0, &SetupError{fmt.Sprintf("cannot start the command: %v", err)}
	}
	l.afterStart()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(killGrace):
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()
	reason := l.status()
	waitErr := cmd.Wait()
	close(done)
	// Kill what the command left behind in its process group.
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if reason != "" {
		return 0, &SetupError{reason}
	}
	var ee *exec.ExitError
	switch {
	case waitErr == nil:
		return 0, nil
	case errors.As(waitErr, &ee):
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return ee.ExitCode(), nil
	}
	return 0, waitErr
}
