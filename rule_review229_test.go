package jactionlint

import (
	"strings"
	"testing"
)

// An explicit "revoke_token: false" is reported even when other inputs are not known.
func TestGitHubAppPeterMurrayExplicitFalseWithUnknownInputs(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  github-app: error\n")
	tests := []struct {
		name string
		with string
		want int
	}{
		{"false next to an insert", "          ${{ insert }}: ${{ fromJSON(vars.W) }}\n          revoke_token: false\n", 1},
		{"nothing known and no revoke_token", "          ${{ insert }}: ${{ fromJSON(vars.W) }}\n", 0},
		{"true next to an insert", "          ${{ insert }}: ${{ fromJSON(vars.W) }}\n          revoke_token: true\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: peter-murray/workflow-application-token-action@v4\n        with:\n" + tc.with
			n := 0
			for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", src) {
				if e.ID == "github-app" && strings.Contains(e.Message, "not revoked") {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("want %d findings, got %d", tc.want, n)
			}
		})
	}
}

// The suggested directories keep what the updates name and do not repeat a directory.
func TestDependabotLocalActionsSuggestionKeepsCoveredDirectories(t *testing.T) {
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
	t.Run("an update with directories keeps them", func(t *testing.T) {
		msg := run(t, map[string]string{
			".github/actions/setup/action.yml": composite,
			"tools/custom/action.yml":          composite,
		}, head+"    directories: [\"/\", \"/tools/custom\"]\n"+tail)
		want := `"directories: ["/", "/tools/custom", "/.github/actions/**/*"]"`
		if !strings.Contains(msg, want) {
			t.Errorf("want %s in %q", want, msg)
		}
	})
	t.Run("a root which is not covered is suggested once", func(t *testing.T) {
		msg := run(t, map[string]string{
			"action.yml":                       composite,
			".github/actions/setup/action.yml": composite,
		}, head+"    directory: /.github/actions/setup\n"+tail)
		want := `"directories: ["/", "/.github/actions/setup"]"`
		if !strings.Contains(msg, want) || strings.Contains(msg, `"/", "/",`) {
			t.Errorf("want %s once in %q", want, msg)
		}
	})
}
