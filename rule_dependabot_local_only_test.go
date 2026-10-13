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

// The message and its advice say where the external actions are: "directory: /" when only the workflows use them, and
// "directories" with the root and the local actions when local actions do, the advice of the finding for an update that
// does not cover them (Bugbot on #229).
func TestDependabotMissingActionsUpdateMessageWhereActionsAre(t *testing.T) {
	cfg := &Config{Profile: ProfilePedantic, Rules: map[string]RuleConfig{"dependabot-cooldown": {Level: SeverityOff}}}
	workflow := func(uses string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n"
	}
	const local = "runs:\n  using: composite\n  steps:\n    - uses: actions/checkout@v4\n"
	const head = "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n"
	const disabled = head + "  - package-ecosystem: github-actions\n    directory: /\n    open-pull-requests-limit: 0\n    schedule:\n      interval: weekly\n"
	cases := []struct {
		name  string
		files map[string]string
		src   string
		want  string
	}{
		{"missing, workflows only", map[string]string{".github/workflows/ci.yml": workflow("actions/checkout@v4")}, head,
			`this repository uses actions in its workflows but no update has the "github-actions" package ecosystem, so Dependabot never updates them. add an update with "package-ecosystem: github-actions" and "directory: /"`},
		{"missing, local actions only", map[string]string{".github/workflows/ci.yml": workflow("./.github/actions/x"), ".github/actions/x/action.yml": local}, head,
			`this repository uses actions in the local composite actions in "/.github/actions/x" but no update has the "github-actions" package ecosystem, so Dependabot never updates them. add an update with "package-ecosystem: github-actions" and "directories: ["/", "/.github/actions/**/*"]"`},
		{"missing, both", map[string]string{".github/workflows/ci.yml": workflow("actions/checkout@v4"), ".github/actions/x/action.yml": local}, head,
			`this repository uses actions in its workflows and in the local composite actions in "/.github/actions/x" but no update has the "github-actions" package ecosystem, so Dependabot never updates them. add an update with "package-ecosystem: github-actions" and "directories: ["/", "/.github/actions/**/*"]"`},
		{"disabled, workflows only", map[string]string{".github/workflows/ci.yml": workflow("actions/checkout@v4")}, disabled,
			`this repository uses actions in its workflows but every update with the "github-actions" package ecosystem is disabled ("open-pull-requests-limit: 0", or an "ignore" of every dependency), so Dependabot never updates them. enable one of them`},
		{"disabled, local actions only", map[string]string{".github/workflows/ci.yml": workflow("./.github/actions/x"), ".github/actions/x/action.yml": local}, disabled,
			`this repository uses actions in the local composite actions in "/.github/actions/x" but every update with the "github-actions" package ecosystem is disabled ("open-pull-requests-limit: 0", or an "ignore" of every dependency), so Dependabot never updates them. enable one of them with "directories: ["/", "/.github/actions/**/*"]"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, e := range lintDependabot(t, tc.src, cfg, &Project{root: writeProject(t, tc.files)}) {
				if e.ID == "dependabot-missing-actions-update" {
					got = append(got, e.Message)
				}
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
