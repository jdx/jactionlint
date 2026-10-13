package jactionlint

import (
	"strings"
	"testing"
)

// In the job which calls a reusable workflow only the values under "secrets:" are passed on (Bugbot on #223).
func TestSecretsOutsideEnvCallSitesAreTheSecretsMapping(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  secrets-outside-env: warn\n")
	call := func(body string) string {
		return "on: push\njobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n" + body
	}
	tests := []struct {
		name string
		src  string
		want []string // the secrets which are reported as passed
	}{
		{"a secret under secrets", call("    secrets:\n      s: ${{ secrets.A }}\n"), []string{"A"}},
		{"inherit with a secret in if", call("    if: ${{ secrets.A != '' }}\n    secrets: inherit\n"), nil},
		{"inherit with a secret in with", call("    with:\n      x: ${{ secrets.A }}\n    secrets: inherit\n"), nil},
		{"a secret in with next to secrets", call("    with:\n      x: ${{ secrets.B }}\n    secrets:\n      s: ${{ secrets.A }}\n"), []string{"A"}},
		{"a secret in the name", call("    name: ${{ secrets.B }}\n    secrets:\n      s: ${{ secrets.A }}\n"), []string{"A"}},
		{"a secret in the strategy", call("    strategy:\n      matrix:\n        k: [\"${{ secrets.B }}\"]\n    secrets:\n      s: ${{ secrets.A }}\n"), []string{"A"}},
		{"two secrets under secrets", call("    secrets:\n      s: ${{ secrets.A }}\n      t: ${{ secrets.B }}\n"), []string{"A", "B"}},
		{"the token under secrets", call("    secrets:\n      s: ${{ secrets.GITHUB_TOKEN }}\n"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", tc.src) {
				if e.ID == "secrets-outside-env" {
					if !strings.Contains(e.Message, "is passed to a reusable workflow") {
						t.Errorf("not the message of a passed secret: %q", e.Message)
					}
					got = append(got, strings.SplitN(e.Message, `"`, 3)[1])
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}
