package jactionlint

import (
	"strings"
	"testing"
)

// The version 1 of tibdex/github-app-token has no "repositories" input (Bugbot on #229).
func TestGitHubAppTibdexVersion1(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  github-app: error\n")
	step := func(uses string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n        with:\n          app_id: 1\n          private_key: x\n          permissions: '{\"contents\": \"read\"}'\n"
	}
	tests := []struct {
		name string
		uses string
		want int // findings about "repositories"
	}{
		{"version 2", "tibdex/github-app-token@v2", 1},
		{"version 1", "tibdex/github-app-token@v1", 0},
		{"version 1.2", "tibdex/github-app-token@v1.2.0", 0},
		{"a commit of version 1", "tibdex/github-app-token@3beb63f4bd073e61482598c45c71c1019b59b73a # v1.9.0", 0},
		{"a commit of version 2", "tibdex/github-app-token@3beb63f4bd073e61482598c45c71c1019b59b73a # v2.1.0", 1},
		{"a branch", "tibdex/github-app-token@main", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", step(tc.uses)) {
				if e.ID == "github-app" && strings.Contains(e.Message, `"repositories"`) {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("want %d findings, got %d", tc.want, n)
			}
		})
	}
}

// The message does not assume that the update has "directory: /", and "/" is suggested only where it belongs.
func TestDependabotLocalActionsMessageAndRoot(t *testing.T) {
	const composite = "runs:\n  using: composite\n  steps:\n    - uses: actions/setup-node@v4\n"
	cfg := &Config{Profile: ProfilePedantic, Rules: map[string]RuleConfig{"dependabot-cooldown": {Level: SeverityOff}}}
	run := func(t *testing.T, files map[string]string, src string) string {
		t.Helper()
		root := writeProject(t, files)
		for _, e := range lintDependabot(t, src, cfg, &Project{root: root}) {
			if e.ID == "dependabot-missing-actions-update" {
				return e.Message
			}
		}
		return ""
	}
	head := "version: 2\nupdates:\n  - package-ecosystem: github-actions\n"
	tail := "    schedule:\n      interval: weekly\n"
	t.Run("the message does not say that the update has directory /", func(t *testing.T) {
		msg := run(t, map[string]string{".github/actions/setup/action.yml": composite}, head+"    directories: [\"/\", \"/tools\"]\n"+tail)
		if strings.Contains(msg, `"directory: /" is the workflows`) || !strings.Contains(msg, "reads the workflows and the") {
			t.Errorf("message: %s", msg)
		}
	})
	t.Run("no root when nothing names it and it is not an action", func(t *testing.T) {
		msg := run(t, map[string]string{".github/actions/setup/action.yml": composite}, head+"    directory: /tools\n"+tail)
		want := `"directories: ["/tools", "/.github/actions/**/*"]"`
		if !strings.Contains(msg, want) {
			t.Errorf("want %s in %q", want, msg)
		}
	})
	t.Run("the root when an update names it", func(t *testing.T) {
		msg := run(t, map[string]string{".github/actions/setup/action.yml": composite}, head+"    directories: [\"/tools\", \"/\"]\n"+tail)
		want := `"directories: ["/", "/tools", "/.github/actions/**/*"]"`
		if !strings.Contains(msg, want) {
			t.Errorf("want %s in %q", want, msg)
		}
	})
}

// The version 1 has "permissions" (a missing one is reported), but no "revoke" and no "repositories".
func TestGitHubAppTibdexVersion1Inputs(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  github-app: error\n")
	src := func(uses, with string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n        with:\n          app_id: 1\n          private_key: x\n" + with
	}
	count := func(s, text string) int {
		n := 0
		for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", s) {
			if e.ID == "github-app" && strings.Contains(e.Message, text) {
				n++
			}
		}
		return n
	}
	if got := count(src("tibdex/github-app-token@v1", ""), `no "permissions" input`); got != 1 {
		t.Errorf("v1 without permissions: want 1 finding, got %d", got)
	}
	if got := count(src("tibdex/github-app-token@v1", "          permissions: '{}'\n"), `no "permissions" input`); got != 0 {
		t.Errorf("v1 with permissions: want none, got %d", got)
	}
	if got := count(src("tibdex/github-app-token@v1", "          permissions: '{}'\n          revoke: false\n"), "revoke: false"); got != 0 {
		t.Errorf("v1 has no revoke input: want none, got %d", got)
	}
	if got := count(src("tibdex/github-app-token@v2", "          permissions: '{}'\n          revoke: false\n"), "revoke: false"); got != 1 {
		t.Errorf("v2 revoke false: want 1, got %d", got)
	}
}
