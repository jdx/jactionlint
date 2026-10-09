package jactionlint

import (
	"bytes"
	"regexp"
)

// RuleConcurrencyLimits is a rule to detect workflows without a concurrency limit. By default GitHub
// runs every instance of a workflow at once even when a newer run supersedes the older ones, which
// wastes runner minutes and can race on artifacts.
// https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#concurrency
type RuleConcurrencyLimits struct {
	RuleBase
	src []byte
}

// NewRuleConcurrencyLimits creates a new RuleConcurrencyLimits instance. The source is used to find
// where to report a workflow without any concurrency setting. It can be empty.
func NewRuleConcurrencyLimits(src []byte) *RuleConcurrencyLimits {
	return &RuleConcurrencyLimits{
		RuleBase: RuleBase{
			name: "concurrency-limits",
			desc: "Checks that workflows limit concurrent runs with \"concurrency:\"",
		},
		src: src,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleConcurrencyLimits) VisitWorkflowPre(n *Workflow) error {
	if c := n.Concurrency; c != nil {
		// Whether runs are cancelled is a choice: serializing a release pipeline is as valid as cancelling
		// the superseded runs of a test pipeline. Only the form which cannot cancel at all is reported.
		if c.Bare && !onlyWorkflowCall(n) {
			rule.ReportID("concurrency-limits", c.Pos, "\"concurrency:\" is only a group name, so it cannot cancel superseded runs. use the mapping form with \"group:\" and \"cancel-in-progress: true\"")
		}
		return nil
	}
	if onlyWorkflowCall(n) {
		// The caller decides how many runs of a reusable workflow exist
		return nil
	}
	// Jobs which call a reusable workflow are limited by that workflow. Every other job needs a limit,
	// at the workflow or on the job itself.
	hasJob := false
	limited := true
	for _, j := range n.Jobs {
		if j == nil || j.WorkflowCall != nil {
			continue
		}
		hasJob = true
		if j.Concurrency == nil {
			limited = false
		}
	}
	if !hasJob || limited {
		return nil
	}

	pos := &Pos{Line: 1, Col: 1}
	if line, ok := onKeyLine(rule.src); ok {
		pos = &Pos{Line: line, Col: 1}
	} else if line, ok := firstKeyLine(rule.src); ok {
		pos = &Pos{Line: line, Col: 1}
	}
	rule.ReportID("concurrency-limits", pos, "workflow has no \"concurrency:\", so every run of it executes at the same time even when a newer run supersedes the older ones. add a top-level \"concurrency:\" with a \"group:\" and \"cancel-in-progress: true\"")
	return nil
}

var onKeyRegexp = regexp.MustCompile(`^(?:on|"on"|'on')[ \t]*:`)

// onKeyLine returns the 1-based line of the top-level "on:" key.
func onKeyLine(src []byte) (int, bool) {
	for i, l := range bytes.Split(src, []byte("\n")) {
		if onKeyRegexp.Match(l) {
			return i + 1, true
		}
	}
	return 0, false
}

// onlyWorkflowCall reports whether the workflow can be started only by being called.
func onlyWorkflowCall(n *Workflow) bool {
	if len(n.On) == 0 {
		return false
	}
	for _, e := range n.On {
		if _, ok := e.(*WorkflowCallEvent); !ok {
			return false
		}
	}
	return true
}

func init() {
	registerRules(
		RuleInfo{ID: "concurrency-limits", Group: RuleGroupPolicy, Summary: "A workflow does not cancel superseded runs with concurrency:.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-concurrency-limits"},
	)
	registerRuleFactory("concurrency-limits", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("concurrency-limits") {
			return nil
		}
		return []Rule{NewRuleConcurrencyLimits(env.src)}
	})
}
