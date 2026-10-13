package jactionlint

import (
	"strings"
	"testing"
)

// A repository whose external actions are all in its local actions needs an active github-actions update as much as one whose
// workflows use them (Bugbot on #229).
func TestDependabotMissingActionsUpdateExternalActionsOnlyInLocalActions(t *testing.T) {
	cfg := &Config{Profile: ProfilePedantic, Rules: map[string]RuleConfig{"dependabot-cooldown": {Level: SeverityOff}}}
	files := map[string]string{
		".github/workflows/ci.yml":         "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/x\n",
		".github/actions/x/action.yml":     "runs:\n  using: composite\n  steps:\n    - uses: actions/checkout@v4\n",
		".github/actions/plain/action.yml": "runs:\n  using: composite\n  steps:\n    - run: echo\n      shell: bash\n",
	}
	const head = "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n"
	update := func(extra string) string {
		return head + "  - package-ecosystem: github-actions\n    directory: /\n    schedule:\n      interval: weekly\n" + extra
	}
	tests := []struct {
		name  string
		files map[string]string
		src   string
		want  string
	}{
		{"all updates disabled", files, update("    open-pull-requests-limit: 0\n"), "is disabled"},
		{"no github-actions update", files, head, "no update has"},
		{"an active update which does not cover the action", files, update(""), "does not cover the local composite actions"},
		{"no external action anywhere", map[string]string{".github/workflows/ci.yml": "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/plain\n", ".github/actions/plain/action.yml": files[".github/actions/plain/action.yml"]}, update("    open-pull-requests-limit: 0\n"), ""},
		{"no external action and no update", map[string]string{".github/workflows/ci.yml": "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"}, head, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeProject(t, tc.files)
			var got []string
			for _, e := range lintDependabot(t, tc.src, cfg, &Project{root: root}) {
				if e.ID == "dependabot-missing-actions-update" {
					got = append(got, e.Message)
				}
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no finding, got %q", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("want one finding with %q, got %q", tc.want, got)
			}
		})
	}
}
