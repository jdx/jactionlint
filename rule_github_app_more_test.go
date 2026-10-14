package jactionlint

import (
	"strings"
	"testing"
)

// tibdex/github-app-token and peter-murray/workflow-application-token-action issue app tokens too.
func TestGitHubAppOtherActions(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  github-app: error\n")
	step := func(uses, with string) string {
		s := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n"
		if with != "" {
			s += "        with:\n" + with
		}
		return s
	}
	const tib = "tibdex/github-app-token@v2"
	const pm = "peter-murray/workflow-application-token-action@v4"
	tests := []struct {
		name string
		src  string
		want []string // a text of each message
	}{
		{"tibdex with nothing", step(tib, "          app_id: 1\n          private_key: x\n"), []string{`no "repositories" input`, `no "permissions" input`}},
		{"tibdex with both", step(tib, "          app_id: 1\n          private_key: x\n          repositories: '[\"o/r\"]'\n          permissions: '{\"contents\": \"read\"}'\n"), nil},
		{"tibdex that does not revoke", step(tib, "          repositories: '[\"o/r\"]'\n          permissions: '{\"contents\": \"read\"}'\n          revoke: false\n"), []string{`"revoke: false" keeps the GitHub App token valid`}},
		{"tibdex that revokes", step(tib, "          repositories: '[\"o/r\"]'\n          permissions: '{\"contents\": \"read\"}'\n          revoke: true\n"), nil},
		{"tibdex with an expression as the inputs", step(tib, "          ${{ insert }}: ${{ fromJSON(vars.W) }}\n"), nil},
		{"tibdex with the permissions in an expression", step(tib, "          repositories: ${{ vars.R }}\n          permissions: ${{ vars.P }}\n"), nil},
		{"peter-murray with nothing", step(pm, "          application_id: 1\n          application_private_key: x\n"), []string{`the GitHub App token is not revoked`, `no "permissions" input`}},
		{"peter-murray with everything", step(pm, "          permissions: contents:read\n          revoke_token: true\n"), nil},
		{"peter-murray with revoke_token false", step(pm, "          permissions: contents:read\n          revoke_token: false\n"), []string{`the GitHub App token is not revoked`}},
		{"peter-murray with an expression", step(pm, "          permissions: contents:read\n          revoke_token: ${{ vars.REVOKE }}\n"), nil},
		{"peter-murray with an organization is no list of repositories", step(pm, "          permissions: contents:read\n          revoke_token: true\n          organization: o\n"), nil},
		{"another action", step("actions/checkout@v4", ""), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", tc.src) {
				if e.ID == "github-app" {
					got = append(got, e.Message)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("want %d findings, got %q", len(tc.want), got)
			}
			for _, w := range tc.want {
				found := false
				for _, g := range got {
					found = found || strings.Contains(g, w)
				}
				if !found {
					t.Errorf("want %q in one of %q", w, got)
				}
			}
		})
	}
}
