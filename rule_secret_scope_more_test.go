package jactionlint

import "testing"

// With the option job-level, a secret in the env of a job with several steps is reported (ghalint job_secrets).
func TestWorkflowSecretScopeJobLevel(t *testing.T) {
	on := mustParseConfig(t, "rules:\n  workflow-secret-scope: {level: warn, job-level: true}\n")
	off := ruleConfig("workflow-secret-scope")
	const steps = "    steps:\n      - run: echo a\n      - run: echo b\n"
	job := func(env string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n" + env + steps
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"a secret", job("      T: ${{ secrets.T }}\n"), []int{6}},
		{"index access", job("      T: ${{ secrets['T'] }}\n"), []int{6}},
		{"two secrets", job("      A: ${{ secrets.A }}\n      B: ${{ secrets.B }}\n"), []int{6, 7}},
		{"the token", job("      T: ${{ secrets.GITHUB_TOKEN }}\n"), nil},
		{"not a secret", job("      T: ${{ vars.T }}\n      U: literal\n"), nil},
		{"one step", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n      T: ${{ secrets.T }}\n    steps:\n      - run: echo a\n", nil},
		{"one step of two overrides it", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n      T: ${{ secrets.T }}\n    steps:\n      - run: echo a\n        env:\n          T: x\n      - run: echo b\n", nil},
		{"the secret is in the step", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo a\n        env:\n          T: ${{ secrets.T }}\n      - run: echo b\n", nil},
		{"an env which is an expression", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    env: ${{ fromJSON(vars.E) }}\n" + steps, nil},
		{"a job which calls a workflow has no env", "on: push\njobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, on, "ci.yaml", tc.src), "workflow-secret-scope", tc.lines...)
			wantLines(t, lintFileWithConfig(t, off, "ci.yaml", tc.src), "workflow-secret-scope")
		})
	}
}

func TestUnusedWorkflowSecret(t *testing.T) {
	cfg := ruleConfig("unused-workflow-secret")
	const on = "on:\n  workflow_call:\n    secrets:\n      token:\n        required: true\n      other:\n"
	job := func(body string) string {
		return on + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n" + body
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"none used", job(""), []int{4, 6}},
		{"one used", on + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          T: ${{ secrets.token }}\n", []int{6}},
		{"case", on + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          T: ${{ secrets.TOKEN }}\n          O: ${{ secrets.Other }}\n", nil},
		{"index access", on + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          T: ${{ secrets['token'] }}\n          O: ${{ secrets['other'] }}\n", nil},
		{"all of them", on + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo '${{ toJSON(secrets) }}'\n", nil},
		{"passed to another workflow", on + "jobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n    secrets:\n      t: ${{ secrets.token }}\n      o: ${{ secrets.other }}\n", nil},
		{"inherited by another workflow", on + "jobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n    secrets: inherit\n", nil},
		{"read in a condition", on + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        if: ${{ secrets.token != '' && secrets.other != '' }}\n", nil},
		{"no secrets", "on:\n  workflow_call:\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"not a reusable workflow", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"the same name in the inputs is not a read", on + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          T: ${{ inputs.token }} ${{ vars.other }}\n", []int{4, 6}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), "unused-workflow-secret", tc.lines...)
		})
	}
}
