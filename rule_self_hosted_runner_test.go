package jactionlint

import "testing"

func TestSelfHostedRunner(t *testing.T) {
	cfg := ruleConfig("self-hosted-runner")
	tests := []struct {
		what  string
		runs  string
		extra string
		lines []int
	}{
		{"label", "[self-hosted, linux]", "", []int{4}},
		{"case", "SELF-HOSTED", "", []int{4}},
		{"hosted", "ubuntu-latest", "", nil},
		{"provider label is not reported", "namespace-profile-default", "", nil},
		{"matrix value", "${{ matrix.os }}", "    strategy:\n      matrix:\n        os: [ubuntu-latest, self-hosted]\n", []int{7}},
		{"unresolvable expression", "${{ inputs.runner }}", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: " + tc.runs + "\n" + tc.extra + "    steps:\n      - run: echo\n"
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", src), "self-hosted-runner", tc.lines...)
		})
	}
}
