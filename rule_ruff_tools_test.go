package jactionlint

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRuffIsTheToolOfThePythonScripts: with --ruff the status of the Python scripts is about ruff.
func TestRuffIsTheToolOfThePythonScripts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake ruff is a shell script")
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat is not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ruff"), []byte("#!/bin/sh\n"+cat+" >/dev/null\necho '[]'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	f := ruffProject(t, "print(1)\n")
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Ruff: "ruff", Pyflakes: "pyflakes", PyflakesRequired: true})
	if err != nil {
		t.Fatalf("pyflakes is not needed when ruff replaces it: %v", err)
	}
	if _, err := l.LintFile(f, nil); err != nil {
		t.Fatal(err)
	}
	got := l.ExternalTools()["pyflakes"]
	if got.Status != ExternalToolRan || got.Command != "ruff" || got.Scripts != 1 {
		t.Errorf("status of the Python scripts = %+v", got)
	}
}
