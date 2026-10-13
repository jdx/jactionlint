package jactionlint

import (
	"io"
	"path/filepath"
	"testing"
)

// github.event of a reusable workflow is the event of its caller, so the path of the workflow file that started a
// workflow_run counts in a workflow which only a workflow_run workflow calls.
func TestTemplateInjectionWorkflowRunPathThroughCallers(t *testing.T) {
	const callee = "on: workflow_call\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo '${{ github.event.workflow_run.path }}'\n"
	caller := func(on string) string {
		return "on:\n  " + on + "\njobs:\n  c:\n    uses: ./.github/workflows/callee.yml\n"
	}
	tests := []struct {
		name  string
		files map[string]string
		want  int
	}{
		{"called by a workflow_run workflow", map[string]string{"a.yml": caller("workflow_run:\n    workflows: [ci]")}, 1},
		{"called by a push workflow", map[string]string{"a.yml": caller("push:")}, 0},
		{"called by both", map[string]string{"a.yml": caller("push:"), "b.yml": caller("workflow_run:\n    workflows: [ci]")}, 1},
		{"no local caller", map[string]string{}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{".github/workflows/callee.yml": callee}
			for k, v := range tc.files {
				files[".github/workflows/"+k] = v
			}
			root := writeProject(t, files)
			l := newActionLinter(t, root, io.Discard, LinterOptions{}, "profile: default\nrules:\n  dangerous-triggers: off\n  unpinned-uses: off\n  missing-permissions: off\n  missing-timeout: off\n  concurrency-limits: off\n")
			errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "callee.yml")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := errsWithID(errs, "template-injection"); len(got) != tc.want {
				t.Fatalf("want %d findings, got %v", tc.want, got)
			}
		})
	}
}
