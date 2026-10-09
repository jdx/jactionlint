package jactionlint

import "testing"

func TestConcurrencyLimits(t *testing.T) {
	cfg := ruleConfig("concurrency-limits")
	job := "jobs:\n  a:\n" + batchAJob
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"missing", "name: x\non: push\n" + job, []int{2}},
		{"missing with quoted on", "\"on\": push\n" + job, []int{1}},
		{"cancel", "on: push\nconcurrency:\n  group: g\n  cancel-in-progress: true\n" + job, nil},
		{"cancel by expression", "on: push\nconcurrency:\n  group: g\n  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}\n" + job, nil},
		{"explicitly not cancelling is a choice", "on: push\nconcurrency:\n  group: g\n  cancel-in-progress: false\n" + job, nil},
		{"mapping without cancel is not reported", "on: push\nconcurrency:\n  group: g\n" + job, nil},
		{"bare group name", "on: push\nconcurrency: g\n" + job, []int{2}},
		{"queue", "on: push\nconcurrency:\n  group: g\n  queue: max\n" + job, nil},
		{"only workflow_call", "on: workflow_call\n" + job, nil},
		{"workflow_call and push", "on:\n  workflow_call:\n  push:\n" + job, []int{1}},
		{"every job is limited", "on: push\njobs:\n  a:\n    concurrency: g\n" + batchAJob, nil},
		{"one job is not limited", "on: push\njobs:\n  a:\n    concurrency:\n      group: g\n" + batchAJob + "  b:\n" + batchAJob, []int{1}},
		{"only reusable workflow calls are limited by the caller", "on: push\njobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n", []int{1}},
		{"reusable workflow calls with a workflow limit", "on: push\nconcurrency:\n  group: g\n  cancel-in-progress: true\njobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n", nil},
		{"a reusable workflow itself", "on: workflow_call\njobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), "concurrency-limits", tc.lines...)
		})
	}
}

func TestPathsWithoutNamesOrConcurrencyForCopilot(t *testing.T) {
	cfg := mustParseConfig(t, "profile: pedantic\n")
	src := "on: workflow_dispatch\njobs:\n  copilot-setup-steps:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	for _, path := range []string{"copilot-setup-steps.yml", ".github/workflows/copilot-setup-steps.yaml"} {
		errs := lintFileWithConfig(t, cfg, path, src)
		for _, id := range []string{"concurrency-limits", "anonymous-definition"} {
			if got := errsWithID(errs, id); len(got) != 0 {
				t.Errorf("%s: %s must not be reported: %v", path, id, got)
			}
		}
		// A timeout is a key that file accepts, so the rule that asks for one still does
		if got := errsWithID(errs, "missing-timeout"); len(got) != 1 {
			t.Errorf("%s: missing-timeout: %v", path, got)
		}
	}
	errs := lintFileWithConfig(t, cfg, "other.yml", src)
	if len(errsWithID(errs, "concurrency-limits")) != 1 || len(errsWithID(errs, "anonymous-definition")) == 0 {
		t.Errorf("another file is reported: %v", errs)
	}
}
