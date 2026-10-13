package jactionlint

import (
	"path/filepath"
	"testing"
)

// The check of the workflows and the check of the local actions of dependabot-missing-actions-update share one predicate and one
// walk, so they agree on which `uses:` forms Dependabot can update (Bugbot on #229).
func TestDependabotUsesFormsAgreeInWorkflowsAndActions(t *testing.T) {
	tests := []struct {
		name string
		uses string
		want bool
	}{
		{"a local action", "./.github/actions/x", false},
		{"a local action by the self repository form", "$/.github/actions/x", false},
		{"a docker image", "docker://alpine:3", false},
		{"an action", "actions/checkout@v4", true},
		{"an action in a subdirectory", "github/codeql-action/init@v3", true},
		{"a pinned action", "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683", true},
		{"a reusable workflow", "octo/repo/.github/workflows/x.yml@v1", true},
		{"an expression", "${{ vars.ACTION }}", false},
		{"an expression in the ref", "actions/checkout@${{ vars.REF }}", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// a step of a workflow
			root := writeProject(t, map[string]string{
				".github/workflows/ci.yml": "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + tc.uses + "\n",
			})
			if got := hasExternalActions(filepath.Join(root, ".github", "workflows")); got != tc.want {
				t.Errorf("workflow step: want %v, got %v", tc.want, got)
			}
			// a step of a local composite action
			root = writeProject(t, map[string]string{
				".github/workflows/ci.yml":     "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/x\n",
				".github/actions/x/action.yml": "runs:\n  using: composite\n  steps:\n    - uses: " + tc.uses + "\n",
			})
			dirs := localActionsWithExternalUses(&Project{root: root})
			if got := len(dirs) == 1; got != tc.want {
				t.Errorf("composite action step: want %v, got %v (%v)", tc.want, got, dirs)
			}
		})
	}
	t.Run("the call of a reusable workflow by a job", func(t *testing.T) {
		for uses, want := range map[string]bool{
			"octo/repo/.github/workflows/x.yml@v1": true,
			"./.github/workflows/x.yml":            false,
			"${{ vars.W }}":                        false,
		} {
			root := writeProject(t, map[string]string{
				".github/workflows/ci.yml": "on: push\njobs:\n  a:\n    uses: " + uses + "\n",
			})
			if got := hasExternalActions(filepath.Join(root, ".github", "workflows")); got != want {
				t.Errorf("job uses %q: want %v, got %v", uses, want, got)
			}
		}
	})
}
