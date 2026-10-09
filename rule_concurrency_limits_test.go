package jactionlint

import (
	"strconv"
	"strings"
	"testing"
)

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
		{"cron only", "on:\n  schedule:\n    - cron: '0 6 * * 1'\n" + job, nil},
		{"cron and manual only", "on:\n  schedule:\n    - cron: '0 6 * * 1'\n  workflow_dispatch:\n" + job, nil},
		{"manual only", "on: workflow_dispatch\n" + job, nil},
		{"bare group on cron only", "on:\n  schedule:\n    - cron: '0 6 * * 1'\nconcurrency: g\n" + job, nil},
		{"bare group on manual only", "on: workflow_dispatch\nconcurrency: g\n" + job, nil},
		{"bare group on cron and push", "on:\n  schedule:\n    - cron: '0 6 * * 1'\n  push:\nconcurrency: g\n" + job, []int{5}},
		{"cron and push", "on:\n  schedule:\n    - cron: '0 6 * * 1'\n  push:\n" + job, []int{1}},
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
	src := "on:\n  push:\n  workflow_dispatch:\njobs:\n  copilot-setup-steps:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
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

// concurrency-limits and concurrency-cancels-release must not contradict each other: the block that the first
// recommends must not be reported by the second (the packslip site.yml deployment of the bug bash).
func TestConcurrencyLimitsAgreesWithCancelsRelease(t *testing.T) {
	cfg := mustParseConfig(t, "profile: default\nrules:\n  missing-permissions: off\n  missing-timeout: off\n  unpinned-uses: off\n  excessive-permissions: off\n  artipacked: off\n  mutable-runner-label: off\n")
	deploy := "jobs:\n  deploy:\n    runs-on: ubuntu-latest\n    environment: github-pages\n    steps:\n      - run: echo deploy\n"
	tests := []struct {
		what    string
		src     string
		cancels bool // the advice may say cancel-in-progress: true
	}{
		{"a deployment with an environment", "on:\n  push:\n    branches: [main]\n" + deploy, false},
		{"an environment chosen by an expression", "on: push\njobs:\n  deploy:\n    runs-on: ubuntu-latest\n    environment: ${{ github.ref_name == 'main' && 'prod' || '' }}\n    steps:\n      - run: echo deploy\n", false},
		{"a release event", "on:\n  release:\n    types: [published]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", false},
		{"pushed tags", "on:\n  push:\n    tags: ['v*']\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", false},
		{"a publishing command", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", false},
		{"tags that no pattern lets through", "on:\n  push:\n    tags: ['!**']\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", true},
		{"a test workflow", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo test\n", true},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := errsWithID(lintWithConfig(t, cfg, tc.src), "concurrency-limits")
			if len(errs) != 1 {
				t.Fatalf("want one finding but got %v", errs)
			}
			msg := errs[0].Message
			if got := strings.Contains(msg, "cancel-in-progress: true"); got != tc.cancels {
				t.Errorf("advice to cancel: want %v but got %q", tc.cancels, msg)
			}
			if !tc.cancels && !strings.Contains(msg, "cancel-in-progress: false") {
				t.Errorf("a release workflow is advised to queue: %q", msg)
			}
			if !tc.cancels && errs[0].Fix != nil {
				t.Errorf("a release workflow must get no fix that cancels: %v", errs[0].Fix)
			}
			// The recommended block, applied, is not reported by the other rule
			block := "concurrency:\n  group: g\n  cancel-in-progress: " + strconv.FormatBool(tc.cancels) + "\n"
			fixed := strings.Replace(tc.src, "jobs:\n", block+"jobs:\n", 1)
			for _, e := range lintWithConfig(t, cfg, fixed) {
				if e.ID == "concurrency-cancels-release" || e.ID == "concurrency-limits" {
					t.Errorf("after following the advice: %v", e)
				}
			}
		})
	}
	// The bare form serializes the runs, which is what a release workflow wants
	if got := errsWithID(lintWithConfig(t, cfg, "on:\n  release:\n    types: [published]\nconcurrency: release\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"), "concurrency-limits"); len(got) != 0 {
		t.Errorf("the bare group of a release workflow: %v", got)
	}
}

func TestConcurrencyLimitsAdviceForReleaseCalledFromAnotherWorkflow(t *testing.T) {
	cfg := ruleConfig("concurrency-limits")
	// The caller of a reusable workflow decides github.event_name, so a job that publishes for the release event
	// counts as a release job in a workflow which is also called
	src := "on:\n  workflow_call:\n  push:\njobs:\n  a:\n    if: github.event_name == 'release'\n    runs-on: ubuntu-latest\n    steps:\n      - run: cargo publish\n"
	errs := lintFileWithConfig(t, cfg, "ci.yaml", src)
	var msg string
	for _, e := range errs {
		if e.ID == "concurrency-limits" {
			msg = e.Message
		}
	}
	if !strings.Contains(msg, "cancel-in-progress: false") || !strings.Contains(msg, "queue: max") {
		t.Errorf("a release job must be advised cancel-in-progress false and queue max: %q", msg)
	}
}
