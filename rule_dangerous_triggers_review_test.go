package jactionlint

import (
	"strings"
	"testing"
)

// Permissions which are an expression are not known, so the token may write; and a job without permissions next to one with
// them is described by both tokens.
func TestDangerousTriggersExpressionAndMixedPermissions(t *testing.T) {
	steps := "    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	tests := []struct {
		name string
		src  string
		want string // text of the message, "" for no finding in the default profile
	}{
		{"a scope that is an expression", "on: pull_request_target\npermissions:\n  contents: ${{ inputs.level }}\njobs:\n  a:\n" + steps, "a token whose permissions are an expression, so it may write"},
		{"a scope that is an expression in a job", "on: pull_request_target\npermissions: {}\njobs:\n  a:\n    permissions:\n      issues: ${{ vars.LEVEL }}\n" + steps, "a token whose permissions are an expression, so it may write"},
		{"a block that is an expression", "on: pull_request_target\npermissions: ${{ fromJSON(vars.PERMS) }}\njobs:\n  a:\n" + steps, "a token whose permissions are an expression, so it may write"},
		{"read scopes are not an expression", "on: pull_request_target\npermissions:\n  contents: read\n  issues: none\njobs:\n  a:\n" + steps, ""},
		{"a job with permissions next to one without", "on: pull_request_target\njobs:\n  a:\n    permissions:\n      issues: write\n" + steps + "  b:\n" + steps, "a write token (issues) and the default token of the repository (it can write unless the settings of the repository restrict it) and no secrets"},
		{"write-all next to a job without", "on: pull_request_target\njobs:\n  a:\n    permissions: write-all\n" + steps + "  b:\n" + steps, "a token that can write every scope and the default token of the repository"},
		{"an expression next to a write scope", "on: pull_request_target\njobs:\n  a:\n    permissions:\n      issues: write\n" + steps + "  b:\n    permissions:\n      contents: ${{ vars.L }}\n" + steps, "a write token (issues) and a token whose permissions are an expression"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mustParseConfig(t, "rules:\n  dangerous-triggers: error\n")
			var got []string
			for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", tc.src) {
				if e.ID == "dangerous-triggers" {
					got = append(got, e.Message)
				}
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no finding, got %q", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], " with "+tc.want) {
				t.Fatalf("want one finding with %q, got %q", tc.want, got)
			}
		})
	}
}
