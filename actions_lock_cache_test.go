package jactionlint

import (
	"io"
	"path/filepath"
	"testing"
)

// The lock file is read once for the run of a Linter and is kept with it, not in a global: another Linter over the same
// project reads the file again, so a change between two runs of a process (a language server, a test) is seen, and
// nothing outlives the Linter.
func TestActionsLockIsScopedToTheLinter(t *testing.T) {
	root := t.TempDir()
	const lockPath = ".github/workflows/actions.lock"
	pinned := "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n        - 'actions/checkout@v6.0.2'\n" +
		"dependencies:\n    'actions/checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 1\n        repo_id: 2\n"
	wf := workflowWith("uses: actions/checkout@v6.0.2")
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), wf)
	writeFile(t, filepath.Join(root, filepath.FromSlash(lockPath)), pinned)
	proj := &Project{root: root} // the same Project for both runs

	lint := func() int {
		t.Helper()
		l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = fixtureConfig("unpinned-uses")
		errs, err := l.LintFile(filepath.Join(root, ".github", "workflows", "ci.yml"), proj)
		if err != nil {
			t.Fatal(err)
		}
		return len(errs)
	}

	if n := lint(); n != 0 {
		t.Fatalf("the lock file pins the action, got %d findings", n)
	}
	writeFile(t, filepath.Join(root, filepath.FromSlash(lockPath)), "version: 'v0.0.3'\n")
	if n := lint(); n != 1 {
		t.Fatalf("the lock file changed and lists nothing now, a new run must read it again: got %d findings", n)
	}
}

// One Linter reads the lock file of a project once, whatever the number of files.
func TestActionsLockIsReadOncePerLinter(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), "version: 'v0.0.3'\n")
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	proj := &Project{root: root}
	a, b := l.actionsLockOf(proj), l.actionsLockOf(proj)
	if a == nil || a != b {
		t.Fatalf("want the same lock for both calls, got %p and %p", a, b)
	}
}
