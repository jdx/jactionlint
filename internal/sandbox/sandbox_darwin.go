package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const sandboxExec = "/usr/bin/sandbox-exec"

// Available reports whether this machine can apply the sandbox, and if not, why.
func Available(net Net) error {
	if _, err := os.Stat(sandboxExec); err != nil {
		return fmt.Errorf("%s is missing; the macOS sandbox needs it", sandboxExec)
	}
	return nil
}

// Describe says which restrictions are in force on this machine, for diagnostics.
func Describe() string {
	if err := Available(NetOpen); err != nil {
		return "macOS: " + err.Error()
	}
	return "macOS: Seatbelt (sandbox-exec)"
}

func prepare(p *Policy) (*launch, error) {
	if err := Available(p.Net); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "jactionlint-seatbelt-")
	if err != nil {
		return nil, err
	}
	profile := filepath.Join(dir, "profile.sb")
	if err := os.WriteFile(profile, []byte(seatbeltProfile(p)), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	argv := append([]string{"-f", profile}, p.Argv...)
	cmd := exec.Command(sandboxExec, argv...)
	cmd.Env = p.Env
	cmd.Dir = p.Dir
	return &launch{
		cmd:        cmd,
		afterStart: func() {},
		status:     func() string { return "" },
		cleanup:    func() { os.RemoveAll(dir) },
	}, nil
}
