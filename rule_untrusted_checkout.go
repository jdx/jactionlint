package jactionlint

import (
	"maps"
	"slices"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleUntrustedCheckout reports pull_request_target and workflow_run workflows that check out the code
// of a pull request (or of the run that triggered the workflow) and then run it. Those events run
// with a write token and with secrets, so whoever opened the pull request can use them.
type RuleUntrustedCheckout struct {
	RuleBase
	project    *Project
	siblings   *siblingWorkflows // nil reads the workflows of the project for each file
	privileged []string
	wf         *Workflow
	job        *Job
	// fetches are the fetches of an untrusted revision in the steps before the current one that no command has put
	// in the working tree yet, see pendingFetch.
	fetches []*pendingFetch
	// carried are the variables that earlier steps of the job wrote to $GITHUB_ENV with an untrusted value, with where
	// the value comes from.
	carried map[string]string
	// carriedNum are the variables that earlier steps wrote to $GITHUB_ENV with the number of the pull request that a comment is on.
	carriedNum map[string]string
	// scripts are the script files that were read, by path, nil for one that cannot be followed.
	scripts map[string]*runscript.Script
	// gates tells with the online checks whether the environment of a job holds it back, see environmentGates.
	gates *environmentGates
	// envJob is whether the job being checked has an environment, which is trusted unless the gates say it is no gate.
	envJob bool
	// guard is which events of the workflow the protection of actions/checkout covers, see untrusted_checkout_guard.go.
	guard checkoutEvents
	// path is the file being linted. calls are the calls of it as a reusable workflow or composite action that reach it with a
	// privileged event, inputTaint the inputs that they fill with the head of a pull request.
	path       string
	calls      []callContext
	inputTaint map[string]callInput
	// inputNumber are the inputs that the calls fill with the number of the pull request that a comment is on, with where it
	// comes from.
	inputNumber map[string]string
	// actions are the local composite actions that were read, by directory
	actions map[string]*Workflow
	// dispatch are the inputs of the workflow_dispatch event that hold the number of a pull request (pedantic only).
	dispatch []string
	// labelRerun is whether the workflow runs again for each push to a pull request, so that a label stays.
	labelRerun bool
	// calleeGates are the gates in front of every call that reaches the reusable workflow or action being checked.
	calleeGates gateInfo
	// allPrivileged are the privileged events of the workflow, privileged those that the job being judged runs for.
	allPrivileged []string
	// stepActions are the steps of the job that have an id and run an action, by id: the lower case name of the action.
	stepActions map[string]string
	// comment is whether a comment starts the workflow (issue_comment), ghAny whether gh pr checkout is a checkout
	// of a pull request whatever it is given: the workflow has an event that carries the head of a pull request.
	comment, ghAny bool
	// anyCheckout is whether a checkout that nothing visibly runs is reported too (the option any-checkout).
	anyCheckout bool
	// loose is whether the pedantic checks run: a checkout that actions/checkout refuses for a fork is reported too.
	loose bool
}

// pendingFetch is a `git fetch` of a revision of a pull request whose script does not put it in the working tree.
// A later step can: `git checkout FETCH_HEAD`, or `git checkout pr` for `git fetch origin <head>:pr`.
type pendingFetch struct {
	pos   *Pos
	what  string
	names []string // what the revision is called afterwards: FETCH_HEAD and the destinations of the refspecs
}

// NewRuleUntrustedCheckout creates a new RuleUntrustedCheckout instance. The project can be nil.
func NewRuleUntrustedCheckout(project *Project) *RuleUntrustedCheckout {
	return &RuleUntrustedCheckout{
		RuleBase: RuleBase{
			name: "untrusted-checkout",
			desc: "Checks that privileged workflows do not run the code of a pull request",
		},
		project: project,
	}
}

// untrustedCheckout is a step that puts code which someone else controls in the workspace.
type untrustedCheckout struct {
	pos *Pos
	// what is the reference that makes the code untrusted.
	what string
	// dir is where the code is, "" for the root of the workspace.
	dir string
	// offset is where the checkout command starts in the script of a run step. Commands before it do
	// not see the code.
	offset int
	step   *Step
	// how is what runs the code already, when the script file that does the checkout runs it itself.
	how string
	// guarded is whether actions/checkout refuses this checkout for a pull request from a fork, so only a pull request
	// from a branch of the repository gets through. Only an `actions/checkout` step can be guarded.
	guarded bool
	// gates are the gates that hold at one moment in front of the checkout (the job, the step, the earlier steps).
	gates gateInfo
	// verified is whether a later step checks the commit that was checked out.
	verified bool
	// ran is whether a step runs the checkout, but not one that the rule counts (a step that only runs for a merged pull request, or
	// after a failure of the permission check of an action).
	ran bool
	// afterFailure is whether the checkout is behind a failed permission check of an action: only a step that runs after a
	// failure runs it.
	afterFailure bool
	// note says why a checkout that the protection could cover is not guarded: the version of actions/checkout predates it.
	note string
}

// mergedCommits are the contexts of a pull request that name a commit that cannot change after the pull request was
// merged: the head the maintainer merged and the merge commit.
var mergedCommits = []string{"github.event.pull_request.head.sha", "github.event.pull_request.merge_commit_sha"}

// immutable reports whether the code is a commit of a pull request, which a branch name or `refs/pull/N/head` are not.
func (c *untrustedCheckout) immutable() bool {
	for _, m := range mergedCommits {
		if strings.EqualFold(c.what, m) {
			return true
		}
	}
	return false
}

// conditionRequiresMerged reports whether the condition is only true for a pull request that was merged:
// `github.event.pull_request.merged == true` (or the context alone), alone or in a conjunction. The merge is a fact of
// the event that cannot be set by the author of the pull request. gato-x treats the check as safe as well
// (SAFE_IF_CHECKS).
func conditionRequiresMerged(s *String) bool {
	exprs, ok := conditionExprs(s)
	return s != nil && ok && len(exprs) == 1 && exprRequiresMerged(exprs[0])
}

func exprRequiresMerged(e ExprNode) bool {
	switch n := e.(type) {
	case *LogicalOpNode:
		if n.Kind == LogicalOpNodeKindAnd {
			return exprRequiresMerged(n.Left) || exprRequiresMerged(n.Right)
		}
		return exprRequiresMerged(n.Left) && exprRequiresMerged(n.Right)
	case *CompareOpNode:
		if n.Kind != CompareOpNodeKindEq {
			return false
		}
		other := n.Right
		if isMergedContext(n.Right) {
			other = n.Left
		} else if !isMergedContext(n.Left) {
			return false
		}
		b, ok := other.(*BoolNode)
		return ok && b.Value
	default:
		return isMergedContext(e)
	}
}

func isMergedContext(e ExprNode) bool {
	chain, _, ok := chainOf(e)
	return ok && len(chain) == 4 && chain[0] == "github" && chain[1] == "event" && chain[2] == "pull_request" && chain[3] == "merged"
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUntrustedCheckout) VisitWorkflowPre(n *Workflow) error {
	rule.privileged, rule.wf, rule.calls, rule.inputTaint, rule.inputNumber, rule.dispatch = nil, n, nil, nil, nil, nil
	if !rule.Config().RuleEnabled("untrusted-checkout") {
		return nil
	}
	rule.privileged = privilegedEvents(n, rule.project, rule.siblings)
	rule.calleeGates, rule.labelRerun = gateInfo{}, false
	calleeKnown := false
	if _, reusable := n.FindWorkflowCallEvent(); reusable || n.Action != nil {
		// a reusable workflow and a composite action run for the events of the workflows that call them
		if key := calleeKey(rule.project, rule.path); key != "" {
			rule.calls = rule.callContexts(key, 0, map[string]bool{})
			// the message names the call that nothing holds back first
			slices.SortStableFunc(rule.calls, func(a, b callContext) int { return len(a.gates.stale) - len(b.gates.stale) })
			rule.inputTaint, rule.inputNumber = map[string]callInput{}, map[string]string{}
			events := slices.Clone(rule.privileged)
			for _, c := range rule.calls {
				events = append(events, c.events...)
				mergeInputs(rule.inputTaint, c.inputs)
				for name, d := range c.numbers {
					if _, ok := rule.inputNumber[name]; !ok {
						rule.inputNumber[name] = d
					}
				}
			}
			// the gates in front of the calls hold the callee back when every call has them and nothing else starts it
			gated := len(rule.calls) > 0 && (n.Action != nil || len(rule.privileged) == 0)
			for _, c := range rule.calls {
				gated = gated && len(c.gates.stale) > 0
				rule.calleeGates = rule.calleeGates.merge(c.gates)
				rule.labelRerun = rule.labelRerun || c.rerun
			}
			if !gated {
				rule.calleeGates = gateInfo{}
			}
			rule.privileged = sortedEvents(events)
			calleeKnown = len(rule.calls) > 0 && ((n.Action != nil && n.Action.Callers.Known()) || (n.callerEvents != nil && n.callerEvents.Known))
		}
	}
	rule.allPrivileged = rule.privileged
	rule.guard = checkoutGuardEvents(n, rule.privileged, rule.project, rule.siblings, calleeKnown)
	rule.loose = rule.pedantic("untrusted-checkout")
	rule.anyCheckout = anyCheckoutOption(rule.Config())
	rule.labelRerun = rule.labelRerun || rerunsOnPush(n)
	if rule.loose {
		// a maintainer who dispatches the workflow for the number of a pull request
		if rule.dispatch = dispatchInputs(n); len(rule.dispatch) > 0 {
			rule.allPrivileged = append(slices.Clone(rule.allPrivileged), "workflow_dispatch")
		}
	}
	rule.setEvents(rule.allPrivileged)
	return nil
}

// setEvents sets the privileged events that the job or step being judged can run for.
func (rule *RuleUntrustedCheckout) setEvents(events []string) {
	rule.privileged = events
	rule.comment = slices.Contains(events, "issue_comment") || slices.Contains(events, "workflow_dispatch")
	rule.ghAny = slices.Contains(events, "pull_request_target") || slices.Contains(events, "workflow_run")
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUntrustedCheckout) VisitJobPre(n *Job) error {
	rule.setEvents(eventsAfterCondition(rule.allPrivileged, n.If))
	if len(rule.privileged) == 0 {
		return nil
	}
	// A gate that holds after the code changes stops the job. One that holds at one moment (an environment, a label, a
	// comment of a maintainer) leaves the job to the check for a ref that can change.
	commentOnly := rule.commentOnly()
	// The reviewers of an environment decide, unless the online checks find that there are none (environmentGate)
	rule.envJob = n.Environment != nil && n.Environment.Name != nil
	// the steps of the job itself are counted as the loop reaches them
	running := rule.ownGates(n, commentOnly, false).merge(rule.neededGates(rule.wf, n, rule.privileged, commentOnly, map[*Job]bool{})).merge(rule.calleeGates)
	if running.safe {
		return nil
	}
	// The checkouts which no step has run the code of yet, by the directory they put it in. Whether a
	// step runs code depends on the directory only, so it is asked once for each directory and not for
	// each checkout: a job with thousands of checkouts is not quadratic.
	rule.job = n
	rule.fetches, rule.carried, rule.carriedNum = nil, nil, nil
	rule.stepActions = map[string]string{}
	for _, s := range flattenSteps(n.Steps) {
		if _, u := stepAction(s); u != nil && s.ID != nil && s.ID.Value != "" && u.Kind == UsesAction {
			rule.stepActions[strings.ToLower(s.ID.Value)] = strings.ToLower(u.CanonicalName())
		}
	}
	removesLabel := jobRemovesLabel(n)
	jobEvents := rule.privileged
	defer rule.setEvents(jobEvents)
	gated := false
	pending := map[string][]*untrustedCheckout{}
	var dirs []string // the keys of pending in the order they were added
	jobMerged := conditionRequiresMerged(n.If)
	for _, s := range flattenSteps(n.Steps) {
		if isPermissionGate(s) {
			g := permissionGate(commentOnly)
			if g.safe {
				gated = true // the steps after a permission check only run for a user who may write
				continue
			}
			running = running.merge(g)
			continue
		}
		if gated && !stepRunsAfterFailure(s) {
			continue
		}
		stepEvents := eventsAfterCondition(jobEvents, s.If)
		if len(stepEvents) == 0 {
			continue // the step does not run for the events that this rule judges
		}
		rule.setEvents(stepEvents) // what ghAny, comment and the event of the message say is about this step
		var stepGate gateInfo
		if conditionIsGuard(s.If) {
			stepGate = classifyCondition(s.If, commentOnly)
			if stepGate.safe || len(stepGate.stale) == 0 {
				continue // a step a maintainer has to allow neither runs the code nor fetches it
			}
		}
		here := running.merge(stepGate)
		if removesLabel {
			here.label = false
		}
		if stepVerifiesRevision(s) {
			for _, cs := range pending {
				for _, c := range cs {
					c.verified = true
				}
			}
		}
		// A step that only runs when the pull request was merged runs code that a maintainer merged, when the code is
		// the immutable commit that was merged. A branch name is not: the author can push to it after the merge.
		merged := jobMerged || conditionRequiresMerged(s.If)
		for _, dir := range dirs {
			cs := pending[dir]
			if len(cs) == 0 {
				continue
			}
			if how, ok := stepRunsCode(s, dir, 0); ok {
				var kept []*untrustedCheckout
				for _, c := range cs {
					if merged && c.immutable() {
						kept = append(kept, c) // a later step that does not need the merge runs it
						c.ran = true
						continue
					}
					if c.afterFailure && !stepRunsAfterFailure(s) {
						kept = append(kept, c) // the step does not run after the check of the action failed
						c.ran = true
						continue
					}
					rule.report(c, how, here)
				}
				pending[dir] = kept
			}
		}
		if !merged && !here.gated() {
			rule.reportOptOut(s)
		}
		for _, c := range rule.checkoutsOf(n, s) {
			if merged && c.immutable() {
				continue // the code is only checked out, or only run, once it was merged
			}
			if c.guarded && !rule.loose {
				continue // actions/checkout refuses it for a fork
			}
			c.gates = here
			if c.how != "" {
				rule.report(c, c.how, gateInfo{})
				continue
			}
			if run, ok := s.Exec.(*ExecRun); ok && run.Run != nil {
				// The same script can run the code it fetched
				if how, ok := stepRunsCode(s, c.dir, c.offset+1); ok {
					rule.report(c, how, gateInfo{})
					continue
				}
			}
			if _, seen := pending[c.dir]; !seen {
				dirs = append(dirs, c.dir)
			}
			pending[c.dir] = append(pending[c.dir], c)
		}
	}
	if rule.anyCheckout && rule.ghAny {
		// the checkout that nothing visibly runs (Scorecard and Semgrep report it, and it catches what the heuristics miss)
		for _, dir := range dirs {
			for _, c := range pending[dir] {
				if holds, _ := rule.environmentGate(); !c.gates.gated() && !holds && !c.ran && !(jobMerged && c.immutable()) {
					rule.reportUnused(c)
				}
			}
		}
	}
	return nil
}

// anyCheckoutRuleOption is the option that reports a checkout of the code of a pull request that nothing visibly runs.
var anyCheckoutRuleOption = RuleOption{Name: "any-checkout", Kind: RuleOptionBool, Summary: "Also report a checkout of the code of a pull request in a privileged workflow when no later step visibly runs it, like Scorecard and Semgrep do. Unset, it is true under the pedantic profile."}

// anyCheckoutOption reports whether the option any-checkout is on: when set, as it says, otherwise under the pedantic profile.
func anyCheckoutOption(c *Config) bool {
	if c == nil {
		return false
	}
	if v, ok := c.ruleOptionBool("untrusted-checkout", "any-checkout"); ok {
		return v
	}
	return c.Profile == ProfilePedantic
}

// reportUnused reports a checkout of the code of a pull request that no later step of the job visibly runs, with the option
// pedantic: the code is in the workspace of a job that has secrets, and what a tool, a cache or an action does with it is not
// known.
func (rule *RuleUntrustedCheckout) reportUnused(c *untrustedCheckout) {
	event := rule.privileged[0]
	rule.ReportIDf(
		"untrusted-checkout",
		c.pos,
		"this step checks out code from %s (%s) in a %q workflow, and no later step of the job visibly runs it. the code is in the workspace of a job that has secrets, and whatever reads the workspace later (a tool that an action runs, a cache restore, an artifact upload) can use it. %s. check out the base branch and only read the pull request as data",
		rule.sourceOf(event), c.what, event, rule.privilegeText(),
	)
}

// commentOnly is whether a comment is the only event of the job that is privileged: then who commented is not who wrote
// the code.
func (rule *RuleUntrustedCheckout) commentOnly() bool {
	return slices.Contains(rule.privileged, "issue_comment") && !rule.ghAny
}

// report reports the checkout that the step runs. runner are the gates of the step that runs the code. A gate that holds at
// one moment only lets the report through for a ref that can change (see untrusted_checkout_toctou.go).
func (rule *RuleUntrustedCheckout) report(c *untrustedCheckout, how string, runner gateInfo) {
	g := c.gates.merge(runner)
	if g.safe {
		return
	}
	if holds, _ := rule.environmentGate(); holds {
		g = g.merge(gateInfo{stale: []string{"an environment"}})
	}
	if len(g.stale) > 0 || rule.privileged[0] == "workflow_dispatch" {
		rule.reportToctou(c, how, g)
		return
	}
	event := rule.privileged[0]
	source, privileges := rule.sourceOf(event), rule.privilegeText()
	if c.guarded {
		rule.ReportIDf(
			"untrusted-checkout",
			c.pos,
			"this step checks out code from %s (%s) in a %q workflow and %s runs it afterwards, but this version of actions/checkout refuses to check out the code of a pull request from a fork here, so only a pull request from a branch of this repository gets through, and whoever pushes such a branch can write to the repository. %s. if the code of other pull requests is not meant to run, check out the base branch and only read the pull request as data",
			source, c.what, event, how, privileges,
		)
		return
	}
	suffix := ""
	if c.note != "" {
		suffix = ". " + c.note
	}
	rule.ReportIDf(
		"untrusted-checkout",
		c.pos,
		"this step checks out code from %s (%s) in a %q workflow and %s runs it afterwards. %s. run untrusted code in a \"pull_request\" workflow without secrets, or check out the base branch and only read the pull request as data%s",
		source, c.what, event, how, privileges, suffix,
	)
}

// sourceOf says where the code comes from for the event of the message.
func (rule *RuleUntrustedCheckout) sourceOf(event string) string {
	switch event {
	case "workflow_run":
		return "the run that triggered this workflow"
	case "issue_comment":
		return "the pull request that was commented on"
	case "workflow_dispatch":
		return "the pull request that the dispatch names"
	}
	return "a pull request"
}

// privilegeText says what the code can use in the job being judged.
func (rule *RuleUntrustedCheckout) privilegeText() string {
	privileges := "the workflow has a write token and secrets, so whoever controls that code can use them"
	if rule.job != nil && rule.wf != nil && tokenIsAbsent(rule.wf, rule.job) {
		privileges = "the job's GITHUB_TOKEN has no permissions (\"permissions: {}\"), but the code still runs on the runner with everything else the job gives it, such as secrets passed to a step and the cache"
	} else if rule.job != nil && rule.wf != nil && tokenIsReadOnly(rule.wf, rule.job) {
		privileges = "the job's GITHUB_TOKEN can only read, but the code still runs on the runner with everything else the job gives it, such as secrets passed to a step and the cache, and whoever controls it can use them"
	}
	if note, ok := rule.environmentNote(); ok && note != "" {
		privileges += ". " + note // the online checks found that the environment is no gate
	}
	return privileges
}

// environmentHolds tells whether the environment of the job holds it back: it does unless the online checks prove that it has no
// required reviewers. It is the one predicate for the environment of the job being checked, of the jobs that it needs and of
// the jobs that call it.
func (rule *RuleUntrustedCheckout) environmentHolds(job *Job) bool {
	if job == nil || job.Environment == nil || job.Environment.Name == nil {
		return false
	}
	if rule.gates == nil {
		return true
	}
	_, none := rule.gates.noGate(job)
	return !none
}

// environmentGate tells whether the environment of the job being judged holds it back (see environmentHolds), and the note
// that says why it does not, for the message.
func (rule *RuleUntrustedCheckout) environmentGate() (holds bool, note string) {
	if !rule.envJob {
		return false, ""
	}
	if rule.gates == nil {
		return true, ""
	}
	note, none := rule.gates.noGate(rule.job)
	return !none, note
}

// environmentNote tells whether a finding of the job is reported and what the environment adds to the message: a job with an
// environment that holds it back is judged for the code that follows it (see reportToctou), not reported here.
func (rule *RuleUntrustedCheckout) environmentNote() (note string, report bool) {
	holds, note := rule.environmentGate()
	return note, !holds
}

// reportOptOut reports an `actions/checkout` step that turns off the protection against the code of fork pull requests
// (`allow-unsafe-pr-checkout: true`, or an expression) in a workflow whose event the protection covers, when the step
// checks out a ref or a repository that is not a constant: with the input the action checks out whatever they name.
func (rule *RuleUntrustedCheckout) reportOptOut(s *Step) {
	if !rule.guard.applies {
		return
	}
	e, ok := s.Exec.(*ExecAction)
	if !ok {
		return
	}
	if _, u := stepAction(s); u == nil || !u.isRepoAction("actions/checkout") {
		return
	}
	opt := e.Inputs["allow-unsafe-pr-checkout"]
	if opt == nil || opt.Value == nil || !checkoutOptOut(opt.Value.Value) {
		return
	}
	named := false
	for _, name := range []string{"ref", "repository"} {
		if in := e.Inputs[name]; in != nil && in.Value != nil && (strings.Contains(in.Value.Value, "${{") || strings.Contains(in.Value.Value, "refs/pull/")) {
			named = true
		}
	}
	if !named {
		return
	}
	gate, ok := rule.environmentNote()
	if !ok {
		return // the reviewers of the environment decide
	}
	if gate != "" {
		gate = ". " + gate
	}
	rule.ReportIDf(
		"untrusted-checkout",
		opt.Value.Pos,
		"this step sets \"allow-unsafe-pr-checkout\", which turns off the protection of actions/checkout that refuses to check out the code of a pull request from a fork in a %q workflow. the step then checks out whatever \"ref\" and \"repository\" name, so nothing stops the code of the pull request from reaching a privileged workflow, which has secrets and may have a write token%s. remove the input and check out the base branch, or document why the code is safe to run",
		rule.privileged[0], gate,
	)
}

// checkoutOptOut reports whether the value of `allow-unsafe-pr-checkout` turns the protection off or may do so: true, or an
// expression that is not known to be false.
func checkoutOptOut(v string) bool {
	v = strings.TrimSpace(v)
	if strings.Contains(v, "${{") {
		return true
	}
	return strings.EqualFold(v, "true")
}

// tokenIsAbsent reports whether the job sets "permissions: {}", so GITHUB_TOKEN is created with no scope at all.
func tokenIsAbsent(w *Workflow, j *Job) bool {
	p := j.Permissions
	if p == nil {
		p = w.Permissions
	}
	return p != nil && p.All == nil && len(p.Scopes) == 0
}

// tokenIsReadOnly reports whether the job sets permissions and none of them is write.
func tokenIsReadOnly(w *Workflow, j *Job) bool {
	p := j.Permissions
	if p == nil {
		p = w.Permissions
	}
	if p == nil || (p.All == nil && len(p.Scopes) == 0) {
		return false
	}
	if p.All != nil && !strings.EqualFold(p.All.Value, "read-all") {
		return false
	}
	for _, sc := range p.Scopes {
		if sc == nil || sc.Value == nil || (!strings.EqualFold(sc.Value.Value, "read") && !strings.EqualFold(sc.Value.Value, "none")) {
			return false
		}
	}
	return true
}

// actionCheckout judges a step of actions/checkout: the checkout of code that someone else controls, nil if it is not one.
// subst turns a value of the step into what it is in the run (nil for itself): the values that a call passes replace the
// inputs of a composite action. pos is where to report, nil for the value of the input.
func (rule *RuleUntrustedCheckout) actionCheckout(s *Step, e *ExecAction, u *UsesRef, comments *CommentIndex, subst func(string) string, pos *Pos) *untrustedCheckout {
	plain := subst == nil // the values are the ones of the step: its strings share their parsed expressions
	if plain {
		subst = func(v string) string { return v }
	}
	dir := ""
	if p, ok := e.input("path"); ok {
		dir = normalizeDir(subst(p))
	}
	var found *untrustedCheckout
	forks := false
	for _, name := range []string{"ref", "repository"} {
		in := e.Inputs[name]
		if in == nil || in.Value == nil {
			continue
		}
		v := subst(in.Value.Value)
		if checkoutInputForksPR(name, v) || rule.inputForksPR(name, v) {
			forks = true
		}
		if found != nil {
			continue
		}
		var what string
		var bad bool
		if plain {
			what, bad = rule.untrustedStr(in.Value)
		} else {
			what, bad = rule.untrusted(v)
		}
		if bad {
			at := pos
			if at == nil {
				at = in.Value.Pos
			}
			found = &untrustedCheckout{pos: at, what: what, dir: dir, step: s}
		}
	}
	if found == nil {
		return nil
	}
	if forks && rule.guard.all {
		var comment *Comment
		if e.Uses != nil {
			comment = comments.Inline(e.Uses.Pos.Line)
		}
		ver := versionOfCheckout(u, comment)
		optOut := false
		if opt := e.Inputs["allow-unsafe-pr-checkout"]; opt != nil && opt.Value != nil {
			optOut = checkoutOptOut(subst(opt.Value.Value))
		}
		switch {
		case optOut:
		case ver.guarded:
			found.guarded = true
		case ver.known && ver.floor != "":
			found.note = "the pinned actions/checkout (" + ver.text + ") predates the protection that refuses the code of a fork pull request here, upgrade to " + ver.floor + " or later"
		case ver.known:
			found.note = "the pinned actions/checkout (" + ver.text + ") has no protection that refuses the code of a fork pull request, upgrade to v7 or to a release of a supported major version that has it"
		}
	}
	return found
}

// checkoutsOf returns the untrusted code that the step checks out.
func (rule *RuleUntrustedCheckout) checkoutsOf(j *Job, s *Step) []*untrustedCheckout {
	switch e := s.Exec.(type) {
	case *ExecAction:
		_, u := stepAction(s)
		if u == nil {
			return nil
		}
		if u.Kind == UsesLocal {
			return rule.localActionCheckouts(s, e)
		}
		if !u.isRepoAction("actions/checkout") {
			return nil
		}
		var comments *CommentIndex
		if rule.wf != nil {
			comments = rule.wf.Comments
		}
		if c := rule.actionCheckout(s, e, u, comments, nil, nil); c != nil {
			return []*untrustedCheckout{c}
		}
		return nil
	case *ExecRun:
		script, _ := analyzeRun(e)
		if script == nil {
			return nil
		}
		// the environment of the step: the env of the workflow and the job, what earlier steps wrote to $GITHUB_ENV,
		// then the env of the step
		process := map[string]string{}
		if rule.wf != nil {
			envDescs(rule.wf.Env, process, rule.untrustedStr)
		}
		envDescs(j.Env, process, rule.untrustedStr)
		maps.Copy(process, rule.carried)
		envDescs(s.Env, process, rule.untrustedStr)
		taint := rule.newTaint(script, process)
		var number *revTaint // the number of the pull request that a comment is on, for gh pr checkout
		if rule.comment && !rule.ghAny {
			numEnv := map[string]string{}
			if rule.wf != nil {
				envDescs(rule.wf.Env, numEnv, rule.numberStr)
			}
			envDescs(j.Env, numEnv, rule.numberStr)
			maps.Copy(numEnv, rule.carriedNum) // what earlier steps wrote to $GITHUB_ENV
			envDescs(s.Env, numEnv, rule.numberStr)
			number = newNumberTaint(script, numEnv, rule.numberValue)
			taint.num = number
		}
		var ret []*untrustedCheckout
		earlier := rule.fetches
		for i, c := range script.Commands {
			var env map[string]bool
			if c.Name == "git" || c.Name == "gh" {
				env = taint.names(c.Offset)
			}
			fetchAt := c.Offset // where the revision reaches the working tree, for what runs after it
			what, ok := rule.commandChecksOutUntrusted(c, env, number)
			if ok {
				what = taint.describe(what, c.Offset)
			}
			pos := s.Pos
			if e.RunPos != nil {
				pos = e.RunPos
			}
			if !ok && c.Name == "git" && workTreeVerbs[c.Verb()] {
				// the revision that an earlier step fetched
				for k, f := range earlier {
					if f != nil && wordsName(c, f.names) {
						what, ok, pos = f.what, true, f.pos
						earlier[k] = nil
						break
					}
				}
			}
			if !ok {
				// a script of the repository that puts the fetched revision in the working tree
				if f := rule.scriptUsingFetch(c, s, earlier, taint); f != nil {
					// the offset starts one before the command so that the script itself counts as running the code
					ret = append(ret, &untrustedCheckout{pos: f.pos, what: f.what, offset: c.Offset - 1, step: s})
				}
			}
			if !ok {
				// a script file of the project that checks out a revision it is given, or that it takes from the
				// environment, see untrusted_checkout_flow.go
				if w, how, found := rule.followScript(taint, c, process, stepWorkdir(s), maxScriptDepth); found {
					off := c.Offset + 1 // the call itself is not the code that runs
					ret = append(ret, &untrustedCheckout{pos: pos, what: w, offset: off, step: s, how: how})
					continue
				}
			}
			if !ok && c.Name == "git" && c.Verb() == "fetch" {
				// A fetch only downloads the objects: it checks out the code when a later command in
				// the script puts what it fetched in the working tree.
				if w, bad := rule.commandNamesUntrusted(c, env); bad {
					w = taint.describe(w, c.Offset)
					if k := firstWorkTree(script.Commands[i+1:]); k >= 0 {
						what, ok = w, true
						fetchAt = script.Commands[i+1+k].Offset
					} else {
						rule.fetches = append(rule.fetches, &pendingFetch{pos: pos, what: w, names: fetchedNames(c)})
					}
				}
			}
			if ok {
				dir := ""
				if c.Name == "git" && c.Verb() == "clone" && len(c.Positional) > 2 {
					dir = normalizeDir(c.Sub(2))
				}
				ret = append(ret, &untrustedCheckout{pos: pos, what: what, dir: dir, offset: fetchAt, step: s})
			}
		}
		rule.carryEnv(script, taint, s.If != nil, &rule.carried)
		if number != nil {
			// the number of the pull request that a comment is on can be carried the same way: it has the env of the
			// workflow, the job and the step, and what earlier steps wrote
			rule.carryEnv(script, number, s.If != nil, &rule.carriedNum)
		}
		return ret
	}
	return nil
}

// maxScriptSize is the size of the largest script file that is read to look for a checkout.
const maxScriptSize = 256 * 1024

// scriptPath returns the file a command runs as a script of the repository: `./scripts/x.sh`, `bash scripts/x.sh`,
// `sh ./x.sh`. It returns "" for everything else, including a path that is computed or absolute.
func scriptPath(c *runscript.Command) string {
	switch c.Name {
	case "bash", "sh":
		if len(c.Positional) == 0 {
			return ""
		}
		w := c.Positional[0]
		if w.Dynamic() || strings.HasPrefix(w.Value, "-") || strings.HasPrefix(w.Value, "/") {
			return ""
		}
		return w.Value
	}
	if c.NameWord != nil && !c.NameWord.Dynamic() && strings.HasPrefix(c.NameWord.Value, "./") {
		return c.NameWord.Value
	}
	return ""
}

// scriptUsingFetch returns the earlier fetch whose revision the script file that the command runs puts in the working
// tree, and forgets it. The script is read from the project, and only when it names FETCH_HEAD or a destination of
// the fetch literally in a command that changes the working tree, so a script that merely exists is not reported.
func (rule *RuleUntrustedCheckout) scriptUsingFetch(c *runscript.Command, s *Step, earlier []*pendingFetch, t *revTaint) *pendingFetch {
	if !slices.ContainsFunc(earlier, func(f *pendingFetch) bool { return f != nil }) {
		return nil
	}
	rel := scriptPath(c)
	if rel == "" {
		return nil
	}
	cwd, known := t.cwdAt(stepWorkdir(s), c.Offset)
	if !known {
		return nil
	}
	script := rule.readScript(cwd, rel)
	if script == nil {
		return nil
	}
	for k, f := range earlier {
		if f == nil {
			continue
		}
		for _, sc := range script.Commands {
			if sc.Name == "git" && workTreeVerbs[sc.Verb()] && wordsName(sc, f.names) {
				earlier[k] = nil
				return f
			}
		}
	}
	return nil
}

// gitCheckoutVerbs are the git commands that put a revision in the working tree. A "fetch" is not one:
// it only downloads objects, see firstWorkTree.
var gitCheckoutVerbs = map[string]bool{
	"checkout": true, "switch": true, "pull": true, "merge": true, "cherry-pick": true,
	"rebase": true, "clone": true, "reset": true, "restore": true,
}

// commandChecksOutUntrusted reports whether the command brings the code of a pull request into the
// workspace: `gh pr checkout` or a git command whose arguments name the head of the pull request. In a workflow that
// only a comment starts there is no head to name, so `gh pr checkout` counts when it is given the number of the pull
// request that was commented on (number follows it).
func (rule *RuleUntrustedCheckout) commandChecksOutUntrusted(c *runscript.Command, envNames map[string]bool, number *revTaint) (string, bool) {
	switch c.Name {
	case "gh":
		if c.Verb() == "pr" && c.Sub(1) == "checkout" {
			if rule.ghAny {
				return "gh pr checkout", true
			}
			if number != nil {
				for _, w := range c.Words {
					if number.wordAt(w, c.Offset) != "" {
						return "gh pr checkout", true
					}
				}
			}
		}
	case "git":
		if !gitCheckoutVerbs[c.Verb()] {
			return "", false
		}
		return rule.commandNamesUntrusted(c, envNames)
	}
	return "", false
}

// commandNamesUntrusted reports whether a word of the command names the head of a pull request.
func (rule *RuleUntrustedCheckout) commandNamesUntrusted(c *runscript.Command, envNames map[string]bool) (string, bool) {
	for _, w := range c.Words {
		for _, e := range w.Exprs {
			if what, bad := rule.untrusted("${{ " + e + " }}"); bad {
				return what, true
			}
		}
		for _, v := range w.Vars {
			if envNames[v] {
				return "$" + v, true
			}
		}
		if namesPullRef(w.Value) {
			return w.Value, true
		}
	}
	return "", false
}

// workTreeVerbs are the git commands that change the files in the working tree to a revision.
var workTreeVerbs = map[string]bool{
	"checkout": true, "switch": true, "merge": true, "cherry-pick": true, "rebase": true, "reset": true,
	"restore": true, "pull": true, "read-tree": true, "worktree": true,
}

// fetchedNames returns the names a `git fetch` gives the revisions it downloads: FETCH_HEAD, and the destination of
// each refspec (`pull/1/head:pr` names `pr`).
func fetchedNames(c *runscript.Command) []string {
	names := []string{"FETCH_HEAD"}
	for _, p := range c.Positional {
		// the word may hold an expression before the colon (`pull/${{ github.event.number }}/head:pr`)
		if i := strings.LastIndexByte(p.Raw, ':'); i >= 0 && i+1 < len(p.Raw) && !strings.ContainsAny(p.Raw[i+1:], "$`") {
			names = append(names, strings.Trim(strings.TrimPrefix(p.Raw[i+1:], "refs/heads/"), `"'`))
		}
	}
	return names
}

// wordsName reports whether a word of the command is one of the names.
func wordsName(c *runscript.Command, names []string) bool {
	for _, w := range c.Words {
		if !w.Dynamic() && slices.Contains(names, w.Value) {
			return true
		}
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "untrusted-checkout", Group: RuleGroupSecurity, Summary: "A pull_request_target, workflow_run or issue_comment workflow checks out the code of a pull request and runs it, or turns off the protection of actions/checkout against the code of fork pull requests. It also reports a gate that holds at one moment (an environment, a label, the comment of a maintainer) in front of a branch that the author can still push to, and a label that survives a push. With the option pedantic, a checkout that actions/checkout refuses for a fork and a dispatch with the number of a pull request and no commit are reported too, and with the option any-checkout a checkout that nothing visibly runs.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-untrusted-checkout", Options: []RuleOption{pedanticOption, anyCheckoutRuleOption}},
	)
	registerRuleFactory("untrusted-checkout", func(env *RuleEnv) []Rule {
		r := NewRuleUntrustedCheckout(env.project)
		r.siblings = env.localActions.siblings()
		r.path = env.path
		r.gates = newEnvironmentGates(env, r.Debug)
		return []Rule{r}
	})
}
