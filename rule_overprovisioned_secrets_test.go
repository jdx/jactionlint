package jactionlint

import "testing"

func TestOverprovisionedSecrets(t *testing.T) {
	cfg := ruleConfig("overprovisioned-secrets")
	tests := []struct {
		expr string
		want int
	}{
		{"toJSON(secrets)", 1},
		{"toJson(SECRETS)", 1},
		{"secrets[matrix.name]", 1},
		{"secrets.*", 1},
		{"format('{0}', secrets)", 1},
		{"secrets", 1},
		{"secrets.A", 0},
		{"secrets['A']", 0},
		{"secrets.A || secrets.B", 0},
		{"toJSON(env)", 0},
		{"toJSON(secrets) && toJSON(secrets)", 2},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        name: [a]\n    steps:\n      - run: echo\n        env:\n          X: ${{ " + tc.expr + " }}\n"
			if got := len(errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "overprovisioned-secrets")); got != tc.want {
				t.Errorf("want %d errors but got %d", tc.want, got)
			}
		})
	}
}
