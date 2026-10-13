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
	// scripts are the script files that were read, by path, nil for one that cannot be followed.
	scripts map[string]*runscript.Script
	// guard is which events of the workflow the protection of actions/checkout covers, see untrusted_checkout_guard.go.
	guard checkoutEvents
	// allPrivileged are the privileged events of the workflow, privileged those that the job being judged runs for.
	allPrivileged []string
	// stepActions are the steps of the job that have an id and run an action, by id: the lower case name of the action.
	stepActions map[string]string
	// comment is whether a comment starts the workflow (issue_comment), ghAny whether gh pr checkout is a checkout
	// of a pull request whatever it is given: the workflow has an event that carries the head of a pull request.
	comment, ghAny bool
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
	rule.privileged, rule.wf = nil, n
	if !rule.Config().RuleEnabled("untrusted-checkout") {
		return nil
	}
	rule.privileged = privilegedEvents(n, rule.project, rule.siblings)
	rule.allPrivileged = rule.privileged
	rule.guard = checkoutGuardEvents(n, rule.privileged, rule.project, rule.siblings)
	rule.loose = rule.pedantic("untrusted-checkout")
	rule.setEvents(rule.privileged)
	return nil
}

// setEvents sets the privileged events that the job or step being judged can run for.
func (rule *RuleUntrustedCheckout) setEvents(events []string) {
	rule.privileged = events
	rule.comment = slices.Contains(events, "issue_comment")
	rule.ghAny = slices.Contains(events, "pull_request_target") || slices.Contains(events, "workflow_run")
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUntrustedCheckout) VisitJobPre(n *Job) error {
	rule.setEvents(eventsAfterCondition(rule.allPrivileged, n.If))
	if len(rule.privileged) == 0 || (n.Environment != nil && n.Environment.Name != nil) || conditionIsGuard(n.If) {
		return nil
	}
	// The checkouts which no step has run the code of yet, by the directory they put it in. Whether a
	// step runs code depends on the directory only, so it is asked once for each directory and not for
	// each checkout: a job with thousands of checkouts is not quadratic.
	rule.job = n
	rule.fetches, rule.carried = nil, nil
	if neededJobGated(rule.wf, n, map[*Job]bool{}) {
		return nil // a job that this one needs is skipped or stops unless the user is trusted
	}
	rule.stepActions = map[string]string{}
	for _, s := range flattenSteps(n.Steps) {
		if _, u := stepAction(s); u != nil && s.ID != nil && s.ID.Value != "" && u.Kind == UsesAction {
			rule.stepActions[strings.ToLower(s.ID.Value)] = strings.ToLower(u.CanonicalName())
		}
	}
	gated := false
	pending := map[string][]*untrustedCheckout{}
	var dirs []string // the keys of pending in the order they were added
	jobMerged := conditionRequiresMerged(n.If)
	for _, s := range flattenSteps(n.Steps) {
		if gated {
			break // the steps after a permission check only run for a user who may write
		}
		if isPermissionGate(s) {
			gated = true
			continue
		}
		if len(eventsAfterCondition(rule.privileged, s.If)) == 0 {
			continue // the step does not run for the events that this rule judges
		}
		if conditionIsGuard(s.If) {
			continue // a step a maintainer has to allow neither runs the code nor fetches it
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
						continue
					}
					rule.report(c, how)
				}
				pending[dir] = kept
			}
		}
		if !merged {
			rule.reportOptOut(s)
		}
		for _, c := range rule.checkoutsOf(n, s) {
			if merged && c.immutable() {
				continue // the code is only checked out, or only run, once it was merged
			}
			if c.guarded && !rule.loose {
				continue // actions/checkout refuses it for a fork
			}
			if c.how != "" {
				rule.report(c, c.how)
				continue
			}
			if run, ok := s.Exec.(*ExecRun); ok && run.Run != nil {
				// The same script can run the code it fetched
				if how, ok := stepRunsCode(s, c.dir, c.offset+1); ok {
					rule.report(c, how)
					continue
				}
			}
			if _, seen := pending[c.dir]; !seen {
				dirs = append(dirs, c.dir)
			}
			pending[c.dir] = append(pending[c.dir], c)
		}
	}
	return nil
}

func (rule *RuleUntrustedCheckout) report(c *untrustedCheckout, how string) {
	event := rule.privileged[0]
	source := "a pull request"
	switch event {
	case "workflow_run":
		source = "the run that triggered this workflow"
	case "issue_comment":
		source = "the pull request that was commented on"
	}
	privileges := "the workflow has a write token and secrets, so whoever controls that code can use them"
	if rule.job != nil && rule.wf != nil && tokenIsAbsent(rule.wf, rule.job) {
		privileges = "the job's GITHUB_TOKEN has no permissions (\"permissions: {}\"), but the code still runs on the runner with everything else the job gives it, such as secrets passed to a step and the cache"
	} else if rule.job != nil && rule.wf != nil && tokenIsReadOnly(rule.wf, rule.job) {
		privileges = "the job's GITHUB_TOKEN can only read, but the code still runs on the runner with everything else the job gives it, such as secrets passed to a step and the cache, and whoever controls it can use them"
	}
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
	rule.ReportIDf(
		"untrusted-checkout",
		opt.Value.Pos,
		"this step sets \"allow-unsafe-pr-checkout\", which turns off the protection of actions/checkout that refuses to check out the code of a pull request from a fork in a %q workflow. the step then checks out whatever \"ref\" and \"repository\" name, so nothing stops the code of the pull request from reaching a privileged workflow, which has secrets and may have a write token. remove the input and check out the base branch, or document why the code is safe to run",
		rule.privileged[0],
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

// checkoutsOf returns the untrusted code that the step checks out.
func (rule *RuleUntrustedCheckout) checkoutsOf(j *Job, s *Step) []*untrustedCheckout {
	switch e := s.Exec.(type) {
	case *ExecAction:
		_, u := stepAction(s)
		if u == nil || !u.isRepoAction("actions/checkout") {
			return nil
		}
		dir := ""
		if p, ok := e.input("path"); ok {
			dir = normalizeDir(p)
		}
		var found *untrustedCheckout
		forks := false
		for _, name := range []string{"ref", "repository"} {
			in := e.Inputs[name]
			if in == nil || in.Value == nil {
				continue
			}
			if checkoutInputForksPR(name, in.Value.Value) {
				forks = true
			}
			if found != nil {
				continue
			}
			if what, bad := rule.untrustedStr(in.Value); bad {
				found = &untrustedCheckout{pos: in.Value.Pos, what: what, dir: dir, step: s}
			}
		}
		if found == nil {
			return nil
		}
		if forks && rule.guard.all {
			var comment *Comment
			if rule.wf != nil && e.Uses != nil {
				comment = rule.wf.Comments.Inline(e.Uses.Pos.Line)
			}
			ver := versionOfCheckout(u, comment)
			optOut := false
			if opt := e.Inputs["allow-unsafe-pr-checkout"]; opt != nil && opt.Value != nil {
				optOut = checkoutOptOut(opt.Value.Value)
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
		return []*untrustedCheckout{found}
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
				envDescs(rule.wf.Env, numEnv, issueNumberString)
			}
			envDescs(j.Env, numEnv, issueNumberString)
			envDescs(s.Env, numEnv, issueNumberString)
			number = newNumberTaint(script, numEnv)
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
		rule.carryEnv(script, taint, s.If != nil)
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
		RuleInfo{ID: "untrusted-checkout", Group: RuleGroupSecurity, Summary: "A pull_request_target, workflow_run or issue_comment workflow checks out the code of a pull request and runs it, or turns off the protection of actions/checkout against the code of fork pull requests. With the option pedantic, a checkout that actions/checkout refuses for a fork is reported too.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-untrusted-checkout", Options: []RuleOption{pedanticOption}},
	)
	registerRuleFactory("untrusted-checkout", func(env *RuleEnv) []Rule {
		r := NewRuleUntrustedCheckout(env.project)
		r.siblings = env.localActions.siblings()
		return []Rule{r}
	})
}
