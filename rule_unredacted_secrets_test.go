package jactionlint

import "testing"

func TestUnredactedSecrets(t *testing.T) {
	step := func(env string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n" + env
	}
	cfg := ruleConfig("unredacted-secrets")
	tests := []struct {
		expr string
		want int
	}{
		{"fromJSON(secrets.A).b", 1},
		{"fromJSON(secrets.A)['b']", 1},
		{"fromJSON(secrets['A']).b.c", 1},
		{"fromJSON(secrets.A).*", 1},
		{"fromJson(secrets.A || secrets.B).b", 1},
		{"fromJSON(secrets.A)", 1},
		{"toJSON(fromJSON(secrets.A))", 1},
		{"fromJSON(fromJSON(secrets.A).b)", 1},
		{"fromJSON(github.event.inputs.a).b", 0},
		{"fromJSON('{\"a\": 1}').a", 0},
		{"secrets.A", 0},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			src := step("          X: ${{ " + tc.expr + " }}\n")
			if got := len(errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "unredacted-secrets")); got != tc.want {
				t.Errorf("want %d errors but got %d", tc.want, got)
			}
		})
	}
}
