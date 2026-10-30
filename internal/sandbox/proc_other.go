//go:build !unix

package sandbox

import (
	"os/exec"
	"syscall"
)

const (
	listSep = ";"
	sep     = "\\"
)

func setProcessGroup(cmd *exec.Cmd) {}

func signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}

func exitStatus(ee *exec.ExitError) int { return ee.ExitCode() }
