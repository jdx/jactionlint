package jactionlint

import "strings"

// RuleSelfHostedRunner is a rule to point out jobs which run on self-hosted runners, i.e. jobs with
// the "self-hosted" label. Such runners are hard to secure and GitHub does not recommend them for
// public repositories. The rule is informational: it cannot tell how a runner is configured, and a
// custom label alone (for example one of a hosted runner provider) is not reported.
// https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/about-self-hosted-runners#self-hosted-runner-security
type RuleSelfHostedRunner struct {
	RuleBase
	labels *RuleRunnerLabel // only to resolve matrix values
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

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleSelfHostedRunner) VisitJobPre(n *Job) error {
	r := n.RunsOn
	if r == nil {
		return nil
	}
	var m *Matrix
	if n.Strategy != nil {
		m = n.Strategy.Matrix
	}

	var labels []*String
	collect := func(l *String) {
		if l.ContainsExpression() {
			labels = append(labels, rule.labels.tryToGetLabelsInMatrix(l, m)...)
		} else {
			labels = append(labels, l)
		}
	}
	if r.LabelsExpr != nil {
		collect(r.LabelsExpr)
	}
	for _, l := range r.Labels {
		collect(l)
	}

	for _, l := range labels {
		if strings.EqualFold(l.Value, "self-hosted") {
			rule.ReportIDf("self-hosted-runner", l.Pos, "job runs on a self-hosted runner (label %q). self-hosted runners are hard to secure and should not run workflows of untrusted pull requests of a public repository", l.Value)
			return nil
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "self-hosted-runner", Group: RuleGroupSecurity, Summary: "A job runs on a self-hosted runner.", DefaultLevel: SeverityInfo, Profile: ProfileAll, DocsAnchor: "check-self-hosted-runner"},
	)
	registerRuleFactory("self-hosted-runner", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("self-hosted-runner") {
			return nil
		}
		return []Rule{NewRuleSelfHostedRunner()}
	})
}
