package jactionlint

import (
	"io"
	"path/filepath"
	"testing"
)

const lockDepsForActions = "dependencies:\n" +
	"    'jdx/mise-action@v5.1.1':\n        ref: 'v5.1.1'\n        commit: 'sha1-1111111111111111111111111111111111111111'\n        owner_id: 1\n        repo_id: 2\n" +
	"    'actions/cache@v6.1.0':\n        ref: 'v6.1.0'\n        commit: 'sha1-2222222222222222222222222222222222222222'\n        owner_id: 3\n        repo_id: 4\n"

const compositeAction = "name: setup\ndescription: setup\nruns:\n  using: composite\n  steps:\n    - uses: jdx/mise-action@v5.1.1\n    - uses: actions/cache@v6.1.0\n      with: {path: x, key: y}\n"

func workflowRunningSetup(job string) string {
	return "on: push\njobs:\n  " + job + ":\n    runs-on: ubuntu-latest\n    steps:\n      - uses: $/.github/actions/setup\n"
}

// The lock file records the actions which a local action uses under the workflows which run it, so the steps
// of the action are pinned when all of those workflows list them.
func TestActionsLockCoversLocalActions(t *testing.T) {
	lock := func(ci, release string) string {
		s := "version: 'v0.0.2'\nworkflows:\n"
		if ci != "" {
			s += "    '.github/workflows/ci.yml':\n" + ci
		}
		if release != "" {
			s += "    '.github/workflows/release.yml':\n" + release
		}
		return s + lockDepsForActions
	}
	both := "        - 'actions/cache@v6.1.0'\n        - 'jdx/mise-action@v5.1.1'\n"
	onlyMise := "        - 'jdx/mise-action@v5.1.1'\n"
	tests := []struct {
		name      string
		lock      string
		workflows []string // the names of the workflows which run the action
		want      int      // findings of unpinned-uses in the action
	}{
		{"the only caller lists both", lock(both, ""), []string{"ci"}, 0},
		{"the caller lists one of them", lock(onlyMise, ""), []string{"ci"}, 1},
		{"two callers list both", lock(both, both), []string{"ci", "release"}, 0},
		{"one of two callers lists one", lock(both, onlyMise), []string{"ci", "release"}, 1},
		{"one of two callers is not listed", lock(both, ""), []string{"ci", "release"}, 2},
		{"no workflow runs the action", lock(both, ""), nil, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), tc.lock)
			writeFile(t, filepath.Join(root, ".github", "actions", "setup", "action.yml"), compositeAction)
			writeFile(t, filepath.Join(root, ".github", "workflows", "other.yml"), "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n")
			for _, n := range tc.workflows {
				writeFile(t, filepath.Join(root, ".github", "workflows", n+".yml"), workflowRunningSetup("build"))
			}
			l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = fixtureConfig("unpinned-uses")
			errs, err := l.LintFile(filepath.Join(root, ".github", "actions", "setup", "action.yml"), &Project{root: root})
			if err != nil {
				t.Fatal(err)
			}
			got := 0
			for _, e := range errs {
				if e.ID == "unpinned-uses" {
					got++
				}
			}
			if got != tc.want {
				t.Errorf("want %d findings of unpinned-uses, got %d: %v", tc.want, got, lineIDsOf(errs))
			}
		})
	}
}
