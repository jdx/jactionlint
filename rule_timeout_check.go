package jactionlint

// RuleTimeoutCheck is a rule to check "timeout-minutes" of jobs. It is opt-in and configured by the
// "timeout-minutes" section of the configuration file.
// https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#jobsjob_idtimeout-minutes
type RuleTimeoutCheck struct {
	RuleBase
}

// NewRuleTimeoutCheck creates a new RuleTimeoutCheck instance.
func NewRuleTimeoutCheck() *RuleTimeoutCheck {
	return &RuleTimeoutCheck{
		RuleBase: RuleBase{
			name: "timeout-check",
			desc: "Checks that timeout-minutes is set at jobs and does not exceed the configured maximum (opt-in by config)",
		},
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleTimeoutCheck) VisitJobPre(n *Job) error {
	cfg := rule.Config()
	required := cfg.RuleEnabled("missing-timeout")
	maxMinutes := 0.0
	if cfg.RuleEnabled("timeout-too-long") {
		maxMinutes, _ = cfg.ruleOptionNumber("timeout-too-long", "max")
	}
	if !required && maxMinutes <= 0 {
		return nil
	}

	if n.WorkflowCall != nil {
		// Jobs calling a reusable workflow do not support timeout-minutes
		return nil
	}

	if n.TimeoutMinutes == nil {
		if required {
			rule.ReportID("missing-timeout", n.Pos, "\"timeout-minutes\" is not set at this job. Set it to avoid wasting runner minutes when the job hangs")
		}
		return nil
	}

	// The value is not known when it is an expression
	if n.TimeoutMinutes.Expression == nil && maxMinutes > 0 && n.TimeoutMinutes.Value > maxMinutes {
		rule.ReportIDf("timeout-too-long", n.TimeoutMinutes.Pos, "\"timeout-minutes\" is %v, which is greater than the maximum %v minutes allowed by the configuration", n.TimeoutMinutes.Value, maxMinutes)
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "missing-timeout", Group: RuleGroupPolicy, Summary: "A job does not set timeout-minutes.", DefaultLevel: SeverityError, Profile: ProfileStrict, DocsAnchor: "check-timeout-minutes"},
		RuleInfo{ID: "timeout-too-long", Group: RuleGroupPolicy, Summary: "timeout-minutes of a job exceeds the configured maximum.", DefaultLevel: SeverityError, DocsAnchor: "check-timeout-minutes", Options: []RuleOption{{Name: "max", Kind: RuleOptionNumber, Summary: "The maximum allowed timeout-minutes. The rule does nothing without it."}}},
	)
	registerRuleFactory("timeout-check", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleTimeoutCheck()}
	})
}
