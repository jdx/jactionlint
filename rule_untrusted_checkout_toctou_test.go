package jactionlint

import (
	"strings"
	"testing"
)

func TestRuleUntrustedCheckoutToctou(t *testing.T) {
	// a checkout that actions/checkout does not refuse (a commit before the protection), so the gates are what is judged
	const co = "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2"
	prtJob := func(on, job string) string {
		return "on:\n" + on + "jobs:\n  j:\n" + job + "    runs-on: ubuntu-latest\n    steps:\n"
	}
	const prt = "  pull_request_target:\n"
	const prtLabeled = "  pull_request_target:\n    types: [labeled]\n"
	const prtPush = "  pull_request_target:\n    types: [opened, synchronize, labeled]\n"
	const env = "    environment: untrusted\n"
	const labelIf = "    if: contains(github.event.pull_request.labels.*.name, 'safe to test')\n"
	headRef := "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.ref }} # want\n      - run: make\n"
	headRefNoWant := strings.Replace(headRef, " # want", "", 1)
	headSha := "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n      - run: make\n"
	headShaNoWant := strings.Replace(headSha, " # want", "", 1)
	const comment = "  issue_comment:\n"
	const maintainers = "    if: contains(fromJSON('[\"OWNER\",\"MEMBER\",\"COLLABORATOR\"]'), github.event.comment.author_association)\n"
	tests := []struct {
		what     string
		src      string
		pedantic bool
		text     string
	}{
		// gate + a ref that can change
		{"environment and the branch of the head", prtJob(prt, env) + headRef, false, "an environment lets the job start"},
		{"environment and github.head_ref", prtJob(prt, env) + "      - uses: " + co + "\n        with:\n          ref: ${{ github.head_ref }} # want\n      - run: make\n", false, ""},
		{"environment and the merge ref", prtJob(prt, env) + "      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ github.event.number }}/merge # want\n      - run: make\n", false, ""},
		{"environment and gh pr checkout", prtJob(prt, env) + "      - uses: " + co + "\n      - run: gh pr checkout ${{ github.event.number }} # want\n      - run: make\n", false, ""},
		{"environment and a fetch of the head ref", prtJob(prt, env) + "      - uses: " + co + "\n      - run: | # want\n          git fetch origin pull/${{ github.event.number }}/head:pr\n          git checkout pr\n          make\n", false, ""},
		{"environment and the commit", prtJob(prt, env) + headShaNoWant, false, ""},
		{"environment and the merge commit", prtJob(prt, env) + "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.merge_commit_sha }}\n      - run: make\n", false, ""},
		{"environment and a ref that is not known", prtJob(prt, env) + "      - uses: " + co + "\n        with:\n          ref: ${{ needs.prepare.outputs.ref }}\n      - run: make\n", false, ""},
		{"environment and nothing that runs the code", prtJob(prt, env) + "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.ref }}\n      - run: git diff --stat\n", false, ""},
		{"environment, the branch and a check of the commit", prtJob(prt, env) + "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.ref }}\n      - run: |\n          [ \"$(git rev-parse HEAD)\" = \"$VERIFIED_SHA\" ] || exit 1\n        env:\n          VERIFIED_SHA: ${{ github.event.pull_request.head.sha }}\n      - run: make\n", false, ""},
		{"label for the labeled event and the branch", prtJob(prtLabeled, "    if: github.event.label.name == 'safe to test'\n") + headRef, false, "a label condition lets the job start"},
		{"label for the labeled event and the commit", prtJob(prtLabeled, "    if: github.event.label.name == 'safe to test'\n") + headShaNoWant, false, ""},
		{"label on the pull request, labeled only, and the branch", prtJob(prtLabeled, labelIf) + headRef, false, ""},
		{"a job that needs a job with a label", "on:\n" + prtLabeled + "jobs:\n  check:\n    runs-on: ubuntu-latest\n" + labelIf + "    steps:\n      - run: echo ok\n  j:\n    needs: check\n    runs-on: ubuntu-latest\n    steps:\n" + headRef, false, ""},
		{"an author who is a member, and the branch", prtJob(prt, "    if: github.event.pull_request.author_association == 'MEMBER'\n") + headRefNoWant, false, ""},
		{"the repository of the head, and the branch", prtJob(prt, "    if: github.event.pull_request.head.repo.full_name == github.repository\n") + headRefNoWant, false, ""},
		{"a label and an author who is a member", prtJob(prt, "    if: github.event.pull_request.author_association == 'MEMBER' && contains(github.event.pull_request.labels.*.name, 'x')\n") + headRefNoWant, false, ""},
		{"a permission check of the author in a pull_request_target workflow", prtJob(prt, "") + "      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n" + headRefNoWant, false, ""},
		{"a label on a step that runs the code, with the branch", prtJob(prtLabeled, "") + "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.ref }} # want\n      - run: make\n        if: github.event.label.name == 'safe to test'\n", false, ""},

		{"a label that lasts and an environment: the commit", prtJob(prtPush, env+labelIf) + headShaNoWant, false, ""},
		{"a label that lasts and an environment: the branch", prtJob(prtPush, env+labelIf) + headRef, false, "a label condition and an environment lets the job start, but"},
		{"the branch that an action looks up behind an environment", prtJob(prt, env) + "      - id: o\n        uses: potiuk/get-workflow-origin@v1\n      - uses: " + co + "\n        with:\n          ref: ${{ steps.o.outputs.sourceHeadBranch }} # want\n      - run: make\n", false, ""},
		{"the commit that an action looks up behind an environment", prtJob(prt, env) + "      - id: o\n        uses: potiuk/get-workflow-origin@v1\n      - uses: " + co + "\n        with:\n          ref: ${{ steps.o.outputs.sourceHeadSha }}\n      - run: make\n", false, ""},

		// comments: the commenter is not the author of the pull request
		{"a comment of a maintainer and the number of the issue", prtJob(comment, maintainers) + "      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head # want\n      - run: make\n", false, "a check of the user who commented lets the job start"},
		{"a comment of a maintainer and gh pr checkout", prtJob(comment, maintainers) + "      - run: gh pr checkout ${{ github.event.issue.number }} # want\n      - run: make\n", false, ""},
		{"a comment of a maintainer and the commit that an action found", prtJob(comment, maintainers) + "      - id: pr\n        uses: xt0rted/pull-request-comment-branch@v3\n      - uses: " + co + "\n        with:\n          ref: ${{ steps.pr.outputs.head_sha }}\n      - run: make\n", false, ""},
		{"a comment of a maintainer and the branch that an action found", prtJob(comment, maintainers) + "      - id: pr\n        uses: xt0rted/pull-request-comment-branch@v3\n      - uses: " + co + "\n        with:\n          ref: ${{ steps.pr.outputs.head_ref }} # want\n      - run: make\n", false, ""},
		{"a permission check of the commenter and the number of the issue", prtJob(comment, "") + "      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head # want\n      - run: make\n", false, "a permission check of the user who commented"},
		{"the author of the pull request is a member", prtJob(comment, "    if: contains(fromJSON('[\"OWNER\",\"MEMBER\"]'), github.event.issue.author_association)\n") + "      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head\n      - run: make\n", false, ""},
		{"a comment of a maintainer, the number of the issue and a check of the commit", prtJob(comment, maintainers) + "      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head\n      - name: Verify the commit\n        run: |\n          SHA=$(git log -1 --format=%H)\n          if [ \"$SHA\" != \"$EXPECTED_SHA\" ]; then exit 1; fi\n        env:\n          EXPECTED_SHA: ${{ needs.pr.outputs.sha }}\n      - run: make\n", false, ""},

		// the events of the step, not of the job, say who a check is about
		{"a permission check in a job for a pull request and a comment, and a step for the comment", "on:\n  pull_request_target:\n  issue_comment:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: " + co + "\n        if: github.event_name == 'issue_comment'\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head # want\n      - run: make\n        if: github.event_name == 'issue_comment'\n", false, "a permission check of the user who commented"},
		{"a permission check in a job for a pull request and a comment, and a step for the pull request", "on:\n  pull_request_target:\n  issue_comment:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: " + co + "\n        if: github.event_name == 'pull_request_target'\n        with:\n          ref: ${{ github.event.pull_request.head.ref }}\n      - run: make\n        if: github.event_name == 'pull_request_target'\n", false, ""},

		// what a permission check of the commenter does not hold back
		{"a permission check of the commenter and a commit, behind it", prtJob(comment, "") + "      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - id: pr\n        uses: xt0rted/pull-request-comment-branch@v3\n" + "      - uses: " + co + "\n        with:\n          ref: ${{ steps.pr.outputs.head_sha }}\n      - run: make\n", false, ""},
		{"a permission check of the commenter and a commit, in steps that run after a failure", prtJob(comment, "") + "      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - id: pr\n        uses: xt0rted/pull-request-comment-branch@v3\n        if: always()\n" + "      - uses: " + co + "\n        if: always()\n        with:\n          ref: ${{ steps.pr.outputs.head_sha }} # want\n      - run: make\n        if: always()\n", false, ""},
		{"a permission check of the commenter and a commit, and a step that runs the code beside it in a parallel group", prtJob(comment, "") + "      - id: pr\n        uses: xt0rted/pull-request-comment-branch@v3\n      - uses: " + co + "\n        with:\n          ref: ${{ steps.pr.outputs.head_sha }} # want\n      - parallel:\n          - uses: sushichop/action-repository-permission@v3\n            with:\n              required-permission: write\n          - run: make\n", false, ""},

		// a step with a gate that holds at one moment does not stop a later step from running the code
		{"a step for a label that runs the code of a commit, and a step that runs it again", prtJob(prtLabeled, "") + "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n      - run: make\n        if: github.event.label.name == 'safe to test'\n      - run: make test\n", false, ""},

		// an expression with alternatives is a branch when one of them is
		{"environment and a commit with the branch as a fallback", prtJob(prt, env) + "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha || github.head_ref }} # want\n      - run: make\n", false, ""},
		{"environment and two commits", prtJob(prt, env) + "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha || github.event.pull_request.merge_commit_sha }}\n      - run: make\n", false, ""},

		// a dispatch: who started the run is not what the pull request contains at the checkout
		{"dispatch, an actor and a pull request number", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\njobs:\n  j:\n    if: github.actor == 'maintainer'\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ inputs.pr_number }}/head # want\n      - run: make\n", true, "who dispatched"},
		{"dispatch, a permission check and a pull request number", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ inputs.pr_number }}/head # want\n      - run: make\n", true, "who dispatched"},
		{"dispatch, an actor and gh pr checkout", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\njobs:\n  j:\n    if: github.actor == 'maintainer'\n    runs-on: ubuntu-latest\n    steps:\n      - run: gh pr checkout ${{ inputs.pr_number }} # want\n      - run: make\n", true, ""},

		// a label that lasts
		{"a label that lasts and the commit", prtJob(prtPush, labelIf) + headSha, false, "stays true after the review"},
		{"a label that lasts, default types and the commit", prtJob(prt, labelIf) + headSha, false, ""},
		{"a label that lasts, on a step", prtJob(prtPush, "") + "      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n      - run: make\n        if: contains(github.event.pull_request.labels.*.name, 'safe to test')\n", false, ""},
		{"a label that lasts, and the job takes it away", prtJob(prtPush, labelIf) + headShaNoWant + "      - run: gh pr edit ${{ github.event.number }} --remove-label 'safe to test'\n", false, ""},
		{"a label that lasts, for the labeled action only", prtJob(prtPush, "    if: github.event.action == 'labeled' && contains(github.event.pull_request.labels.*.name, 'safe to test')\n") + headShaNoWant, false, ""},
		{"a label that lasts, for the synchronize action", prtJob(prtPush, "    if: github.event.action == 'synchronize' && contains(github.event.pull_request.labels.*.name, 'safe to test')\n") + headSha, false, "stays true after the review"},
		{"a label that lasts, for every action but labeled", prtJob(prtPush, "    if: github.event.action != 'labeled' && contains(github.event.pull_request.labels.*.name, 'safe to test')\n") + headSha, false, "stays true after the review"},
		{"a label that lasts, for the labeled action in another case", prtJob(prtPush, "    if: github.event.action == 'LABELED' && contains(github.event.pull_request.labels.*.name, 'safe to test')\n") + headShaNoWant, false, ""},
		{"the label of the event, for all types", prtJob(prtPush, "    if: github.event.label.name == 'safe to test'\n") + headShaNoWant, false, ""},
		{"a label on the pull request that runs for labeled only", prtJob(prtLabeled, labelIf) + headShaNoWant, false, ""},
		{"a label that lasts, with a member", prtJob(prtPush, "    if: github.event.pull_request.author_association == 'MEMBER' && contains(github.event.pull_request.labels.*.name, 'x')\n") + headShaNoWant, false, ""},

		// a dispatch with the number of a pull request
		{"dispatch with a number, without the option", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ inputs.pr_number }}/head\n      - run: make\n", false, ""},
		{"dispatch with a number", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ inputs.pr_number }}/head # want\n      - run: make\n", true, "a maintainer dispatches the workflow"},
		{"dispatch with a number named in another case", "on:\n  workflow_dispatch:\n    inputs:\n      PR_Number:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ inputs.PR_Number }}/head # want\n      - run: make\n", true, ""},
		{"dispatch with a number named in another case and gh pr checkout", "on:\n  workflow_dispatch:\n    inputs:\n      PR_Number:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n      - run: gh pr checkout ${{ inputs.pr_number }} # want\n      - run: make\n", true, ""},
		{"dispatch with a number and gh pr checkout", "on:\n  workflow_dispatch:\n    inputs:\n      pr:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n      - run: gh pr checkout ${{ github.event.inputs.pr }} # want\n      - run: make\n", true, ""},
		{"dispatch with a number and a required commit", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\n      sha:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ inputs.pr_number }}/head\n      - run: make\n", true, ""},
		{"dispatch with a number and an input that is not required", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\n      sha:\n        required: false\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ inputs.pr_number }}/head # want\n      - run: make\n", true, ""},
		{"dispatch with an input that is not a pull request", "on:\n  workflow_dispatch:\n    inputs:\n      version:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: ${{ inputs.version }}\n      - run: make\n", true, ""},
		{"dispatch with a number that checks out the base", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n      - run: make\n", true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			cfg := "rules:\n  untrusted-checkout: error\n"
			if tc.pedantic {
				cfg = "rules:\n  untrusted-checkout: {level: error, pedantic: true}\n"
			}
			errs := onlyID(lintInProject(t, nil, tc.src, cfg), "untrusted-checkout")
			checkLines(t, errs, markedWantLines(tc.src)...)
			if tc.text != "" && (len(errs) == 0 || !strings.Contains(errs[0].Message, tc.text)) {
				t.Errorf("the message lacks %q: %v", tc.text, errs)
			}
		})
	}
}

// With the option any-checkout a checkout of the code of a pull request is reported although nothing visibly runs it.
func TestRuleUntrustedCheckoutAnyCheckout(t *testing.T) {
	const prt = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n"
	const co = "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2"
	reads := prt + "    steps:\n      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n      - run: git diff --stat origin/main\n"
	readsNoWant := strings.Replace(reads, " # want", "", 1)
	tests := []struct {
		what string
		src  string
		cfg  string
		want bool
	}{
		{"a last step that is for another event", "on: [pull_request_target, issue_comment]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n      - run: git diff --stat origin/main\n      - run: echo commented\n        if: github.event_name == 'issue_comment'\n", "rules:\n  untrusted-checkout: {level: error, any-checkout: true}\n", true},
		{"the option is on", reads, "rules:\n  untrusted-checkout: {level: error, any-checkout: true}\n", true},
		{"the option is off", readsNoWant, "rules:\n  untrusted-checkout: error\n", false},
		{"the option is off in a pedantic profile", readsNoWant, "profile: pedantic\nrules:\n  untrusted-checkout: {level: error, any-checkout: false}\n", false},
		{"the pedantic profile", reads, "profile: pedantic\nrules:\n  untrusted-checkout: error\n", true},
		{"the option pedantic is not the option", readsNoWant, "rules:\n  untrusted-checkout: {level: error, pedantic: true}\n", false},
		{"a step for the merged pull request runs it", "on:\n  pull_request_target:\n    types: [closed]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n      - run: make\n        if: github.event.pull_request.merged == true\n", "rules:\n  untrusted-checkout: {level: error, any-checkout: true}\n", false},
		{"a gate in front of it", prt + "    environment: ci\n    steps:\n      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n      - run: git diff --stat\n", "rules:\n  untrusted-checkout: {level: error, any-checkout: true}\n", false},
		{"the code is run", prt + "    steps:\n      - uses: " + co + "\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n      - run: make\n", "rules:\n  untrusted-checkout: {level: error, any-checkout: true}\n", true},
		{"a checkout of the base", prt + "    steps:\n      - uses: " + co + "\n      - run: git diff --stat\n", "rules:\n  untrusted-checkout: {level: error, any-checkout: true}\n", false},
		{"a comment event", "on: issue_comment\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + co + "\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head\n      - run: git diff --stat\n", "rules:\n  untrusted-checkout: {level: error, any-checkout: true}\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := onlyID(lintInProject(t, nil, tc.src, tc.cfg), "untrusted-checkout")
			checkLines(t, errs, markedWantLines(tc.src)...)
		})
	}
}

// The kind of a ref is judged from the value that the caller passes, not from the name of the input or the file of the caller.
func TestRefIsMutableOfDescriptions(t *testing.T) {
	tests := []struct {
		what string
		want bool
	}{
		{"inputs.ref, given github.event.pull_request.head.ref by .github/workflows/pr.yaml", true},
		{"inputs.head_sha, given github.event.pull_request.head.ref by .github/workflows/pr.yaml", true},
		{"inputs.ref, given github.event.pull_request.head.ref by .github/workflows/pr_sha.yaml", true},
		{"inputs.head_sha, given github.event.pull_request.head.sha by .github/workflows/pr.yaml", false},
		{"inputs.ref, given github.event.pull_request.head.sha by .github/workflows/pr_head.yaml", false},
		{"inputs.ref, given github.event.pull_request.head.sha by a.yml, or github.event.pull_request.head.ref by b_sha.yml", true},
		{"inputs.ref, given github.event.pull_request.head.ref by a.yml, passed on by .github/workflows/c_sha.yml", true},
		{"github.event.pull_request.head.ref", true},
		{"github.event.pull_request.head.sha", false},
	}
	for _, tc := range tests {
		if got := refIsMutable(tc.what); got != tc.want {
			t.Errorf("refIsMutable(%q) = %v, want %v", tc.what, got, tc.want)
		}
	}
}
