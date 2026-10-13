package jactionlint

import (
	"strings"
	"testing"
)

// The model of the gates of untrusted-checkout, in one table (the same text is in the checks document):
//
// A gate is in front of the job or the step that checks out the code of a pull request and runs it.
//
//   - A gate that holds AFTER the code changes stops the job: nothing is reported. It checks who wrote the code (the author
//     of the pull request: `pull_request.author_association`, `github.actor` for a pull_request_target or a workflow_run, a
//     permission check of a pull_request_target) or what the run is (a condition on the repository of the head).
//   - A gate that holds at ONE MOMENT lets the job start, and the author can still push to the branch afterwards: the checkout is
//     reported when the ref can change (a branch, `refs/pull/N/head`, `gh pr checkout`), not when it is a commit. These are an
//     environment with reviewers, a label of the event (`github.event.label.name`), and, for a comment or a dispatch, every
//     check of the user (`github.actor`, the association of the commenter, a permission check): who commented or dispatched is
//     not who wrote the code.
//   - A label that is on the pull request (`labels.*.name`) lasts: a workflow that runs again for each push runs the new code
//     with the label, so the checkout is reported whatever the ref is, unless the label is tied to the event or taken away.
//
// A gate that reads a context that the event does not have (the labels of a pull request in a comment, the association of the
// author in a dispatch) is not a case of the table.
//
// The events: pull_request_target and workflow_run (the author is the one who started the run), issue_comment and
// workflow_dispatch with the option pedantic (the user is not the author). A dispatch names the pull request by its number, so
// it has no commit to check out.
func TestUntrustedCheckoutGateModel(t *testing.T) {
	const co = "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2"
	const perm = "      - uses: sushichop/action-repository-permission@v3\n        with:\n          required-permission: write\n"
	const lookup = "      - id: pr\n        uses: xt0rted/pull-request-comment-branch@v3\n"

	type event struct {
		name     string
		on       string
		branch   string // a ref that can change
		commit   string // a ref that cannot, "" when the event has none
		pre      string // steps before the checkout for the commit
		pedantic bool
	}
	events := []event{
		{"pull_request_target", "on: pull_request_target\n", "${{ github.event.pull_request.head.ref }}", "${{ github.event.pull_request.head.sha }}", "", false},
		{"workflow_run", "on:\n  workflow_run:\n    workflows: [PR]\n    types: [completed]\n", "${{ github.event.workflow_run.head_branch }}", "${{ github.event.workflow_run.head_sha }}", "", false},
		{"issue_comment", "on: issue_comment\n", "refs/pull/${{ github.event.issue.number }}/head", "${{ steps.pr.outputs.head_sha }}", lookup, false},
		{"workflow_dispatch", "on:\n  workflow_dispatch:\n    inputs:\n      pr_number:\n        required: true\n", "refs/pull/${{ inputs.pr_number }}/head", "", "", true},
	}
	const (
		no  = "-" // nothing is reported
		yes = "R" // reported
		na  = "n" // the gate does not exist for the event
	)
	type gate struct {
		name string
		// job is a line of the job (`if:`, `environment:`), step a step in front of the checkout
		job, step string
		// want is the result for each event, for the branch and for the commit, as "BC" (R, -, n)
		want map[string]string
		// on replaces the trigger when the gate needs types
		on map[string]string
	}
	maintainers := "    if: contains(fromJSON('[\"OWNER\",\"MEMBER\",\"COLLABORATOR\"]'), github.event.comment.author_association)\n"
	gates := []gate{
		{name: "no gate", want: map[string]string{"pull_request_target": "RR", "workflow_run": "RR", "issue_comment": "RR", "workflow_dispatch": "Rn"}},
		{name: "environment", job: "    environment: untrusted\n", want: map[string]string{"pull_request_target": "R-", "workflow_run": "R-", "issue_comment": "R-", "workflow_dispatch": "Rn"}},
		{name: "permission check", step: perm, want: map[string]string{"pull_request_target": "--", "workflow_run": "--", "issue_comment": "R-", "workflow_dispatch": "Rn"}},
		{name: "actor", job: "    if: github.actor == 'maintainer'\n", want: map[string]string{"pull_request_target": "--", "workflow_run": "--", "issue_comment": "R-", "workflow_dispatch": "Rn"}},
		{name: "association of the author of the pull request", job: "    if: github.event.pull_request.author_association == 'MEMBER' || github.event.issue.author_association == 'MEMBER'\n", want: map[string]string{"pull_request_target": "--", "workflow_run": "nn", "issue_comment": "--", "workflow_dispatch": "nn"}},
		{name: "association of the commenter", job: maintainers, want: map[string]string{"pull_request_target": "nn", "workflow_run": "nn", "issue_comment": "R-", "workflow_dispatch": "nn"}},
		{name: "label that lasts", job: "    if: contains(github.event.pull_request.labels.*.name, 'safe to test')\n", want: map[string]string{"pull_request_target": "RR", "workflow_run": "nn", "issue_comment": "nn", "workflow_dispatch": "nn"}},
		{name: "label of the event", job: "    if: github.event.label.name == 'safe to test'\n", on: map[string]string{"pull_request_target": "on:\n  pull_request_target:\n    types: [labeled]\n"}, want: map[string]string{"pull_request_target": "R-", "workflow_run": "nn", "issue_comment": "nn", "workflow_dispatch": "nn"}},
	}
	for _, g := range gates {
		for _, ev := range events {
			want, ok := g.want[ev.name]
			if !ok {
				t.Fatalf("%s has no expectation for %s", g.name, ev.name)
			}
			for k, ref := range []string{ev.branch, ev.commit} {
				kind := []string{"branch", "commit"}[k]
				expect := string(want[k])
				if ref == "" {
					if expect != na {
						t.Fatalf("%s %s: no ref but expects %q", g.name, ev.name, expect)
					}
					continue
				}
				if expect == na {
					continue // the context of the gate does not exist for the event
				}
				t.Run(g.name+"/"+ev.name+"/"+kind, func(t *testing.T) {
					on := ev.on
					if o, ok := g.on[ev.name]; ok {
						on = o
					}
					steps := g.step
					if k == 1 {
						steps += ev.pre
					}
					src := on + "jobs:\n  j:\n" + g.job + "    runs-on: ubuntu-latest\n    steps:\n" + steps +
						"      - uses: " + co + "\n        with:\n          ref: " + ref + "\n      - run: make\n"
					cfg := "rules:\n  untrusted-checkout: error\n"
					if ev.pedantic {
						cfg = "rules:\n  untrusted-checkout: {level: error, pedantic: true}\n"
					}
					errs := onlyID(lintInProject(t, nil, src, cfg), "untrusted-checkout")
					got := len(errs) > 0
					if got != (expect == yes) {
						t.Errorf("want %q (R: reported, -: nothing) but got %d findings\n%s", expect, len(errs), strings.TrimSpace(src))
					}
				})
			}
		}
	}
}
