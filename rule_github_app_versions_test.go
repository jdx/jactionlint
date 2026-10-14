package jactionlint

import (
	"strings"
	"testing"
)

// The inputs of tibdex/github-app-token and peter-murray/workflow-application-token-action depend on the version of the
// action: a commit whose version is unknown is not checked for the inputs that the version 1 has not, the versions 1 and 2
// of peter-murray's action cannot revoke the token, and the repositories of tibdex's version 2 are names without the owner
// (CodeRabbit on #229).
func TestGitHubAppVersionedAdvice(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  github-app: error\n")
	const sha = "3beb63f4bd073e61482598c45c71c1019b59b73a"
	lint := func(uses, with string) []string {
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n        with:\n" + with
		var msgs []string
		for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", src) {
			if e.ID == "github-app" {
				msgs = append(msgs, e.Message)
			}
		}
		return msgs
	}
	const tibWith = "          app_id: 1\n          private_key: x\n          permissions: '{\"contents\": \"read\"}'\n          revoke: false\n"
	const pmWith = "          application_id: 1\n          application_private_key: x\n          permissions: contents:read\n"

	t.Run("tibdex commit of unknown version", func(t *testing.T) {
		if got := lint("tibdex/github-app-token@"+sha, tibWith); len(got) != 0 {
			t.Errorf("want no finding for the inputs of the version 2, got %q", got)
		}
	})
	t.Run("tibdex version 2 still reports both", func(t *testing.T) {
		got := strings.Join(lint("tibdex/github-app-token@v2", tibWith), "\n")
		if !strings.Contains(got, `"revoke: false"`) || !strings.Contains(got, `no "repositories" input`) {
			t.Errorf("got %q", got)
		}
	})
	t.Run("tibdex advice names the repositories without the owner", func(t *testing.T) {
		got := strings.Join(lint("tibdex/github-app-token@v2", "          permissions: '{}'\n"), "\n")
		if !strings.Contains(got, `'["repo"]'`) || strings.Contains(got, "owner/repo") {
			t.Errorf("got %q", got)
		}
	})

	tests := []struct {
		name string
		uses string
		with string
		want string // in the only finding about the revocation, "" for none
	}{
		{"version 1", "peter-murray/workflow-application-token-action@v1", pmWith, "versions 1 and 2 of the action have no"},
		{"version 2 with revoke_token", "peter-murray/workflow-application-token-action@v2", pmWith + "          revoke_token: true\n", "versions 1 and 2 of the action have no"},
		{"commit of version 2", "peter-murray/workflow-application-token-action@" + sha + " # v2.0.0", pmWith, "versions 1 and 2 of the action have no"},
		{"version 3", "peter-murray/workflow-application-token-action@v3", pmWith, `set "revoke_token: true" so that`},
		{"version 3 which revokes", "peter-murray/workflow-application-token-action@v3", pmWith + "          revoke_token: true\n", ""},
		{"commit of unknown version", "peter-murray/workflow-application-token-action@" + sha, pmWith, "the earlier versions cannot revoke the token"},
		{"branch", "peter-murray/workflow-application-token-action@main", pmWith, `set "revoke_token: true" so that`},
	}
	for _, tc := range tests {
		t.Run("peter-murray "+tc.name, func(t *testing.T) {
			var got []string
			for _, m := range lint(tc.uses, tc.with) {
				if strings.Contains(m, "not revoked") {
					got = append(got, m)
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
			if strings.Contains(tc.name, "version 1") || strings.Contains(tc.name, "version 2") {
				if strings.Contains(got[0], "set \"revoke_token: true\" so that the token is revoked in the post step") && !strings.Contains(got[0], "use the version 3 or later") {
					t.Errorf("the advice of the old versions must be the upgrade: %q", got[0])
				}
			}
		})
	}
}
