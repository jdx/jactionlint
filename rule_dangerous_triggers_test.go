package jactionlint

import "testing"

func TestDangerousTriggers(t *testing.T) {
	cfg := ruleConfig("dangerous-triggers")
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"scalar", "on: pull_request_target\n" + "jobs:\n  a:\n" + batchAJob, []int{1}},
		{"list", "on: [push, workflow_run]\n" + "jobs:\n  a:\n" + batchAJob, []int{1}},
		{"mapping", "on:\n  push:\n  issue_comment:\n    types: [created]\n" + "jobs:\n  a:\n" + batchAJob, []int{3}},
		{"safe triggers", "on: [push, pull_request]\n" + "jobs:\n  a:\n" + batchAJob, nil},
		{
			"labeler only",
			"on: pull_request_target\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/labeler@v5\n",
			nil,
		},
		{
			"labeler and a script",
			"on: pull_request_target\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/labeler@v5\n      - run: echo\n",
			[]int{1},
		},
		{
			"labeler does not excuse workflow_run",
			"on: workflow_run\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/labeler@v5\n",
			[]int{1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), "dangerous-triggers", tc.lines...)
		})
	}
}
