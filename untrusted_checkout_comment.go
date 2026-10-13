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

// issueNumberRefs reports whether the references read one of the contexts that name the pull request that was commented on.
func issueNumberRefs(refs []exprRef) (string, bool) {
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

// actionOutput reports whether the references read an output of an earlier step of the job that gives the head of a pull
// request (`steps.x.outputs.head_sha` of `xt0rted/pull-request-comment-branch`).
func (rule *RuleUntrustedCheckout) actionOutput(refs []exprRef) (string, bool) {
	if len(rule.stepActions) == 0 {
		return "", false
	}
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
	refs, _ := stringExprRefs(value)
	return rule.untrustedIn(value, refs)
}

// untrustedStr is untrusted for a string of the workflow, which shares its parsed expressions with the other rules.
func (rule *RuleUntrustedCheckout) untrustedStr(s *String) (string, bool) {
	refs, _ := s.exprRefs()
	return rule.untrustedIn(s.Value, refs)
}

func (rule *RuleUntrustedCheckout) untrustedIn(value string, refs []exprRef) (string, bool) {
	if rule.ghAny || !rule.comment {
		if strings.Contains(value, "refs/pull/") {
			return value, true
		}
		if what, ok := untrustedRefs(refs); ok {
			return what, true
		}
	} else if strings.Contains(value, "refs/pull/") {
		// a comment starts the workflow and nothing else does: github.head_ref and the contexts of a pull request are
		// empty, only a ref that is built from the number of the issue names the pull request
		return value, true
	}
	if what, ok := rule.actionOutput(refs); ok {
		return what, true
	}
	return rule.inputUntrusted(refs)
}

// trustedAssociations are the associations of someone who can write; the others (`NONE`, `CONTRIBUTOR`, `FIRST_TIMER`,
// `FIRST_TIME_CONTRIBUTOR`, `MANNEQUIN`) can be anyone with a merged change or no access at all.
var trustedAssociations = map[string]bool{"OWNER": true, "MEMBER": true, "COLLABORATOR": true}

// associationRestricts reports whether a condition that reads `author_association` is true only for people who can write. It
// follows the polarity of the expression: `contains(['OWNER', 'MEMBER'], association)` and `association == 'OWNER'` restrict,
// their negations (`!contains(...)`, `association != 'OWNER'`, `... == false`) let everyone else through, a double negation
// restricts again, `a && b` restricts when one side does, and `a || b` only when both do (`association == 'MEMBER' ||
// startsWith(body, '/test')` lets everyone who writes the command through). A list or a value that is not a literal is given
// the benefit of the doubt, like the other guards.
func associationRestricts(exprs []ExprNode) bool {
	for _, e := range exprs {
		if associationHolds(e, true) {
			return true
		}
	}
	return false
}

// isAssociation reports whether the expression reads the association of the author of a comment, an issue or a pull request.
func isAssociation(n ExprNode) bool {
	chain, _, ok := chainOf(n)
	return ok && len(chain) == 4 && chain[3] == "author_association"
}

// associationHolds reports whether the expression, when its value is `value`, is only possible for a trusted association.
func associationHolds(n ExprNode, value bool) bool {
	switch n := n.(type) {
	case *NotOpNode:
		return associationHolds(n.Operand, !value)
	case *LogicalOpNode:
		l, r := associationHolds(n.Left, value), associationHolds(n.Right, value)
		if (n.Kind == LogicalOpNodeKindAnd) == value {
			return l || r // both sides hold (a && b true, a || b false): one of them is enough
		}
		return l && r // one of the sides holds (a || b true, a && b false): each of them must
	case *CompareOpNode:
		if !n.Kind.IsEqualityOp() {
			return false
		}
		equal := n.Kind == CompareOpNodeKindEq
		for _, side := range [][2]ExprNode{{n.Left, n.Right}, {n.Right, n.Left}} {
			if b, ok := side[1].(*BoolNode); ok {
				return associationHolds(side[0], value == (equal == b.Value)) // `x == false` is `!x`
			}
			if isAssociation(side[0]) {
				if lit, ok := side[1].(*StringNode); ok {
					// association == 'LIT' restricts when it is true, and `!=` when it is false
					return value == equal && trustedAssociations[strings.ToUpper(strings.TrimSpace(lit.Value))]
				}
				return value == equal // not a literal: the benefit of the doubt for `==`, none for `!=`
			}
		}
	case *FuncCallNode:
		if !strings.EqualFold(n.Callee, "contains") || len(n.Args) != 2 || !isAssociation(n.Args[1]) {
			return false
		}
		return value && associationListTrusted(n.Args[0])
	}
	return false
}

// associationListTrusted reports whether the list that is searched for the association only has trusted ones: a literal
// (`fromJSON('["OWNER","MEMBER"]')`, `'OWNER MEMBER'`) with none of the others in it. What is not a literal is given the benefit
// of the doubt.
func associationListTrusted(n ExprNode) bool {
	var lits []*StringNode
	VisitExprNode(n, func(c, _ ExprNode, entering bool) {
		if l, ok := c.(*StringNode); ok && !entering {
			lits = append(lits, l)
		}
	})
	if len(lits) == 0 {
		return true
	}
	found := false
	for _, l := range lits {
		for _, a := range reAssociation.FindAllString(strings.ToUpper(l.Value), -1) {
			if !trustedAssociations[a] {
				switch a {
				case "NONE", "CONTRIBUTOR", "FIRST_TIMER", "FIRST_TIME_CONTRIBUTOR", "MANNEQUIN":
					return false
				}
				continue // another word of the text
			}
			found = true
		}
	}
	return found
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

// jobHasGate reports whether a step of the job is a permission gate that stops the jobs that need it: not when the job
// goes on after a failure (`continue-on-error`).
func jobHasGate(j *Job) bool {
	if j.ContinueOnError != nil && (j.ContinueOnError.Expression != nil || j.ContinueOnError.Value) {
		return false
	}
	for _, s := range flattenSteps(j.Steps) {
		if isPermissionGate(s) {
			return true
		}
	}
	return false
}

// neededEvents narrows the events by the conditions of the jobs that the job needs, at any depth: the job runs for the events
// for which all of them run, and is skipped for the others (unless its own condition lets it run after a skipped job).
func neededEvents(w *Workflow, j *Job, events []string, seen map[*Job]bool) []string {
	if w == nil || j == nil || seen[j] || runsAfterSkippedNeeds(j.If) {
		return events
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
		if events = eventsAfterCondition(events, need.If); len(events) == 0 {
			return nil
		}
		events = neededEvents(w, need, events, seen)
	}
	return events
}

// neededJobGated reports whether a job that the job needs, at any depth, is skipped or fails unless the user is trusted:
// its condition is a guard or is false for the events (it is skipped), it has an environment that holds it back (envHolds,
// see environmentHolds) or a permission gate. The job is skipped with it, unless its own condition lets it run after a
// skipped job (`always()`).
func neededJobGated(w *Workflow, j *Job, events []string, envHolds func(*Job) bool, seen map[*Job]bool) bool {
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
		if len(eventsAfterCondition(events, need.If)) == 0 || conditionIsGuard(need.If) || envHolds(need) || jobHasGate(need) || neededJobGated(w, need, events, envHolds, seen) {
			return true
		}
	}
	return false
}

// numberValue reports whether the string reads the number of the pull request that a comment is on, or an input that the
// calls fill with it.
func (rule *RuleUntrustedCheckout) numberValue(value string) (string, bool) {
	refs, _ := stringExprRefs(value)
	return rule.numberRefs(refs)
}

// numberStr is numberValue for a string of the workflow, which shares its parsed expressions.
func (rule *RuleUntrustedCheckout) numberStr(s *String) (string, bool) {
	refs, _ := s.exprRefs()
	return rule.numberRefs(refs)
}

func (rule *RuleUntrustedCheckout) numberRefs(refs []exprRef) (string, bool) {
	if what, ok := issueNumberRefs(refs); ok {
		return what, true
	}
	for _, name := range inputsOfRefs(refs) {
		if d, ok := rule.inputNumber[name]; ok {
			return "inputs." + name + ", given " + d, true
		}
	}
	return "", false
}
