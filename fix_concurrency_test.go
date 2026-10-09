package jactionlint

import (
	"strings"
	"testing"
)

func TestFixConcurrencyLimits(t *testing.T) {
	const group = "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}"
	block := "concurrency:\n  group: " + group + "\n  cancel-in-progress: true\n"
	tests := []struct {
		what string
		src  string
		want string // the source after the safe fixes, "" when there is no fix
	}{
		{
			"pull request only",
			"name: CI\non:\n  pull_request:\n    branches: [main]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n",
			"name: CI\non:\n  pull_request:\n    branches: [main]\n" + block + "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n",
		},
		{
			"flow list of events",
			"on: [pull_request]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n",
			"on: [pull_request]\n" + block + "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n",
		},
		{
			"pull_request_target, after permissions, with four spaces",
			"on:\n    pull_request_target:\npermissions: {}\njobs:\n    test:\n        runs-on: ubuntu-latest\n        steps:\n            - run: make\n",
			"on:\n    pull_request_target:\nconcurrency:\n    group: " + group + "\n    cancel-in-progress: true\npermissions: {}\njobs:\n    test:\n        runs-on: ubuntu-latest\n        steps:\n            - run: make\n",
		},
		{
			"comment after the trigger stays with it",
			"on:\n  pull_request: # CI\n\n# jobs\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n",
			"on:\n  pull_request: # CI\n" + block + "\n# jobs\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n",
		},
		{
			"CRLF",
			"on: pull_request\r\njobs:\r\n  test:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - run: make\r\n",
			"on: pull_request\r\n" + strings.ReplaceAll(block, "\n", "\r\n") + "jobs:\r\n  test:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - run: make\r\n",
		},
		// What must not get the block
		{"push too", "on: [push, pull_request]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n", ""},
		{"manual run too", "on:\n  pull_request:\n  workflow_dispatch:\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n", ""},
		{"tags too", "on:\n  pull_request:\n  push:\n    tags: ['v*']\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n", ""},
		{"release too", "on:\n  pull_request:\n  release:\n    types: [published]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n", ""},
		{"a job with an environment", "on: pull_request\njobs:\n  preview:\n    runs-on: ubuntu-latest\n    environment: preview\n    steps:\n      - run: make\n", ""},
		{"a job that publishes", "on: pull_request\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", ""},
		{"a job that deploys", "on: pull_request\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: wrangler deploy\n", ""},
		{"a job with its own concurrency", "on: pull_request\njobs:\n  a:\n    runs-on: ubuntu-latest\n    concurrency: x\n    steps:\n      - run: make\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n", ""},
		{"an anchor", "on: pull_request\nx-steps: &steps\n  - run: make\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps: *steps\n", ""},
		{"a flow mapping for the whole file", "{on: pull_request, jobs: {test: {runs-on: ubuntu-latest, steps: [{run: make}]}}}\n", ""},
		{"a multi-line flow mapping for the events", "on: {\n  pull_request: {}\n}\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n", ""},
	}
	cfg := mustParseConfig(t, "rules:\n  concurrency-limits: error\n")
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := errsWithID(lintFileWithConfig(t, withoutMissingTimeout(cfg), "ci.yaml", tc.src), "concurrency-limits")
			if len(errs) != 1 {
				t.Fatalf("want one finding but got %v", errs)
			}
			e := errs[0]
			if tc.want == "" {
				if e.Fix != nil {
					t.Fatalf("want no fix but got %+v", e.Fix)
				}
				return
			}
			if e.Fix == nil || e.Fix.Unsafe {
				t.Fatalf("want a safe fix but got %+v", e.Fix)
			}
			got, n := applyFixes([]byte(tc.src), errs, FixModeSafe)
			if n != 1 || string(got) != tc.want {
				t.Fatalf("applied %d:\n%q\nwant:\n%q", n, got, tc.want)
			}
			// The fixed file is clean for the rule, and for the rule that wants a group per pull request
			again := lintFileWithConfig(t, withoutMissingTimeout(mustParseConfig(t, "rules:\n  concurrency-limits: error\n  concurrency-cancels-prs: error\n")), "ci.yaml", string(got))
			if left := append(errsWithID(again, "concurrency-limits"), errsWithID(again, "concurrency-cancels-prs")...); len(left) != 0 {
				t.Fatalf("the fixed file still has %v", left)
			}
		})
	}
}

func TestConcurrencyLimitsMessageExplainsTheGroupForPullRequests(t *testing.T) {
	cfg := withoutMissingTimeout(mustParseConfig(t, "rules:\n  concurrency-limits: error\n"))
	pr := "on: pull_request\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n"
	push := strings.Replace(pr, "pull_request", "push", 1)
	if e := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", pr), "concurrency-limits"); len(e) != 1 || !strings.Contains(e[0].Message, "a group per pull request") {
		t.Errorf("a pull request workflow: %v", e)
	}
	if e := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", push), "concurrency-limits"); len(e) != 1 || strings.Contains(e[0].Message, "per pull request") {
		t.Errorf("a push workflow: %v", e)
	}
}
