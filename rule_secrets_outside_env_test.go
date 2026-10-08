package jactionlint

import "testing"

func TestSecretsOutsideEnv(t *testing.T) {
	step := func(env string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n" + env
	}
	cfg := mustParseConfig(t, "rules:\n  secrets-outside-env: {level: error, allow: [Coverage_Token]}\n")
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"outside", step("          X: ${{ secrets.A }}\n"), []int{8}},
		{"once per use", step("          X: ${{ secrets.A }} ${{ secrets.B || secrets['C'] }}\n"), []int{8, 8, 8}},
		{"github token", step("          X: ${{ secrets.GITHUB_TOKEN }}\n"), nil},
		{"allowed", step("          X: ${{ secrets.COVERAGE_TOKEN }}\n"), nil},
		{"environment", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    environment: prod\n    steps:\n      - run: echo ${{ secrets.A }}\n", nil},
		{"environment mapping", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    environment:\n      name: prod\n    steps:\n      - run: echo ${{ secrets.A }}\n", nil},
		{"with of an action", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: octo/repo@v1\n        with:\n          token: ${{ secrets.A }}\n", []int{8}},
		{"reusable workflow", "on: workflow_call\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ secrets.A }}\n", nil},
		{"reusable workflow mapping", "on:\n  workflow_call:\n    inputs: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ secrets.A }}\n", nil},
		{"reusable workflow which also has other triggers", "on: [push, workflow_call]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ secrets.A }}\n", []int{6}},
		{"matrix row", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        t: [\"${{ secrets.A }}\"]\n    steps:\n      - run: echo\n", []int{7}},
		{"matrix include", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        include:\n          - t: ${{ secrets.A }}\n    steps:\n      - run: echo\n", []int{8}},
		{"timeout", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: ${{ secrets.A }}\n    steps:\n      - run: echo\n", []int{5}},
		{"workflow level env is not a job", "on: push\nenv:\n  X: ${{ secrets.A }}\njobs:\n  a:\n" + batchAJob, nil},
		{"job calling a workflow", "on: push\njobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n    secrets:\n      s: ${{ secrets.A }}\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), "secrets-outside-env", tc.lines...)
		})
	}
}
