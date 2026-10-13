package jactionlint

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseChangedDiff(t *testing.T) {
	const diff = `diff --git a/.github/workflows/a.yaml b/.github/workflows/a.yaml
index 1111111..2222222 100644
--- a/.github/workflows/a.yaml
+++ b/.github/workflows/a.yaml
@@ -3 +3 @@ jobs:
-    old
+    new
@@ -10,2 +10,0 @@ jobs:
-    gone
-    gone too
@@ -20,0 +19,3 @@ jobs:
+    one
+    two
+    three
diff --git a/new.yaml b/new.yaml
new file mode 100644
--- /dev/null
+++ b/new.yaml
@@ -0,0 +1,2 @@
+a
+b
diff --git a/removed.yaml b/removed.yaml
deleted file mode 100644
--- a/removed.yaml
+++ /dev/null
@@ -1,2 +0,0 @@
-a
-b
diff --git a/with space.yaml b/with space.yaml
--- a/with space.yaml
+++ b/with space.yaml
@@ -1 +1 @@
-x
+y
`
	got := parseChangedDiff(diff)
	for name, want := range map[string][][2]int{
		".github/workflows/a.yaml": {{3, 3}, {10, 11}, {19, 21}},
		"new.yaml":                 {{1, 2}},
		"with space.yaml":          {{1, 1}},
	} {
		c := got[name]
		if c == nil {
			t.Errorf("%s is not in %v", name, got)
			continue
		}
		if d := cmp.Diff(want, c.ranges); d != "" {
			t.Errorf("%s (-want +got): %s", name, d)
		}
	}
	if got["removed.yaml"] != nil {
		t.Errorf("a deleted file has no changed lines: %+v", got["removed.yaml"])
	}
	c := got[".github/workflows/a.yaml"]
	for line, want := range map[int]bool{2: false, 3: true, 4: false, 10: true, 11: true, 12: false, 19: true, 21: true, 22: false} {
		if c.touches(line, line) != want {
			t.Errorf("line %d touched: %v, want %v", line, !want, want)
		}
	}
	if !c.touches(1, 3) || c.touches(5, 9) {
		t.Errorf("a region that spans lines is changed when one of them is")
	}
}

type gitRepoForChanged struct {
	t   *testing.T
	dir string
}

func (r gitRepoForChanged) git(args ...string) string {
	r.t.Helper()
	full := append([]string{"-C", r.dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = cleanGitEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func (r gitRepoForChanged) write(name, content string) {
	r.t.Helper()
	p := filepath.Join(r.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

const changedWorkflowA = `on: push
permissions: {}
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo a
`

// newChangedRepo makes a repository with a workflow on the branch main, and a branch feature that adds a job to it.
func newChangedRepo(t *testing.T) gitRepoForChanged {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	r := gitRepoForChanged{t: t, dir: t.TempDir()}
	r.git("init", "-q")
	r.git("checkout", "-q", "-b", "main")
	r.write(".github/workflows/a.yaml", changedWorkflowA)
	r.git("add", ".")
	r.git("commit", "-q", "-m", "base")
	r.git("checkout", "-q", "-b", "feature")
	r.write(".github/workflows/a.yaml", changedWorkflowA+"  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo b\n")
	r.git("commit", "-q", "-am", "add job b")
	return r
}

func lintChanged(t *testing.T, r gitRepoForChanged, scope ChangedScope, format string, since string) ([]*Error, string, error) {
	t.Helper()
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{
		WorkingDir: r.dir, Format: format, ChangedSince: since, ChangedScope: scope, LogWriter: &out,
		Config: mustParseConfig(t, "profile: correctness\nrules:\n  missing-timeout: error\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	errs, err := l.LintRepository(r.dir)
	return errs, out.String(), err
}

func missingTimeoutLines(errs []*Error) string {
	var ids []string
	for _, e := range errs {
		if e.ID == "missing-timeout" {
			ids = append(ids, strings.Join([]string{filepath.ToSlash(e.Filepath), itoa(e.Line)}, ":"))
		}
	}
	return strings.Join(ids, " ")
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestChangedSinceShowsTheChangedLinesOnly(t *testing.T) {
	r := newChangedRepo(t)

	all, _, err := lintChanged(t, r, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := missingTimeoutLines(all); !strings.Contains(got, "a.yaml:4") || !strings.Contains(got, "a.yaml:8") {
		t.Fatalf("a full run reports both jobs: %q", got)
	}

	errs, out, err := lintChanged(t, r, "", "", "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := missingTimeoutLines(errs); !strings.HasSuffix(got, "a.yaml:8") || strings.Contains(got, "a.yaml:4") {
		t.Errorf("only the job that was added is shown: %q", got)
	}
	if !strings.Contains(out, "note: ") || !strings.Contains(out, "are not shown (--changed-since)") || !strings.Contains(out, "outside the changed lines since main") {
		t.Errorf("the text output says what was left out:\n%s", out)
	}

	// The whole file when the scope is files
	errs, out, err = lintChanged(t, r, ChangedScopeFiles, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := missingTimeoutLines(errs); !strings.Contains(got, "a.yaml:4") || !strings.Contains(got, "a.yaml:8") {
		t.Errorf("the scope files shows every finding of the file: %q", got)
	}
	if strings.Contains(out, "are not shown") {
		t.Errorf("nothing was left out:\n%s", out)
	}
}

func TestChangedSinceCountsTheWorkingTree(t *testing.T) {
	r := newChangedRepo(t)
	// Not committed: a change of the first job, a new file, and a file that is staged only
	r.write(".github/workflows/a.yaml", strings.Replace(changedWorkflowA, "echo a", "echo changed", 1)+"  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo b\n")
	r.write(".github/workflows/new.yaml", changedWorkflowA)
	r.write(".github/workflows/staged.yaml", changedWorkflowA)
	r.git("add", ".github/workflows/staged.yaml")

	errs, _, err := lintChanged(t, r, "", "", "main")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range errs {
		if e.ID == "missing-timeout" {
			got[filepath.ToSlash(e.Filepath)+":"+itoa(e.Line)] = true
		}
	}
	for _, want := range []string{".github/workflows/new.yaml:4", ".github/workflows/staged.yaml:4"} {
		if !got[want] {
			t.Errorf("%s: a file that is not committed is all changed (%v)", want, got)
		}
	}
	if got[".github/workflows/a.yaml:4"] {
		t.Errorf("the finding on the job line, which did not change, is shown (%v)", got)
	}
}

func TestChangedSinceFailsLoudly(t *testing.T) {
	r := newChangedRepo(t)
	_, _, err := lintChanged(t, r, "", "", "no-such-branch")
	if err == nil || !strings.Contains(err.Error(), "does not know the revision") {
		t.Errorf("an unknown revision is an error, not an empty report: %v", err)
	}
	if _, _, err := lintChanged(t, r, "", "", "--output=x"); err == nil {
		t.Errorf("a revision that looks like an option is refused")
	}
	if _, err := NewLinter(&bytes.Buffer{}, &LinterOptions{ChangedSince: "main", ChangedScope: "everything"}); err == nil {
		t.Errorf("an unknown scope is refused")
	}
}

func TestChangedSinceSARIFSaysSo(t *testing.T) {
	r := newChangedRepo(t)
	_, out, err := lintChanged(t, r, "", FormatSARIF, "main")
	if err != nil {
		t.Fatal(err)
	}
	run := sarifRunOf(t, out)
	props, _ := run["properties"].(sarifDoc)
	cs, _ := props["changedSince"].(sarifDoc)
	if cs["since"] != "main" || cs["scope"] != "lines" || cs["hidden"] != float64(1) {
		t.Errorf("properties.changedSince: %v", props)
	}
	// A run without the option has no such property
	_, out, _ = lintChanged(t, r, "", FormatSARIF, "")
	if p, _ := sarifRunOf(t, out)["properties"].(sarifDoc); p["changedSince"] != nil {
		t.Errorf("a full run has changedSince: %v", p)
	}
	// JSON and the other formats print no note: a program reads them
	_, out, _ = lintChanged(t, r, "", FormatJSON, "main")
	if strings.Contains(out, "--changed-since") {
		t.Errorf("the JSON output has a note:\n%s", out)
	}
}

func TestChangedSinceCommand(t *testing.T) {
	r := newChangedRepo(t)
	t.Chdir(r.dir)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		status := cmd.Main(append([]string{"jactionlint", "--no-color", "--profile", "correctness", "--select", "missing-timeout", "--shellcheck=", "--pyflakes=", "--format", "oneline"}, args...))
		return status, out.String(), errOut.String()
	}
	status, out, _ := run()
	if status != ExitStatusSuccessProblemFound || strings.Count(out, "[missing-timeout]") != 2 {
		t.Fatalf("a full run: status %d\n%s", status, out)
	}
	status, out, stderr := run("--changed-since", "main")
	if status != ExitStatusSuccessProblemFound || strings.Count(out, "[missing-timeout]") != 1 || !strings.Contains(stderr, "note: 1 finding(s) outside the changed lines since main") {
		t.Fatalf("--changed-since: status %d\n%s\n%s", status, out, stderr)
	}
	// Nothing changed since the branch itself: no finding, and the exit status says so
	status, out, _ = run("--changed-since", "feature")
	if status != ExitStatusSuccessNoProblem || strings.Contains(out, "missing-timeout") {
		t.Errorf("nothing changed: status %d\n%s", status, out)
	}
	if status, _, stderr := run("--changed-since", "nope"); status != ExitStatusFailure || !strings.Contains(stderr, "does not know the revision") {
		t.Errorf("an unknown revision: status %d: %s", status, stderr)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--changed-scope", "files"}, "only with --changed-since"},
		{[]string{"--changed-since", "main", "--changed-scope", "all"}, "invalid value"},
		{[]string{"--changed-since", "main", "--fix"}, "cannot be combined"},
		{[]string{"--changed-since", "main", "--diff"}, "cannot be combined"},
		{[]string{"--changed-since", "main", "-"}, "stdin"},
		{[]string{"--changed-since="}, "needs a revision"},
	} {
		if status, _, stderr := run(c.args...); status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, c.want) {
			t.Errorf("%v: status %d: %s", c.args, status, stderr)
		}
	}
}

// The checks still see the whole repository: a finding on a changed line of a reusable workflow is judged by the events
// of its callers, which did not change, and says so. A change of the caller alone is not shown at the unchanged line of
// the callee that it makes a finding: that is what a filter by line does, and a full run shows it.
func TestChangedSinceKeepsTheCallerEffects(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	const callee = "on:\n  workflow_call:\njobs:\n  j:\n    runs-on: ubuntu-latest\n%s    steps:\n      - run: echo\n"
	const guard = "    if: github.event.sender.id == 29139614\n"
	caller := func(on string) string {
		return "on: " + on + "\njobs:\n  c:\n    uses: ./.github/workflows/reusable.yml\n"
	}
	cfg := mustParseConfig(t, "profile: correctness\nrules:\n  bot-conditions: warn\n")
	lint := func(r gitRepoForChanged, scope ChangedScope) []*Error {
		since := "main"
		if scope == "full" {
			since, scope = "", ""
		}
		l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{WorkingDir: r.dir, ChangedSince: since, ChangedScope: scope, Config: cfg})
		if err != nil {
			t.Fatal(err)
		}
		errs, err := l.LintRepository(r.dir)
		if err != nil {
			t.Fatal(err)
		}
		return errs
	}
	newRepo := func(baseCallee, base string) gitRepoForChanged {
		r := gitRepoForChanged{t: t, dir: t.TempDir()}
		r.git("init", "-q")
		r.git("checkout", "-q", "-b", "main")
		r.write(".github/workflows/reusable.yml", baseCallee)
		r.write(".github/workflows/a.yml", base)
		r.git("add", ".")
		r.git("commit", "-q", "-m", "base")
		r.git("checkout", "-q", "-b", "feature")
		return r
	}

	// The guard is added to the callee, which pull requests call
	r := newRepo(strings.Replace(callee, "%s", "", 1), caller("pull_request"))
	r.write(".github/workflows/reusable.yml", strings.Replace(callee, "%s", guard, 1))
	errs := lint(r, "")
	if len(errs) != 1 || errs[0].ID != "bot-conditions" || !strings.Contains(errs[0].Message, "github.event.pull_request.user.id") {
		t.Fatalf("the finding of the changed line uses the events of the caller: %v", errs)
	}

	// Only the caller changes, from push to a pull request: the guard of the callee did not change
	r = newRepo(strings.Replace(callee, "%s", guard, 1), caller("push"))
	r.write(".github/workflows/a.yml", caller("pull_request"))
	if errs := lint(r, ""); len(errs) != 0 {
		t.Errorf("the line of the callee did not change: %v", errs)
	}
	if errs := lint(r, ChangedScopeFiles); len(errs) != 0 {
		t.Errorf("the callee did not change: %v", errs)
	}
	if errs := lint(r, "full"); len(errs) != 1 || !strings.Contains(errs[0].Message, "github.event.pull_request.user.id") {
		t.Errorf("a full run reports the guard, which the pull request caller now makes a finding: %v", errs)
	}
}
