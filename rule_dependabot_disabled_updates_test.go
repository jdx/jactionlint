package jactionlint

import (
	"strings"
	"testing"
)

// An update with open-pull-requests-limit: 0 (or an ignore of every dependency) opens no pull request, so it covers no
// directory (Bugbot on #229). Both checks of dependabot-missing-actions-update use the same notion of an active update.
func TestDependabotMissingActionsUpdateIgnoresDisabledUpdates(t *testing.T) {
	cfg := &Config{Profile: ProfilePedantic, Rules: map[string]RuleConfig{"dependabot-cooldown": {Level: SeverityOff}}}
	const composite = "runs:\n  using: composite\n  steps:\n    - uses: actions/setup-node@v4\n"
	const workflow = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	withActions := map[string]string{".github/workflows/ci.yml": workflow, ".github/actions/setup/action.yml": composite}
	noExternal := map[string]string{".github/workflows/ci.yml": "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"}
	update := func(dirs, extra string) string {
		return "  - package-ecosystem: github-actions\n" + dirs + "    schedule:\n      interval: weekly\n" + extra
	}
	const head = "version: 2\nupdates:\n"
	tests := []struct {
		name  string
		files map[string]string
		src   string
		want  string // a text of the message, "" for no finding
	}{
		{"only a disabled update", withActions, head + update("    directory: /\n", "    open-pull-requests-limit: 0\n"), "is disabled"},
		{"only an update which ignores everything", withActions, head + update("    directory: /\n", "    ignore:\n      - dependency-name: \"*\"\n"), "is disabled"},
		{"a disabled update next to an active one which covers everything", withActions,
			head + update("    directories: [\"/\", \"/.github/actions/**/*\"]\n", "    open-pull-requests-limit: 5\n") + update("    directory: /\n", "    open-pull-requests-limit: 0\n"), ""},
		{"the active one is the root, the actions are in a disabled one", withActions,
			head + update("    directory: /\n", "    open-pull-requests-limit: 5\n") + update("    directories: [\"/.github/actions/setup\"]\n", "    open-pull-requests-limit: 0\n"), "does not cover the local composite actions"},
		{"the actions are in an active one and the root in a disabled one", withActions,
			head + update("    directory: /\n", "    open-pull-requests-limit: 0\n") + update("    directories: [\"/.github/actions/setup\"]\n", "    open-pull-requests-limit: 5\n"), ""},
		{"a limit of 5", withActions, head + update("    directories: [\"/\", \"/.github/actions/setup\"]\n", "    open-pull-requests-limit: 5\n"), ""},
		{"a disabled update and no action to update", noExternal, head + update("    directory: /\n", "    open-pull-requests-limit: 0\n"), ""},
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
