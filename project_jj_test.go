package jactionlint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectsFindJujutsuWorkspace(t *testing.T) {
	// A secondary workspace of Jujutsu has ".jj" and no ".git"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".jj", "repo"), 0o750); err != nil {
		t.Fatal(err)
	}
	wf := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(wf, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wf, "ci.yaml"), []byte("on: push\njobs: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs := absPath(root)
	for _, path := range []string{root, wf, filepath.Join(wf, "ci.yaml")} {
		p, err := NewProjects().At(path)
		if err != nil {
			t.Fatal(err)
		}
		if p == nil || p.RootDir() != abs {
			t.Errorf("the workspace at %s is not found from %s: %v", abs, path, p)
		}
	}

	// Without either marker there is no project, as before
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, ".github", "workflows"), 0o750); err != nil {
		t.Fatal(err)
	}
	if p, err := NewProjects().At(filepath.Join(other, ".github", "workflows")); err != nil || p != nil {
		t.Errorf("a directory with neither .git nor .jj must not be a project: %v, %v", p, err)
	}
}
