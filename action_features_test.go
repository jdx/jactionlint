package jactionlint

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const actionWithCheckout = `name: x
description: y
runs:
  using: composite
  steps:
    - uses: actions/checkout@v4
    - run: echo hi
      shell: bash
`

func newActionLinter(t *testing.T, root string, out io.Writer, opts LinterOptions, cfg string) *Linter {
	t.Helper()
	opts.WorkingDir = root
	l, err := NewLinter(out, &opts)
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = mustParseConfig(t, cfg)
	return l
}

func TestActionFixes(t *testing.T) {
	root := writeProject(t, map[string]string{
		".github/workflows/ci.yml":     "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/x\n",
		".github/actions/x/action.yml": actionWithCheckout,
	})
	path := filepath.Join(root, ".github", "actions", "x", "action.yml")
	l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  artipacked: warn\n  unused-ignore: error\n")
	res, err := l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 1 || len(res.Fixed) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "with:\n        persist-credentials: false") {
		t.Errorf("the fix was not applied:\n%s", b)
	}
	// Linting again is clean for the fixed rule and applies nothing
	for _, e := range res.Errors {
		if e.ID == "artipacked" {
			t.Errorf("the fixed finding remains: %v", e)
		}
	}
	res, err = l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil || res.Applied != 0 {
		t.Errorf("fixing must be idempotent: %+v %v", res, err)
	}
}

func TestActionSARIFHasFilePathAndFix(t *testing.T) {
	root := writeProject(t, map[string]string{".github/actions/x/action.yml": actionWithCheckout})
	var out bytes.Buffer
	l := newActionLinter(t, root, &out, LinterOptions{Format: FormatSARIF}, "rules:\n  artipacked: warn\n")
	errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "actions", "x", "action.yml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 {
		t.Fatalf("errors: %v", errs)
	}
	s := out.String()
	for _, want := range []string{`".github/actions/x/action.yml"`, `"ruleId": "artipacked"`, `persist-credentials: false`} {
		if !strings.Contains(s, want) {
			t.Errorf("SARIF does not contain %q:\n%s", want, s)
		}
	}
}

func TestActionIgnores(t *testing.T) {
	const withComment = `name: x
description: y
runs:
  using: composite
  steps:
    # jactionlint ignore=artipacked
    - uses: actions/checkout@v4
    - run: echo hi
      shell: bash
`
	root := writeProject(t, map[string]string{
		".github/actions/x/action.yml": withComment,
		".github/actions/y/action.yml": actionWithCheckout,
		".github/actions/z/action.yml": actionWithCheckout,
	})
	cfg := "rules:\n  artipacked: warn\npaths:\n  \".github/actions/y/**\":\n    ignore:\n      - artipacked\n"
	l := newActionLinter(t, root, io.Discard, LinterOptions{}, cfg)
	var files []string
	for _, n := range []string{"x", "y", "z"} {
		files = append(files, filepath.Join(root, ".github", "actions", n, "action.yml"))
	}
	errs, err := l.LintFiles(files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || !strings.Contains(filepath.ToSlash(errs[0].Filepath), "actions/z/") {
		t.Errorf("only the action without an ignore must be reported: %v", errs)
	}

	// An ignore comment that matches nothing is reported for an action too
	root = writeProject(t, map[string]string{".github/actions/x/action.yml": strings.Replace(withComment, "ignore=artipacked", "ignore=shell-name", 1)})
	l = newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  unused-ignore: error\n")
	errs, err = l.LintFiles([]string{filepath.Join(root, ".github", "actions", "x", "action.yml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].ID != "unused-ignore" {
		t.Errorf("want the unused ignore: %v", errs)
	}
}

func TestActionBotFixDependsOnCallers(t *testing.T) {
	const action = `name: x
description: y
runs:
  using: composite
  steps:
    - run: echo skip
      if: github.actor == 'dependabot[bot]'
      shell: bash
`
	wf := func(on string) string {
		return "on: " + on + "\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/x\n"
	}
	tests := map[string]struct {
		workflows map[string]string
		wantFix   bool
	}{
		"pull request only": {map[string]string{".github/workflows/a.yml": wf("pull_request")}, true},
		"also push":         {map[string]string{".github/workflows/a.yml": wf("pull_request"), ".github/workflows/b.yml": wf("push")}, false},
		"no caller":         {map[string]string{}, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			files := map[string]string{".github/actions/x/action.yml": action}
			for k, v := range tc.workflows {
				files[k] = v
			}
			root := writeProject(t, files)
			l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  bot-conditions: warn\n")
			errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "actions", "x", "action.yml")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(errs) != 1 || errs[0].ID != "bot-conditions" {
				t.Fatalf("errors: %v", errs)
			}
			if have := errs[0].Fix != nil; have != tc.wantFix {
				t.Errorf("fix present = %v, want %v", have, tc.wantFix)
			}
		})
	}
}

func TestActionStdinFilename(t *testing.T) {
	l, err := NewLinter(io.Discard, &LinterOptions{StdinFileName: ".github/actions/x/action.yml"})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{}
	errs, err := l.LintStdin(strings.NewReader("name: x\nruns:\n  using: composite\n  steps:\n    - run: echo\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].ID != actionSyntaxID {
		t.Errorf("a file named action.yml is an action even from stdin: %v", errs)
	}
}

// A workflow that has the name of an action is still a workflow.
func TestWorkflowNamedActionYml(t *testing.T) {
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(&Config{})
	errs, err := l.Lint(".github/workflows/action.yml", []byte("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ nope }}\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 || errs[0].ID == actionSyntaxID {
		t.Errorf("must be linted as a workflow: %v", errs)
	}
}
