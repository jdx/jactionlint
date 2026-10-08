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
	if cfg == nil || (!cfg.TimeoutMinutes.Required && cfg.TimeoutMinutes.Max <= 0) {
		return nil
	}

	if n.WorkflowCall != nil {
		// Jobs calling a reusable workflow do not support timeout-minutes
		return nil
	}

	if n.TimeoutMinutes == nil {
		if cfg.TimeoutMinutes.Required {
			rule.ReportID("missing-timeout", n.Pos, "\"timeout-minutes\" is not set at this job. Set it to avoid wasting runner minutes when the job hangs")
		}
		return nil
	}

	// The value is not known when it is an expression
	if n.TimeoutMinutes.Expression == nil && cfg.TimeoutMinutes.Max > 0 && n.TimeoutMinutes.Value > cfg.TimeoutMinutes.Max {
		rule.ReportIDf("timeout-too-long", n.TimeoutMinutes.Pos, "\"timeout-minutes\" is %v, which is greater than the maximum %v minutes allowed by the configuration", n.TimeoutMinutes.Value, cfg.TimeoutMinutes.Max)
	}
	return nil
}
