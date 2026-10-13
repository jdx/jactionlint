package jactionlint

import (
	"strings"
	"testing"
)

// The message says what the workflow can do with the token and the secrets, and a workflow which can write nothing and
// read no secret is a pedantic finding.
func TestDangerousTriggersPrivileges(t *testing.T) {
	steps := "    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	tests := []struct {
		name string
		src  string
		// want is the text which the message has, "" when the finding is not reported
		want string
		// pedantic is the same for the option pedantic
		pedantic string
	}{
		{"nothing declared", "on: pull_request_target\njobs:\n  a:\n" + steps, "the default token of the repository (it can write unless the settings of the repository restrict it) and no secrets", ""},
		{"empty permissions", "on: pull_request_target\npermissions: {}\njobs:\n  a:\n" + steps, "", "a token that cannot write and no secrets"},
		{"read all", "on: pull_request_target\npermissions: read-all\njobs:\n  a:\n" + steps, "", "a token that cannot write and no secrets"},
		{"read scopes", "on: pull_request_target\npermissions:\n  contents: read\n  issues: none\njobs:\n  a:\n" + steps, "", "a token that cannot write and no secrets"},
		{"write all", "on: pull_request_target\npermissions: write-all\njobs:\n  a:\n" + steps, "a token that can write every scope and no secrets", ""},
		{"write scope", "on: pull_request_target\npermissions:\n  contents: read\n  pull-requests: write\njobs:\n  a:\n" + steps, "a write token (pull-requests) and no secrets", ""},
		{"scopes of the jobs", "on: pull_request_target\npermissions: {}\njobs:\n  a:\n    permissions:\n      issues: write\n" + steps + "  b:\n    permissions:\n      contents: write\n" + steps, "a write token (contents, issues) and no secrets", ""},
		{"a job without permissions next to one with", "on: pull_request_target\njobs:\n  a:\n    permissions: {}\n" + steps + "  b:\n" + steps, "the default token of the repository", ""},
		{"every job declares", "on: pull_request_target\njobs:\n  a:\n    permissions: {}\n" + steps + "  b:\n    permissions: read-all\n" + steps, "", "a token that cannot write and no secrets"},
		{"a secret", "on: pull_request_target\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"$T\"\n        env:\n          T: ${{ secrets.NPM_TOKEN }}\n", "a token that cannot write and secrets (NPM_TOKEN)", ""},
		{"the token is not a secret", "on: pull_request_target\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"$T\"\n        env:\n          T: ${{ secrets.GITHUB_TOKEN }}\n", "", "a token that cannot write and no secrets"},
		{"a secret by index", "on: pull_request_target\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"$T\"\n        env:\n          T: ${{ secrets['npm_token'] }}\n", "a token that cannot write and secrets (NPM_TOKEN)", ""},
		{"all the secrets", "on: pull_request_target\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"$T\"\n        env:\n          T: ${{ toJSON(secrets) }}\n", "a token that cannot write and secrets which it passes on without naming them", ""},
		{"secrets of a dynamic name", "on: pull_request_target\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"$T\"\n        env:\n          T: ${{ secrets[matrix.name] }}\n", "a token that cannot write and secrets which it passes on without naming them", ""},
		{"inherited secrets", "on: workflow_run\npermissions: {}\njobs:\n  a:\n    uses: o/r/.github/workflows/w.yaml@v1\n    secrets: inherit\n", "a token that cannot write and secrets which it passes on without naming them", ""},
		{"many secrets", "on: issue_comment\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"${{ secrets.A }} ${{ secrets.B }} ${{ secrets.C }} ${{ secrets.D }} ${{ secrets.E }}\"\n", "a token that cannot write and secrets (A, B, C and 2 more)", ""},
	}
	for _, tc := range tests {
		for _, pedantic := range []bool{false, true} {
			name, want := tc.name, tc.want
			if pedantic {
				name += " (pedantic)"
				if tc.pedantic != "" {
					want = tc.pedantic
				}
			}
			cfg := mustParseConfig(t, "rules:\n  dangerous-triggers:\n    pedantic: "+map[bool]string{false: "false", true: "true"}[pedantic]+"\n")
			t.Run(name, func(t *testing.T) {
				var got []string
				for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", tc.src) {
					if e.ID == "dangerous-triggers" {
						got = append(got, e.Message)
					}
				}
				if want == "" {
					if len(got) != 0 {
						t.Fatalf("want no finding, got %q", got)
					}
					return
				}
				if len(got) != 1 || !strings.Contains(got[0], " with "+want) {
					t.Fatalf("want one finding with %q, got %q", want, got)
				}
				unprivileged := want == "a token that cannot write and no secrets"
				if strings.Contains(got[0], "what is at stake") != unprivileged {
					t.Errorf("the note about a workflow without privileges is wrong: %q", got[0])
				}
			})
		}
	}
}

func TestDangerousTriggersPedanticProfile(t *testing.T) {
	src := "on: pull_request_target\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	for profile, want := range map[string]int{"correctness": 0, "default": 0, "pedantic": 1} {
		cfg := mustParseConfig(t, "profile: "+profile+"\nrules:\n  dangerous-triggers: error\n")
		n := 0
		for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", src) {
			if e.ID == "dangerous-triggers" {
				n++
			}
		}
		if n != want {
			t.Errorf("profile %s: want %d findings, got %d", profile, want, n)
		}
	}
}
