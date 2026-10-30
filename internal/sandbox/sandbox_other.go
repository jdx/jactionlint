//go:build !linux && !darwin

package sandbox

import (
	"context"
	"fmt"
	"io"
	"runtime"
)

// IO connects the command to the caller.
type IO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// SetupError means the policy could not be applied, so the command did not run.
type SetupError struct{ Reason string }

func (e *SetupError) Error() string { return "sandbox: " + e.Reason }

// Available reports why the sandbox cannot be used: this operating system has no backend yet.
func Available(Net) error {
	return fmt.Errorf("the sandbox is not implemented for %s; run in WSL2, use --container, or pass --sandbox=off", runtime.GOOS)
}

// Describe says which restrictions are in force on this machine.
func Describe() string { return runtime.GOOS + ": " + Available(NetOpen).Error() }

// Run refuses: there is no sandbox to run the command in.
func Run(context.Context, *Policy, IO) (int, error) {
	return 0, &SetupError{Available(NetOpen).Error()}
}
