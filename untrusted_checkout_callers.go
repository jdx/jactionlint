package jactionlint

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
)

// This file lets untrusted-checkout follow the head of a pull request into local reusable workflows and local composite
// actions. A reusable workflow and a composite action run in the context of the workflow that calls them: the privileged
// event is the caller's, and the revision is often passed down as an input:
//
//	# caller, on pull_request_target
//	- uses: ./.github/actions/build
//	  with:
//	    ref: ${{ github.event.pull_request.head.sha }}
//	# callee: actions/checkout with `ref: ${{ inputs.ref }}`, then `make`
//
// CodeQL (workflow_call is privileged when its caller is, UntrustedCheckoutQuery) and poutine (the event workflow_call) do
// the same. The rule judges the callee for each call that can reach it, as sibling workflows are known:
//
//   - A call counts when the workflow that makes it has a privileged event (privilegedEvents) and the job and step of the
//     call are not guarded (environment, a guard condition, a permission gate, a needed job that is guarded). The caller
//     can itself be a reusable workflow, and then its own callers count too (three levels).
//   - An input counts as untrusted when a counting call gives it a value that names the head of a pull request (a context,
//     `refs/pull/...`), or that reads an input of the caller that is untrusted in the same way.
//
// What is not known stays out: a callee that no workflow of the project calls, a call through a composite action that calls
// another one, a value from a step output or a matrix, and a `uses:` with an expression do not give a finding.

// maxCallDepth is how many levels of reusable workflows that call each other are followed.
const maxCallDepth = 3

// callRef points at a call of a local reusable workflow or composite action in a workflow of the project.
type callRef struct {
	file string // the workflow, relative to the root of the project with "/"
	job  string // the key of the job in Jobs
	step int    // the index among flattenSteps(job.Steps), -1 for a job that calls a reusable workflow
}

// callIndex is the list of the calls of local workflows and actions in the workflows of the project, by the key of the
// callee: "w:<file>" for a reusable workflow and "a:<directory>" for an action (relative to the root with "/").
type callIndex struct {
	once sync.Once
	refs map[string][]callRef
}

// callRefs returns the calls of the callee. A nil value reads the workflows on every call.
func (s *siblingWorkflows) callRefs(project *Project, key string) []callRef {
	if s == nil {
		return readCallRefs(project)[key]
	}
	s.calls.once.Do(func() { s.calls.refs = readCallRefs(project) })
	return s.calls.refs[key]
}

func readCallRefs(project *Project) map[string][]callRef {
	ret := map[string][]callRef{}
	if project == nil {
		return ret
	}
	dir := project.WorkflowsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ret
	}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (ext != ".yml" && ext != ".yaml") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		w, _ := Parse(b)
		if w == nil {
			continue
		}
		rel, err := filepath.Rel(project.RootDir(), p)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		for id, j := range w.Jobs {
			if j == nil {
				continue
			}
			if j.WorkflowCall != nil && j.WorkflowCall.Uses != nil {
				if t, ok := localTarget(j.WorkflowCall.Uses.Value); ok && t.kind == 'w' {
					ret["w:"+t.path] = append(ret["w:"+t.path], callRef{file: rel, job: id, step: -1})
				}
			}
			for i, s := range flattenSteps(j.Steps) {
				if a, ok := s.Exec.(*ExecAction); ok && a.Uses != nil {
					if t, ok := localTarget(a.Uses.Value); ok && t.kind == 'a' {
						ret["a:"+t.path] = append(ret["a:"+t.path], callRef{file: rel, job: id, step: i})
					}
				}
			}
		}
	}
	for _, refs := range ret {
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].file != refs[j].file {
				return refs[i].file < refs[j].file
			}
			if refs[i].job != refs[j].job {
				return refs[i].job < refs[j].job
			}
			return refs[i].step < refs[j].step
		})
	}
	return ret
}

// callInput is an input of a callee that a call gives an untrusted value.
type callInput struct {
	// desc says where the value comes from, for the message: "github.event.pull_request.head.sha by .github/workflows/pr.yaml".
	desc string
	// sources are the values the callers pass, for the checks that depend on what the value is.
	sources []string
}

// callContext is what one call tells the callee: the privileged events it runs for and the untrusted inputs.
type callContext struct {
	from   string   // the workflow that makes the call
	events []string // privileged events, in the order of privilegedTriggers
	inputs map[string]callInput
	// numbers are the inputs that the call fills with the number of the pull request that a comment is on, with where it
	// comes from: the callee can give them to `gh pr checkout`.
	numbers map[string]string
	// gates are the gates that hold at one moment in front of the call (see untrusted_checkout_toctou.go), rerun whether the
	// workflow that makes the call runs again for each push to a pull request.
	gates gateInfo
	rerun bool

	// values are the values of the inputs that the call gives, as they are written (or the defaults of the callee), by lower case
	// name: the protection of actions/checkout is decided for the value that the step gets.
	values map[string]string
}

// calleeKey returns the key of the file in the call index, "" when it is neither a workflow nor an action of the project.
func calleeKey(project *Project, file string) string {
	if project == nil || file == "" {
		return ""
	}
	root, err := filepath.Abs(project.RootDir())
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(file) {
		// the path is relative to the directory the linter was started in, or to the root of the project
		if _, err := os.Stat(abs); err != nil {
			abs = filepath.Join(root, file)
		}
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if IsActionPath(rel) {
		return "a:" + filepath.ToSlash(filepath.Dir(rel))
	}
	return "w:" + rel
}

// valueUntrustedFor reports whether the value names the head of a pull request in a run of the events: in a run that only a
// comment starts there is no head in the event, and only a ref built from the number of the issue names one.
func valueUntrustedFor(events []string, value string) (string, bool) {
	if slices.Contains(events, "pull_request_target") || slices.Contains(events, "workflow_run") || !slices.Contains(events, "issue_comment") {
		return untrustedValue(value)
	}
	if strings.Contains(value, "refs/pull/") {
		return value, true
	}
	return "", false
}

// callContexts returns the calls that reach the callee with a privileged event. depth counts the reusable workflows
// between the callee and the workflow that makes the call.
func (rule *RuleUntrustedCheckout) callContexts(key string, depth int, seen map[string]bool, defaults map[string]string) []callContext {
	if rule.project == nil || depth >= maxCallDepth || seen[key] {
		return nil
	}
	seen[key] = true
	defer delete(seen, key)
	var ret []callContext
	for _, ref := range rule.siblings.callRefs(rule.project, key) {
		src, err := os.ReadFile(filepath.Join(rule.project.RootDir(), filepath.FromSlash(ref.file)))
		if err != nil {
			continue
		}
		cw, _ := Parse(src)
		if cw == nil {
			continue
		}
		job := cw.Jobs[ref.job]
		if job == nil {
			continue
		}
		var step *Step
		var call map[string]string // the values the call passes, by lower case input name
		if ref.step < 0 {
			if job.WorkflowCall == nil {
				continue
			}
			call = map[string]string{}
			for n, in := range job.WorkflowCall.Inputs {
				if in != nil && in.Value != nil {
					call[strings.ToLower(n)] = in.Value.Value
				}
			}
		} else {
			steps := flattenSteps(job.Steps)
			if ref.step >= len(steps) {
				continue
			}
			step = steps[ref.step]
			a, _ := step.Exec.(*ExecAction)
			if a == nil {
				continue
			}
			call = map[string]string{}
			for n, in := range a.Inputs {
				if in != nil && in.Value != nil {
					call[strings.ToLower(n)] = in.Value.Value
				}
			}
		}

		// the events that the caller runs for: its own, and the ones of the workflows that call it
		events := privilegedEvents(cw, rule.project, rule.siblings)
		var outer []callContext
		if _, ok := cw.FindWorkflowCallEvent(); ok {
			outer = rule.callContexts("w:"+ref.file, depth+1, seen, inputDefaults(cw))
			for _, c := range outer {
				events = append(events, c.events...)
			}
		}
		events = neededEvents(cw, job, eventsAfterCondition(sortedEvents(events), job.If), map[*Job]bool{})
		if step != nil {
			events = eventsAfterCondition(events, step.If) // a step that is not for these events is not a call for them
		}
		if len(events) == 0 {
			continue
		}
		// only the callers that run for the events that reach this call pass something down
		var live []callContext
		for _, o := range outer {
			if slices.ContainsFunc(o.events, func(e string) bool { return slices.Contains(events, e) }) {
				live = append(live, o)
			}
		}
		outer = live
		g := rule.callGates(cw, job, step, events)
		if g.safe {
			continue
		}
		// the gates of the workflows that call the caller count only when every one of those calls has gates
		outerGates := gateInfo{}
		rerun := rerunsOnPush(cw)
		for i, o := range outer {
			if i == 0 {
				outerGates = o.gates
			} else {
				outerGates = outerGates.both(o.gates) // a gate counts when every one of those calls has it
			}
			rerun = rerun || o.rerun
		}
		if len(outerGates.stale) > 0 {
			g = g.merge(outerGates)
		}
		for name, dv := range defaults {
			if _, ok := call[name]; !ok {
				call[name] = dv // an input that the call leaves out has its default
			}
		}
		ctx := callContext{from: ref.file, events: events, inputs: map[string]callInput{}, numbers: map[string]string{}, values: call, gates: g, rerun: rerun}
		for name, v := range call {
			if what, ok := valueUntrustedFor(events, v); ok {
				mergeInputs(ctx.inputs, map[string]callInput{name: {desc: what + " by " + ref.file, sources: []string{v}}})
				continue
			}
			refs, _ := stringExprRefs(v)
			if what, ok := issueNumberRefs(refs); ok && slices.Contains(events, "issue_comment") {
				ctx.numbers[name] = what + " by " + ref.file
			}
			// an input of the caller that is untrusted in the context that reaches the caller: the sources of every call
			// that reaches it count, the one that passes a branch as well as the one that passes a commit
			for _, o := range outer {
				for _, r := range refs {
					if len(r.chain) == 2 && r.chain[0] == "inputs" {
						if in, ok := o.inputs[r.chain[1]]; ok {
							mergeInputs(ctx.inputs, map[string]callInput{name: {desc: in.desc + ", passed on by " + ref.file, sources: in.sources}})
						}
						if d, ok := o.numbers[r.chain[1]]; ok {
							if _, set := ctx.numbers[name]; !set {
								ctx.numbers[name] = d + ", passed on by " + ref.file
							}
						}
					}
				}
			}
		}
		ret = append(ret, ctx)
	}
	return ret
}

// sortedEvents returns the events without repeats in the order of privilegedTriggers.
func sortedEvents(events []string) []string {
	var ret []string
	for _, e := range privilegedTriggers {
		if slices.Contains(events, e) {
			ret = append(ret, e)
		}
	}
	return ret
}

// callGates are the gates in front of the job of the call and of the step: the same as for a checkout (an environment, a
// guard condition, a needed job that is guarded, a permission check before the step). A call that is safe is not a call; the
// gates that hold at one moment go to the callee, which judges them for the code that follows (see reportToctou).
func (rule *RuleUntrustedCheckout) callGates(w *Workflow, job *Job, step *Step, events []string) gateInfo {
	commentOnly := slices.Contains(events, "issue_comment") && !slices.Contains(events, "pull_request_target") && !slices.Contains(events, "workflow_run")
	g := classifyCondition(job.If, commentOnly).merge(rule.neededGates(w, job, events, commentOnly, map[*Job]bool{}))
	if rule.environmentHolds(job) {
		g.stale = append(g.stale, "an environment")
	}
	if step == nil {
		return g
	}
	var gate, staleGate permissionGateState // the permission checks that hold, and the ones that hold at one moment
	var stale gateInfo
	steps, groups := flattenStepsGroup(job.Steps)
	for i, s := range steps {
		if s == step {
			g = g.merge(classifyCondition(s.If, commentOnly))
			// a step that runs after a failure (always(), failure()) still runs when the permission check failed, and a step
			// beside the check in a parallel group is not behind it
			if !stepRunsAfterFailure(s) {
				if gate.holds(groups[i]) {
					g.safe = true
				}
				if staleGate.holds(groups[i]) {
					g = g.merge(stale)
				}
			}
			return g
		}
		if isPermissionGate(s) {
			if pg := permissionGate(commentOnly); pg.safe {
				gate.see(groups[i])
			} else {
				staleGate.see(groups[i])
				stale = stale.merge(pg)
			}
		}
	}
	return g
}

// inputDefaults are the defaults of the inputs of a reusable workflow, by lower case name.
func inputDefaults(w *Workflow) map[string]string {
	ret := map[string]string{}
	if w == nil {
		return ret
	}
	if e, ok := w.FindWorkflowCallEvent(); ok {
		for _, in := range e.Inputs {
			if in != nil && in.Default != nil && in.Name != nil {
				ret[strings.ToLower(in.Name.Value)] = in.Default.Value
			}
		}
	}
	if w.Action != nil {
		for _, in := range w.Action.Inputs {
			if in != nil && in.ID != nil && in.Default != nil {
				ret[strings.ToLower(in.ID.Value)] = in.Default.Value
			}
		}
	}
	return ret
}

// inputsRead returns the names of the inputs that the expressions of the string read (`${{ inputs.ref }}`).
func inputsRead(value string) []string {
	refs, _ := stringExprRefs(value)
	return inputsOfRefs(refs)
}

func inputsOfRefs(refs []exprRef) []string {
	var ret []string
	for _, r := range refs {
		if len(r.chain) >= 2 && r.chain[0] == "inputs" {
			ret = append(ret, r.chain[1])
		}
	}
	return ret
}

// inputUntrusted reports whether the references read an input that a call gives an untrusted value.
func (rule *RuleUntrustedCheckout) inputUntrusted(refs []exprRef) (string, bool) {
	if len(rule.inputTaint) == 0 {
		return "", false
	}
	for _, name := range inputsOfRefs(refs) {
		if in, ok := rule.inputTaint[name]; ok {
			return "inputs." + name + ", given " + in.desc, true
		}
	}
	return "", false
}

// inputForksPR reports whether the string reads an input that every call fills with a value that the protection of
// actions/checkout refuses (see checkoutInputForksPR).
func (rule *RuleUntrustedCheckout) inputForksPR(name, value string) bool {
	found := false
	for _, in := range inputsRead(value) {
		ti, ok := rule.inputTaint[in]
		if !ok {
			continue
		}
		for _, src := range ti.sources {
			if !checkoutInputForksPR(name, src) || rule.uncoveredUntrusted(name, src) {
				return false // a branch that the caller passes beside the commit gets through the protection
			}
		}
		found = true
	}
	return found
}

// descAlternative joins the descriptions of the values that several callers pass for an input.
const descAlternative = ", or "

// uncoveredUntrusted reports whether the value reads something untrusted that the protection of actions/checkout does not look
// at: a ref is only refused when every untrusted value in it is one that the protection knows (`github.head_ref` beside
// `github.event.pull_request.head.sha` is a branch that it lets through). The inputs are judged through their callers.
func (rule *RuleUntrustedCheckout) uncoveredUntrusted(name, value string) bool {
	refs, _ := stringExprRefs(value)
	covered := forkCommitContexts
	if name == "repository" {
		covered = forkRepoContexts
	}
	for _, r := range refs {
		if len(r.chain) > 0 && r.chain[0] == "inputs" {
			continue
		}
		if _, ok := refsMatchAny([]exprRef{r}, covered); ok || (name != "repository" && refIsPullNumber(r.chain)) {
			continue
		}
		if _, ok := untrustedRefs([]exprRef{r}); ok {
			return true
		}
	}
	return false
}

// substituteInputs replaces the expressions that only read an input by the value of the input. An input that is not known stays.
func substituteInputs(v string, value func(name string) (string, bool)) string {
	return reInputExpr.ReplaceAllStringFunc(v, func(m string) string {
		sm := reInputExpr.FindStringSubmatch(m)
		name := sm[1]
		if name == "" {
			name = sm[2]
		}
		if v, ok := value(strings.ToLower(name)); ok {
			return v
		}
		return m
	})
}

// callsForkPR reports whether every call of the file gives the inputs that the value reads values that make the protection of
// actions/checkout refuse the checkout: `refs/pull/${{ inputs.n }}/head` with the number of the pull request of the event.
func (rule *RuleUntrustedCheckout) callsForkPR(name, value string) bool {
	if len(rule.calls) == 0 || len(inputsRead(value)) == 0 {
		return false
	}
	for _, c := range rule.calls {
		sub := substituteInputs(value, func(n string) (string, bool) { v, ok := c.values[n]; return v, ok })
		if sub == value || !checkoutInputForksPR(name, sub) || rule.uncoveredUntrusted(name, sub) {
			return false
		}
	}
	return true
}

// mergeInputs adds the untrusted inputs of a call to the ones of the file.
func mergeInputs(into map[string]callInput, from map[string]callInput) {
	for n, in := range from {
		if cur, ok := into[n]; ok {
			cur.sources = append(cur.sources, in.sources...)
			if !refIsMutable(cur.desc) && refIsMutable(in.desc) {
				cur.desc += descAlternative + in.desc // a branch from one caller is judged as a branch, whatever the others pass
			}
			into[n] = cur
			continue
		}
		into[n] = in
	}
}

// reInputExpr matches an expression that only reads an input: inputs.name, INPUTS.Name and inputs['name'].
var reInputExpr = regexp.MustCompile(`(?i)\$\{\{\s*inputs(?:\.([a-z_][a-z0-9_-]*)|\[\s*'([^']*)'\s*\])\s*\}\}`)

// localActionCheckouts returns the code of a pull request that a local composite action checks out when the step calls
// it. The inputs of the action that the call fills with the head of a pull request (or that it leaves out, with a default that
// names it) are untrusted in the steps of the action, in whatever expression they are read. The checkout is reported at the
// step that calls the action, and the steps of the caller that run the code are found like for any other checkout. A step of
// the action that is guarded, or an action that calls another one, is not followed.
func (rule *RuleUntrustedCheckout) localActionCheckouts(s *Step, e *ExecAction) []*untrustedCheckout {
	if rule.project == nil || e.Uses == nil {
		return nil
	}
	t, ok := localTarget(e.Uses.Value)
	if !ok || t.kind != 'a' {
		return nil
	}
	action := rule.readAction(t.path)
	if action == nil || action.Action == nil || !action.Action.Runs.IsComposite() {
		return nil
	}
	defaults := inputDefaults(action)
	// the value of an input of the action in this call: what the step passes, or the default of the action
	value := func(name string) (string, bool) {
		if in := e.Inputs[name]; in != nil && in.Value != nil {
			return in.Value.Value, true
		}
		v, ok := defaults[name]
		return v, ok
	}
	subst := func(v string) string { return substituteInputs(v, value) }
	// the inputs of the action are the taint of its steps, from the values of the call
	local := map[string]callInput{}
	localNumber := map[string]string{}
	names := map[string]bool{}
	for name := range e.Inputs {
		names[name] = true
	}
	for name := range defaults {
		names[name] = true
	}
	for name := range names {
		v, ok := value(name)
		if !ok {
			continue
		}
		if what, bad := rule.untrusted(v); bad {
			local[name] = callInput{desc: what, sources: []string{v}}
		} else if what, ok := rule.numberValue(v); ok {
			localNumber[name] = what
		}
	}
	savedTaint, savedNumber, savedCalls := rule.inputTaint, rule.inputNumber, rule.calls
	rule.inputTaint, rule.inputNumber, rule.calls = local, localNumber, nil // the inputs are the ones of the action
	defer func() { rule.inputTaint, rule.inputNumber, rule.calls = savedTaint, savedNumber, savedCalls }()

	var ret []*untrustedCheckout
	var gate, staleGate permissionGateState // the permission checks that hold, and the ones that hold at one moment
	var stale gateInfo
	commentOnly := rule.commentOnly()
	for _, j := range action.Jobs {
		steps, stepGroups := flattenStepsGroup(j.Steps)
		for i, cs := range steps {
			g := stepGroups[i]
			if isPermissionGate(cs) {
				// the action fails here for a user who may not write, and so does the step that calls it: what it checked
				// out before or beside the check (a parallel group runs together, and the check does not hold back what runs
				// beside it) is run by the steps of the caller that run after a failure only.
				if pg := permissionGate(commentOnly); pg.safe {
					gate.see(g)
					for _, c := range ret {
						c.afterFailure = true
					}
				} else {
					// the check holds at one moment: what was checked out before it is behind it too, for the ref to be judged
					staleGate.see(g)
					stale = stale.merge(pg)
					for _, c := range ret {
						c.gates = c.gates.merge(pg)
					}
				}
				continue
			}
			if gate.holds(g) && !stepRunsAfterFailure(cs) {
				continue // the steps after a permission check only run for a user who may write
			}
			ce, ok := cs.Exec.(*ExecAction)
			if !ok || len(eventsAfterCondition(rule.privileged, cs.If)) == 0 {
				continue
			}
			// a condition that holds at one moment (a label of the event, the commenter) leaves the checkout to the check
			// of the ref; one that holds after the code changes stops it
			var stepGate gateInfo
			if conditionIsGuard(cs.If) {
				if stepGate = classifyCondition(cs.If, commentOnly); stepGate.safe || len(stepGate.stale) == 0 {
					continue
				}
			}
			if staleGate.holds(g) && !stepRunsAfterFailure(cs) {
				stepGate = stepGate.merge(stale)
			}
			if _, cu := stepAction(cs); cu != nil && cu.isRepoAction("actions/checkout") {
				if c := rule.actionCheckout(cs, ce, cu, action.Comments, subst, e.Uses.Pos); c != nil {
					c.step = s
					c.gates = stepGate
					c.afterFailure = gate.set // after the check or beside it: only a step of the caller that also runs after a failure runs it
					c.what += " in " + t.path
					ret = append(ret, c)
				}
			}
		}
	}
	return ret
}

// readAction parses the metadata of the action in the directory (relative to the root of the project), nil if there is
// none.
func (rule *RuleUntrustedCheckout) readAction(dir string) *Workflow {
	if w, ok := rule.actions[dir]; ok {
		return w
	}
	var w *Workflow
	for _, name := range actionFileNames {
		rel := path.Join(dir, name)
		if !IsActionPath(rel) {
			continue
		}
		if src, err := os.ReadFile(filepath.Join(rule.project.RootDir(), filepath.FromSlash(rel))); err == nil {
			w, _ = ParseAction(src)
			break
		}
	}
	if rule.actions == nil {
		rule.actions = map[string]*Workflow{}
	}
	rule.actions[dir] = w
	return w
}
