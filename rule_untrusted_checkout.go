package jactionlint

import (
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
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUntrustedCheckout) VisitWorkflowPre(n *Workflow) error {
	rule.privileged, rule.wf = nil, n
	if !rule.Config().RuleEnabled("untrusted-checkout") {
		return nil
	}
	rule.privileged = privilegedEvents(n, rule.project, rule.siblings)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUntrustedCheckout) VisitJobPre(n *Job) error {
	if len(rule.privileged) == 0 || (n.Environment != nil && n.Environment.Name != nil) || conditionIsGuard(n.If) {
		return nil
	}
	// The checkouts which no step has run the code of yet, by the directory they put it in. Whether a
	// step runs code depends on the directory only, so it is asked once for each directory and not for
	// each checkout: a job with thousands of checkouts is not quadratic.
	rule.job = n
	pending := map[string][]*untrustedCheckout{}
	var dirs []string // the keys of pending in the order they were added
	for _, s := range flattenSteps(n.Steps) {
		if conditionIsGuard(s.If) {
			continue // a step a maintainer has to allow neither runs the code nor fetches it
		}
		for _, dir := range dirs {
			cs := pending[dir]
			if len(cs) == 0 {
				continue
			}
			if how, ok := stepRunsCode(s, dir, 0); ok {
				for _, c := range cs {
					rule.report(c, how)
				}
				pending[dir] = nil
			}
		}
		for _, c := range rule.checkoutsOf(n, s) {
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
	if event == "workflow_run" {
		source = "the run that triggered this workflow"
	}
	privileges := "the workflow has a write token and secrets, so whoever controls that code can use them"
	if rule.job != nil && rule.wf != nil && tokenIsAbsent(rule.wf, rule.job) {
		privileges = "the job has no token (\"permissions: {}\"), but the code still runs on the runner with everything else the job gives it, such as secrets passed to a step and the cache"
	}
	rule.ReportIDf(
		"untrusted-checkout",
		c.pos,
		"this step checks out code from %s (%s) in a %q workflow and %s runs it afterwards. %s. run untrusted code in a \"pull_request\" workflow without secrets, or check out the base branch and only read the pull request as data",
		source, c.what, event, how, privileges,
	)
}

// tokenIsAbsent reports whether the job sets "permissions: {}", so GITHUB_TOKEN has no scope at all.
func tokenIsAbsent(w *Workflow, j *Job) bool {
	p := j.Permissions
	if p == nil {
		p = w.Permissions
	}
	return p != nil && p.All == nil && len(p.Scopes) == 0
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
		for _, name := range []string{"ref", "repository"} {
			in := e.Inputs[name]
			if in == nil || in.Value == nil {
				continue
			}
			if what, bad := untrustedValue(in.Value.Value); bad {
				return []*untrustedCheckout{{pos: in.Value.Pos, what: what, dir: dir, step: s}}
			}
		}
	case *ExecRun:
		script, _ := analyzeRun(e)
		if script == nil {
			return nil
		}
		env := untrustedEnvNames(rule.wf, j, s)
		var ret []*untrustedCheckout
		for i, c := range script.Commands {
			what, ok := commandChecksOutUntrusted(c, env)
			if !ok && c.Name == "git" && c.Verb() == "fetch" {
				// A fetch only downloads the objects: it checks out the code when a later command in
				// the script puts what it fetched in the working tree.
				if w, bad := commandNamesUntrusted(c, env); bad && fetchedThenUsed(script.Commands[i+1:]) {
					what, ok = w, true
				}
			}
			if ok {
				pos := s.Pos
				if e.RunPos != nil {
					pos = e.RunPos
				}
				dir := ""
				if c.Name == "git" && c.Verb() == "clone" && len(c.Positional) > 2 {
					dir = normalizeDir(c.Sub(2))
				}
				ret = append(ret, &untrustedCheckout{pos: pos, what: what, dir: dir, offset: c.Offset, step: s})
			}
		}
		return ret
	}
	return nil
}

// gitCheckoutVerbs are the git commands that put a revision in the working tree. A "fetch" is not one:
// it only downloads objects, see fetchedThenUsed.
var gitCheckoutVerbs = map[string]bool{
	"checkout": true, "switch": true, "pull": true, "merge": true, "cherry-pick": true,
	"rebase": true, "clone": true, "reset": true, "restore": true,
}

// commandChecksOutUntrusted reports whether the command brings the code of a pull request into the
// workspace: `gh pr checkout` or a git command whose arguments name the head of the pull request.
func commandChecksOutUntrusted(c *runscript.Command, envNames map[string]bool) (string, bool) {
	switch c.Name {
	case "gh":
		if c.Verb() == "pr" && c.Sub(1) == "checkout" {
			return "gh pr checkout", true
		}
	case "git":
		if !gitCheckoutVerbs[c.Verb()] {
			return "", false
		}
		return commandNamesUntrusted(c, envNames)
	}
	return "", false
}

// commandNamesUntrusted reports whether a word of the command names the head of a pull request.
func commandNamesUntrusted(c *runscript.Command, envNames map[string]bool) (string, bool) {
	for _, w := range c.Words {
		for _, e := range w.Exprs {
			if what, bad := untrustedValue("${{ " + e + " }}"); bad {
				return what, true
			}
		}
		for _, v := range w.Vars {
			if envNames[v] {
				return "$" + v, true
			}
		}
		if strings.Contains(w.Value, "refs/pull/") {
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

// fetchedThenUsed reports whether one of the commands puts a fetched revision in the working tree.
func fetchedThenUsed(rest []*runscript.Command) bool {
	for _, c := range rest {
		if c.Name == "git" && workTreeVerbs[c.Verb()] {
			return true
		}
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "untrusted-checkout", Group: RuleGroupSecurity, Summary: "A pull_request_target or workflow_run workflow checks out the code of a pull request and runs it.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-untrusted-checkout"},
	)
	registerRuleFactory("untrusted-checkout", func(env *RuleEnv) []Rule {
		r := NewRuleUntrustedCheckout(env.project)
		r.siblings = env.localActions.siblings()
		return []Rule{r}
	})
}
