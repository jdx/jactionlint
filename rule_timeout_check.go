package jactionlint

import (
	"fmt"
	"strings"
)

// RuleTimeoutCheck is a rule to check "timeout-minutes" of jobs. It is opt-in and configured by the
// "timeout-minutes" section of the configuration file.
// https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#jobsjob_idtimeout-minutes
type RuleTimeoutCheck struct {
	RuleBase
	src *srcDoc
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

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleTimeoutCheck) VisitWorkflowPre(n *Workflow) error {
	rule.src = newSrcDoc(n.Source)
	return nil
}

// fixMinutes is the timeout-minutes to add to a job. There is deliberately no built-in number: the
// fix is only offered when "default-minutes" of the missing-timeout rule is configured. The value is
// lowered to the maximum of timeout-too-long, when that is set, so that the fix does not cause a
// timeout-too-long finding.
func (rule *RuleTimeoutCheck) fixMinutes() (int, bool) {
	cfg := rule.Config()
	v, ok := cfg.ruleOptionNumber("missing-timeout", "default-minutes")
	if !ok || v < 1 {
		return 0, false
	}
	minutes := int(v)
	if cfg.RuleEnabled("timeout-too-long") {
		if max, ok := cfg.ruleOptionNumber("timeout-too-long", "max"); ok && max >= 1 && float64(minutes) > max {
			minutes = int(max)
		}
	}
	return minutes, true
}

// fixMissing makes the fix adding "timeout-minutes" to the job. It returns the reason instead when
// "default-minutes" is not usable or the job is not written in the block style.
func (rule *RuleTimeoutCheck) fixMissing(n *Job) (*Fix, *NoFix) {
	minutes, ok := rule.fixMinutes()
	if !ok {
		if _, set := rule.Config().ruleOptionNumber("missing-timeout", "default-minutes"); set {
			return nil, &NoFix{Code: NoFixOptionInvalid, Option: "missing-timeout.default-minutes", Reason: "the missing-timeout option default-minutes must be at least 1 for the fix to add timeout-minutes"}
		}
		return nil, &NoFix{Code: NoFixOptionRequired, Option: "missing-timeout.default-minutes", Reason: "set the missing-timeout option default-minutes to enable the fix, jactionlint does not choose a timeout"}
	}
	if rule.src == nil {
		return nil, nil
	}
	site, ok := rule.src.locateJob(n)
	if !ok {
		return nil, &NoFix{Code: NoFixUnsupportedShape, Reason: "the job is not written in the block style, so timeout-minutes cannot be added to it"}
	}
	return &Fix{
		Description: fmt.Sprintf("Add timeout-minutes: %d", minutes),
		Edits:       []TextEdit{rule.src.insertAfterLine(site.after, fmt.Sprintf("%stimeout-minutes: %d", strings.Repeat(" ", site.bodyIndent), minutes))},
	}, nil
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
			last := rule.Errs()[len(rule.Errs())-1]
			last.Fix, last.NoFix = rule.fixMissing(n)
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
		RuleInfo{ID: "missing-timeout", Group: RuleGroupPolicy, Summary: "A job does not set timeout-minutes.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-timeout-minutes", Options: []RuleOption{{Name: "default-minutes", Kind: RuleOptionInt, Summary: "The timeout-minutes which --fix adds to a job. There is no default: the rule has no fix unless this is set. It is lowered to the max of timeout-too-long when that is smaller."}}},
		RuleInfo{ID: "timeout-too-long", Group: RuleGroupPolicy, Summary: "timeout-minutes of a job exceeds the configured maximum.", DefaultLevel: SeverityError, DocsAnchor: "check-timeout-minutes", Options: []RuleOption{{Name: "max", Kind: RuleOptionNumber, Summary: "The maximum allowed timeout-minutes. The rule does nothing without it."}}},
	)
	registerRuleFactory("timeout-check", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleTimeoutCheck()}
	})
}
