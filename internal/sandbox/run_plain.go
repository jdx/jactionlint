package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// RunUnrestricted runs the command of the policy without any restriction (argv, env and dir only; the allow lists and
// the network mode are ignored). It is what --sandbox=off does, with the same process-group handling as Run: when the
// context ends, the group gets SIGTERM and then SIGKILL.
func RunUnrestricted(ctx context.Context, p *Policy, io IO) (int, error) {
	if len(p.Argv) == 0 {
		return 0, &SetupError{"policy has no command"}
	}
	path, err := lookPathIn(p.Argv[0], p.Env)
	if err != nil {
		return 0, &SetupError{err.Error()}
	}
	cmd := exec.Command(path, p.Argv[1:]...)
	cmd.Env = p.Env
	cmd.Dir = p.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = io.Stdin, io.Stdout, io.Stderr
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return 0, &SetupError{fmt.Sprintf("cannot start %s: %v", path, err)}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			signalGroup(cmd, syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(killGrace):
				signalGroup(cmd, syscall.SIGKILL)
			}
		case <-done:
		}
	}()
	waitErr := cmd.Wait()
	close(done)
	signalGroup(cmd, syscall.SIGKILL)
	var ee *exec.ExitError
	switch {
	case waitErr == nil:
		return 0, nil
	case errors.As(waitErr, &ee):
		return exitStatus(ee), nil
	}
	return 0, waitErr
}

// lookPathIn finds a program in the PATH of env (not of this process).
func lookPathIn(name string, env []string) (string, error) {
	if strings.ContainsAny(name, "/\\") {
		return name, nil
	}
	pathVar := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			pathVar = v
		}
	}
	for _, dir := range strings.Split(pathVar, listSep) {
		if dir == "" {
			continue
		}
		if p, err := exec.LookPath(dir + sep + name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s: command not found in PATH", name)
}
