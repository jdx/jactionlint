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
		{"only reusable workflow calls", "on: push\njobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), "concurrency-limits", tc.lines...)
		})
	}
}
