package jactionlint

import (
	"strings"
	"testing"
)

func TestRuleUntrustedCheckoutGuard(t *testing.T) {
	const prt = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	const sha = "11bd71901bbe5b1630ceea73d27597364c9af683"
	// the step that checks out the head of the pull request, with the action given
	head := func(uses, extra string) string {
		return "      - uses: " + uses + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n" + extra + "      - run: make\n"
	}
	wantRef := func(uses, extra string) string {
		return "      - uses: " + uses + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n" + extra + "      - run: make\n"
	}
	const wfRun = "on:\n  workflow_run:\n    workflows: [PR]\n    types: [completed]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		what   string
		others map[string]string
		src    string
		// guarded is whether the option pedantic reports the checkout with the message about the protection and the
		// default configuration does not report it
		guarded bool
		// want is a text of the message of the first or the last finding, "" for any
		want string
	}{
		{"floating major of a release line that has the protection", nil, prt + head("actions/checkout@v4", ""), true, ""},
		{"floating major v2", nil, prt + head("actions/checkout@v2", ""), true, ""},
		{"floating major v6", nil, prt + head("actions/checkout@v6", ""), true, ""},
		{"v7", nil, prt + head("actions/checkout@v7", ""), true, ""},
		{"release at the floor", nil, prt + head("actions/checkout@v4.4.0", ""), true, ""},
		{"release above the floor", nil, prt + head("actions/checkout@v5.2.1", ""), true, ""},
		{"commit with the version of the floor in its comment", nil, prt + head("actions/checkout@"+sha+" # v4.4.0", ""), true, ""},
		{"commit with a later major version in its comment", nil, prt + head("actions/checkout@"+sha+" # tag=v7.0.1", ""), true, ""},
		{"the head commit beside a branch name that the protection does not look at", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha || github.head_ref }} # want\n      - run: make\n", false, ""},
		{"the head commit beside the branch of the head", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha || github.event.pull_request.head.ref }} # want\n      - run: make\n", false, ""},
		{"the number of the pull request in refs/pull", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ github.event.pull_request.number }}/head\n      - run: make\n", true, ""},
		{"another property of the pull request in refs/pull", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ github.event.pull_request.title }}/head # want\n      - run: make\n", false, ""},
		{"the head commit twice", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha || github.event.pull_request.merge_commit_sha }}\n      - run: make\n", true, ""},
		{"release below the floor", nil, prt + wantRef("actions/checkout@v4.3.0", ""), false, "upgrade to v4.4.0"},
		{"commit with a version below the floor in its comment", nil, prt + wantRef("actions/checkout@"+sha+" # v4.2.2", ""), false, "(v4.2.2) predates"},
		{"commit with a comment that is only a major version", nil, prt + wantRef("actions/checkout@"+sha+" # v4", ""), false, ""},
		{"commit without a comment", nil, prt + wantRef("actions/checkout@"+sha, ""), false, ""},
		{"commit with a comment about something else", nil, prt + wantRef("actions/checkout@"+sha+" # pinned by renovate", ""), false, ""},
		{"v1 has no protection", nil, prt + wantRef("actions/checkout@v1", ""), false, "upgrade to v7"},
		{"a branch of the action is not known", nil, prt + wantRef("actions/checkout@main", ""), false, ""},
		{"the opt-out is a finding of its own", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n          allow-unsafe-pr-checkout: true # want\n      - run: make\n", false, "turns off the protection"},
		{"the opt-out as an expression", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n          allow-unsafe-pr-checkout: ${{ inputs.unsafe }} # want\n      - run: make\n", false, ""},
		{"the opt-out is reported when nothing runs the code", nil, prt + "      - uses: actions/checkout@v7\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n          allow-unsafe-pr-checkout: true # want\n", false, "turns off the protection"},
		{"the opt-out with a ref that is not an expression", nil, prt + "      - uses: actions/checkout@v7\n        with:\n          ref: main\n          allow-unsafe-pr-checkout: true\n      - run: make\n", false, ""},
		{"the opt-out without a ref", nil, prt + "      - uses: actions/checkout@v7\n        with:\n          allow-unsafe-pr-checkout: true\n      - run: make\n", false, ""},
		{"the opt-out set to false", nil, prt + head("actions/checkout@v7", "          allow-unsafe-pr-checkout: false\n"), true, ""},
		{"the opt-out in a workflow of a trigger that the protection does not cover", nil, "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v7\n        with:\n          ref: ${{ inputs.ref }}\n          allow-unsafe-pr-checkout: true\n", false, ""},
		{"a checkout of a branch name is not what the protection covers", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.ref }} # want\n      - run: make\n", false, ""},
		{"the head repository is refused with a branch name too", nil, prt + head("actions/checkout@v4", "          repository: ${{ github.event.pull_request.head.repo.full_name }}\n"), true, ""},
		{"the merge ref", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ github.event.number }}/merge\n      - run: make\n", true, ""},
		{"the merge commit", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.merge_commit_sha }}\n      - run: make\n", true, ""},
		{"a ref made from a number that is not the one of the pull request", nil, prt + "      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ inputs.number }}/head # want\n      - run: make\n", false, ""},
		{"git in a script is not covered", nil, prt + "      - uses: actions/checkout@v4\n      - run: | # want\n          git fetch origin refs/pull/${{ github.event.number }}/head\n          git checkout FETCH_HEAD\n          make\n", false, ""},
		{"gh pr checkout is not covered", nil, prt + "      - uses: actions/checkout@v4\n      - run: gh pr checkout ${{ github.event.number }} # want\n      - run: make\n", false, ""},
		{"an event the protection leaves out in the same workflow", nil, "on:\n  pull_request_target:\n  issue_comment:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + wantRef("actions/checkout@v4", ""), false, ""},
		{"a workflow that others call", nil, "on:\n  pull_request_target:\n  workflow_call:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + wantRef("actions/checkout@v4", ""), false, ""},
		{"workflow_run of a workflow that only pull requests start", map[string]string{"pr.yml": "name: PR\non: pull_request\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"},
			wfRun + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.workflow_run.head_sha }}\n      - run: make\n", true, ""},
		{"workflow_run of a workflow that pull requests and pushes start", map[string]string{"pr.yml": "name: PR\non: [pull_request, push]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"},
			wfRun + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.workflow_run.head_sha }} # want\n      - run: make\n", false, ""},
		{"workflow_run of a workflow that is not in the project", nil,
			wfRun + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.workflow_run.head_sha }} # want\n      - run: make\n", false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			def := onlyID(lintInProject(t, tc.others, tc.src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
			checkLines(t, def, markedWantLines(tc.src)...)
			if tc.want != "" {
				if len(def) == 0 || !strings.Contains(def[0].Message+def[len(def)-1].Message, tc.want) {
					t.Fatalf("want %q in the message, got %v", tc.want, def)
				}
			}
			ped := onlyID(lintInProject(t, tc.others, tc.src, pedanticCheckout), "untrusted-checkout")
			if tc.guarded {
				// what the protection refuses is reported with the option pedantic, and the message says so
				if len(ped) == 0 || !strings.Contains(ped[0].Message, "refuses to check out the code of a pull request from a fork") {
					t.Fatalf("want the finding about the protection with the option pedantic, got %v", ped)
				}
				return
			}
			checkLines(t, ped, markedWantLines(tc.src)...)
		})
	}
}

// Only the contexts that hold the number of a pull request are numbers that the protection knows, not what is below them.
func TestRefIsPullNumber(t *testing.T) {
	tests := []struct {
		expr string
		want bool
	}{
		{"github.event.number", true},
		{"github.event.pull_request.number", true},
		{"github.event.workflow_run.pull_requests[0].number", true},
		{"github.event.workflow_run.pull_requests.*.number", true},
		{"github.event.workflow_run.pull_requests[0].milestone.number", false},
		{"github.event.workflow_run.pull_requests[0].base.repo.id", false},
		{"github.event.workflow_run.pull_requests[0].head.repo.number", false},
		{"github.event.workflow_run.pull_requests[0].number.x", false},
		{"github.event.workflow_run.pull_requests.number", false},
		{"github.event.pull_request.title", false},
		{"github.event.pull_request.milestone.number", false},
		{"github.event.number.x", false},
	}
	for _, tc := range tests {
		refs, ok := stringExprRefs("${{ " + tc.expr + " }}")
		if !ok || len(refs) != 1 {
			t.Fatalf("%s: no single reference: %v", tc.expr, refs)
		}
		if got := refIsPullNumber(refs[0].chain); got != tc.want {
			t.Errorf("refIsPullNumber(%s) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}
