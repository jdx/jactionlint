package jactionlint

import (
	"regexp"
	"strings"
)

// This file has what untrusted-checkout knows about workflows that a comment starts (`issue_comment`): where the pull
// request comes from, and the gates that make such a workflow safe.
//
// An `issue_comment` workflow runs in the context of the default branch with a write token and secrets, like
// `pull_request_target`, but its payload carries no head: a bot that wants to build the pull request has to find it from
// the number of the issue (`gh pr checkout ${{ github.event.issue.number }}`, `refs/pull/<number>/head`) or from an action
// that looks it up (`xt0rted/pull-request-comment-branch`). poutine (`untrusted_checkout_exec`, events
// `pull_request_target`, `issues`, `issue_comment`) and CodeQL (`UntrustedCheckoutQuery`, `ControlChecks`) cover it.
//
// `issues` is not handled: an issue event is never one of a pull request, so its payload names no revision either, and
// the text of an issue that names one is an injection, which template-injection reports.

// issueNumberContexts are the contexts that name the pull request that was commented on. They are only a revision for
// `gh pr checkout`: `git checkout -b fix-${{ github.event.issue.number }}` creates a branch.
var issueNumberContexts = [][]string{
	{"github", "event", "issue", "number"},
	{"github", "event", "issue", "html_url"},
	{"github", "event", "issue", "pull_request", "html_url"},
	{"github", "event", "issue", "pull_request", "url"},
}

// issueNumberValue reports whether the string reads one of the contexts that name the pull request that was commented on.
func issueNumberValue(value string) (string, bool) {
	refs, _ := stringExprRefs(value)
	if r, ok := refsMatchAny(refs, issueNumberContexts); ok {
		return r.String(), true
	}
	return "", false
}

// commentBranchOutputs are the outputs of the actions that find the branch of the pull request that a comment is on (or
// the origin of a workflow run), by the lower case name of the action. A step that uses one of them gives the head of the
// pull request to whoever reads these outputs. The action `gotson/pull-request-comment-branch` of the list of poutine and
// CodeQL does not exist any more.
var commentBranchOutputs = map[string][]string{
	"xt0rted/pull-request-comment-branch":   {"head_ref", "head_sha"},
	"alessbell/pull-request-comment-branch": {"head_ref", "head_sha", "head_owner", "head_repo"},
	"eficode/resolve-pr-refs":               {"head_ref"},
	"potiuk/get-workflow-origin":            {"sourceheadrepo", "sourceheadbranch", "sourceheadsha", "mergecommitsha", "targetcommitsha"},
	"github/branch-deploy":                  {"ref", "sha", "fork_ref", "fork_full_name"},
}

// actionOutput reports whether the string reads an output of an earlier step of the job that gives the head of a pull
// request (`steps.x.outputs.head_sha` of `xt0rted/pull-request-comment-branch`).
func (rule *RuleUntrustedCheckout) actionOutput(value string) (string, bool) {
	if len(rule.stepActions) == 0 {
		return "", false
	}
	refs, _ := stringExprRefs(value)
	for _, r := range refs {
		if len(r.chain) < 4 || r.chain[0] != "steps" || r.chain[2] != "outputs" {
			continue
		}
		action := rule.stepActions[r.chain[1]]
		for _, name := range commentBranchOutputs[action] {
			if r.chain[3] == name {
				return r.String(), true
			}
		}
	}
	return "", false
}

// untrusted reports whether the string names the head of a pull request: what untrustedValue knows and the outputs of the
// actions that look it up.
func (rule *RuleUntrustedCheckout) untrusted(value string) (string, bool) {
	if rule.ghAny || !rule.comment {
		if what, ok := untrustedValue(value); ok {
			return what, true
		}
	} else if strings.Contains(value, "refs/pull/") {
		// a comment starts the workflow and nothing else does: github.head_ref and the contexts of a pull request are
		// empty, only a ref that is built from the number of the issue names the pull request
		return value, true
	}
	return rule.actionOutput(value)
}

// associationIsGuard reports whether the conditions that read `author_association` restrict it to people who can write:
// they name no association of someone who has no access (`NONE`, `CONTRIBUTOR`, `FIRST_TIMER`, `FIRST_TIME_CONTRIBUTOR`,
// `MANNEQUIN`). `!= 'NONE'` lets every contributor through, who may be anyone with a merged change. Conditions with no
// literal are given the benefit of the doubt, like the other guards.
func associationIsGuard(exprs []ExprNode) bool {
	guard := true
	for _, e := range exprs {
		VisitExprNode(e, func(n, _ ExprNode, entering bool) {
			s, ok := n.(*StringNode)
			if !ok || entering {
				return
			}
			for _, a := range reAssociation.FindAllString(strings.ToUpper(s.Value), -1) {
				switch a {
				case "NONE", "CONTRIBUTOR", "FIRST_TIMER", "FIRST_TIME_CONTRIBUTOR", "MANNEQUIN":
					guard = false
				}
			}
		})
	}
	return guard
}

var reAssociation = regexp.MustCompile(`[A-Z_]+`)

// isPermissionGate reports whether the step stops the job unless the user who started it, the author of the comment, has
// the right to write: an action that checks the permission and fails (not with `continue-on-error`, not with an option that
// turns the failure off), or a `github-script` that checks the permission or the membership of the user and throws. The
// step must always run. CodeQL (`PermissionActionCheck`, `AssociationActionCheck`) has a longer list; actions that only
// set an output do not stop anything unless a later `if:` reads the output, which is a guard of its own.
func isPermissionGate(s *Step) bool {
	e, ok := s.Exec.(*ExecAction)
	if !ok || s.If != nil || (s.ContinueOnError != nil && (s.ContinueOnError.Expression != nil || s.ContinueOnError.Value)) {
		return false
	}
	_, u := stepAction(s)
	if u == nil || u.Kind != UsesAction {
		return false
	}
	level := func(name string, def string) bool {
		v, ok := e.input(name)
		if !ok {
			v = def
		}
		v = strings.ToLower(v)
		return v == "write" || v == "admin"
	}
	switch strings.ToLower(u.CanonicalName()) {
	case "sushichop/action-repository-permission":
		if v, ok := e.input("relax"); ok && !strings.EqualFold(v, "false") {
			return false
		}
		return level("required-permission", "admin")
	case "prince-chrismc/check-actor-permissions-action", "lannonbr/repo-permission-check-action":
		return level("permission", "")
	case "xt0rted/slash-command-action":
		return level("permission-level", "write")
	case "themoddinginquisition/actions-team-membership":
		v, ok := e.input("exit")
		return !ok || strings.EqualFold(v, "true")
	case "actions-cool/check-user-permission":
		if _, ok := e.input("username"); ok {
			return false
		}
		v, _ := e.input("error-if-missing")
		return level("require", "") && strings.EqualFold(v, "true")
	case "actions/github-script":
		script, _ := e.input("script")
		if !reScriptChecksUser.MatchString(script) {
			return false
		}
		return strings.Contains(script, "throw ") || strings.Contains(script, "setFailed") || strings.Contains(script, "process.exit")
	}
	return false
}

// reScriptChecksUser matches the calls of a `github-script` that ask who the user is: the permission level of a
// collaborator, or the membership in an organization or a team.
var reScriptChecksUser = regexp.MustCompile(`getCollaboratorPermissionLevel|getMembershipForUserInOrg|checkMembershipForUser|checkCollaborator|getMembershipForUser|getTeamMembershipForUserInOrg`)

// jobHasGate reports whether a step of the job is a permission gate.
func jobHasGate(j *Job) bool {
	for _, s := range flattenSteps(j.Steps) {
		if isPermissionGate(s) {
			return true
		}
	}
	return false
}

// neededJobGated reports whether a job that the job needs, at any depth, is skipped or fails unless the user is trusted:
// its condition is a guard, it has an environment or a permission gate. The job is skipped with it, unless its own
// condition lets it run after a skipped job (`always()`).
func neededJobGated(w *Workflow, j *Job, seen map[*Job]bool) bool {
	if w == nil || j == nil || seen[j] || runsAfterSkippedNeeds(j.If) {
		return false
	}
	seen[j] = true
	for _, n := range j.Needs {
		if n == nil {
			continue
		}
		need := w.Jobs[strings.ToLower(n.Value)]
		if need == nil {
			continue
		}
		if conditionIsGuard(need.If) || (need.Environment != nil && need.Environment.Name != nil) || jobHasGate(need) || neededJobGated(w, need, seen) {
			return true
		}
	}
	return false
}
