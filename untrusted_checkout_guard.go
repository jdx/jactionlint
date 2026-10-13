package jactionlint

import (
	"regexp"
	"strconv"
	"strings"
)

// This file has what untrusted-checkout knows about the protection of actions/checkout against the code of fork pull
// requests: https://github.blog/changelog/2026-06-18-safer-pull_request_target-defaults-for-github-actions-checkout/
//
// Since v7, and backported on 2026-07-20 to the floating tags of every major but v1, `actions/checkout` refuses to check
// out the code of a fork pull request in a `pull_request_target` workflow, and in a `workflow_run` workflow started by a
// pull request event: when `repository` is the fork, when `ref` is `refs/pull/N/head` or `refs/pull/N/merge`, or when
// `ref` is the head or merge commit of the fork pull request. The input `allow-unsafe-pr-checkout: true` turns it off.
// Not covered by it: `git` and `gh` in a `run:` step, other events (`issue_comment`, `issues`, `workflow_call`...), a
// checkout of a branch name, and versions older than the fix: the release lines v2 to v6 got it in v2.8.0, v3.7.0, v4.4.0,
// v5.1.0 and v6.1.0 (the same floors poutine uses, poutine#450), v1 never.

// checkoutGuardFloors are the first release of each major version of actions/checkout that has the protection, as
// [major, minor, patch]. v7 and later have it from the start; v1 never has it.
var checkoutGuardFloors = map[int][3]int{
	2: {2, 8, 0},
	3: {3, 7, 0},
	4: {4, 4, 0},
	5: {5, 1, 0},
	6: {6, 1, 0},
}

var reCheckoutVersion = regexp.MustCompile(`^[vV]?(\d+)(?:\.(\d+))?(?:\.(\d+))?$`)

// checkoutVersion is the version of actions/checkout a step uses, as far as it is known.
type checkoutVersion struct {
	// known is false for a reference that does not say which release it is: a branch, a commit without a version comment,
	// a comment that is only a major version.
	known bool
	// text is how the version is written, for the message.
	text string
	// guarded is whether the release has the protection.
	guarded bool
	// floor is the first release of the line that has it, "" for v1 and for a version that has it from the start.
	floor string
}

// versionOfCheckout judges the ref of an `actions/checkout` step. A tag is read as it is: `v4` is the floating tag, which
// GitHub moved to the fixed release, and `v4.2.2` is that release. A commit has the version of its comment (`# v4.4.0`),
// which has to name a full release (major.minor.patch): a comment with a major version only says nothing about the
// commit. The comment is not verified (ref-version-mismatch does that with the network), so a guarded commit is only as
// good as its comment.
func versionOfCheckout(u *UsesRef, comment *Comment) checkoutVersion {
	ref := u.Ref
	full := false // the version has all three numbers
	if u.RefKind == RefFullSHA {
		if comment == nil {
			return checkoutVersion{}
		}
		v, ok := commentVersion(comment.Text)
		if !ok {
			return checkoutVersion{}
		}
		ref = v
		full = true
	}
	m := reCheckoutVersion.FindStringSubmatch(ref)
	if m == nil {
		return checkoutVersion{}
	}
	var n [3]int
	have := 0
	for i := 0; i < 3; i++ {
		if m[i+1] == "" {
			continue
		}
		v, err := strconv.Atoi(m[i+1])
		if err != nil {
			return checkoutVersion{}
		}
		n[i] = v
		have = i + 1
	}
	if full && have < 3 {
		return checkoutVersion{}
	}
	ver := checkoutVersion{known: true, text: ref}
	switch major := n[0]; {
	case major == 1:
		return ver
	case major >= 7:
		ver.guarded = true
	default:
		fl, ok := checkoutGuardFloors[major]
		if !ok {
			return checkoutVersion{}
		}
		ver.floor = "v" + strconv.Itoa(fl[0]) + "." + strconv.Itoa(fl[1]) + "." + strconv.Itoa(fl[2])
		// the floating major tag (`v4`) is what GitHub moved
		ver.guarded = have == 1 || compareVersion(n, fl) >= 0
	}
	return ver
}

func compareVersion(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// forkCommitContexts are the contexts that name the head or merge commit of a pull request that the protection refuses;
// forkRepoContexts are the ones that name the repository of its head.
var (
	forkCommitContexts = [][]string{
		{"github", "event", "pull_request", "head", "sha"},
		{"github", "event", "pull_request", "merge_commit_sha"},
		{"github", "event", "workflow_run", "head_sha"},
		{"github", "event", "workflow_run", "head_commit", "id"},
	}
	forkRepoContexts = [][]string{
		{"github", "event", "pull_request", "head", "repo"},
		{"github", "event", "workflow_run", "head_repository"},
	}
	// pullRequestNumberContexts are the contexts that hold the number of a pull request of the event that started the run, so a
	// `refs/pull/<number>/merge` made from them is the pull request that the protection looks at.
	pullRequestNumberContexts = [][]string{
		{"github", "event", "number"},
		{"github", "event", "pull_request"},
		{"github", "event", "workflow_run", "pull_requests"},
	}
)

var reRefsPull = regexp.MustCompile(`(?i)^refs/pull/((?:\$\{\{[^}]*\}\}|[^/])+)/(head|merge)$`)

// checkoutInputForksPR reports whether the value of the input `ref` or `repository` of `actions/checkout` is a fork pull
// request that the protection refuses: the head repository, the head or merge commit, or `refs/pull/N/head|merge` where N
// comes from the event of the pull request (a number from `inputs` or a comment is not one the protection checks).
func checkoutInputForksPR(name, value string) bool {
	value = strings.TrimSpace(value)
	refs, _ := stringExprRefs(value)
	switch name {
	case "repository":
		_, ok := refsMatchAny(refs, forkRepoContexts)
		return ok
	case "ref":
		if _, ok := refsMatchAny(refs, forkCommitContexts); ok {
			return true
		}
		m := reRefsPull.FindStringSubmatch(value)
		if m == nil {
			return false
		}
		// the number must be literal or come from the pull request of the event
		nums, ok := stringExprRefs(m[1])
		if !ok {
			return false
		}
		if len(nums) == 0 {
			return true
		}
		for _, r := range nums {
			if _, ok := refsMatchAny([]exprRef{r}, pullRequestNumberContexts); !ok {
				return false
			}
		}
		return true
	}
	return false
}

// checkoutEvents tells which events of a workflow the protection of actions/checkout covers.
type checkoutEvents struct {
	// applies is whether some event of the workflow is one that the protection covers: `pull_request_target`, or a
	// `workflow_run` that only waits for workflows that pull requests start.
	applies bool
	// all is whether the protection covers every event that can reach the checkout: the privileged events of the workflow
	// are all covered ones, and the workflow has no event that the protection leaves out and that can carry a pull
	// request (`issue_comment`, `issues`, `workflow_call`).
	all bool
}

// checkoutGuardEvents classifies the events of the workflow, privileged being what privilegedEvents returned (and the events
// of the calls that reach a reusable workflow). calleeKnown is true for a reusable workflow whose callers are all known.
func checkoutGuardEvents(w *Workflow, privileged []string, project *Project, sib *siblingWorkflows, calleeKnown bool) checkoutEvents {
	var ev checkoutEvents
	ev.all = len(privileged) > 0
	for _, event := range privileged {
		switch event {
		case "pull_request_target":
			ev.applies = true
		case "workflow_run":
			covered := true
			for _, we := range webhookEvents(w, event) {
				if !upstreamWorkflowsPROnly(project, sib, we) {
					covered = false
				}
			}
			if covered {
				ev.applies = true
			} else {
				ev.all = false
			}
		default:
			ev.all = false
		}
	}
	// workflow_call is a gap, unless every caller is known and the events that reach the workflow are the covered ones
	for _, e := range []string{"issue_comment", "issues", "workflow_call"} {
		if hasEvent(w, e) && !(e == "workflow_call" && calleeKnown) {
			ev.all = false
		}
	}
	return ev
}
