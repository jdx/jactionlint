package jactionlint

import (
	"strings"
	"testing"
)

// A workflow that only a schedule or a manual run starts is reported with the option pedantic only, with advice that
// never cancels a run (zizmor reports these workflows; nothing supersedes such a run, so by default they are not).
func TestConcurrencyLimitsScheduledPedantic(t *testing.T) {
	cron := "on:\n  schedule:\n    - cron: '0 6 * * 1'\n"
	job := "jobs:\n  a:\n" + batchAJob
	reusable := "jobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n"
	deploy := "jobs:\n  a:\n    runs-on: ubuntu-latest\n    environment: production\n    steps:\n      - run: echo deploy\n"
	tests := []struct {
		what    string
		src     string
		path    string
		regular []int // lines reported by default
		lines   []int // lines reported with pedantic
	}{
		{what: "cron only", src: cron + job, lines: []int{1}},
		{what: "manual only", src: "on: workflow_dispatch\n" + job, lines: []int{1}},
		{what: "cron and manual", src: cron + "  workflow_dispatch:\n" + job, lines: []int{1}},
		{what: "manual with inputs", src: "on:\n  workflow_dispatch:\n    inputs:\n      env:\n        type: string\n" + job, lines: []int{1}},
		{what: "bare group on cron", src: cron + "concurrency: g\n" + job, lines: []int{4}},
		{what: "bare group on manual only", src: "on: workflow_dispatch\nconcurrency: g\n" + job, lines: []int{2}},
		{what: "mapping with a group", src: cron + "concurrency:\n  group: g\n" + job},
		{what: "mapping that cancels", src: cron + "concurrency:\n  group: g\n  cancel-in-progress: true\n" + job},
		{what: "mapping that does not cancel", src: cron + "concurrency:\n  group: g\n  cancel-in-progress: false\n" + job},
		{what: "every job is limited", src: cron + "jobs:\n  a:\n    concurrency: g\n" + batchAJob},
		{what: "one job is not limited", src: cron + "jobs:\n  a:\n    concurrency:\n      group: g\n" + batchAJob + "  b:\n" + batchAJob, lines: []int{1}},
		{what: "a schedule that deploys has a bare group on purpose", src: cron + "concurrency: g\n" + deploy},
		{what: "calls a reusable workflow", src: cron + reusable, lines: []int{1}},
		{what: "a reusable workflow itself", src: "on: workflow_call\n" + reusable},
		{what: "a schedule and a push is a regular finding", src: cron + "  push:\n" + job, regular: []int{1}, lines: []int{1}},
		{what: "copilot setup steps", src: "on: workflow_dispatch\n" + job, path: ".github/workflows/copilot-setup-steps.yml"},
	}
	regularCfg := mustParseConfig(t, "profile: default\nrules:\n  concurrency-limits: error\n")
	optionCfg := mustParseConfig(t, "profile: default\nrules:\n  concurrency-limits:\n    level: error\n    pedantic: true\n")
	profileCfg := mustParseConfig(t, "profile: pedantic\nrules:\n  concurrency-limits: error\n")
	offCfg := mustParseConfig(t, "profile: pedantic\nrules:\n  concurrency-limits:\n    level: error\n    pedantic: false\n")
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			path := tc.path
			if path == "" {
				path = "ci.yaml"
			}
			wantLines(t, lintFileWithConfig(t, regularCfg, path, tc.src), "concurrency-limits", tc.regular...)
			wantLines(t, lintFileWithConfig(t, optionCfg, path, tc.src), "concurrency-limits", tc.lines...)
			wantLines(t, lintFileWithConfig(t, profileCfg, path, tc.src), "concurrency-limits", tc.lines...)
			wantLines(t, lintFileWithConfig(t, offCfg, path, tc.src), "concurrency-limits", tc.regular...)
		})
	}
}

// The advice for a workflow that a schedule or a person starts never suggests to cancel runs, and no fix is offered:
// the group is the choice of the author, and a cancelled scheduled run can lose work.
func TestConcurrencyLimitsScheduledAdvice(t *testing.T) {
	cfg := mustParseConfig(t, "profile: default\nrules:\n  concurrency-limits:\n    level: error\n    pedantic: true\n")
	job := "jobs:\n  a:\n" + batchAJob
	reusable := "jobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n"
	msg := func(src string) *Error {
		t.Helper()
		errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "concurrency-limits")
		if len(errs) != 1 {
			t.Fatalf("want one finding, got %v", errs)
		}
		return errs[0]
	}

	e := msg("on:\n  schedule:\n    - cron: '0 6 * * 1'\n" + job)
	for _, want := range []string{"${{ github.workflow }}", "a new run waits for the running one", "do not add \"cancel-in-progress: true\""} {
		if !strings.Contains(e.Message, want) {
			t.Errorf("message lacks %q: %s", want, e.Message)
		}
	}
	if strings.Contains(e.Message, "pull request") {
		t.Errorf("advice for a pull request where there is none: %s", e.Message)
	}
	// Only one run can be pending: a newer pending run replaces the older one, so an occurrence can be skipped.
	// "queue: max" (up to 100 pending runs) is documented for concurrency, so it is the way to run every occurrence.
	for _, want := range []string{"replaces the older one that is waiting", "\"queue: max\"", "every occurrence"} {
		if !strings.Contains(e.Message, want) {
			t.Errorf("message lacks %q: %s", want, e.Message)
		}
	}
	if e.Fix != nil {
		t.Errorf("a fix is offered: %+v", e.Fix)
	}

	e = msg("on:\n  workflow_dispatch:\n    inputs:\n      env:\n        type: string\n" + job)
	if !strings.Contains(e.Message, "add them to the group") || !strings.Contains(e.Message, "\"queue: max\"") {
		t.Errorf("a manual run with inputs gets no advice on the group: %s", e.Message)
	}

	// The called workflows see the name of the caller in github.workflow, so the group of the caller has its own part
	e = msg("on: workflow_dispatch\n" + reusable)
	if !strings.Contains(e.Message, "${{ github.workflow }}-caller") || !strings.Contains(e.Message, "cancelled at once") {
		t.Errorf("no advice on the group of a caller: %s", e.Message)
	}

	// A group is shared by the workflow and its jobs, and one run holds it: a job in the group of its own workflow run
	// would wait for the run that contains it. The group that is suggested must not be one that a job already uses.
	jobGroup := func(group string) string {
		return "jobs:\n  a:\n    concurrency:\n      group: " + group + "\n" + batchAJob + "  b:\n" + batchAJob
	}
	cron := "on:\n  schedule:\n    - cron: '0 6 * * 1'\n"
	e = msg(cron + jobGroup("${{ github.workflow }}"))
	if !strings.Contains(e.Message, "such as \"${{ github.workflow }}-workflow\"") {
		t.Errorf("the suggested group is the one a job uses: %s", e.Message)
	}
	e = msg(cron + jobGroup("${{github.workflow}}"))
	if !strings.Contains(e.Message, "such as \"${{ github.workflow }}-workflow\"") {
		t.Errorf("the suggested group is the one a job uses, written without spaces: %s", e.Message)
	}
	e = msg(cron + "jobs:\n  a:\n    concurrency: ${{ github.workflow }}\n" + batchAJob + "  b:\n    concurrency:\n      group: ${{ github.workflow }}-workflow\n" + batchAJob + "  c:\n" + batchAJob)
	if !strings.Contains(e.Message, "such as \"${{ github.workflow }}-workflow-workflow\"") {
		t.Errorf("the second choice is used by a job too: %s", e.Message)
	}
	e = msg(cron + jobGroup("other-${{ github.workflow }}"))
	if !strings.Contains(e.Message, "such as \"${{ github.workflow }}\"") {
		t.Errorf("a group that no job uses is not changed: %s", e.Message)
	}

	e = msg("on: workflow_dispatch\nconcurrency: g\n" + job)
	if !strings.Contains(e.Message, "cancel-in-progress: false") || strings.Contains(e.Message, "cancel-in-progress: true") {
		t.Errorf("a bare group on a schedule is asked to say that it does not cancel: %s", e.Message)
	}
}
