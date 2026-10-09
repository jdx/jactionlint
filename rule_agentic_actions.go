package jactionlint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// agentUntrustedTriggers are the events that people without write access can cause, and whose payload
// they write: an issue, a comment, a review, a discussion, or the metadata of a pull request from a fork
// which a privileged workflow processes. pull_request is not among them: a workflow it starts from a fork
// has a read-only token and no secrets, so an agent in it has nothing to steal or write.
//
// workflow_run is not among them either: it is as safe as the workflow it follows, which this rule
// cannot see. Its documented use for pull requests of contributors is an open gate, see rule.gateTrigger.
var agentUntrustedTriggers = map[string]bool{
	"issues": true, "issue_comment": true, "pull_request_target": true, "discussion": true,
	"discussion_comment": true, "pull_request_review": true, "pull_request_review_comment": true,
}

// agentCheckoutTriggers are the events whose workflows run in the base repository while a pull request
// from a fork is there to be checked out.
var agentCheckoutTriggers = map[string]bool{"pull_request_target": true, "issue_comment": true, "workflow_run": true, "pull_request_review": true, "pull_request_review_comment": true}

// agentGuardMarkers are the texts of a condition that restrict a job to people the workflow trusts.
var agentGuardMarkers = []string{"author_association", "github.actor", "triggering_actor", "sender.login", "user.login", "event.label.name"}

// agentPRHeadMarkers are the texts of a checkout input that select code of a pull request.
var agentPRHeadMarkers = []string{"pull_request.head", "head_ref", "workflow_run.head", "refs/pull/", "pull_request.merge_commit_sha"}

var shellVarRe = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)`)

// RuleAgenticActions finds AI agent actions that outsiders can steer or that run with their safeguards
// off. An agent reads text that an attacker writes (an issue, a comment, the code of a pull request) as
// instructions, and then uses its tools with the secrets and the token of the job. It is a risk that
// template-injection cannot describe: there is no escaping that makes text safe to read for an agent.
type RuleAgenticActions struct {
	RuleBase
	wf *Workflow
	// trigger is the first event of the workflow that outsiders can cause, or empty.
	trigger string
	// gateTrigger is the first event of the workflow for which an open gate matters: trigger, or workflow_run.
	gateTrigger string
	// checkoutTrigger is the first event of the workflow that runs with a fork's pull request at hand.
	checkoutTrigger string
	anyTrigger      bool
}

// NewRuleAgenticActions creates a new RuleAgenticActions instance. With anyTrigger the unsafe settings of
// an agent are also reported in a workflow that outsiders cannot trigger.
func NewRuleAgenticActions(anyTrigger bool) *RuleAgenticActions {
	return &RuleAgenticActions{
		RuleBase: RuleBase{
			name: "agentic-actions",
			desc: "Checks for AI agent actions which outsiders can steer or whose safeguards are off",
		},
		anyTrigger: anyTrigger,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleAgenticActions) VisitWorkflowPre(n *Workflow) error {
	rule.wf, rule.trigger, rule.gateTrigger, rule.checkoutTrigger = n, "", "", ""
	for _, e := range n.On {
		name := e.EventName()
		if agentUntrustedTriggers[name] && rule.trigger == "" {
			rule.trigger = name
		}
		if (agentUntrustedTriggers[name] || name == "workflow_run") && rule.gateTrigger == "" {
			rule.gateTrigger = name
		}
		if agentCheckoutTriggers[name] && rule.checkoutTrigger == "" {
			rule.checkoutTrigger = name
		}
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleAgenticActions) VisitJobPre(n *Job) error {
	steps := flattenSteps(n.Steps)
	for i, s := range steps {
		a, ok := s.Exec.(*ExecAction)
		if !ok {
			continue
		}
		if agent := agentOf(a); agent != nil {
			rule.checkAgent(n, steps[:i], s, a, agent)
		}
	}
	return nil
}

func (rule *RuleAgenticActions) checkAgent(job *Job, prev []*Step, step *Step, a *ExecAction, agent *agentAction) {
	ctx := tiContext{wf: rule.wf, job: job, step: step}
	token := tokenPermissionsNote(rule.wf, job)
	exposed := false

	// A job whose token can only write issues, pull requests or discussions is the setup that the actions
	// document for triage and labeling, and so is an agent that is limited to a few tools or to a read-only
	// sandbox: a steered agent can do little. It still holds its API key.
	limited := tokenIsLimited(rule.wf, job) || (agent.restricted != nil && agent.restricted(a))

	// A gated action runs for users with write access only, unless an input opens the gate.
	// exposed is whether outsiders can steer the agent, limited or not: a limited agent still holds its secrets
	// and can print them, so the secrets of its environment are checked for it as well.
	if agent.Gated && rule.gateTrigger != "" {
		for _, g := range agent.OpenGate {
			if v, ok := a.input(g.Input); ok && v == g.Wildcard {
				exposed = true
				if limited {
					continue
				}
				who := "anyone can cause on a public repository"
				if rule.gateTrigger == "workflow_run" {
					who = "follows a workflow that outsiders can start"
				}
				rule.ReportIDf("agentic-actions", a.Inputs[g.Input].Value.Pos,
					"%q is %q, so %s can start %s, although the action checks for write access otherwise. this workflow runs on %q, which %s, and the text they write steers the agent%s. list the users you trust instead of the wildcard",
					g.Input, g.Wildcard, g.Who, agent.Title, rule.gateTrigger, who, token)
			}
		}
	}
	if !agent.Gated && rule.trigger != "" && !rule.agentGuarded(job, prev, step) {
		exposed = true
		if !limited {
			rule.ReportIDf("agentic-actions", a.Uses.Pos,
				"%s does not check who started it, and this workflow runs on %q, which anyone can cause on a public repository. the text they write steers the agent, which has the secrets of the job%s. restrict the job with an if: on github.event.comment.author_association (OWNER, MEMBER or COLLABORATOR) or on a label that only maintainers add, run it in an environment with required reviewers, or limit the agent to the few tools it needs",
				agent.Title, rule.trigger, token)
		}
	}

	if agent.unsafe != nil && (rule.trigger != "" || rule.anyTrigger) {
		for _, is := range agent.unsafe(a, runsOnWindows(job)) {
			when := "an agent can be steered by any text it reads"
			if rule.trigger != "" {
				when = fmt.Sprintf("this workflow runs on %q, so outsiders steer the agent with the text they write", rule.trigger)
			}
			rule.ReportIDf("agentic-actions", is.Str.Pos, "%s: %s. %s. allow only the exact commands that the task needs, and keep the token and the secrets of the job to the minimum", agent.Title, is.Msg, when)
		}
	}

	// Data of an attacker that reaches the prompt by an environment variable is as dangerous as data pasted
	// in it; template-injection reports the ${{ env.X }} spelling and this the one the agent reads itself.
	// Triage has to show the text to the agent, so an agent that can do little (limited) is accepted.
	for _, name := range agent.Prompts {
		in := a.Inputs[name]
		if in == nil || in.Value == nil || limited {
			continue
		}
		seen := map[string]bool{}
		for _, m := range shellVarRe.FindAllStringSubmatch(in.Value.Value, -1) {
			if seen[m[1]] {
				continue
			}
			seen[m[1]] = true
			if src := ctx.envTaint(m[1], 0); src != "" {
				rule.ReportIDf("agentic-actions", in.Value.Pos,
					"the %q input of %s tells the agent to read the environment variable %q, which holds the potentially untrusted input %q. the agent follows instructions in it as it would in the prompt. do not give the agent text of outsiders as instructions: let it read the data with a read-only tool, and keep the permissions and secrets of the job to the minimum",
					name, agent.Title, m[1], src)
			}
		}
	}

	if exposed {
		trigger := rule.trigger
		if trigger == "" {
			trigger = rule.gateTrigger
		}
		rule.checkSecretsInEnv(job, step, a, agent, trigger)
	}

	if rule.checkoutTrigger != "" {
		rule.checkPullRequestCheckout(prev, a, agent)
	}
}

// checkSecretsInEnv reports a secret other than GITHUB_TOKEN in the environment of an agent that outsiders can
// steer: its shell tool can print the environment.
func (rule *RuleAgenticActions) checkSecretsInEnv(job *Job, step *Step, a *ExecAction, agent *agentAction, trigger string) {
	for _, env := range []*Env{step.Env, job.Env, rule.wf.Env} {
		if env == nil {
			continue
		}
		names := make([]string, 0, len(env.Vars))
		for n := range env.Vars {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			v := env.Vars[n]
			if v == nil || v.Value == nil {
				continue
			}
			if ref, src := secretRefOf(v.Value); ref != nil {
				rule.ReportIDf("agentic-actions", v.Value.Pos,
					"the secret %q is in the environment of %s, which outsiders can steer through the text of this workflow's %q trigger. its shell can print the environment. pass the secret only to the steps that need it, never to the agent, and use the GITHUB_TOKEN of the job with minimal permissions for GitHub",
					ref.Display(src), agent.Title, trigger)
			}
		}
	}
	// Some actions take the environment of the agent as an input
	for _, name := range agent.EnvInputs {
		in := a.Inputs[name]
		if in == nil || in.Value == nil {
			continue
		}
		if ref, src := secretRefOf(in.Value); ref != nil {
			rule.ReportIDf("agentic-actions", in.Value.Pos,
				"the secret %q is in the environment of %s (the %q input), which outsiders can steer through the text of this workflow's %q trigger. its shell can print the environment. pass the secret only to the steps that need it, never to the agent, and use the GITHUB_TOKEN of the job with minimal permissions for GitHub",
				ref.Display(src), agent.Title, name, trigger)
		}
	}
}

// secretRefOf returns the first secret other than GITHUB_TOKEN which the value expands, and the text of its
// expression.
func secretRefOf(v *String) (*ctxRef, string) {
	for _, sp := range scanExprs(v) {
		for _, r := range exprContextRefs(sp.Node) {
			if r.Path[0] == "secrets" && (len(r.Path) < 2 || r.Path[1] != "github_token") {
				r := r
				return &r, sp.Src
			}
		}
	}
	return nil, ""
}

// checkPullRequestCheckout reports an agent that runs in a workspace holding the code of a pull request.
func (rule *RuleAgenticActions) checkPullRequestCheckout(prev []*Step, a *ExecAction, agent *agentAction) {
	for _, p := range prev {
		if !checksOutPullRequest(p) {
			continue
		}
		rule.ReportIDf("agentic-actions", a.Uses.Pos,
			"%s runs in a workspace that holds the code of a pull request (checked out in the step %s), and this workflow runs on %q, with the secrets of the base repository. the agent reads its instructions and configuration from the workspace (files such as CLAUDE.md, AGENTS.md, GEMINI.md, .claude/settings.json, .mcp.json and .gemini/settings.json), so the pull request can add instructions, hooks and tool servers. check out the base branch in the workspace and put the pull request in a subdirectory with \"path:\"",
			agent.Title, stepLabel(p), rule.checkoutTrigger)
		return
	}
}

func stepLabel(s *Step) string {
	if s.Name != nil && s.Name.Value != "" {
		return fmt.Sprintf("%q", s.Name.Value)
	}
	if s.Pos != nil {
		return fmt.Sprintf("at line %d", s.Pos.Line)
	}
	return "before it"
}

// checksOutPullRequest reports whether the step puts code of a pull request in the workspace root.
func checksOutPullRequest(s *Step) bool {
	switch e := s.Exec.(type) {
	case *ExecAction:
		_, u := stepAction(s)
		if u == nil || !u.isRepoAction("actions/checkout") {
			return false
		}
		if p, ok := e.input("path"); ok && p != "" && p != "." && p != "./" {
			return false // a subdirectory is the pattern the actions recommend
		}
		for _, name := range []string{"ref", "repository"} {
			if v, ok := e.input(name); ok {
				lower := strings.ToLower(v)
				for _, m := range agentPRHeadMarkers {
					if strings.Contains(lower, m) {
						return true
					}
				}
			}
		}
	case *ExecRun:
		if e.Run != nil {
			lower := strings.ToLower(e.Run.Value)
			return strings.Contains(lower, "gh pr checkout") || strings.Contains(lower, "refs/pull/")
		}
	}
	return false
}

// agentGuarded reports whether something in the job or in the job it needs restricts who can reach the
// agent: a condition on the actor, the association with the repository or a label, an environment, or a
// step that checks the permission of the actor. It is a heuristic: it cannot tell a condition that is
// correct from one that merely mentions the actor.
func (rule *RuleAgenticActions) agentGuarded(job *Job, prev []*Step, step *Step) bool {
	if job.Environment != nil {
		return true
	}
	if guardInCond(job.If) || guardInCond(step.If) {
		return true
	}
	if stepsGuard(prev) {
		return true
	}
	for _, id := range job.Needs {
		if id == nil {
			continue
		}
		if j := rule.wf.Jobs[strings.ToLower(id.Value)]; j != nil {
			if j.Environment != nil || guardInCond(j.If) || stepsGuard(flattenSteps(j.Steps)) {
				return true
			}
		}
	}
	return false
}

func guardInCond(c *String) bool {
	if c == nil {
		return false
	}
	lower := strings.ToLower(c.Value)
	for _, m := range agentGuardMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func stepsGuard(steps []*Step) bool {
	for _, s := range steps {
		if guardInCond(s.If) {
			return true
		}
		switch e := s.Exec.(type) {
		case *ExecRun:
			if e.Run != nil {
				lower := strings.ToLower(e.Run.Value)
				if strings.Contains(lower, "collaborators/") || strings.Contains(lower, "/permission") || strings.Contains(lower, "author_association") {
					return true
				}
			}
		case *ExecAction:
			if e.Uses != nil && strings.Contains(strings.ToLower(e.Uses.Value), "permission") {
				return true
			}
		}
	}
	return false
}

func runsOnWindows(job *Job) bool {
	if job.RunsOn == nil {
		return false
	}
	for _, l := range job.RunsOn.Labels {
		if l != nil && strings.Contains(strings.ToLower(l.Value), "windows") {
			return true
		}
	}
	return false
}

// tokenIsLimited reports whether the permissions of the job are set and grant no write access other than
// to issues, pull requests and discussions.
func tokenIsLimited(w *Workflow, j *Job) bool {
	p := j.Permissions
	if p == nil {
		p = w.Permissions
	}
	if p == nil {
		return false // the default token of the repository may be able to write everything
	}
	if p.All != nil {
		return p.All.Value != "write-all"
	}
	for name, s := range p.Scopes {
		if s == nil || s.Value == nil || s.Value.Value != "write" {
			continue
		}
		switch strings.ToLower(name) {
		case "issues", "pull-requests", "discussions":
		default:
			return false
		}
	}
	return true
}

// tokenPermissionsNote says which scopes the token of the job can write, as a suffix for a message. It is
// empty when the job sets no permissions or only reads.
func tokenPermissionsNote(w *Workflow, j *Job) string {
	p := j.Permissions
	if p == nil {
		p = w.Permissions
	}
	if p == nil {
		return ""
	}
	if p.All != nil {
		if p.All.Value == "write-all" {
			return ", and its token can write every scope"
		}
		return ""
	}
	var scopes []string
	for name, s := range p.Scopes {
		if s != nil && s.Value != nil && s.Value.Value == "write" {
			scopes = append(scopes, name)
		}
	}
	if len(scopes) == 0 {
		return ""
	}
	sort.Strings(scopes)
	return ", and its token can write " + strings.Join(scopes, ", ")
}

func init() {
	registerRules(
		RuleInfo{
			ID: "agentic-actions", Group: RuleGroupSecurity,
			Summary:      "An AI agent action can be steered by outsiders, runs on code of a pull request, or has its safeguards turned off.",
			DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-agentic-actions",
			Options: []RuleOption{{Name: "any-trigger", Kind: RuleOptionBool, Default: false, Summary: "Also report the unsafe settings of an agent (tools that allow any command, permission checks switched off) in a workflow that no outsider can trigger."}},
		},
	)
	registerRuleFactory("agentic-actions", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("agentic-actions") {
			return nil
		}
		v, _ := env.config.RuleOption("agentic-actions", "any-trigger")
		b, _ := v.(bool)
		return []Rule{NewRuleAgenticActions(b)}
	})
}
