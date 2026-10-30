package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// checkSandbox implements `run --check-sandbox`: it says what the sandbox can do on this machine and proves it by
// running a command in it, once reading a file it may read and once reading one it may not.
func checkSandbox(stdout, stderr io.Writer) int {
	fmt.Fprintln(stdout, sandbox.Describe())
	if err := sandbox.Available(sandbox.NetOpen); err != nil {
		fmt.Fprintf(stderr, "jactionlint run: the sandbox is not available: %v\n", err)
		return ExitError
	}
	base, err := os.MkdirTemp("", "jactionlint-check-")
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitError
	}
	defer os.RemoveAll(base)
	// The directory next to the work area is not allowed; the work area is.
	work := filepath.Join(base, "work")
	forbidden := filepath.Join(os.Getenv("HOME"), ".jactionlint-check-secret")
	if os.Getenv("HOME") == "" {
		forbidden = filepath.Join(filepath.Dir(base), ".jactionlint-check-secret")
	}
	if err := os.WriteFile(forbidden, []byte("secret"), 0o600); err != nil {
		fmt.Fprintf(stderr, "jactionlint run: cannot create the probe file: %v\n", err)
		return ExitError
	}
	defer os.Remove(forbidden)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitError
	}

	run := func(script string) (string, int, error) {
		pol := sandbox.DefaultPolicy(sandbox.Options{
			Workspace: work, Home: work, Temp: work, ToolCache: work,
			PathDirs: sandbox.PathDirs(os.Getenv("PATH")), Net: sandbox.NetOpen,
		})
		pol.Argv = []string{"sh", "-c", script}
		pol.Env = sandbox.ScrubEnv(os.Environ(), sandbox.DefaultPassEnv, nil)
		pol.Dir = work
		var out bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		code, err := sandbox.Run(ctx, pol, sandbox.IO{Stdout: &out, Stderr: &out})
		return strings.TrimSpace(out.String()), code, err
	}
	out, code, err := run("echo hello > probe.txt && cat probe.txt")
	if err != nil || code != 0 || out != "hello" {
		fmt.Fprintf(stderr, "jactionlint run: a command could not run in the sandbox (status %d, %v): %s\n", code, err, out)
		return ExitError
	}
	fmt.Fprintln(stdout, "ok: a command can write to its workspace")
	out, code, err = run("cat " + forbidden)
	if err != nil || code == 0 {
		fmt.Fprintf(stderr, "jactionlint run: the sandbox did NOT stop a command from reading %s (%v): %s\n", forbidden, err, out)
		return ExitError
	}
	fmt.Fprintln(stdout, "ok: a command cannot read a file in your home directory that is outside the workspace")
	return ExitOK
}
