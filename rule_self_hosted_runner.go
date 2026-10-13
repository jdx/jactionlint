package jactionlint

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// RuleSelfHostedRunner is a rule to point out jobs which run on self-hosted runners: jobs with the "self-hosted"
// label, with a label that "self-hosted-runner.labels" of the configuration lists (a runner of an ARC runner set has no
// "self-hosted" label) and jobs in a runner group. Such runners are hard to secure and GitHub does not recommend them
// for public repositories. The rule is informational: it cannot tell how a runner is configured, and a custom label
// alone (for example one of a hosted runner provider) is not reported.
// https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/about-self-hosted-runners#self-hosted-runner-security
type RuleSelfHostedRunner struct {
	RuleBase
	labels *RuleRunnerLabel // only to resolve matrix values
	wf     *Workflow
	// pullRequests are the pull request events which start the workflow, in the order of the workflow.
	pullRequests []string
	// callersUnknown is true for a reusable workflow which may be called by workflows that are not known (nothing in the
	// repository calls it, or a caller cannot be followed): a pull request event may reach it, which is not known.
	callersUnknown bool
}

// NewRuleSelfHostedRunner creates a new RuleSelfHostedRunner instance.
func NewRuleSelfHostedRunner() *RuleSelfHostedRunner {
	return &RuleSelfHostedRunner{
		RuleBase: RuleBase{
			name: "self-hosted-runner",
			desc: "Points out jobs which run on self-hosted runners",
		},
		labels: NewRuleRunnerLabel(),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleSelfHostedRunner) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	rule.pullRequests = nil
	// The events of the workflow itself and, for a reusable workflow, those of the workflows that call it
	rule.callersUnknown = false
	var events []Event
	called := false
	for _, e := range n.On {
		if _, ok := e.(*WorkflowCallEvent); ok {
			called = true
		} else {
			events = append(events, e)
		}
	}
	if called {
		if n.callerEvents != nil && n.callerEvents.Known {
			events = append(events, n.callerEvents.Events...)
		} else {
			rule.callersUnknown = true
		}
	}
	if n.Action != nil {
		events, _ = n.actorEvents()
	}
	for _, e := range events {
		if w, ok := e.(*WebhookEvent); ok && w.Hook != nil && slices.Contains(pullRequestEvents, w.Hook.Value) && !slices.Contains(rule.pullRequests, w.Hook.Value) {
			rule.pullRequests = append(rule.pullRequests, w.Hook.Value)
		}
	}
	return nil
}

// matchesLabel reports whether the label (names of runners are case-insensitive) matches one of the glob patterns.
func matchesLabel(patterns []string, label string) bool {
	if strings.Contains(label, "${{") {
		return false // an expression which is not resolved can be any label, a glob cannot say that it is an allowed one
	}
	for _, p := range patterns {
		if m, err := path.Match(strings.ToLower(p), strings.ToLower(label)); err == nil && m {
			return true
		}
	}
	return false
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleSelfHostedRunner) VisitJobPre(n *Job) error {
	r := n.RunsOn
	if r == nil {
		return nil
	}
	cfg := rule.Config()
	allow := cfg.ruleOptionStrings("self-hosted-runner", "allow")
	var configured []string
	if cfg != nil {
		configured = cfg.SelfHostedRunner.Labels
	}
	var m *Matrix
	if n.Strategy != nil {
		m = n.Strategy.Matrix
	}

	// The labels written in the file are all labels of the runner. The values which a matrix gives to an expression are
	// alternatives: each is a run of its own, so an allowed one leaves the others to be judged.
	var labels []*String
	fixedAllowed := false
	collect := func(l *String) {
		if l.ContainsExpression() {
			for _, alt := range rule.labels.tryToGetLabelsInMatrix(l, m) {
				if !matchesLabel(allow, alt.Value) {
					labels = append(labels, alt)
				}
			}
			return
		}
		if matchesLabel(allow, l.Value) {
			fixedAllowed = true
		}
		labels = append(labels, l)
	}
	if r.LabelsExpr != nil {
		collect(r.LabelsExpr)
	}
	for _, l := range r.Labels {
		collect(l)
	}

	// A runner which the configuration calls ephemeral or trusted is not reported
	if fixedAllowed || (r.Group != nil && matchesLabel(allow, r.Group.Value)) {
		return nil
	}

	var why string
	var pos *Pos
	for _, l := range labels {
		if strings.EqualFold(l.Value, "self-hosted") {
			why, pos = fmt.Sprintf("label %q", l.Value), l.Pos
			break
		}
	}
	if pos == nil {
		for _, l := range labels {
			if matchesLabel(configured, l.Value) {
				why, pos = fmt.Sprintf("label %q, which \"self-hosted-runner.labels\" of the configuration lists", l.Value), l.Pos
				break
			}
		}
	}
	if pos == nil && r.Group != nil && !r.Group.ContainsExpression() {
		why, pos = fmt.Sprintf("runner group %q, which holds self-hosted runners or larger runners of the organization", r.Group.Value), r.Group.Pos
	}
	if pos == nil {
		return nil
	}
	if only, _ := cfg.ruleOptionBool("self-hosted-runner", "pull-request-only"); only && len(rule.pullRequests) == 0 && !rule.callersUnknown {
		return nil
	}
	msg := fmt.Sprintf("job runs on a self-hosted runner (%s). self-hosted runners are hard to secure and should not run workflows of untrusted pull requests of a public repository", why)
	if len(rule.pullRequests) > 0 {
		msg += fmt.Sprintf(". this workflow is started by %s, so code of a pull request of a fork can run on the runner. use an ephemeral runner, or require approval for outside contributors", strings.Join(rule.pullRequests, " and "))
	}
	rule.ReportID("self-hosted-runner", pos, msg)
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "self-hosted-runner", Group: RuleGroupSecurity, Summary: "A job runs on a self-hosted runner: the label self-hosted, a label of self-hosted-runner.labels or a runner group.", DefaultLevel: SeverityInfo, Profile: ProfilePedantic, DocsAnchor: "check-self-hosted-runner",
			Options: []RuleOption{
				{Name: "allow", Kind: RuleOptionStrings, Summary: "Glob patterns of the labels and the runner groups of runners which are ephemeral or trusted. A job with one of them is not reported."},
				{Name: "pull-request-only", Kind: RuleOptionBool, Default: false, Summary: "Report only the jobs of workflows that a pull request event can start, which is the case that poutine reports."},
			},
		},
	)
	registerRuleFactory("self-hosted-runner", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("self-hosted-runner") {
			return nil
		}
		return []Rule{NewRuleSelfHostedRunner()}
	})
}
