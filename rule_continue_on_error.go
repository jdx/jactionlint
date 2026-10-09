package jactionlint

import "fmt"

// RuleContinueOnError reports jobs with `continue-on-error: true`, which let the workflow pass when
// the job fails.
type RuleContinueOnError struct {
	RuleBase
}

// NewRuleContinueOnError creates a new RuleContinueOnError instance.
func NewRuleContinueOnError() *RuleContinueOnError {
	return &RuleContinueOnError{
		RuleBase: RuleBase{
			name: "continue-on-error",
			desc: "Checks for jobs whose failure does not fail the workflow",
		},
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleContinueOnError) VisitJobPre(n *Job) error {
	c := n.ContinueOnError
	// An expression is the usual way to allow failures of some matrix entries, so only the literal
	// is reported
	if c == nil || c.Expression != nil || !c.Value || !rule.Config().RuleEnabled("continue-on-error") {
		return nil
	}
	rule.ReportIDf(
		"continue-on-error",
		c.Pos,
		"\"continue-on-error: true\" makes the workflow pass even when job %q fails, which hides failures. remove it, or limit it to what is allowed to fail, for example with an expression on a matrix entry",
		n.ID.Value,
	)
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleContinueOnError) VisitStep(n *Step) error {
	c := n.ContinueOnError
	if c == nil || c.Expression != nil || !c.Value || !rule.Config().RuleEnabled("continue-on-error") {
		return nil
	}
	if steps, _ := rule.Config().ruleOptionBool("continue-on-error", "steps"); !steps {
		return nil
	}
	what := "this step"
	if n.Name != nil && n.Name.Value != "" {
		what = fmt.Sprintf("step %q", n.Name.Value)
	}
	rule.ReportIDf(
		"continue-on-error",
		c.Pos,
		"\"continue-on-error: true\" makes the job pass even when %s fails, which hides failures. remove it, or check the outcome of the step in a later step",
		what,
	)
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "continue-on-error", Group: RuleGroupPolicy, Summary: "A job has continue-on-error: true, so its failure does not fail the workflow.", DefaultLevel: SeverityInfo, Profile: ProfileStrict, DocsAnchor: "check-continue-on-error",
			Options: []RuleOption{{Name: "steps", Kind: RuleOptionBool, Default: false, Summary: "Also report steps with continue-on-error: true. By default only jobs are reported."}}},
	)
	registerRuleFactory("continue-on-error", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleContinueOnError()}
	})
}
