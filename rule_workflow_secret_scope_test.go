package jactionlint

import "testing"

func TestWorkflowSecretScope(t *testing.T) {
	cfg := ruleConfig("workflow-secret-scope")
	const jobs = "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo a\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo b\n"
	head := "on: workflow_dispatch\nenv:\n  TOKEN: ${{ secrets.TOKEN }}\n"
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"multiple jobs", head + jobs, []int{3}},
		{"index access", "on: push\nenv:\n  TOKEN: ${{ secrets['TOKEN'] }}\n" + jobs, []int{3}},
		{"single job", head + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"github token", "on: push\nenv:\n  T: ${{ secrets.GITHUB_TOKEN }}\n" + jobs, nil},
		{"not a secret", "on: push\nenv:\n  T: ${{ vars.X }}\n" + jobs, nil},
		{"job env at the job", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n      T: ${{ secrets.T }}\n    steps:\n      - run: echo\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"one job overrides", head + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n      token: ''\n    steps:\n      - run: echo\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"steps override", head + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          TOKEN: x\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"one step of a job overrides", head + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          TOKEN: x\n      - run: echo\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", []int{3}},
		{"reusable workflow calls", head + "jobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n  b:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n  c:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"one job and a reusable workflow call", head + "jobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n  c:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"two vars", "on: push\nenv:\n  A: ${{ secrets.A }}\n  B: ${{ secrets.B }}\n" + jobs, []int{3, 4}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), "workflow-secret-scope", tc.lines...)
		})
	}
}
