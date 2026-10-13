package jactionlint

import (
	"strings"
	"testing"
)

func TestRuleUntrustedCheckoutComment(t *testing.T) {
	const ic = "on: issue_comment\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	// a job with a condition
	icIf := func(cond string) string {
		return "on: issue_comment\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: " + cond + "\n    steps:\n"
	}
	const checkoutHead = `      - uses: actions/checkout@v4
        with:
          ref: refs/pull/${{ github.event.issue.number }}/head # want
      - run: make
`
	const checkoutHeadNoWant = `      - uses: actions/checkout@v4
        with:
          ref: refs/pull/${{ github.event.issue.number }}/head
      - run: make
`
	const maintainers = `contains(fromJSON('["OWNER","MEMBER","COLLABORATOR"]'), github.event.comment.author_association)`
	tests := []struct {
		what   string
		others map[string]string
		src    string
	}{
		{"gh pr checkout of the issue number", nil, ic + "      - run: gh pr checkout ${{ github.event.issue.number }} # want\n      - run: make\n"},
		{"gh pr checkout of the url of the pull request", nil, ic + "      - run: gh pr checkout ${{ github.event.issue.pull_request.html_url }} # want\n      - run: make\n"},
		{"gh pr checkout of a number in the env of the step", nil, ic + "      - run: gh pr checkout \"$PR\" # want\n        env:\n          PR: ${{ github.event.issue.number }}\n      - run: make\n"},
		{"gh pr checkout of a number in the env of the job", nil, "on: issue_comment\njobs:\n  j:\n    runs-on: ubuntu-latest\n    env:\n      PR: ${{ github.event.issue.number }}\n    steps:\n      - run: gh pr checkout $PR # want\n      - run: make\n"},
		{"gh pr checkout of a number in a variable of the script", nil, ic + "      - run: | # want\n          PR=${{ github.event.issue.number }}\n          gh pr checkout $PR\n          make\n"},
		{"gh pr checkout of a fixed pull request", nil, ic + "      - run: gh pr checkout 123\n      - run: make\n"},
		{"gh pr checkout of a number that is not known", nil, ic + "      - run: gh pr checkout ${{ steps.find.outputs.number }}\n      - run: make\n"},
		{"gh pr checkout that nothing runs", nil, ic + "      - run: gh pr checkout ${{ github.event.issue.number }}\n      - run: git diff --stat\n"},
		{"a branch named after the number is not the pull request", nil, ic + "      - run: |\n          git checkout -b fix-${{ github.event.issue.number }}\n          make\n"},
		{"a variable that holds the number is not a revision for git", nil, ic + "      - run: |\n          git checkout -b \"fix-$PR\"\n          make\n        env:\n          PR: ${{ github.event.issue.number }}\n"},
		{"checkout of refs/pull of the issue number", nil, ic + checkoutHead},
		{"checkout of the merge ref of the issue number with the repository of the event", nil, ic + "      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/merge # want\n      - run: npm ci\n"},
		{"fetch of pull/N/head of the issue number", nil, ic + "      - run: | # want\n          git fetch origin pull/${{ github.event.issue.number }}/head:pr\n          git checkout pr\n          make\n"},
		{"head of the pull request from xt0rted/pull-request-comment-branch", nil, ic + "      - id: pr\n        uses: xt0rted/pull-request-comment-branch@v3\n      - uses: actions/checkout@v4\n        with:\n          ref: ${{ steps.pr.outputs.head_sha }} # want\n      - run: make\n"},
		{"head ref from a fork of that action, with the repository", nil, ic + "      - id: pr\n        uses: alessbell/pull-request-comment-branch@v2\n      - uses: actions/checkout@v4\n        with:\n          repository: ${{ steps.pr.outputs.head_owner }}/${{ steps.pr.outputs.head_repo }}\n          ref: ${{ steps.pr.outputs.head_ref }} # want\n      - run: make\n"},
		{"head ref from eficode/resolve-pr-refs, used with git", nil, ic + "      - id: refs\n        uses: eficode/resolve-pr-refs@v1\n      - run: | # want\n          git fetch origin ${{ steps.refs.outputs.head_ref }}\n          git checkout FETCH_HEAD\n          make\n"},
		{"sha from github/branch-deploy", nil, ic + "      - id: bd\n        uses: github/branch-deploy@v11\n      - uses: actions/checkout@v4\n        with:\n          ref: ${{ steps.bd.outputs.sha }} # want\n      - run: make\n"},
		{"the base of the pull request from that action is not the head", nil, ic + "      - id: pr\n        uses: xt0rted/pull-request-comment-branch@v3\n      - uses: actions/checkout@v4\n        with:\n          ref: ${{ steps.pr.outputs.base_ref }}\n      - run: make\n"},
		{"an output of another action", nil, ic + "      - id: pr\n        uses: actions/github-script@v7\n      - uses: actions/checkout@v4\n        with:\n          ref: ${{ steps.pr.outputs.head_sha }}\n      - run: make\n"},
		{"an output of a step that is not in the job", nil, ic + "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ steps.pr.outputs.head_sha }}\n      - run: make\n"},
		{"checkout of the default branch", nil, ic + "      - uses: actions/checkout@v4\n      - run: make\n"},
		{"a workflow that issues start", nil, "on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: gh pr checkout ${{ github.event.issue.number }}\n      - run: make\n"},

		// gates
		{"association: members only", nil, icIf(maintainers) + checkoutHeadNoWant},
		{"association: the owner only", nil, icIf("github.event.comment.author_association == 'OWNER'") + checkoutHeadNoWant},
		{"association: in a conjunction with the command", nil, icIf("github.event.issue.pull_request && startsWith(github.event.comment.body, '/test') && "+maintainers) + checkoutHeadNoWant},
		{"association of the pull request author", nil, icIf("contains(fromJSON('[\"OWNER\",\"MEMBER\"]'), github.event.issue.author_association)") + checkoutHeadNoWant},
		{"association: everyone but NONE is not a gate", nil, icIf("github.event.comment.author_association != 'NONE'") + checkoutHead},
		{"association: contributors are not trusted", nil, icIf("contains(fromJSON('[\"OWNER\",\"MEMBER\",\"COLLABORATOR\",\"CONTRIBUTOR\"]'), github.event.comment.author_association)") + checkoutHead},
		{"the command alone is not a gate", nil, icIf("github.event.issue.pull_request && contains(github.event.comment.body, '/test')") + checkoutHead},
		{"the author of the comment by name", nil, icIf("github.event.comment.user.login == 'maintainer'") + checkoutHeadNoWant},
		{"the actor", nil, icIf("github.actor == 'maintainer'") + checkoutHeadNoWant},
		{"an association condition on the step that runs the code", nil, ic + "      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head\n      - run: make\n        if: " + maintainers + "\n"},
		{"an environment", nil, "on: issue_comment\njobs:\n  j:\n    runs-on: ubuntu-latest\n    environment: ci\n    steps:\n" + checkoutHeadNoWant},
		{"gate: sushichop/action-repository-permission with write", nil, ic + "      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n" + checkoutHeadNoWant},
		{"gate: sushichop/action-repository-permission with its default (admin)", nil, ic + "      - uses: sushichop/action-repository-permission@v3\n" + checkoutHeadNoWant},
		{"gate: sushichop/action-repository-permission with read", nil, ic + "      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: read\n" + checkoutHead},
		{"gate: sushichop/action-repository-permission that is relaxed", nil, ic + "      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n          relax: true\n" + checkoutHead},
		{"gate: with continue-on-error", nil, ic + "      - uses: prince-chrismc/check-actor-permissions-action@v3\n        continue-on-error: true\n        with:\n          permission: write\n" + checkoutHead},
		{"gate: with a condition of its own", nil, ic + "      - uses: prince-chrismc/check-actor-permissions-action@v3\n        if: github.event.issue.pull_request\n        with:\n          permission: write\n" + checkoutHead},
		{"gate: prince-chrismc/check-actor-permissions-action", nil, ic + "      - uses: prince-chrismc/check-actor-permissions-action@v3\n        with:\n          permission: admin\n" + checkoutHeadNoWant},
		{"gate: lannonbr/repo-permission-check-action", nil, ic + "      - uses: lannonbr/repo-permission-check-action@2.0.0\n        with:\n          permission: write\n" + checkoutHeadNoWant},
		{"gate: xt0rted/slash-command-action with its default level", nil, ic + "      - uses: xt0rted/slash-command-action@v2\n        with:\n          command: test\n" + checkoutHeadNoWant},
		{"gate: xt0rted/slash-command-action with read", nil, ic + "      - uses: xt0rted/slash-command-action@v2\n        with:\n          command: test\n          permission-level: read\n" + checkoutHead},
		{"gate: actions-team-membership that exits", nil, ic + "      - uses: TheModdingInquisition/actions-team-membership@v1\n        with:\n          team: maintainers\n" + checkoutHeadNoWant},
		{"gate: actions-team-membership that does not exit", nil, ic + "      - uses: TheModdingInquisition/actions-team-membership@v1\n        with:\n          team: maintainers\n          exit: false\n" + checkoutHead},
		{"gate: actions-cool/check-user-permission only sets an output", nil, ic + "      - uses: actions-cool/check-user-permission@v2\n        with:\n          require: write\n" + checkoutHead},
		{"gate: actions-cool/check-user-permission with error-if-missing", nil, ic + "      - uses: actions-cool/check-user-permission@v2\n        with:\n          require: write\n          error-if-missing: true\n" + checkoutHeadNoWant},
		{"gate: github-script that asks for the permission and throws", nil, ic + "      - uses: actions/github-script@v7\n        with:\n          script: |\n            const { data } = await github.rest.repos.getCollaboratorPermissionLevel({ owner: context.repo.owner, repo: context.repo.repo, username: context.actor });\n            if (!['admin', 'write'].includes(data.permission)) throw new Error('not allowed');\n" + checkoutHeadNoWant},
		{"gate: github-script that asks for the permission and only reports it", nil, ic + "      - uses: actions/github-script@v7\n        with:\n          script: |\n            const { data } = await github.rest.repos.getCollaboratorPermissionLevel({ owner: context.repo.owner, repo: context.repo.repo, username: context.actor });\n            core.setOutput('permission', data.permission);\n" + checkoutHead},
		{"gate: after the checkout, before the build", nil, ic + "      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n      - run: make\n"},
		{"gate: after the build", nil, ic + "      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head # want\n      - run: make\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n"},
		{"gate: in a job that this one needs", nil, "on: issue_comment\njobs:\n  permission:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n  j:\n    runs-on: ubuntu-latest\n    needs: permission\n    steps:\n" + checkoutHeadNoWant},
		{"gate: in a job that this one does not need", nil, "on: issue_comment\njobs:\n  permission:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + checkoutHead},
		{"gate: not an action of the list", nil, ic + "      - uses: some/permission-check@v1\n" + checkoutHead},
		{"needs: a job that only maintainers start", nil, "on: issue_comment\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    if: " + maintainers + "\n    steps:\n      - run: echo ok\n  j:\n    runs-on: ubuntu-latest\n    needs: gate\n    steps:\n" + checkoutHeadNoWant},
		{"needs: a job that only maintainers start, two levels up", nil, "on: issue_comment\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    if: " + maintainers + "\n    steps:\n      - run: echo ok\n  mid:\n    runs-on: ubuntu-latest\n    needs: gate\n    steps:\n      - run: echo ok\n  j:\n    runs-on: ubuntu-latest\n    needs: [mid]\n    steps:\n" + checkoutHeadNoWant},
		{"needs: a job that only maintainers start, when this one runs after skipped jobs", nil, "on: issue_comment\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    if: " + maintainers + "\n    steps:\n      - run: echo ok\n  j:\n    runs-on: ubuntu-latest\n    needs: gate\n    if: always()\n    steps:\n" + checkoutHead},
		{"needs: a job that is for another event is skipped, and so is this one", nil, "on:\n  issue_comment:\n  workflow_dispatch:\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    if: github.event_name == 'workflow_dispatch'\n    steps:\n      - run: echo ok\n  j:\n    runs-on: ubuntu-latest\n    needs: gate\n    steps:\n" + checkoutHeadNoWant},
		{"gh pr checkout of the number in a script of the repository", map[string]string{"../../scripts/co.sh": "gh pr checkout \"$1\"\n"}, ic + "      - run: ./scripts/co.sh ${{ github.event.issue.number }} # want\n      - run: make\n"},
		{"gh pr checkout of something else in a script of the repository", map[string]string{"../../scripts/co.sh": "gh pr checkout \"$1\"\n"}, ic + "      - run: ./scripts/co.sh 123\n      - run: make\n"},
		{"the number written to GITHUB_ENV by an earlier step", nil, ic + "      - run: echo \"PR=${{ github.event.issue.number }}\" >> \"$GITHUB_ENV\"\n      - run: gh pr checkout \"$PR\" # want\n      - run: make\n"},
		{"the number of the env of the step copied to GITHUB_ENV by an earlier step", nil, ic + "      - run: echo \"N=$PR\" >> \"$GITHUB_ENV\"\n        env:\n          PR: ${{ github.event.issue.number }}\n      - run: gh pr checkout \"$N\" # want\n      - run: make\n"},
		{"the number of the env of the job copied to GITHUB_ENV by an earlier step", nil, "on: issue_comment\njobs:\n  j:\n    runs-on: ubuntu-latest\n    env:\n      PR: ${{ github.event.issue.number }}\n    steps:\n      - run: echo \"N=$PR\" >> \"$GITHUB_ENV\"\n      - run: gh pr checkout \"$N\" # want\n      - run: make\n"},
		{"the number copied to GITHUB_ENV and a clean value written by a step with a condition", nil, ic + "      - run: echo \"N=$PR\" >> \"$GITHUB_ENV\"\n        env:\n          PR: ${{ github.event.issue.number }}\n      - run: echo \"N=123\" >> \"$GITHUB_ENV\"\n        if: github.event.action == 'created'\n      - run: gh pr checkout \"$N\" # want\n      - run: make\n"},
		{"the number copied to GITHUB_ENV and a clean value written by a later step", nil, ic + "      - run: echo \"N=$PR\" >> \"$GITHUB_ENV\"\n        env:\n          PR: ${{ github.event.issue.number }}\n      - run: echo \"N=123\" >> \"$GITHUB_ENV\"\n      - run: gh pr checkout \"$N\"\n      - run: make\n"},
		{"another number of the env copied to GITHUB_ENV by an earlier step", nil, ic + "      - run: echo \"N=$PR\" >> \"$GITHUB_ENV\"\n        env:\n          PR: '123'\n      - run: gh pr checkout \"$N\"\n      - run: make\n"},
		{"another value written to GITHUB_ENV by an earlier step", nil, ic + "      - run: echo \"PR=123\" >> \"$GITHUB_ENV\"\n      - run: gh pr checkout \"$PR\"\n      - run: make\n"},
		{"a step for the comment of a workflow that also has pull_request_target: head_ref is empty", nil, "on:\n  issue_comment:\n  pull_request_target:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        if: github.event_name == 'issue_comment'\n        with:\n          ref: ${{ github.head_ref }}\n      - run: make\n        if: github.event_name == 'issue_comment'\n"},
		{"needs: a job with a permission check that goes on after a failure", nil, "on: issue_comment\njobs:\n  check:\n    runs-on: ubuntu-latest\n    continue-on-error: true\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n  j:\n    runs-on: ubuntu-latest\n    needs: check\n    steps:\n" + checkoutHead},
		{"needs: a job with a permission check", nil, "on: issue_comment\njobs:\n  check:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n  j:\n    runs-on: ubuntu-latest\n    needs: check\n    steps:\n" + checkoutHeadNoWant},
		{"needs: a job that runs for the comment only is judged for the comment: gh pr checkout of a fixed number", nil, "on:\n  issue_comment:\n  pull_request_target:\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    if: github.event_name == 'issue_comment'\n    steps:\n      - run: echo ok\n  j:\n    runs-on: ubuntu-latest\n    needs: gate\n    steps:\n      - run: gh pr checkout 123\n      - run: make\n"},
		{"needs: a job that runs for the comment only is judged for the comment: the number of the issue", nil, "on:\n  issue_comment:\n  pull_request_target:\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    if: github.event_name == 'issue_comment'\n    steps:\n      - run: echo ok\n  j:\n    runs-on: ubuntu-latest\n    needs: gate\n    steps:\n      - run: gh pr checkout ${{ github.event.issue.number }} # want\n      - run: make\n"},
		{"needs: a job that is not guarded", nil, "on: issue_comment\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    if: contains(github.event.comment.body, '/test')\n    steps:\n      - run: echo ok\n  j:\n    runs-on: ubuntu-latest\n    needs: gate\n    steps:\n" + checkoutHead},
		{"needs: a job that has an environment", nil, "on: issue_comment\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    environment: approval\n    steps:\n      - run: echo ok\n  j:\n    runs-on: ubuntu-latest\n    needs: gate\n    steps:\n" + checkoutHeadNoWant},
		{"needs: a job with a permission gate", nil, "on: issue_comment\njobs:\n  gate:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: prince-chrismc/check-actor-permissions-action@v3\n        with:\n          permission: write\n  j:\n    runs-on: ubuntu-latest\n    needs: gate\n    steps:\n" + checkoutHeadNoWant},
		{"the job is for other events", nil, "on:\n  issue_comment:\n  workflow_dispatch:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: github.event_name == 'workflow_dispatch'\n    steps:\n" + checkoutHeadNoWant},
		{"the job is for other events, with an or", nil, "on:\n  issue_comment:\n  merge_group:\n  workflow_dispatch:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: ${{ github.event_name == 'merge_group' || github.event_name == 'workflow_dispatch' }}\n    steps:\n" + checkoutHeadNoWant},
		{"the job is for this event and others", nil, "on:\n  issue_comment:\n  push:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: github.event_name == 'push' || github.event_name == 'issue_comment'\n    steps:\n" + checkoutHead},
		{"the step is for other events", nil, "on:\n  issue_comment:\n  push:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        if: github.event_name == 'push'\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head\n      - run: make\n"},
		{"the step that runs the code is for this event", nil, "on:\n  issue_comment:\n  push:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          ref: refs/pull/${{ github.event.issue.number }}/head # want\n      - run: make\n        if: github.event_name == 'issue_comment'\n"},
		{"github.head_ref is empty when only a comment starts the workflow", nil, "on:\n  issue_comment:\n  pull_request:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.head_ref }}\n      - run: make\n"},
		{"the head of a pull request in a workflow with pull_request_target and a comment", nil, "on:\n  issue_comment:\n  pull_request_target:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v1\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n      - run: make\n"},
		{"pull_request_target: the job is for another event", nil, "on:\n  pull_request_target:\n  push:\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: github.event_name == 'push'\n    steps:\n      - uses: actions/checkout@v1\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n      - run: make\n"},
		{"pull_request_target: everyone but NONE is not a gate", nil, "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: github.event.pull_request.author_association != 'NONE'\n    steps:\n      - uses: actions/checkout@v1\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n      - run: make\n"},
		{"pull_request_target: members only is a gate", nil, "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: github.event.pull_request.author_association == 'MEMBER'\n    steps:\n      - uses: actions/checkout@v1\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n      - run: make\n"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := onlyID(lintInProject(t, tc.others, tc.src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
			checkLines(t, errs, markedWantLines(tc.src)...)
			for _, e := range errs {
				if strings.HasPrefix(tc.src, "on: issue_comment\n") && !strings.Contains(e.Message, `"issue_comment" workflow`) {
					t.Errorf("the message does not name the event: %s", e.Message)
				}
			}
		})
	}
}

func TestRuleUntrustedCheckoutReadOnlyTokenMessage(t *testing.T) {
	for _, perms := range []string{"permissions: read-all", "permissions:\n      contents: read\n      pull-requests: read"} {
		src := "on: issue_comment\njobs:\n  j:\n    runs-on: ubuntu-latest\n    " + perms + "\n    steps:\n      - run: gh pr checkout ${{ github.event.issue.number }}\n      - run: make\n"
		errs := onlyID(lintInProject(t, nil, src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
		if len(errs) != 1 {
			t.Fatalf("%s: want one finding, got %v", perms, errs)
		}
		if strings.Contains(errs[0].Message, "write token") || !strings.Contains(errs[0].Message, "can only read") {
			t.Fatalf("%s: the message claims a write token: %s", perms, errs[0].Message)
		}
	}
	src := "on: issue_comment\njobs:\n  j:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: read\n      pull-requests: write\n    steps:\n      - run: gh pr checkout ${{ github.event.issue.number }}\n      - run: make\n"
	errs := onlyID(lintInProject(t, nil, src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "write token") {
		t.Fatalf("a job that can write: %v", errs)
	}
}
