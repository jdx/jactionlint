package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuleWorkflowRunNames(t *testing.T) {
	const caller = "on:\n  workflow_run:\n    workflows: [%s]\n    types: [completed]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"

	tests := []struct {
		what  string
		files map[string]string
		names string
		want  []string // Expected error messages (substrings)
	}{
		{
			what:  "name exists",
			files: map[string]string{"build.yml": "name: Build\non: push\njobs: {}\n"},
			names: "Build",
		},
		{
			what:  "case insensitive",
			files: map[string]string{"build.yml": "name: Build\non: push\njobs: {}\n"},
			names: "build",
		},
		{
			what:  "file path when name is missing",
			files: map[string]string{"build.yaml": "on: push\njobs: {}\n"},
			names: "'.github/workflows/build.yaml'",
		},
		{
			what:  "name does not exist",
			files: map[string]string{"build.yml": "name: Build\non: push\njobs: {}\n"},
			names: "Buidl",
			want:  []string{`workflow "Buidl" specified at "workflows"`},
		},
		{
			what:  "file name does not count when workflow is named",
			files: map[string]string{"build.yml": "name: Build\non: push\njobs: {}\n"},
			names: "'.github/workflows/build.yml'",
			want:  []string{"is not found in the repository"},
		},
		{
			what:  "partial errors",
			files: map[string]string{"a.yml": "name: A\non: push\njobs: {}\n"},
			names: "A, B, C",
			want:  []string{`"B"`, `"C"`},
		},
		{
			what:  "glob pattern is skipped",
			files: map[string]string{"a.yml": "name: A\non: push\njobs: {}\n"},
			names: "'Build *'",
		},
		{
			what: "dynamic workflow name disables the check",
			files: map[string]string{
				"a.yml": "name: ${{ vars.X }}\non: push\njobs: {}\n",
			},
			names: "Nope",
		},
		{
			what: "broken workflow file disables the check",
			files: map[string]string{
				"a.yml": "name: [\n",
			},
			names: "Nope",
		},
		{
			what: "non-workflow files are ignored",
			files: map[string]string{
				"a.yml":     "name: A\non: push\njobs: {}\n",
				"README.md": "{{{{",
			},
			names: "A",
		},
	}

	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, ".github", "workflows")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			for n, c := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			src := strings.Replace(caller, "%s", tc.names, 1)

			l, err := NewLinter(io.Discard, &LinterOptions{})
			if err != nil {
				t.Fatal(err)
			}
			proj := &Project{root: root, config: &Config{}}
			errs, err := l.Lint(filepath.Join(dir, "caller.yml"), []byte(src), proj)
			if err != nil {
				t.Fatal(err)
			}

			var got []string
			for _, e := range errs {
				if e.Kind == "workflow-run" {
					got = append(got, e.Message)
				} else {
					t.Errorf("unexpected error: %v", e)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d errors but got %d: %v", len(tc.want), len(got), got)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("error %d %q does not contain %q", i, got[i], w)
				}
			}
		})
	}
}

func TestRuleWorkflowRunEnabledByDefaultAndCanBeTurnedOff(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "on:\n  workflow_run:\n    workflows: [Nope]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	errs, err := l.Lint(filepath.Join(dir, "caller.yml"), []byte(src), &Project{root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].ID != "workflow-run-names" {
		t.Fatalf("the rule must be enabled by default: %v", errs)
	}

	errs, err = l.Lint(filepath.Join(dir, "caller.yml"), []byte(src), &Project{root: root, config: ruleSwitch("workflow-run-names", false)})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("the rule must be turned off by the config: %v", errs)
	}
}

func TestRuleWorkflowRunNoProject(t *testing.T) {
	r := NewRuleWorkflowRun(nil)
	r.SetConfig(&Config{})
	w := &Workflow{On: []Event{&WebhookEvent{
		Hook:      &String{Value: "workflow_run"},
		Workflows: []*String{{Value: "x", Pos: &Pos{}}},
	}}}
	if err := r.VisitWorkflowPre(w); err != nil {
		t.Fatal(err)
	}
	if len(r.Errs()) != 0 {
		t.Fatal(r.Errs())
	}
}
