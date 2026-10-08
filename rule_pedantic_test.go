package jactionlint

import (
	"strings"
	"testing"
)

func TestRuleDuplicateTriggers(t *testing.T) {
	const jobs = "jobs:\n  j:\n    runs-on: ubuntu-22.04\n    steps:\n      - run: echo\n"
	tests := []struct {
		what string
		src  string
		want bool
	}{
		{"both without filters", "on:\n  push:\n  pull_request:\n" + jobs, true},
		{"list form", "on: [push, pull_request]\n" + jobs, true},
		{"push has only paths", "on:\n  push:\n    paths: ['src/**']\n  pull_request:\n" + jobs, true},
		{"push ignores branches", "on:\n  push:\n    branches-ignore: [gh-pages]\n  pull_request:\n" + jobs, true},
		{"push for every branch", "on:\n  push:\n    branches: ['**']\n  pull_request:\n" + jobs, true},
		{"pull request has a base filter only", "on:\n  push:\n  pull_request:\n    branches: [main]\n" + jobs, true},
		{"push has a branch filter", "on:\n  push:\n    branches: [main]\n  pull_request:\n" + jobs, false},
		{"push has only tags", "on:\n  push:\n    tags: ['v*']\n  pull_request:\n" + jobs, false},
		{"only push", "on: push\n" + jobs, false},
		{"only pull_request", "on: pull_request\n" + jobs, false},
		{"pull_request_target is another event", "on: [push, pull_request_target]\n" + jobs, false},
		{"pull request is only for closing", "on:\n  push:\n  pull_request:\n    types: [closed]\n" + jobs, false},
		{"pull request for new commits and labels", "on:\n  push:\n  pull_request:\n    types: [opened, labeled]\n" + jobs, true},
		{"every job checks the event", "on: [push, pull_request]\njobs:\n  j:\n    if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name != github.repository\n    runs-on: ubuntu-22.04\n    steps:\n      - run: echo\n", false},
		{"one job does not check the event", "on: [push, pull_request]\njobs:\n  j:\n    if: github.event_name == 'push'\n    runs-on: ubuntu-22.04\n    steps:\n      - run: echo\n  k:\n    runs-on: ubuntu-22.04\n    steps:\n      - run: echo\n", true},
		{"one group and cancel", "on: [push, pull_request]\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.head_ref || github.ref_name }}\n  cancel-in-progress: true\n" + jobs, false},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintBatchH(t, pedanticCfg, tc.src, "duplicate-triggers")
			if (len(errs) == 1) != tc.want || len(errs) > 1 {
				t.Errorf("want report=%v but got %v", tc.want, errs)
			}
			if len(lintBatchH(t, "", tc.src, "duplicate-triggers")) != 0 {
				t.Errorf("the rule must be off in the default profile")
			}
		})
	}
}

func TestRuleContinueOnError(t *testing.T) {
	tests := []struct {
		what string
		coe  string
		want bool
	}{
		{"literal", "true", true},
		{"false", "false", false},
		{"matrix expression", "${{ matrix.experimental }}", false},
		{"none", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-22.04\n    strategy:\n      matrix:\n        experimental: [true]\n"
			if tc.coe != "" {
				src += "    continue-on-error: " + tc.coe + "\n"
			}
			src += "    steps:\n      - run: echo\n        continue-on-error: true\n"
			errs := lintBatchH(t, pedanticCfg, src, "continue-on-error")
			if (len(errs) == 1) != tc.want || len(errs) > 1 {
				t.Errorf("want report=%v but got %v", tc.want, errs)
			}
			if tc.want && errs[0].Severity != SeverityInfo {
				t.Errorf("want info but got %v", errs[0].Severity)
			}
		})
	}
}

func TestRuleContinueOnErrorSteps(t *testing.T) {
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-24.04\n    steps:\n      - name: Lint\n        run: echo\n        continue-on-error: true\n      - run: echo\n        continue-on-error: ${{ matrix.x }}\n      - run: echo\n        continue-on-error: true\n"
	if errs := lintBatchH(t, pedanticCfg, src, "continue-on-error"); len(errs) != 0 {
		t.Errorf("steps must not be reported by default: %v", errs)
	}
	errs := lintBatchH(t, pedanticCfg+"rules:\n  continue-on-error:\n    steps: true\n", src, "continue-on-error")
	checkLines(t, errs, "8", "12")
	if len(errs) == 2 && (!strings.Contains(errs[0].Message, `step "Lint"`) || !strings.Contains(errs[1].Message, "this step")) {
		t.Errorf("unexpected messages: %v", errs)
	}
}

func TestRuleMutableRunnerLabel(t *testing.T) {
	tests := []struct {
		what string
		src  string
	}{
		{"single label", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest # want\n    steps:\n      - run: echo\n"},
		{"other systems and sizes", "on: push\njobs:\n  a:\n    runs-on: macos-latest # want\n    steps:\n      - run: echo\n  b:\n    runs-on: windows-latest # want\n    steps:\n      - run: echo\n  c:\n    runs-on: macos-latest-xlarge # want\n    steps:\n      - run: echo\n"},
		{"list", "on: push\njobs:\n  j:\n    runs-on: [ubuntu-latest, linux] # want\n    steps:\n      - run: echo\n"},
		{"upper case", "on: push\njobs:\n  j:\n    runs-on: Ubuntu-Latest # want\n    steps:\n      - run: echo\n"},
		{"matrix", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os:\n          - ubuntu-latest # want\n          - ubuntu-22.04\n        include:\n          - os: macos-latest # want\n    steps:\n      - run: echo\n"},
		{"fixed labels", "on: push\njobs:\n  j:\n    runs-on: ubuntu-24.04\n    steps:\n      - run: echo\n  k:\n    runs-on: [self-hosted, linux]\n    steps:\n      - run: echo\n  l:\n    runs-on: macos-15\n    steps:\n      - run: echo\n"},
		{"self-hosted runners with the label", "on: push\njobs:\n  j:\n    runs-on: [self-hosted, ubuntu-latest]\n    steps:\n      - run: echo\n"},
		{"expression", "on: push\njobs:\n  j:\n    runs-on: ${{ vars.RUNNER }}\n    steps:\n      - run: echo\n"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			checkLines(t, lintBatchH(t, pedanticCfg, tc.src, "mutable-runner-label"), wantLines(tc.src)...)
			if len(lintBatchH(t, "", tc.src, "mutable-runner-label")) != 0 {
				t.Errorf("the rule must be off in the default profile")
			}
		})
	}
}

func TestMutableRunnerLabelMessage(t *testing.T) {
	for label, want := range map[string]string{
		"ubuntu-latest":         `"ubuntu-24.04"`,
		"macos-latest":          `"macos-15"`,
		"windows-latest":        `"windows-2022"`,
		"macos-latest-xlarge":   `"macos-15-xlarge"`,
		"ubuntu-latest-4-cores": "use a label with a version",
	} {
		src := "on: push\njobs:\n  j:\n    runs-on: " + label + "\n    steps:\n      - run: echo\n"
		errs := lintBatchH(t, pedanticCfg, src, "mutable-runner-label")
		if len(errs) != 1 || !strings.Contains(errs[0].Message, want) {
			t.Errorf("%s: want %s in %v", label, want, errs)
		}
	}
}

func TestMutableRunnerLabelFix(t *testing.T) {
	cfg := pedanticCfg + "rules:\n  mutable-runner-label:\n    pin:\n      ubuntu-latest: ubuntu-24.04\n"
	for _, nl := range []string{"\n", "\r\n"} {
		src := strings.ReplaceAll("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n  b:\n    runs-on: [\"ubuntu-latest\", linux]\n    steps:\n      - run: echo\n  c:\n    runs-on: macos-latest\n    steps:\n      - run: echo\n  d:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n    steps:\n      - run: echo\n", "\n", nl)
		errs := lintBatchH(t, cfg, src, "mutable-runner-label")
		if len(errs) != 4 {
			t.Fatalf("want 4 errors but got %v", errs)
		}
		fixable := 0
		for _, e := range errs {
			if e.Fix != nil {
				fixable++
				if e.Fix.Unsafe {
					t.Errorf("a fix of a configured pin is safe: %+v", e.Fix)
				}
			}
		}
		if fixable != 2 {
			t.Fatalf("want 2 fixes (the label of a and b) but got %d", fixable)
		}
		out, n := applyFixes([]byte(src), errs, FixModeSafe)
		if n != 2 {
			t.Fatalf("applied %d fixes", n)
		}
		want := strings.Replace(strings.Replace(src, "runs-on: ubuntu-latest", "runs-on: ubuntu-24.04", 1), `["ubuntu-latest"`, `["ubuntu-24.04"`, 1)
		if string(out) != want {
			t.Errorf("want %q\nbut got %q", want, out)
		}
		again := lintBatchH(t, cfg, string(out), "mutable-runner-label")
		if len(again) != 2 { // macos-latest and the matrix entry have no pin
			t.Errorf("want 2 remaining errors but got %v", again)
		}
	}

	// Without the option there is no fix
	errs := lintBatchH(t, pedanticCfg, "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", "mutable-runner-label")
	if len(errs) != 1 || errs[0].Fix != nil {
		t.Errorf("want one error without a fix but got %v", errs)
	}

	// A pin must be a fixed label
	if _, err := ParseConfig([]byte("rules:\n  mutable-runner-label:\n    pin:\n      ubuntu-latest: macos-latest\n")); err == nil {
		t.Error("a pin to another moving label must be refused")
	}
}
