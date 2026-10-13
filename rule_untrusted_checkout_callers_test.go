package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

const (
	headSHA = "${{ github.event.pull_request.head.sha }}"

	// a reusable workflow that checks out the ref it is given and builds
	ucReusable = `on:
  workflow_call:
    inputs:
      ref:
        type: string
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v1
        with:
          ref: ${{ inputs.ref }}
      - run: make
`
	// a composite action that does the same
	ucAction = `name: build
description: checks out a ref and builds
inputs:
  ref:
    description: the ref
runs:
  using: composite
  steps:
    - uses: actions/checkout@v1
      with:
        ref: ${{ inputs.ref }}
    - run: make
      shell: bash
`
)

func ucCaller(on, extra, with string) string {
	return "on: " + on + "\njobs:\n  call:\n" + extra + "    uses: ./.github/workflows/build.yml\n    with:\n      ref: " + with + "\n"
}

func ucActionCaller(on, extra, with string) string {
	return "on: " + on + "\njobs:\n  call:\n    runs-on: ubuntu-latest\n" + extra + "    steps:\n      - uses: ./.github/actions/build\n        with:\n          ref: " + with + "\n"
}

// The head of a pull request that a privileged workflow passes to a local reusable workflow or composite action is followed
// into the callee, which is judged for the events of the workflows that call it.
func TestUntrustedCheckoutCallers(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		target string // the file to lint
		want   int
		text   string // what the message of the first finding contains
	}{
		{"reusable workflow, head passed as an input", map[string]string{
			"build.yml": ucReusable, "pr.yml": ucCaller("pull_request_target", "", headSHA),
		}, "build.yml", 1, "inputs.ref, given github.event.pull_request.head.sha by .github/workflows/pr.yml"},
		{"reusable workflow, a value that is not the head", map[string]string{
			"build.yml": ucReusable, "pr.yml": ucCaller("pull_request_target", "", "${{ github.sha }}"),
		}, "build.yml", 0, ""},
		{"reusable workflow, a literal", map[string]string{
			"build.yml": ucReusable, "pr.yml": ucCaller("pull_request_target", "", "main"),
		}, "build.yml", 0, ""},
		{"reusable workflow, a value that is not known", map[string]string{
			"build.yml": ucReusable, "pr.yml": ucCaller("pull_request_target", "", "${{ needs.prepare.outputs.ref }}"),
		}, "build.yml", 0, ""},
		{"reusable workflow, called by a pull_request workflow", map[string]string{
			"build.yml": ucReusable, "pr.yml": ucCaller("pull_request", "", headSHA),
		}, "build.yml", 0, ""},
		{"reusable workflow that nothing calls", map[string]string{"build.yml": ucReusable}, "build.yml", 0, ""},
		{"reusable workflow, the call has an environment", map[string]string{
			"build.yml": ucReusable, "pr.yml": ucCaller("pull_request_target", "    environment: ci\n", headSHA),
		}, "build.yml", 0, ""},
		{"reusable workflow, the call is for maintainers", map[string]string{
			"build.yml": ucReusable, "pr.yml": ucCaller("pull_request_target", "    if: github.event.pull_request.author_association == 'MEMBER'\n", headSHA),
		}, "build.yml", 0, ""},
		{"reusable workflow, the call is for another event", map[string]string{
			"build.yml": ucReusable, "pr.yml": ucCaller("[pull_request_target, push]", "    if: github.event_name == 'push'\n", headSHA),
		}, "build.yml", 0, ""},
		{"reusable workflow, a guarded job is needed", map[string]string{
			"build.yml": ucReusable,
			"pr.yml": "on: pull_request_target\njobs:\n  ok:\n    runs-on: ubuntu-latest\n    if: github.event.pull_request.author_association == 'MEMBER'\n    steps:\n      - run: echo\n" +
				"  call:\n    needs: ok\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: " + headSHA + "\n",
		}, "build.yml", 0, ""},
		{"reusable workflow, called through another reusable workflow", map[string]string{
			"build.yml": ucReusable,
			"mid.yml":   "on:\n  workflow_call:\n    inputs:\n      sha:\n        type: string\njobs:\n  b:\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: ${{ inputs.sha }}\n",
			"pr.yml":    "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/mid.yml\n    with:\n      sha: " + headSHA + "\n",
		}, "build.yml", 1, "passed on by .github/workflows/mid.yml"},
		{"reusable workflow, called through another one with a value that is not the head", map[string]string{
			"build.yml": ucReusable,
			"mid.yml":   "on:\n  workflow_call:\n    inputs:\n      sha:\n        type: string\njobs:\n  b:\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: ${{ inputs.sha }}\n",
			"pr.yml":    "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/mid.yml\n    with:\n      sha: main\n",
		}, "build.yml", 0, ""},
		{"reusable workflow, another input is the head", map[string]string{
			"build.yml": strings.Replace(ucReusable, "ref: ${{ inputs.ref }}", "ref: ${{ inputs.other }}", 1),
			"pr.yml":    ucCaller("pull_request_target", "", headSHA),
		}, "build.yml", 0, ""},
		{"reusable workflow with the head in its own context", map[string]string{
			"build.yml": strings.Replace(ucReusable, "ref: ${{ inputs.ref }}", "ref: "+headSHA, 1),
			"pr.yml":    "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n",
		}, "build.yml", 1, "github.event.pull_request.head.sha"},
		{"reusable workflow with the head in its own context, called on push", map[string]string{
			"build.yml": strings.Replace(ucReusable, "ref: ${{ inputs.ref }}", "ref: "+headSHA, 1),
			"push.yml":  "on: push\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n",
		}, "build.yml", 0, ""},
		{"reusable workflow, called on a comment with the number of the issue", map[string]string{
			"build.yml": ucReusable,
			"c.yml":     ucCaller("issue_comment", "", "refs/pull/${{ github.event.issue.number }}/head"),
		}, "build.yml", 1, "refs/pull/${{ github.event.issue.number }}/head by .github/workflows/c.yml"},
		{"composite action, head passed as an input", map[string]string{
			"pr.yml": ucActionCaller("pull_request_target", "", headSHA),
		}, ".github/actions/build/action.yml", 1, "inputs.ref, given github.event.pull_request.head.sha by .github/workflows/pr.yml"},
		{"composite action, a value that is not the head", map[string]string{
			"pr.yml": ucActionCaller("pull_request_target", "", "${{ github.sha }}"),
		}, ".github/actions/build/action.yml", 0, ""},
		{"composite action, called on push", map[string]string{
			"push.yml": ucActionCaller("push", "", headSHA),
		}, ".github/actions/build/action.yml", 0, ""},
		{"composite action, the call has an environment", map[string]string{
			"pr.yml": ucActionCaller("pull_request_target", "    environment: ci\n", headSHA),
		}, ".github/actions/build/action.yml", 0, ""},
		{"composite action, a permission check before the call", map[string]string{
			"pr.yml": "on: pull_request_target\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: ./.github/actions/build\n        with:\n          ref: " + headSHA + "\n",
		}, ".github/actions/build/action.yml", 0, ""},
		{"composite action, one call is guarded and another is not", map[string]string{
			"a.yml": ucActionCaller("pull_request_target", "    environment: ci\n", headSHA),
			"b.yml": ucActionCaller("pull_request_target", "", headSHA),
		}, ".github/actions/build/action.yml", 1, ".github/workflows/b.yml"},
		{"composite action, the call step runs after the permission check failed", map[string]string{
			"pr.yml": "on: pull_request_target\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: ./.github/actions/build\n        if: always()\n        with:\n          ref: " + headSHA + "\n",
		}, ".github/actions/build/action.yml", 1, ".github/workflows/pr.yml"},
		{"composite action, the call step after a permission check", map[string]string{
			"pr.yml": "on: pull_request_target\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: ./.github/actions/build\n        with:\n          ref: " + headSHA + "\n",
		}, ".github/actions/build/action.yml", 0, ""},
		{"composite action, an input and a call in another case", map[string]string{
			"pr.yml": "on: pull_request_target\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/build\n        with:\n          REF: " + headSHA + "\n",
		}, ".github/actions/build/action.yml", 1, "inputs.ref"},
		{"composite action, the call step after a permission check runs for cancelled runs only", map[string]string{
			"pr.yml": "on: pull_request_target\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: ./.github/actions/build\n        if: cancelled()\n        with:\n          ref: " + headSHA + "\n",
		}, ".github/actions/build/action.yml", 0, ""},
		{"composite action, the call step after a permission check runs unless cancelled", map[string]string{
			"pr.yml": "on: pull_request_target\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: ./.github/actions/build\n        if: ${{ !cancelled() }}\n        with:\n          ref: " + headSHA + "\n",
		}, ".github/actions/build/action.yml", 1, ".github/workflows/pr.yml"},
		{"composite action, an input that the call leaves out has a default that is the head", map[string]string{
			"pr.yml":                      "on: pull_request_target\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/build\n",
			"../actions/build/action.yml": strings.Replace(ucAction, "    description: the ref\n", "    description: the ref\n    default: "+headSHA+"\n", 1),
		}, ".github/actions/build/action.yml", 1, "github.event.pull_request.head.sha"},
		{"reusable workflow, an input that the call leaves out has a default that is the head", map[string]string{
			"build.yml": strings.Replace(ucReusable, "      ref:\n        type: string\n", "      ref:\n        type: string\n        default: "+headSHA+"\n", 1),
			"pr.yml":    "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n",
		}, "build.yml", 1, "github.event.pull_request.head.sha"},
		{"reusable workflow, a caller whose events the call drops passes nothing down", map[string]string{
			"build.yml": strings.Replace(ucReusable, "actions/checkout@v1", "actions/checkout@v4", 1),
			"mid.yml":   "on:\n  workflow_call:\n    inputs:\n      sha:\n        type: string\njobs:\n  b:\n    if: github.event_name == 'pull_request_target'\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: ${{ inputs.sha }}\n",
			"a.yml":     "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/mid.yml\n    with:\n      sha: " + headSHA + "\n",
			"z.yml":     "on: issue_comment\njobs:\n  call:\n    uses: ./.github/workflows/mid.yml\n    with:\n      sha: refs/pull/${{ github.event.issue.number }}/head\n",
		}, "build.yml", 0, ""},
		{"composite action, the call step is for another event", map[string]string{
			"pr.yml": "on: [pull_request_target, push]\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/build\n        if: github.event_name == 'push'\n        with:\n          ref: " + headSHA + "\n",
		}, ".github/actions/build/action.yml", 0, ""},
		{"reusable workflow, two callers through another workflow, a branch and a commit", map[string]string{
			"build.yml": strings.Replace(ucReusable, "actions/checkout@v1", "actions/checkout@v4", 1),
			"mid.yml":   "on:\n  workflow_call:\n    inputs:\n      sha:\n        type: string\njobs:\n  b:\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: ${{ inputs.sha }}\n",
			"a.yml":     "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/mid.yml\n    with:\n      sha: ${{ github.event.pull_request.head.ref }}\n",
			"z.yml":     "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/mid.yml\n    with:\n      sha: " + headSHA + "\n",
		}, "build.yml", 1, "head.ref by .github/workflows/a.yml"},
		{"reusable workflow, a commit and a branch passed to a checkout that refuses commits", map[string]string{
			"build.yml": strings.Replace(ucReusable, "actions/checkout@v1", "actions/checkout@v4", 1),
			"pr.yml":    "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: ${{ github.event.pull_request.head.sha || github.head_ref }}\n",
		}, "build.yml", 1, "inputs.ref, given"},
		{"reusable workflow, a commit passed to a checkout that refuses commits", map[string]string{
			"build.yml": strings.Replace(ucReusable, "actions/checkout@v1", "actions/checkout@v4", 1),
			"pr.yml":    "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: ${{ github.event.pull_request.head.sha || github.event.pull_request.merge_commit_sha }}\n",
		}, "build.yml", 0, ""},
		{"reusable workflow, the number of the pull request in the ref of a checkout that refuses it", map[string]string{
			"build.yml": "on:\n  workflow_call:\n    inputs:\n      n:\n        type: string\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ inputs.n }}/head\n      - run: make\n",
			"pr.yml":    "on: pull_request_target\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n    with:\n      n: ${{ github.event.number }}\n",
		}, "build.yml", 0, ""},
		{"reusable workflow, the number of the pull request of a comment", map[string]string{
			"build.yml": "on:\n  workflow_call:\n    inputs:\n      pr:\n        type: string\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: gh pr checkout ${{ inputs.pr }}\n      - run: make\n",
			"c.yml":     "on: issue_comment\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n    with:\n      pr: ${{ github.event.issue.number }}\n",
		}, "build.yml", 1, "gh pr checkout"},
		{"reusable workflow, a number that is not the one of the comment", map[string]string{
			"build.yml": "on:\n  workflow_call:\n    inputs:\n      pr:\n        type: string\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: gh pr checkout ${{ inputs.pr }}\n      - run: make\n",
			"c.yml":     "on: issue_comment\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n    with:\n      pr: '123'\n",
		}, "build.yml", 0, ""},
		{"reusable workflow, the number of a comment in a script that the callee runs", map[string]string{
			"build.yml":   "on:\n  workflow_call:\n    inputs:\n      pr:\n        type: string\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: ./co.sh \"${{ inputs.pr }}\"\n      - run: make\n",
			"../../co.sh": "gh pr checkout \"$1\"\n",
			"c.yml":       "on: issue_comment\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n    with:\n      pr: ${{ github.event.issue.number }}\n",
		}, "build.yml", 1, "gh pr checkout"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{".github/actions/build/action.yml": ucAction}
			if tc.target == "build.yml" || strings.HasPrefix(tc.name, "reusable") {
				delete(files, ".github/actions/build/action.yml")
			}
			if _, ok := tc.files["../actions/build/action.yml"]; ok {
				delete(files, ".github/actions/build/action.yml") // the case brings its own, at the same path
			}
			for k, v := range tc.files {
				files[".github/workflows/"+k] = v
			}
			target := filepath.Join(".github", "workflows", tc.target)
			if strings.HasPrefix(tc.target, ".github/") {
				target = tc.target
			}
			root := writeProject(t, files)
			l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  untrusted-checkout: error\n")
			errs, err := l.LintFiles([]string{filepath.Join(root, target)}, nil)
			if err != nil {
				t.Fatal(err)
			}
			errs = errsWithID(errs, "untrusted-checkout")
			if len(errs) != tc.want {
				t.Fatalf("want %d findings but got %v", tc.want, errs)
			}
			if tc.want > 0 && !strings.Contains(errs[0].Message, tc.text) {
				t.Errorf("message lacks %q: %s", tc.text, errs[0].Message)
			}
		})
	}
}

// A local composite action that only checks out the head of a pull request is a checkout of the step that calls it: the
// caller runs the code afterwards.
func TestUntrustedCheckoutLocalActionCheckout(t *testing.T) {
	action := func(uses, ref string) string {
		return "name: checkout-pr\ndescription: d\ninputs:\n  ref:\n    description: r\n    default: main\nruns:\n  using: composite\n  steps:\n    - uses: " + uses + "\n      with:\n        ref: " + ref + "\n"
	}
	numberAction := func(uses string) string {
		return "name: checkout-pr\ndescription: d\ninputs:\n  n:\n    description: n\nruns:\n  using: composite\n  steps:\n    - uses: " + uses + "\n      with:\n        ref: refs/pull/${{ inputs.n }}/head\n"
	}
	const prt = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		name   string
		action string
		src    string
	}{
		{"head passed to the input", action("actions/checkout@v1", "${{ inputs.ref }}"), prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"head in the action itself", action("actions/checkout@v1", headSHA), prt + "      - uses: ./.github/actions/checkout-pr # want\n      - run: make\n"},
		{"the default of the input is not the head", action("actions/checkout@v1", "${{ inputs.ref }}"), prt + "      - uses: ./.github/actions/checkout-pr\n      - run: make\n"},
		{"a value that is not the head", action("actions/checkout@v1", "${{ inputs.ref }}"), prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: ${{ github.sha }}\n      - run: make\n"},
		{"nothing runs the code", action("actions/checkout@v1", "${{ inputs.ref }}"), prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: git diff --stat\n"},
		{"the protection refuses it", action("actions/checkout@v4", "${{ inputs.ref }}"), prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"the action is not composite", "name: x\ndescription: d\nruns:\n  using: node20\n  main: index.js\n", prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"a permission check before the checkout in the action", "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - uses: sushichop/action-repository-permission@v3\n      with:\n        required-permission: write\n    - uses: actions/checkout@v1\n      with:\n        ref: ${{ inputs.ref }}\n", prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"the checkout in the action is for another event", action("actions/checkout@v1", "${{ inputs.ref }}") + "      if: github.event_name == 'push'\n", "on: [pull_request_target, push]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"a permission check in the action, and a checkout that runs after a failure", "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - uses: sushichop/action-repository-permission@v3\n      with:\n        required-permission: write\n    - uses: actions/checkout@v1\n      if: always()\n      with:\n        ref: ${{ inputs.ref }}\n", prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          ref: " + headSHA + "\n      - run: make\n        if: always()\n"},
		{"a permission check in the action, a checkout that runs after a failure and a step that does not", "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - uses: sushichop/action-repository-permission@v3\n      with:\n        required-permission: write\n    - uses: actions/checkout@v1\n      if: always()\n      with:\n        ref: ${{ inputs.ref }}\n", prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"a checkout in the action before a permission check, and a step that runs after a failure", "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - uses: actions/checkout@v1\n      with:\n        ref: ${{ inputs.ref }}\n    - uses: sushichop/action-repository-permission@v3\n      with:\n        required-permission: write\n", prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          ref: " + headSHA + "\n      - run: make\n        if: failure()\n"},
		{"a checkout in the action before a permission check, and a step for cancelled runs", "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - uses: actions/checkout@v1\n      with:\n        ref: ${{ inputs.ref }}\n    - uses: sushichop/action-repository-permission@v3\n      with:\n        required-permission: write\n", prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: make\n        if: cancelled()\n"},
		{"a checkout in the action before a permission check", "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - uses: actions/checkout@v1\n      with:\n        ref: ${{ inputs.ref }}\n    - uses: sushichop/action-repository-permission@v3\n      with:\n        required-permission: write\n", prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"an input read in an index access and an expression", action("actions/checkout@v1", "${{ inputs['ref'] || 'main' }}"), prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"an input read in format and in another case", action("actions/checkout@v1", "${{ format('{0}', INPUTS.Ref) }}"), prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"an input that the call leaves out and whose default is the head", "name: x\ndescription: d\ninputs:\n  ref:\n    description: r\n    default: " + headSHA + "\nruns:\n  using: composite\n  steps:\n    - uses: actions/checkout@v1\n      with:\n        ref: ${{ inputs.ref }}\n", prt + "      - uses: ./.github/actions/checkout-pr # want\n      - run: make\n"},
		{"a checkout in a parallel group beside a permission check, and a step that runs after a failure", "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - parallel:\n        - uses: actions/checkout@v1\n          with:\n            ref: ${{ inputs.ref }}\n        - uses: sushichop/action-repository-permission@v3\n          with:\n            required-permission: write\n", prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          ref: " + headSHA + "\n      - run: make\n        if: always()\n"},
		{"a checkout in a parallel group beside a permission check", "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - parallel:\n        - uses: actions/checkout@v1\n          with:\n            ref: ${{ inputs.ref }}\n        - uses: sushichop/action-repository-permission@v3\n          with:\n            required-permission: write\n", prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
		{"the number of the pull request in the ref, and a checkout that refuses it", numberAction("actions/checkout@v4"), prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          n: ${{ github.event.number }}\n      - run: make\n"},
		{"the number of the pull request in the ref, and a checkout that does not refuse it", numberAction("actions/checkout@v1"), prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          n: ${{ github.event.number }}\n      - run: make\n"},
		{"the number of the pull request in the ref, in another case and an index access", strings.Replace(numberAction("actions/checkout@v4"), "${{ inputs.n }}", "${{ INPUTS['N'] }}", 1), prt + "      - uses: ./.github/actions/checkout-pr\n        with:\n          n: ${{ github.event.number }}\n      - run: make\n"},
		{"a commit and a branch passed to a checkout that refuses commits", action("actions/checkout@v4", "${{ inputs.ref }}"), prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          ref: ${{ github.event.pull_request.head.sha || github.head_ref }}\n      - run: make\n"},
		{"free text in the ref, and a checkout that refuses a number", numberAction("actions/checkout@v4"), prt + "      - uses: ./.github/actions/checkout-pr # want\n        with:\n          n: ${{ github.head_ref }}\n      - run: make\n"},
		{"a number in the ref that the call leaves out", numberAction("actions/checkout@v4"), prt + "      - uses: ./.github/actions/checkout-pr # want\n      - run: make\n"},
		{"the step is for maintainers", action("actions/checkout@v1", "${{ inputs.ref }}"), prt + "      - uses: ./.github/actions/checkout-pr\n        if: github.event.pull_request.author_association == 'MEMBER'\n        with:\n          ref: " + headSHA + "\n      - run: make\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			others := map[string]string{"../actions/checkout-pr/action.yml": tc.action}
			errs := onlyID(lintInProject(t, others, tc.src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
			checkLines(t, errs, markedWantLines(tc.src)...)
		})
	}
}

// The protection of actions/checkout is judged for the events that reach the callee.
func TestUntrustedCheckoutCallersGuard(t *testing.T) {
	floating := strings.Replace(ucReusable, "actions/checkout@v1", "actions/checkout@v4", 1)
	tests := []struct {
		name  string
		files map[string]string
		want  int // without the option pedantic
	}{
		{"floating tag, head of the pull request", map[string]string{"build.yml": floating, "pr.yml": ucCaller("pull_request_target", "", headSHA)}, 0},
		{"floating tag, a number from a comment", map[string]string{"build.yml": floating, "c.yml": ucCaller("issue_comment", "", "refs/pull/${{ github.event.issue.number }}/head")}, 1},
		{"floating tag, called on a comment and on a pull request", map[string]string{
			"build.yml": floating, "pr.yml": ucCaller("pull_request_target", "", headSHA), "c.yml": ucCaller("issue_comment", "", "refs/pull/${{ github.event.issue.number }}/head"),
		}, 1},
		{"floating tag, called on workflow_run of a workflow that is not known to be a pull request one", map[string]string{
			"build.yml": floating, "wr.yml": "on:\n  workflow_run:\n    workflows: [Other]\n    types: [completed]\njobs:\n  call:\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: ${{ github.event.workflow_run.head_sha }}\n",
		}, 1},
		{"floating tag, a caller that is not known", map[string]string{
			"build.yml": floating, "mid.yml": "on: workflow_call\njobs:\n  b:\n    uses: ./.github/workflows/build.yml\n    with:\n      ref: " + headSHA + "\n",
			"pr.yml": ucCaller("pull_request_target", "", headSHA),
		}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			for k, v := range tc.files {
				files[".github/workflows/"+k] = v
			}
			root := writeProject(t, files)
			for _, pedantic := range []bool{false, true} {
				cfg := "rules:\n  untrusted-checkout: error\n"
				if pedantic {
					cfg = "rules:\n  untrusted-checkout: {level: error, pedantic: true}\n"
				}
				l := newActionLinter(t, root, io.Discard, LinterOptions{}, cfg)
				errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "build.yml")}, nil)
				if err != nil {
					t.Fatal(err)
				}
				errs = errsWithID(errs, "untrusted-checkout")
				want := tc.want
				if pedantic {
					want = max(tc.want, 1)
				}
				if len(errs) != want {
					t.Fatalf("pedantic=%v: want %d findings but got %v", pedantic, want, errs)
				}
			}
		})
	}
}
