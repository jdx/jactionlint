package jactionlint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// batchHFixtureOffRules are the rules of batch H that the default profile enables. The fixtures in
// testdata/err and testdata/examples turn them off (see withFixtureRules) unless a ".config" file next
// to the fixture enables them, so that a fixture of another rule is not full of their findings.
var batchHFixtureOffRules = []string{
	"concurrency-cancels-prs",
	"concurrency-cancels-release",
	"gate-job-skipped-on-failure",
	"untrusted-checkout",
	"untrusted-artifact",
	"unused-job-output",
}

// pedanticCfg selects the profile with the pedantic rules of batch H.
const pedanticCfg = "profile: pedantic\n"

// lintBatchH lints the workflow with the configuration (YAML, "" for the default one) and returns
// the errors with the ID given. An empty id returns all of them.
func lintBatchH(t *testing.T, cfg, src, id string) []*Error {
	t.Helper()
	c := &Config{}
	if cfg != "" {
		c = mustParseConfig(t, cfg)
	}
	var ret []*Error
	for _, e := range lintWithConfig(t, c, src) {
		if id == "" || e.ID == id {
			ret = append(ret, e)
		}
	}
	return ret
}

// lines returns the sorted "line: message prefix" strings of the errors for comparing.
func errLines(errs []*Error) []string {
	ret := make([]string, 0, len(errs))
	for _, e := range errs {
		ret = append(ret, fmt.Sprintf("%d", e.Line))
	}
	sort.Strings(ret)
	return ret
}

func checkLines(t *testing.T, errs []*Error, want ...string) {
	t.Helper()
	got := errLines(errs)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		var b strings.Builder
		for _, e := range errs {
			fmt.Fprintf(&b, "\n  %s", e)
		}
		t.Errorf("want errors at lines %v but got %v:%s", want, got, b.String())
	}
}

// markedWantLines returns the lines of the source that end with the marker "# want".
func markedWantLines(src string) []string {
	var ret []string
	for i, l := range strings.Split(src, "\n") {
		if strings.HasSuffix(strings.TrimRight(l, " "), "# want") {
			ret = append(ret, fmt.Sprintf("%d", i+1))
		}
	}
	return ret
}

// lintInProject lints a workflow that is stored next to the other workflows (name -> content) of a
// temporary project, with the configuration (YAML, "" for the default one).
func lintInProject(t *testing.T, others map[string]string, src, cfg string) []*Error {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, c := range others {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := &Config{}
	if cfg != "" {
		c = mustParseConfig(t, cfg)
	}
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	errs, err := l.Lint(filepath.Join(dir, "main.yml"), []byte(src), &Project{root: root, config: c})
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func onlyID(errs []*Error, id string) []*Error {
	var ret []*Error
	for _, e := range errs {
		if e.ID == id {
			ret = append(ret, e)
		}
	}
	return ret
}
