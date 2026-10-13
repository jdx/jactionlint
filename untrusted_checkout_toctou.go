package jactionlint

import (
	"regexp"
	"slices"
	"strings"
)

// This file is the time-of-check to time-of-use part of untrusted-checkout (CWE-367). A gate that lets a job run (an
// environment with reviewers, a label, a comment of a maintainer) holds at one moment, and the code is the one that the
// reviewer looked at only when it is an immutable commit. With the name of a branch, `github.head_ref` or
// `refs/pull/N/head`, the author pushes after the review and the job checks out the new head. CodeQL
// (UntrustedCheckoutTOCTOUCritical/High, ControlChecks), gato-x (`approval_gate`, `Complexity.TOCTOU`) and sisakulint report
// it; the background is https://github.com/AdnaneKhan/ActionsTOCTOU.
//
// Three findings, all of untrusted-checkout:
//
//   - a gate that holds at one moment and a ref that can change (the job runs the code of a branch). Gates that also hold
//     after a push are not touched: the author of the pull request is checked (`author_association`, `github.actor`, the
//     head repository, a permission check in a `pull_request_target` workflow), and so are conditions on `needs` and `steps`,
//     which the rule cannot judge;
//   - a label condition on `github.event.pull_request.labels` in a workflow that runs again for each push
//     (`pull_request_target` with the types `opened`, `synchronize` or `reopened`, which is the default): the label stays
//     after the review, so the code of the new push runs without one, whatever the ref is, unless the job takes the label
//     away;
//   - with the option pedantic: `workflow_dispatch` with an input for the number of a pull request and no required `sha`
//     input, that checks out `refs/pull/<input>/head` or runs `gh pr checkout <input>`: the maintainer dispatches after the
//     review, and a push in between runs unreviewed code (gato-x `dispatch_toctou`).
//
// A step after the checkout that reads the commit (`git rev-parse HEAD`, `git log -1 --format=%H`), compares it with
// something named sha, revision, commit, expected or verified and stops the job is taken for a check that it is the reviewed
// one (huggingface/transformers, argumentcomputer/ix and nodejs/node do that), which makes the ref immutable. A required
// input of the dispatch with sha, commit or rev in its name is taken for the commit that the maintainer reviewed.

// gateInfo is what the gates of a job or step say about the code that runs.
type gateInfo struct {
	// safe is whether a gate holds after the code changes (or cannot be judged): the author is checked.
	safe bool
	// stale are the gates that hold at one moment, for the message.
	stale []string
	// label is whether one of them is a condition on the labels of the pull request, which a push does not take away.
	label bool
}

func (g gateInfo) merge(o gateInfo) gateInfo {
	g.safe = g.safe || o.safe
	g.label = g.label || o.label
	for _, s := range o.stale {
		if !slices.Contains(g.stale, s) {
			g.stale = append(g.stale, s)
		}
	}
	return g
}

// both are the gates that every one of two calls has: a gate counts for the callee only when each call is behind it.
func (g gateInfo) both(o gateInfo) gateInfo {
	r := gateInfo{safe: g.safe && o.safe, label: g.label && o.label}
	for _, s := range g.stale {
		if slices.Contains(o.stale, s) {
			r.stale = append(r.stale, s)
		}
	}
	return r
}

func (g gateInfo) gated() bool { return g.safe || len(g.stale) > 0 }

var (
	labelsContext      = []string{"github", "event", "pull_request", "labels"}
	eventLabelContext  = []string{"github", "event", "label"}
	commenterContexts  = [][]string{{"github", "event", "comment"}, {"github", "event", "sender"}, {"github", "actor"}, {"github", "triggering_actor"}}
	actionEventContext = []string{"github", "event", "action"}
)

// classifyCondition says what the guard condition is. commentOnly is whether a comment is the only event that
// starts the run: then the commenter is not the author of the pull request, and a check of the commenter holds at the
// moment of the comment only.
func classifyCondition(cond *String, commentOnly bool) gateInfo {
	var g gateInfo
	if cond == nil || !conditionIsGuard(cond) {
		return g
	}
	exprs, ok := conditionExprs(cond)
	if !ok {
		g.safe = true // not parsable: given the benefit of the doubt, like every guard
		return g
	}
	var refs []exprRef
	for _, e := range exprs {
		collectExprRefs(e, nil, &refs)
	}
	tiedToEvent := false
	for _, r := range refs {
		if refCovers(r.chain, eventLabelContext) || (refCovers(r.chain, actionEventContext) && requiresLabeledAction(r)) {
			tiedToEvent = true
		}
	}
	for _, r := range refs {
		switch {
		case refCovers(r.chain, labelsContext):
			g.stale = append(g.stale, "a label condition")
			g.label = !tiedToEvent
		case refCovers(r.chain, eventLabelContext):
			g.stale = append(g.stale, "a label condition")
		case commentOnly && func() bool { _, ok := refsMatchAny([]exprRef{r}, commenterContexts); return ok }():
			g.stale = append(g.stale, "a check of the user who commented")
		case func() bool { _, ok := refsMatchAny([]exprRef{r}, guardContexts); return ok }():
			g.safe = true
		}
	}
	if !g.safe && len(g.stale) == 0 {
		g.safe = true // a guard that is not one of the above: not judged
	}
	return g
}

// requiresLabeledAction reports whether the reference to the action of the event is compared for equality with `labeled`: only
// then does the condition belong to the one event of the label. `== 'synchronize'`, `!= 'labeled'` and the other actions keep a
// label that is on the pull request true for every later push.
func requiresLabeledAction(r exprRef) bool {
	c, ok := r.parent.(*CompareOpNode)
	if !ok || c.Kind != CompareOpNodeKindEq {
		return false
	}
	other := c.Right
	if c.Right == r.node {
		other = c.Left
	}
	lit, ok := other.(*StringNode)
	return ok && strings.EqualFold(lit.Value, "labeled")
}

// neededGates are the gates of the jobs that the job needs, at any depth: the job runs after them.
func (rule *RuleUntrustedCheckout) neededGates(w *Workflow, j *Job, events []string, commentOnly bool, seen map[*Job]bool) gateInfo {
	var g gateInfo
	if w == nil || j == nil || seen[j] || runsAfterSkippedNeeds(j.If) {
		return g
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
		if len(eventsAfterCondition(events, need.If)) == 0 {
			g.safe = true // the job is skipped for these events, and so is the one that needs it
			continue
		}
		g = g.merge(rule.ownGates(need, commentOnly, true))
		g = g.merge(rule.neededGates(w, need, events, commentOnly, seen))
	}
	return g
}

// ownGates are the gates of the job itself: its condition and, for a job that the job being judged needs, its environment and
// the permission checks among its steps.
func (rule *RuleUntrustedCheckout) ownGates(j *Job, commentOnly, needed bool) gateInfo {
	var g gateInfo
	g = g.merge(classifyCondition(j.If, commentOnly))
	if !needed {
		return g // the environment of the job is judged where a finding is made (environmentGate), its steps as they come
	}
	if rule.environmentHolds(j) {
		g.stale = append(g.stale, "an environment")
	}
	if j.ContinueOnError != nil && (j.ContinueOnError.Expression != nil || j.ContinueOnError.Value) {
		return g // a failed permission check does not stop the jobs that need this one
	}
	for _, s := range flattenSteps(j.Steps) {
		if isPermissionGate(s) {
			g = g.merge(permissionGate(commentOnly))
		}
	}
	return g
}

// permissionGate is what a permission check of the user who started the run says: for a comment, the commenter is not the
// author of the pull request.
func permissionGate(commentOnly bool) gateInfo {
	if commentOnly {
		return gateInfo{stale: []string{"a permission check of the user who commented"}}
	}
	return gateInfo{safe: true}
}

// stepVerifiesRevision reports whether the step reads the commit that is checked out and can stop the job: taken for a check
// that it is the commit that was reviewed.
func stepVerifiesRevision(s *Step) bool {
	e, ok := s.Exec.(*ExecRun)
	if !ok || e.Run == nil {
		return false
	}
	l := strings.ToLower(e.Run.Value)
	reads := strings.Contains(l, "rev-parse") || strings.Contains(l, "git log -1") || strings.Contains(l, "git show")
	stops := strings.Contains(l, "exit") || (strings.Contains(l, "[ ") && (strings.Contains(l, " = ") || strings.Contains(l, " != ") || strings.Contains(l, " -eq ")))
	names := strings.Contains(l, "sha") || strings.Contains(l, "revision") || strings.Contains(l, "commit") || strings.Contains(l, "expected") || strings.Contains(l, "verified")
	return reads && stops && names
}

// removesLabel reports whether a step of the job takes a label away from the pull request, which ends a label gate that
// lasts: `gh pr edit --remove-label`, `removeLabel` of github-script, an action that removes labels.
func jobRemovesLabel(j *Job) bool {
	for _, s := range flattenSteps(j.Steps) {
		switch e := s.Exec.(type) {
		case *ExecRun:
			if e.Run != nil {
				l := strings.ToLower(e.Run.Value)
				if strings.Contains(l, "--remove-label") || strings.Contains(l, "remove-label") || strings.Contains(l, "/labels/") && strings.Contains(l, "delete") {
					return true
				}
			}
		case *ExecAction:
			if _, u := stepAction(s); u != nil && strings.Contains(strings.ToLower(u.Raw), "remove-label") {
				return true
			}
			if in, ok := e.input("script"); ok && (strings.Contains(in, "removeLabel") || strings.Contains(in, "removeAllLabels")) {
				return true
			}
		}
	}
	return false
}

// rerunsOnPush reports whether the workflow runs again for the new commits of a pull request: `pull_request_target` with
// no `types` (opened, synchronize and reopened are the default) or with one of them.
func rerunsOnPush(w *Workflow) bool {
	for _, we := range webhookEvents(w, "pull_request_target") {
		if len(we.Types) == 0 {
			return true
		}
		for _, t := range we.Types {
			if t != nil && (t.Value == "synchronize" || t.Value == "opened" || t.Value == "reopened") {
				return true
			}
		}
	}
	return false
}

// reDispatchPR matches the name of an input that holds the number of a pull request.
var reDispatchPR = regexp.MustCompile(`(?i)(^|[_-])(pr|pull|pull_request|pullrequest|pr_number|pr-number|prnumber)([_-]|$)`)

// dispatchInputs returns the inputs of a `workflow_dispatch` event that hold the number of a pull request when no required
// input is a commit (an input with `sha` in its name). It is empty for a workflow that is not at risk.
func dispatchInputs(w *Workflow) []string {
	var names []string
	for _, e := range w.On {
		d, ok := e.(*WorkflowDispatchEvent)
		if !ok {
			continue
		}
		for name, in := range d.Inputs {
			if l := strings.ToLower(name); (strings.Contains(l, "sha") || strings.Contains(l, "commit") || strings.Contains(l, "rev")) && in != nil && in.Required != nil && in.Required.Value {
				return nil
			}
			if reDispatchPR.MatchString(name) {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return names
}

// dispatchInputValue reports whether the string reads one of the inputs of the dispatch that hold the number of a pull request.
func dispatchInputRefs(refs []exprRef, names []string) (string, bool) {
	for _, r := range refs {
		switch {
		case len(r.chain) >= 2 && r.chain[0] == "inputs" && slices.Contains(names, r.chain[1]):
			return r.String(), true
		case len(r.chain) >= 4 && r.chain[0] == "github" && r.chain[1] == "event" && r.chain[2] == "inputs" && slices.Contains(names, r.chain[3]):
			return r.String(), true
		}
	}
	return "", false
}

// reportToctou reports a checkout that gates which hold at one moment let through: the gates, and the way the ref can change.
// A gate is effective for a commit, so nothing is reported for one; a label that lasts is reported whatever the ref is. It says
// whether it reported.
func (rule *RuleUntrustedCheckout) reportToctou(c *untrustedCheckout, how string, g gateInfo) bool {
	event := rule.privileged[0]
	source, privileges := rule.sourceOf(event), rule.privilegeText()
	if len(g.stale) == 0 {
		// workflow_dispatch: the dispatch is the gate, and it names the pull request, not a commit
		if c.verified || !sourcesMutable(c.sources) {
			return false
		}
		rule.ReportIDf(
			"untrusted-checkout",
			c.pos,
			"this step checks out code from %s (%s) in a %q workflow and %s runs it afterwards. a maintainer dispatches the workflow after reading the pull request, and whoever can push to it changes the code between that review and the run, because the input names the pull request and not a commit. %s. add a required \"sha\" input and check out that commit",
			source, c.what, event, how, privileges,
		)
		return true
	}
	mutable := !c.verified && sourcesMutable(c.sources)
	// an environment with reviewers pauses every run, the one of a new push too: the label is not the only gate then
	lasts := g.label && rule.labelRerun && !slices.Contains(g.stale, "an environment")
	gates := strings.Join(g.stale, " and ")
	if event == "workflow_dispatch" {
		// who started the run, not who wrote a comment
		gates = strings.ReplaceAll(strings.ReplaceAll(gates, "who commented", "who dispatched"), "of the commenter", "of the user who dispatched")
	}
	switch {
	case lasts:
		rule.ReportIDf(
			"untrusted-checkout",
			c.pos,
			"this step checks out code from %s (%s) in a %q workflow and %s runs it afterwards. %s lets the job start, but it stays true after the review: the workflow runs again for each push to the pull request, so the code of a new push runs with the label although nobody has looked at it. %s. run only for the \"labeled\" event and check out the commit of that event, or take the label away in the job (\"gh pr edit --remove-label\")",
			source, c.what, event, how, gates, privileges,
		)
		return true
	case mutable:
		rule.ReportIDf(
			"untrusted-checkout",
			c.pos,
			"this step checks out code from %s (%s) in a %q workflow and %s runs it afterwards. %s lets the job start, but %s is not a commit: whoever wrote the pull request can push to it after the check (an approval, a label, a comment) and the job runs what is there then, not what was reviewed. %s. check out the commit that was reviewed (\"github.event.pull_request.head.sha\") and not the branch",
			source, c.what, event, how, gates, c.what, privileges,
		)
		return true
	}
	return false
}
