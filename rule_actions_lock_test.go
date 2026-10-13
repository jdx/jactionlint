package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lintLockRepository lints the files of the repository the way the repository mode does: the workflows, the
// actions and the lock file.
func lintLockRepository(t *testing.T, root string, cfg *Config, opts LinterOptions) []*Error {
	t.Helper()
	opts.WorkingDir = root
	l, err := NewLinter(io.Discard, &opts)
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	proj := &Project{root: root}
	files, err := projectWorkflowFiles(proj.WorkflowsDir())
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, proj.actionsLockFiles()...)
	files = append(files, l.callGraphOf(proj).actionPaths()...)
	errs, err := l.LintFiles(files, proj)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func TestActionsLockCheck(t *testing.T) {
	pedantic := "rules:\n  actions-lock:\n    level: error\n    pedantic: true\n"
	tests := []struct {
		name string
		repo string
		cfg  *Config
	}{
		{"check_ok", "check_ok", fixtureConfig("actions-lock", "unpinned-uses")},
		{"check_ok_pedantic", "check_ok", mustParseConfigFixture(t, pedantic+"  unpinned-uses: error\n")},
		{"check_err", "check_err", fixtureConfig("actions-lock")},
		{"check_err_pedantic", "check_err", mustParseConfigFixture(t, pedantic)},
		{"check_gone", "check_gone", fixtureConfig("actions-lock")},
		// A repeated key: the file is reported and not trusted, so the tag is reported by unpinned-uses as well
		{"check_dup", "check_dup", fixtureConfig("actions-lock", "unpinned-uses")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintLockRepository(t, filepath.Join("testdata", "actions_lock", tc.repo), tc.cfg, LinterOptions{})
			checkErrors(t, filepath.Join("testdata", "actions_lock", tc.name+".out"), errs)
		})
	}
}

// The pins are compared with the recorded answers of GitHub in testdata/online/github.json.
func TestActionsLockDriftOnline(t *testing.T) {
	root := filepath.Join("testdata", "actions_lock", "check_online")
	cfg := mustParseConfigFixture(t, "rules:\n  actions-lock-drift: error\n  impostor-commit: off\n  stale-action-refs: off\n  ref-version-mismatch: off\n  ref-confusion: off\n  archived-uses: off\n  known-vulnerable-actions: off\n")
	errs := lintLockRepository(t, root, cfg, LinterOptions{Online: true, GitHubClient: onlineFixtureClient(t), LogWriter: io.Discard})
	checkErrors(t, filepath.Join("testdata", "actions_lock", "check_online.out"), errs)

	// Without the online checks nothing is looked up, and so nothing is reported
	for _, e := range lintLockRepository(t, root, cfg, LinterOptions{}) {
		t.Errorf("offline: %v", e)
	}
}

// A repository without a lock file is never reported, whatever the workflows use.
func TestActionsLockCheckNeverFiresWithoutLockFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n")
	cfg := mustParseConfigFixture(t, "rules:\n  actions-lock:\n    level: error\n    pedantic: true\n  actions-lock-drift: error\n")
	for _, e := range lintLockRepository(t, root, cfg, LinterOptions{}) {
		t.Errorf("unexpected finding: %v", e)
	}
}

// Linting a lock file which was not read from the disk (the content was given) works, and so does a path which is not in a project.
func TestActionsLockCheckGivenContent(t *testing.T) {
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig("actions-lock")
	errs, err := l.Lint(".github/workflows/actions.lock", []byte("version: 'v0.0.3'\nworkflows: [\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].ID != "actions-lock" || !strings.Contains(errs[0].Message, "not valid YAML") {
		t.Fatalf("%v", errs)
	}
	for _, src := range []string{"", "- a\n", "version: 1\n", "version: 'v0.1.0'\nworkflows: 3\n", "version: 'v0.0.1'\nworkflows: {}\n"} {
		errs, err = l.Lint(".github/workflows/actions.lock", []byte(src), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%q: %v", src, lineIDsOf(errs))
	}
}

// The repository mode lints the lock file with the workflows, and only when there is one.
func TestActionsLockIsAFileOfTheRepository(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n")
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := l.repositoryFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "actions.lock") {
			t.Fatalf("no lock file exists, but %s is linted", f)
		}
	}
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), "version: 'v0.0.3'\n")
	files, _, err = l.repositoryFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		found = found || strings.HasSuffix(f, filepath.Join(".github", "workflows", "actions.lock"))
	}
	if !found {
		t.Fatalf("the lock file is not among %v", files)
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "actions.lock")); err != nil {
		t.Fatal(err)
	}
}
