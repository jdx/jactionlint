package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// A commit SHA written in uses: is a ref the lock file has no entry for, so for a workflow which the lock
// file lists the online fix which pins a tag to a SHA is not offered. It is for a workflow which is not listed.
func TestActionsLockNoPinFixInLockedWorkflow(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n        - 'actions/checkout@v4.2.2'\n"+
		"dependencies:\n    'actions/checkout@v4.2.2':\n        ref: 'v4.2.2'\n        commit: 'sha1-"+pinCheckoutV422+"'\n        owner_id: 1\n        repo_id: 2\n")
	src := workflowWith("uses: actions/checkout@v4.2.2", "uses: actions/setup-node@v4")
	proj := &Project{root: root}

	lint := func(name string) []*Error {
		t.Helper()
		l, err := NewLinter(io.Discard, &LinterOptions{Online: true, GitHubClient: onlineFixtureClient(t), LogWriter: io.Discard, WorkingDir: root})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = unpinnedConfig()
		errs, err := l.Lint(filepath.Join(root, ".github", "workflows", name), []byte(src), proj)
		if err != nil {
			t.Fatal(err)
		}
		return errs
	}

	// The listed workflow: the pinned action is accepted, the other has no fix, and it says why
	errs := lint("ci.yml")
	if len(errs) != 1 || errs[0].ID != "unpinned-uses" || errs[0].Line != 7 {
		t.Fatalf("want one finding for setup-node: %v", lineIDsOf(errs))
	}
	if errs[0].Fix != nil {
		t.Errorf("no fix for a workflow of the lock file: %+v", errs[0].Fix)
	}
	if nf := errs[0].NoFix; nf == nil || nf.Code != NoFixNeedsJudgment || !strings.Contains(nf.Reason, "gh actions-lock") {
		t.Errorf("the reason must name the tool which adds the entry: %+v", nf)
	}

	// A workflow which the lock file does not list gets the fix of every tag
	errs = lint("other.yml")
	fixes := 0
	for _, e := range errs {
		if e.ID == "unpinned-uses" && e.Fix != nil {
			fixes++
		}
	}
	if fixes != 2 {
		t.Errorf("want 2 fixes in the workflow which is not listed, got %d: %v", fixes, lineIDsOf(errs))
	}
}

// With "actions-lock: false" the lock file is ignored, also by the pin fix: the workflow is fixed as if it was not listed.
func TestActionsLockOptionOffKeepsThePinFix(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n        - 'actions/checkout@v4.2.2'\n"+
		"dependencies:\n    'actions/checkout@v4.2.2':\n        ref: 'v4.2.2'\n        commit: 'sha1-"+pinCheckoutV422+"'\n        owner_id: 1\n        repo_id: 2\n")
	l, err := NewLinter(io.Discard, &LinterOptions{Online: true, GitHubClient: onlineFixtureClient(t), LogWriter: io.Discard, WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = mustParseConfigFixture(t, "rules:\n  unpinned-uses:\n    level: error\n    actions-lock: false\n")
	errs, err := l.Lint(filepath.Join(root, ".github", "workflows", "ci.yml"), []byte(workflowWith("uses: actions/setup-node@v4")), &Project{root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].Fix == nil {
		t.Fatalf("the finding must carry the pin fix: %+v", errs)
	}
}
