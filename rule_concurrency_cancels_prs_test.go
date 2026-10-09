package jactionlint

import "testing"

func TestRuleConcurrencyCancelsPRs(t *testing.T) {
	const jobs = "jobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	tests := []struct {
		what string
		src  string
		want []string // lines
	}{
		{"constant group", "on: pull_request\nconcurrency:\n  group: ci\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"workflow name only", "on: pull_request\nconcurrency:\n  group: ${{ github.workflow }}\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"base branch is the same for all", "on: pull_request\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.base_ref }}\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"base ref of the event", "on: pull_request\nconcurrency:\n  group: ${{ github.event.pull_request.base.ref }}\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"no cancel", "on: pull_request\nconcurrency:\n  group: ci\n" + jobs, nil},
		{"cancel false", "on: pull_request\nconcurrency:\n  group: ci\n  cancel-in-progress: false\n" + jobs, nil},
		{"github.ref", "on: pull_request\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"head_ref", "on: pull_request\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.head_ref }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"head_ref or run_id", "on: [pull_request, push]\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.head_ref || github.run_id }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"pr number", "on: pull_request\nconcurrency:\n  group: pr-${{ github.event.pull_request.number }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"whole event", "on: pull_request\nconcurrency:\n  group: ${{ toJSON(github.event) }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"index access", "on: pull_request\nconcurrency:\n  group: ${{ github['head_ref'] }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"format", "on: pull_request\nconcurrency:\n  group: ${{ format('{0}-{1}', github.workflow, github.head_ref) }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"github.ref is the base branch for pull_request_target", "on: pull_request_target\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"head_ref for pull_request_target", "on: pull_request_target\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.head_ref }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"head_ref is empty for pull_request_review", "on: pull_request_review\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.head_ref }}\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"event number for pull_request", "on: pull_request\nconcurrency:\n  group: pr-${{ github.event.number }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"event number for pull_request_target", "on: pull_request_target\nconcurrency:\n  group: pr-${{ github.event.number }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"event number does not exist for pull_request_review", "on: pull_request_review\nconcurrency:\n  group: pr-${{ github.event.number }}\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"event number does not exist for pull_request_review_comment", "on: pull_request_review_comment\nconcurrency:\n  group: pr-${{ github.event.number }}\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"pull request number for pull_request_review", "on: pull_request_review\nconcurrency:\n  group: pr-${{ github.event.pull_request.number }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"ref for pull_request_review", "on: pull_request_review\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"mixed idiom is true for pull requests", "on: [pull_request, push]\nconcurrency:\n  group: ci\n  cancel-in-progress: ${{ github.event_name == 'pull_request' }}\n" + jobs, []string{"3"}},
		{"mixed idiom with a discriminator", "on: [pull_request, push]\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: ${{ github.event_name == 'pull_request' }}\n" + jobs, nil},
		{"never true for pull requests", "on: [pull_request, push]\nconcurrency:\n  group: ci\n  cancel-in-progress: ${{ github.event_name != 'pull_request' }}\n" + jobs, nil},
		{"expression that depends on something unknown is not judged", "on: pull_request\nconcurrency:\n  group: ci\n  cancel-in-progress: ${{ vars.CANCEL == 'yes' }}\n" + jobs, nil},
		{"not the default branch", "on: pull_request\nconcurrency:\n  group: ci\n  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}\n" + jobs, []string{"3"}},
		{"only on the default branch", "on: pull_request\nconcurrency:\n  group: ci\n  cancel-in-progress: ${{ github.ref == 'refs/heads/main' }}\n" + jobs, nil},
		{"group from env is not judged", "on: pull_request\nenv:\n  G: x\nconcurrency:\n  group: ${{ env.G }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"group from vars is not judged", "on: pull_request\nconcurrency:\n  group: ${{ vars.G }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"push only", "on: push\nconcurrency:\n  group: ci\n  cancel-in-progress: true\n" + jobs, nil},
		{"issue_comment is not a pull request event", "on: issue_comment\nconcurrency:\n  group: ci\n  cancel-in-progress: true\n" + jobs, nil},
		{"job level", "on: pull_request\njobs:\n  t:\n    runs-on: ubuntu-latest\n    concurrency:\n      group: ci\n      cancel-in-progress: true\n    steps:\n      - run: echo\n", []string{"6"}},
		{"job level with discriminator", "on: pull_request\njobs:\n  t:\n    runs-on: ubuntu-latest\n    concurrency:\n      group: ci-${{ github.head_ref }}\n      cancel-in-progress: true\n    steps:\n      - run: echo\n", nil},
		{"job only for push", "on: [pull_request, push]\njobs:\n  t:\n    if: github.event_name == 'push'\n    runs-on: ubuntu-latest\n    concurrency:\n      group: pages\n      cancel-in-progress: true\n    steps:\n      - run: echo\n", nil},
		{"job only for issues in a workflow with pull_request_target", "on:\n  pull_request_target:\n  issues:\njobs:\n  t:\n    if: github.event_name == 'issues'\n    runs-on: ubuntu-latest\n    concurrency:\n      group: x\n      cancel-in-progress: true\n    steps:\n      - run: echo\n", nil},
		{"job for pull requests is still reported", "on: [pull_request, push]\njobs:\n  t:\n    if: github.event_name == 'pull_request'\n    runs-on: ubuntu-latest\n    concurrency:\n      group: x\n      cancel-in-progress: true\n    steps:\n      - run: echo\n", []string{"7"}},
		{"job with an unknown condition is still reported", "on: pull_request\njobs:\n  t:\n    if: vars.X == 'y'\n    runs-on: ubuntu-latest\n    concurrency:\n      group: x\n      cancel-in-progress: true\n    steps:\n      - run: echo\n", []string{"7"}},
		{"workflow group, every job excludes pull requests", "on: [pull_request, push]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    if: github.event_name != 'pull_request'\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"workflow group, one job runs for pull requests", "on: [pull_request, push]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    if: github.event_name != 'pull_request'\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n  u:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", []string{"3"}},
		{"review id chain for pull_request_review", "on: [issues, pull_request_review]\nconcurrency:\n  group: t-${{ github.event.comment.id || github.event.issue.number || github.event.review.id }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"review id is not a discriminator of pull_request", "on: pull_request\nconcurrency:\n  group: t-${{ github.event.review.id }}\n  cancel-in-progress: true\n" + jobs, []string{"3"}},
		{"comment id for pull_request_review_comment", "on: pull_request_review_comment\nconcurrency:\n  group: t-${{ github.event.comment.id }}\n  cancel-in-progress: true\n" + jobs, nil},
		{"short form has no cancel", "on: pull_request\nconcurrency: ci\n" + jobs, nil},
		{"syntax error in group is not judged", "on: pull_request\nconcurrency:\n  group: ${{ github. }}\n  cancel-in-progress: true\n" + jobs, nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			checkLines(t, lintBatchH(t, "rules:\n  concurrency-cancels-prs: error\n", tc.src, "concurrency-cancels-prs"), tc.want...)
		})
	}
}
