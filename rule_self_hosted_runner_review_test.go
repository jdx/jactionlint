package jactionlint

import (
	"strings"
	"testing"
)

// The values of a matrix are alternatives, not labels which all apply: an allowed one does not hide the others.
func TestSelfHostedRunnerAllowAndMatrixAlternatives(t *testing.T) {
	const cfg = "rules:\n  self-hosted-runner:\n    level: info\n    allow: [\"arc-*\"]\n"
	tests := []struct {
		what string
		src  string
		want int
	}{
		{"an allowed alternative hides a self-hosted one", "on: push\njobs:\n  a:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [arc-1, self-hosted]\n    steps:\n      - run: echo\n", 1},
		{"every alternative is allowed", "on: push\njobs:\n  a:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [arc-1, arc-2]\n    steps:\n      - run: echo\n", 0},
		{"a hosted alternative", "on: push\njobs:\n  a:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [arc-1, ubuntu-latest]\n    steps:\n      - run: echo\n", 0},
		{"a label of the list is allowed for every run", "on: push\njobs:\n  a:\n    runs-on: [self-hosted, arc-1]\n    steps:\n      - run: echo\n", 0},
		{"an alternative next to a label of the list", "on: push\njobs:\n  a:\n    runs-on: [linux, \"${{ matrix.os }}\"]\n    strategy:\n      matrix:\n        os: [arc-1, self-hosted]\n    steps:\n      - run: echo\n", 1},
		{"a group which is an expression is not matched", "on: push\njobs:\n  a:\n    runs-on:\n      group: ${{ vars.GROUP }}\n      labels: [self-hosted]\n    steps:\n      - run: echo\n", 1},
		{"a group expression which looks like an allowed name", "on: push\njobs:\n  a:\n    runs-on:\n      group: arc-${{ vars.GROUP }}\n      labels: [self-hosted]\n    steps:\n      - run: echo\n", 1},
		{"an allowed group", "on: push\njobs:\n  a:\n    runs-on:\n      group: arc-pool\n      labels: [self-hosted]\n    steps:\n      - run: echo\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			got := 0
			for _, e := range lintFileWithConfig(t, mustParseConfig(t, cfg), "ci.yaml", tc.src) {
				if e.ID == "self-hosted-runner" {
					got++
				}
			}
			if got != tc.want {
				t.Errorf("want %d findings, got %d", tc.want, got)
			}
		})
	}
}

// A reusable workflow with a trigger of its own: the events of the workflow count even when the callers are not known,
// and callers which are not known are not "no pull request".
func TestSelfHostedRunnerOwnEventsAndUnknownCallers(t *testing.T) {
	const jobs = "jobs:\n  a:\n    runs-on: [self-hosted]\n    steps:\n      - run: echo\n"
	only := "rules:\n  self-hosted-runner:\n    level: info\n    pull-request-only: true\n"
	plain := "rules:\n  self-hosted-runner: info\n"
	tests := []struct {
		what     string
		on       string
		cfg      string
		want     int
		forkNote bool
	}{
		{"a pull request next to workflow_call", "on:\n  pull_request:\n  workflow_call:\n", plain, 1, true},
		{"a pull request next to workflow_call, only pull requests", "on:\n  pull_request:\n  workflow_call:\n", only, 1, true},
		{"workflow_call only, nothing calls it, only pull requests", "on:\n  workflow_call:\n", only, 1, false},
		{"workflow_call and push, nothing calls it, only pull requests", "on:\n  push:\n  workflow_call:\n", only, 1, false},
		{"push only, only pull requests", "on: push\n", only, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			var got []string
			for _, e := range lintFileWithConfig(t, mustParseConfig(t, tc.cfg), "ci.yaml", tc.on+jobs) {
				if e.ID == "self-hosted-runner" {
					got = append(got, e.Message)
				}
			}
			if len(got) != tc.want {
				t.Fatalf("want %d findings, got %q", tc.want, got)
			}
			if tc.want == 1 && strings.Contains(got[0], "pull request of a fork") != tc.forkNote {
				t.Errorf("fork note should be %v: %s", tc.forkNote, got[0])
			}
		})
	}
}
