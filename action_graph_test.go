package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func callerNames(c *ActionCallers) []string {
	var ret []string
	for _, cl := range c.Callers {
		ret = append(ret, cl.Workflow)
	}
	return ret
}

func TestActionCallersSeveralWorkflows(t *testing.T) {
	g := newCallGraph(copyActionFixture(t, "callers"))
	c := g.callersOf(".github/actions/setup")
	want := []string{".github/workflows/ci.yaml", ".github/workflows/pr.yaml", ".github/workflows/release.yaml"}
	if diff := cmp.Diff(want, callerNames(c)); diff != "" {
		t.Fatal(diff)
	}
	cl, trigger := c.Dangerous()
	if cl == nil || cl.Workflow != ".github/workflows/pr.yaml" || trigger != "pull_request_target" {
		t.Errorf("the most dangerous caller is %v on %q", cl, trigger)
	}
	if _, ok := c.RunsOn("release"); !ok {
		t.Error("release.yaml runs on release")
	}
	if _, ok := c.RunsOn("schedule"); ok {
		t.Error("no workflow runs on schedule")
	}
	if d := c.Describe(); !strings.Contains(d, "pr.yaml and 2 other") || !strings.Contains(d, `"pull_request_target"`) {
		t.Errorf("Describe() = %q", d)
	}

	only := g.callersOf(".github/actions/pr-only")
	if diff := cmp.Diff([]string{".github/workflows/pr.yaml"}, callerNames(only)); diff != "" {
		t.Fatal(diff)
	}

	none := g.callersOf(".github/actions/unused")
	if none.Known() || len(none.Events()) != 0 {
		t.Errorf("nothing calls the action: %v", none)
	}
	if d := none.Describe(); !strings.Contains(d, "no local workflow calls this action") {
		t.Errorf("Describe() = %q", d)
	}
	var nilCallers *ActionCallers
	if nilCallers.Known() || nilCallers.Events() != nil {
		t.Error("nil callers must be unknown")
	}
	if g.callersOf(".github/actions/setup") != c {
		t.Error("callers must be memoized")
	}
}

func TestActionCallersThroughReusableWorkflowAndCycles(t *testing.T) {
	g := newCallGraph(copyActionFixture(t, "nested"))
	for _, dir := range []string{".github/actions/inner", ".github/actions/outer"} {
		c := g.callersOf(dir)
		if diff := cmp.Diff([]string{".github/workflows/release.yaml"}, callerNames(c)); diff != "" {
			t.Fatalf("%s: %s", dir, diff)
		}
		if len(c.Callers[0].Events) != 1 {
			t.Errorf("%s: events of release.yaml: %v", dir, c.Callers[0].Events)
		}
	}
	// outer is reached through the reusable workflow; inner through outer as well
	if diff := cmp.Diff([]string{".github/workflows/build.yaml"}, g.callersOf(".github/actions/outer").Callers[0].Via); diff != "" {
		t.Error(diff)
	}
	if diff := cmp.Diff([]string{".github/workflows/build.yaml", ".github/actions/outer"}, g.callersOf(".github/actions/inner").Callers[0].Via); diff != "" {
		t.Error(diff)
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCallGraphCyclesAndMissingTargets(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".github/workflows/ci.yml": `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - uses: ./a
      - uses: ./missing
      - uses: ./../outside
      - uses: ./${{ matrix.dir }}
      - uses: $/b/
`,
		"a/action.yml": "runs:\n  using: composite\n  steps:\n    - uses: ./b\n    - uses: ./a\n",
		"b/action.yml": "runs:\n  using: composite\n  steps:\n    - uses: ./a\n",
	})
	g := newCallGraph(root)
	for _, d := range []string{"a", "b"} {
		c := g.callersOf(d)
		if diff := cmp.Diff([]string{".github/workflows/ci.yml"}, callerNames(c)); diff != "" {
			t.Errorf("%s: %s", d, diff)
		}
	}
	if diff := cmp.Diff([]string{filepath.Join(root, "a", "action.yml"), filepath.Join(root, "b", "action.yml")}, g.actionPaths()); diff != "" {
		t.Error(diff)
	}
}

func TestLinterDiscoversActions(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".github/workflows/ci.yml":     "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./tools/custom\n      - uses: ./no/metadata\n",
		"action.yaml":                  "runs:\n  using: node20\n  main: a.js\n",
		".github/actions/x/action.yml": "runs:\n  using: node20\n  main: a.js\n",
		"tools/custom/action.yml":      "runs:\n  using: node20\n  main: a.js\n",
		"vendor/other/action.yml":      "runs:\n  using: node20\n  main: a.js\n", // nothing refers to it
	})
	p := &Project{root: root}
	var rel []string
	for _, f := range p.ActionFiles() {
		r, _ := filepath.Rel(root, f)
		rel = append(rel, filepath.ToSlash(r))
	}
	want := []string{".github/actions/x/action.yml", "action.yaml", "tools/custom/action.yml"}
	if diff := cmp.Diff(want, rel); diff != "" {
		t.Error(diff)
	}
}

func TestCallGraphIsBuiltOncePerRun(t *testing.T) {
	root := copyActionFixture(t, "callers")
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := &Project{root: root}
	first := l.callGraphOf(p)
	if second := l.callGraphOf(p); first != second {
		t.Error("the call graph must be cached by the linter")
	}
}
