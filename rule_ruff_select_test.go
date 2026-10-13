package jactionlint

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSelectDoesNotRunRuff: --select without pyflakes does not start ruff either, so a ruff which fails cannot fail the run.
func TestSelectDoesNotRunRuff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake ruff is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ruff"), []byte("#!/bin/sh\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	f := ruffProject(t, "print(1)\n")
	if status, out, stderr := runRuffCommand(t, "--ruff=ruff", "--select=missing-timeout", f); status == ExitStatusFailure {
		t.Errorf("ruff was started although pyflakes is not selected: status %d %s %s", status, out, stderr)
	}
	// Selected, it runs, and its failure is an error of the run
	if status, _, _ := runRuffCommand(t, "--ruff=ruff", "--select=pyflakes", f); status != ExitStatusFailure {
		t.Errorf("with pyflakes selected the failing ruff must fail the run: status %d", status)
	}
}
