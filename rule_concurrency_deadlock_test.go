package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestConcurrencyDeadlock(t *testing.T) {
	const caller = ".github/workflows/ci.yml"
	callerWith := func(conc string) string {
		return "on: push\n" + conc + "jobs:\n  call:\n    uses: ./.github/workflows/build.yml\n"
	}
	calleeWith := func(top, job string) string {
		return "on: workflow_call\n" + top + "jobs:\n  build:\n" + job + "    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	}
	group := func(g string) string { return "concurrency:\n  group: " + g + "\n" }
	jobGroup := func(g string) string { return "    concurrency:\n      group: " + g + "\n" }
	tests := []struct {
		what   string
		caller string
		callee string
		lines  []int // the lines of the caller
	}{
		{"the same group with expressions", callerWith(group("${{ github.workflow }}-${{ github.sha }}")), calleeWith(group("${{ github.workflow }}-${{ github.sha }}"), ""), []int{3}},
		{"spaces and case do not matter", callerWith(group("${{github.workflow}}-${{ GitHub.Ref }}")), calleeWith(group("${{ github.workflow }}-${{github.ref}}"), ""), []int{3}},
		{"the same static group", callerWith(group("ci")), calleeWith(group("CI"), ""), []int{3}},
		{"the group of a job of the called workflow", callerWith(group("${{ github.workflow }}-${{ github.ref }}")), calleeWith("", jobGroup("${{ github.workflow }}-${{ github.ref }}")), []int{3}},
		{"the pull request", callerWith(group("${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}")), calleeWith(group("${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}"), ""), []int{3}},
		{"the bare form", callerWith("concurrency: ci\n"), calleeWith("concurrency: ci\n", ""), []int{2}},

		{"different groups", callerWith(group("${{ github.workflow }}-${{ github.sha }}")), calleeWith(group("${{ github.workflow }}-${{ github.ref }}"), ""), nil},
		{"the caller has its own part", callerWith(group("${{ github.workflow }}-caller-${{ github.sha }}")), calleeWith(group("${{ github.workflow }}-${{ github.sha }}"), ""), nil},
		{"the called workflow has no group", callerWith(group("${{ github.workflow }}-${{ github.sha }}")), calleeWith("", ""), nil},
		{"the caller has no group", callerWith(""), calleeWith(group("${{ github.workflow }}"), ""), nil},
		{"an input is not known", callerWith(group("g-${{ inputs.x }}")), calleeWith(group("g-${{ inputs.x }}"), ""), nil},
		{"the job of the called workflow is not the job of the caller", callerWith(group("g-${{ github.job }}")), calleeWith(group("g-${{ github.job }}"), ""), nil},
		{"matrix is not known", callerWith(group("g-${{ matrix.x }}")), calleeWith("", jobGroup("g-${{ matrix.x }}")), nil},
		{"a function that is not known", callerWith(group("g-${{ hashFiles('x') }}")), calleeWith(group("g-${{ hashFiles('x') }}"), ""), nil},
		{"the whole concurrency is an expression", callerWith("concurrency: ${{ fromJSON(vars.C) }}\n"), calleeWith("concurrency: ${{ fromJSON(vars.C) }}\n", ""), nil},
		{"a job group of the caller is not compared", "on: push\njobs:\n  call:\n    concurrency:\n      group: g\n    uses: ./.github/workflows/build.yml\n", calleeWith(group("g"), ""), nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			root := writeProject(t, map[string]string{caller: tc.caller, ".github/workflows/build.yml": tc.callee})
			l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  concurrency-deadlock: error\n")
			errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "ci.yml")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantLines(t, errs, "concurrency-deadlock", tc.lines...)
		})
	}
}

func TestConcurrencyDeadlockNeedsProject(t *testing.T) {
	cfg := ruleConfig("concurrency-deadlock")
	src := "on: push\nconcurrency:\n  group: ci\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n"
	wantLines(t, lintFileWithConfig(t, cfg, "ci.yml", src), "concurrency-deadlock")
}

func TestConcurrencyDeadlockMessage(t *testing.T) {
	root := writeProject(t, map[string]string{
		".github/workflows/ci.yml":    "on: push\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.sha }}\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n",
		".github/workflows/build.yml": "on: workflow_call\njobs:\n  build:\n    concurrency:\n      group: ${{ github.workflow }}-${{ github.sha }}\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
	})
	l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  concurrency-deadlock: error\n")
	errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "ci.yml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	es := errsWithID(errs, "concurrency-deadlock")
	if len(es) != 1 || es[0].Line != 3 || es[0].Column != 10 {
		t.Fatalf("unexpected findings: %v", es)
	}
	for _, want := range []string{`job "build"`, ".github/workflows/build.yml (line 5)", `job "call"`, "deadlock"} {
		if !strings.Contains(es[0].Message, want) {
			t.Errorf("the message must have %q: %s", want, es[0].Message)
		}
	}
}
