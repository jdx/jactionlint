package jactionlint

import (
	"path/filepath"
	"strings"
	"testing"
)

// A stale pin is reported only from a complete picture of what the workflow uses. A workflow which does not parse
// without errors may have lost the step that uses the action, so its pins are not reported.
func TestActionsLockStaleOnlyFromCompleteUsage(t *testing.T) {
	const lock = "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n        - 'actions/checkout@v6.0.2'\n" +
		"dependencies:\n    'actions/checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 1\n        repo_id: 2\n"
	head := "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		name  string
		steps string
		stale bool
	}{
		{"a valid workflow without the action", "      - run: echo\n", true},
		{"a valid workflow with the action", "      - uses: actions/checkout@v6.0.2\n", false},
		{"a step with both uses and run", "      - uses: actions/checkout@v6.0.2\n        run: echo\n", false},
		{"a step which lost its uses to an error", "      - name: x\n        uses: [actions/checkout@v6.0.2]\n", false},
		{"an unknown key next to the step", "      - uses: actions/other@v1\n        bogus: 1\n", false},
		{"an expression in uses", "      - uses: ${{ matrix.action }}\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), head+tc.steps)
			writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), lock)
			stale := false
			for _, e := range lintLockRepository(t, root, fixtureConfig("actions-lock"), LinterOptions{}) {
				if strings.Contains(e.Message, "does not use it") {
					stale = true
				}
			}
			if stale != tc.stale {
				t.Errorf("stale pin reported: %v, want %v", stale, tc.stale)
			}
		})
	}

	// The same for a local action: its metadata has an error, so what it uses is not known
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), head+"      - uses: $/.github/actions/setup\n")
	writeFile(t, filepath.Join(root, ".github", "actions", "setup", "action.yml"), "name: setup\ndescription: setup\nruns:\n  using: composite\n  steps:\n    - name: x\n      uses: [actions/checkout@v6.0.2]\n")
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), lock+"")
	for _, e := range lintLockRepository(t, root, fixtureConfig("actions-lock"), LinterOptions{}) {
		if strings.Contains(e.Message, "does not use it") {
			t.Errorf("a local action with a parse error is not fully read: %v", e)
		}
	}
}
