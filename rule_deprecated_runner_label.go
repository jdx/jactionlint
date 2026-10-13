package jactionlint

import (
	"fmt"
	"strings"
	"time"
)

// RuleDeprecatedRunnerLabel warns ahead of the retirement of a GitHub-hosted runner label (macos-14, ubuntu-22.04). The
// label still works until its day, so this is not a mistake yet; from that day on unknown-runner-label reports it as an
// error. The dates are in runnerLabelRetirements.
type RuleDeprecatedRunnerLabel struct {
	RuleBase
	now func() time.Time
}

// NewRuleDeprecatedRunnerLabel creates a new RuleDeprecatedRunnerLabel instance. now returns the current time; nil means
// time.Now.
func NewRuleDeprecatedRunnerLabel(now func() time.Time) *RuleDeprecatedRunnerLabel {
	if now == nil {
		now = time.Now
	}
	return &RuleDeprecatedRunnerLabel{
		RuleBase: RuleBase{
			name: "deprecated-runner-label",
			desc: "Checks for runner labels that GitHub is going to retire",
		},
		now: now,
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleDeprecatedRunnerLabel) VisitJobPre(n *Job) error {
	if n.RunsOn == nil {
		return nil
	}
	var m *Matrix
	if n.Strategy != nil {
		m = n.Strategy.Matrix
	}
	if runsOnSelfHosted(n.RunsOn, m) {
		return nil // the other labels are labels of the runner pool
	}
	if n.RunsOn.LabelsExpr != nil {
		for _, l := range matrixLabels(n.RunsOn.LabelsExpr, m) {
			rule.check(l)
		}
		return nil
	}
	for _, l := range n.RunsOn.Labels {
		if l.ContainsExpression() {
			for _, v := range matrixLabels(l, m) {
				rule.check(v)
			}
			continue
		}
		rule.check(l)
	}
	return nil
}

// runsOnSelfHosted reports whether the labels of runs-on have "self-hosted", also as a value of the matrix that one
// of several labels selects (`[${{ matrix.pool }}, macos-14]`). When the matrix value is all of runs-on, each value makes a
// job of its own, so "self-hosted" in it does not make the others self-hosted.
func runsOnSelfHosted(r *Runner, m *Matrix) bool {
	for _, l := range r.Labels {
		if strings.EqualFold(l.Value, "self-hosted") {
			return true
		}
	}
	if r.LabelsExpr != nil || len(r.Labels) < 2 {
		return false
	}
	for _, l := range r.Labels {
		for _, v := range matrixLabels(l, m) {
			if strings.EqualFold(v.Value, "self-hosted") {
				return true
			}
		}
	}
	return false
}

func (rule *RuleDeprecatedRunnerLabel) check(l *String) {
	r := runnerLabelRetirementOf(l.Value)
	if r == nil || r.isRetired(rule.now()) {
		return // a label which is gone is reported by unknown-runner-label
	}
	msg := fmt.Sprintf("runner label %q is deprecated: GitHub retires it on %s", l.Value, r.retired)
	if r.note != "" {
		msg += " and " + strings.TrimPrefix(r.note, "GitHub ")
	}
	msg += fmt.Sprintf(". use %s instead (%s)", r.useText(), r.source)
	rule.ReportID("deprecated-runner-label", l.Pos, msg)
}

func init() {
	registerRules(
		RuleInfo{
			ID: "deprecated-runner-label", Group: RuleGroupPolicy, Summary: "A runner label is deprecated and GitHub is going to retire it on a known date, such as macos-14.",
			DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-deprecated-runner-label",
		},
	)
	registerRuleFactory("deprecated-runner-label", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("deprecated-runner-label") {
			return nil
		}
		return []Rule{NewRuleDeprecatedRunnerLabel(env.Now)}
	})
}
