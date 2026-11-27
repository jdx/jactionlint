package jactionlint

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixLockRepository copies a fixture repository and fixes its workflows and its lock file. It returns the
// lock file after the fixes and the findings which remain.
func fixLockRepository(t *testing.T, fixture string, cfg *Config) (string, *FixResult) {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "actions_lock", fixture))); err != nil {
		t.Fatal(err)
	}
	return fixLockRoot(t, root, cfg)
}

func fixLockRoot(t *testing.T, root string, cfg *Config) (string, *FixResult) {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root, LogWriter: io.Discard})
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
	res, err := l.FixFilesWithOptions(files, proj, FixOptions{Mode: FixModeSafe})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Failures {
		t.Errorf("fix refused: %+v", f)
	}
	b, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "actions.lock"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b), res
}

// The entries which nothing uses are removed and the rest of the file is left as it was. Fixing again changes nothing.
func TestActionsLockFixPrunes(t *testing.T) {
	for _, name := range []string{"check_err", "check_gone", "docs_example"} {
		t.Run(name, func(t *testing.T) {
			got, res := fixLockRepository(t, name, fixtureConfig("actions-lock"))
			want, err := os.ReadFile(filepath.Join("testdata", "actions_lock", name+".fixed"))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("lock file after the fixes:\n%s\nwant:\n%s", got, want)
			}
			if res.ByRule["actions-lock"] == 0 {
				t.Errorf("no fix of actions-lock was applied: %+v", res)
			}
			for _, e := range res.Errors {
				if strings.Contains(e.Message, "does not use it") || strings.Contains(e.Message, "is not used by any workflow") || strings.Contains(e.Message, "does not exist") {
					t.Errorf("a stale entry remains: %s", e)
				}
			}
		})
	}
}

// Every finding of the lock file which reports an entry nothing uses carries a fix or says why it has none,
// and the findings which need GitHub say to run gh actions-lock.
func TestActionsLockFixOrNoFix(t *testing.T) {
	errs := lintLockRepository(t, filepath.Join("testdata", "actions_lock", "check_err"), fixtureConfig("actions-lock"), LinterOptions{})
	fixes := 0
	for _, e := range errs {
		if e.Fix == nil && e.NoFix == nil {
			t.Errorf("neither a fix nor a reason: %s", e)
		}
		if e.Fix != nil {
			fixes++
			if e.Fix.Unsafe {
				t.Errorf("removing an entry nothing uses is safe: %s", e)
			}
		}
		if strings.HasSuffix(e.Filepath, ".yml") && (e.NoFix == nil || !strings.Contains(e.NoFix.Reason, "gh actions-lock")) {
			t.Errorf("a missing entry must name gh actions-lock: %s %+v", e, e.NoFix)
		}
	}
	if fixes == 0 {
		t.Error("no finding carries a fix")
	}
}

// A lock file whose version jactionlint does not know is never trusted, so it is never edited either.
func TestActionsLockFixUnknownVersion(t *testing.T) {
	root := t.TempDir()
	lock := "version: 'v9.9.9'\nworkflows:\n    '.github/workflows/gone.yml':\n        - 'actions/checkout@v4'\ndependencies: {}\n"
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), lock)
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n")
	got, res := fixLockRoot(t, root, fixtureConfig("actions-lock"))
	if got != lock || res.Applied != 0 {
		t.Fatalf("the lock file of an unknown version was changed (%d fixes):\n%s", res.Applied, got)
	}
}

// CRLF line ends are kept, and the whole lines of the entries are removed.
func TestActionsLockFixCRLF(t *testing.T) {
	root := t.TempDir()
	lf := "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n        - 'actions/checkout@v4'\n    '.github/workflows/gone.yml':\n        - 'actions/checkout@v4'\n" +
		"dependencies:\n    'actions/checkout@v4':\n        ref: 'v4'\n        commit: 'sha1-11d5960a326750d5838078e36cf38b85af677262'\n        owner_id: 44036562\n        repo_id: 197814629\n"
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), strings.ReplaceAll(lf, "\n", "\r\n"))
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n")
	got, _ := fixLockRoot(t, root, fixtureConfig("actions-lock"))
	want := strings.ReplaceAll(strings.Replace(lf, "    '.github/workflows/gone.yml':\n        - 'actions/checkout@v4'\n", "", 1), "\n", "\r\n")
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// An entry written in the flow style is not rewritten: the finding says why it has no fix.
func TestActionsLockFixFlowStyle(t *testing.T) {
	root := t.TempDir()
	lock := "version: 'v0.0.3'\nworkflows: {'.github/workflows/gone.yml': ['actions/checkout@v4']}\ndependencies: {}\n"
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), lock)
	errs := lintLockRepository(t, root, fixtureConfig("actions-lock"), LinterOptions{})
	gone := 0
	for _, e := range errs {
		if strings.Contains(e.Message, "gone.yml\" does not exist") {
			gone++
			if e.Fix != nil || e.NoFix == nil || e.NoFix.Code != NoFixUnsupportedShape {
				t.Errorf("a flow collection must not be edited: %s fix=%+v nofix=%+v", e, e.Fix, e.NoFix)
			}
		}
	}
	if gone == 0 {
		t.Fatalf("the gone workflow is not reported: %v", lineIDsOf(errs))
	}
}

// When only some workflows are given, the lock file of their repository is linted with them, once.
func TestWithActionsLockFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	ci := filepath.Join(root, ".github", "workflows", "ci.yml")
	other := filepath.Join(root, ".github", "workflows", "other.yaml")
	action := filepath.Join(root, "act", "action.yml")
	readme := filepath.Join(root, "README.md")
	writeFile(t, ci, "on: push\n")
	writeFile(t, other, "on: push\n")
	writeFile(t, action, "runs:\n  using: node20\n  main: x.js\n")
	writeFile(t, readme, "")
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}

	got, err := l.withActionsLockFiles([]string{ci, action})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{ci, action}) {
		t.Fatalf("no lock file exists, but the files changed: %v", got)
	}

	lock := filepath.Join(root, ".github", "workflows", "actions.lock")
	writeFile(t, lock, "version: 'v0.0.3'\n")
	for _, tc := range []struct {
		files []string
		lock  bool
	}{
		{[]string{ci}, true},
		{[]string{ci, other}, true},
		{[]string{action, readme}, false},
		{[]string{ci, lock}, false},
		{[]string{"-"}, false},
	} {
		got, err := l.withActionsLockFiles(tc.files)
		if err != nil {
			t.Fatal(err)
		}
		want := tc.files
		if tc.lock {
			want = append(slices.Clone(tc.files), lock)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%v: got %v, want %v", tc.files, got, want)
		}
	}

	// A relative path is resolved against the working directory
	got, err = l.withActionsLockFiles([]string{filepath.Join(".github", "workflows", "ci.yml"), filepath.Join(".github", "workflows", "actions.lock")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("the lock file is given already: %v", got)
	}
}

// The case of a hook which passes only the changed files: a workflow was deleted, the lock file still lists it, and
// only another workflow is given. The stale entry is reported, and --fix removes it from the lock file.
func TestCommandLintsTheLockFileOfTheGivenWorkflows(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n")
	lock := filepath.Join(root, ".github", "workflows", "actions.lock")
	writeFile(t, lock, "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n        - 'actions/checkout@v4'\n    '.github/workflows/two.yml':\n        - 'actions/checkout@v4'\n"+
		"dependencies:\n    'actions/checkout@v4':\n        ref: 'v4'\n        commit: 'sha1-11d5960a326750d5838078e36cf38b85af677262'\n        owner_id: 44036562\n        repo_id: 197814629\n")
	t.Chdir(root)
	ci := filepath.Join(".github", "workflows", "ci.yml")

	var out bytes.Buffer
	cmd := Command{Stdin: os.Stdin, Stdout: &out, Stderr: &out}
	if status := cmd.Main([]string{"jactionlint", "--format", "gcc", "--select", "actions-lock", ci}); status != 1 || !strings.Contains(out.String(), `two.yml" does not exist`) {
		t.Fatalf("status %d, the stale entry must be reported: %s", status, out.String())
	}

	out.Reset()
	if status := cmd.Main([]string{"jactionlint", "--fix", "--select", "actions-lock", ci}); status != 0 {
		t.Fatalf("status %d after --fix: %s", status, out.String())
	}
	b, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "two.yml") || !strings.Contains(string(b), "ci.yml") {
		t.Fatalf("the entry of two.yml must be removed and nothing else:\n%s", b)
	}
}

// Shapes of YAML which gh actions-lock does not write but a person can: the fixes keep the file what it was without
// the entries, or are not offered.
func TestActionsLockFixShapes(t *testing.T) {
	const ci = "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	const checkout = "    'actions/checkout@v4':\n        ref: 'v4'\n        commit: 'sha1-11d5960a326750d5838078e36cf38b85af677262'\n        owner_id: 44036562\n        repo_id: 197814629\n"
	const cache = "    'actions/cache@v4':\n        ref: 'v4'\n        commit: 'sha1-0057852bfaa89a56745cba8c7296529d2fc39830'\n        owner_id: 44036562\n        repo_id: 54212171\n"
	for _, tc := range []struct {
		name, lock, want string
	}{
		{
			// The items of a list at the indentation of the key of the workflow belong to it
			"same indent list",
			"version: 'v0.0.3'\nworkflows:\n    '.github/workflows/gone.yml':\n    - 'actions/cache@v4'\n    '.github/workflows/ci.yml':\n    - 'actions/checkout@v4'\ndependencies:\n" + checkout + cache,
			"version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n    - 'actions/checkout@v4'\ndependencies:\n" + checkout,
		},
		{
			// The comments between the entries of a collection which is emptied stay, after the empty value
			"comments of an emptied collection",
			"version: 'v0.0.3'\nworkflows:\n    # onboarded one by one\n    '.github/workflows/gone.yml':\n        - 'actions/cache@v4'\ndependencies:\n    # resolved by gh actions-lock\n" + cache,
			"version: 'v0.0.3'\nworkflows: {}\n    # onboarded one by one\ndependencies: {}\n    # resolved by gh actions-lock\n",
		},
		{
			// The comment of the key line stays after the empty value
			"comment of the key",
			"version: 'v0.0.3'\nworkflows: # one by one\n    '.github/workflows/gone.yml':\n        - 'actions/cache@v4'\ndependencies:   # resolved\n" + cache,
			"version: 'v0.0.3'\nworkflows: {} # one by one\ndependencies: {}   # resolved\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), tc.lock)
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), ci)
			got, res := fixLockRoot(t, root, fixtureConfig("actions-lock"))
			if got != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
			for _, e := range res.Errors {
				t.Errorf("a finding remains: %s", e)
			}
		})
	}
}

// A workflow which exists but cannot be read is not reported as deleted, so its entry is not removed.
func TestActionsLockUnreadableWorkflowIsKept(t *testing.T) {
	root := t.TempDir()
	lock := "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/odd.yml':\n        - 'actions/checkout@v4'\ndependencies:\n    'actions/checkout@v4':\n        ref: 'v4'\n        commit: 'sha1-11d5960a326750d5838078e36cf38b85af677262'\n        owner_id: 44036562\n        repo_id: 197814629\n"
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), lock)
	// A directory where the workflow is: it exists, and reading it fails
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows", "odd.yml"), 0o755); err != nil {
		t.Fatal(err)
	}
	errs := lintLockRepository(t, root, fixtureConfig("actions-lock"), LinterOptions{})
	for _, e := range errs {
		if strings.Contains(e.Message, "does not exist") || strings.Contains(e.Message, "is not used by any workflow") || e.Fix != nil {
			t.Errorf("the entry of a workflow which was not read must stay: %s", e)
		}
	}
}

// An anchor which an alias outside the deleted entry refers to: deleting its line would break the file, so the
// result is read again and the fix is not offered.
func TestActionsLockFixKeepsAnchors(t *testing.T) {
	root := t.TempDir()
	lock := "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/gone.yml': &saved []\ndependencies: {}\nmetadata: *saved\n"
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), lock)
	errs := lintLockRepository(t, root, fixtureConfig("actions-lock"), LinterOptions{})
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "does not exist") {
			found = true
			if e.Fix != nil || e.NoFix == nil || e.NoFix.Code != NoFixUnsupportedShape {
				t.Errorf("the anchor must not be deleted: fix=%+v nofix=%+v", e.Fix, e.NoFix)
			}
		}
	}
	if !found {
		t.Fatalf("the gone workflow is not reported: %v", lineIDsOf(errs))
	}
}

// A finding which an ignore comment drops keeps its entry, and the dependencies the entry uses: the fixes of the
// others delete only their own entries.
func TestActionsLockFixRespectsIgnores(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "actions_lock", "docs_example"))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".github", "workflows", "actions.lock")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := strings.Replace(string(b), "        - 'actions/cache@v4'\n", "        - 'actions/cache@v4' # jactionlint ignore=actions-lock\n", 1)
	writeFile(t, path, src)
	got, _ := fixLockRoot(t, root, fixtureConfig("actions-lock"))
	if !strings.Contains(got, "'actions/cache@v4' # jactionlint ignore=actions-lock") || !strings.Contains(got, "    'actions/cache@v4':\n") {
		t.Errorf("the ignored pin and its dependency must stay:\n%s", got)
	}
	if strings.Contains(got, "checkout") {
		t.Errorf("the pin which is not ignored and its dependency must be removed:\n%s", got)
	}
}

// --baseline-write with only a workflow records the findings of the lock file too, which the next run lints with it.
func TestBaselineWriteRecordsTheLockFile(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "actions_lock", "docs_example"))); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	t.Chdir(root)
	ci := filepath.Join(".github", "workflows", "ci.yml")
	var out bytes.Buffer
	cmd := Command{Stdin: os.Stdin, Stdout: &out, Stderr: &out}
	if status := cmd.Main([]string{"jactionlint", "--select", "actions-lock", "--baseline-write=base.json", ci}); status != 0 {
		t.Fatalf("status %d: %s", status, out.String())
	}
	out.Reset()
	if status := cmd.Main([]string{"jactionlint", "--format", "gcc", "--select", "actions-lock", "--baseline=base.json", ci}); status != 0 {
		t.Fatalf("the baseline must cover the findings of the lock file: status %d: %s", status, out.String())
	}
}
