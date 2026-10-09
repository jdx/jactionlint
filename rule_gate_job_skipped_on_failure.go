package jactionlint

import (
	"strings"
)

// RuleGateJobSkippedOnFailure reports jobs that read the result of the jobs they need but are
// skipped by GitHub when one of them fails, so the check never sees a failure.
type RuleGateJobSkippedOnFailure struct {
	RuleBase
	jobs map[string]*Job
}

// NewRuleGateJobSkippedOnFailure creates a new RuleGateJobSkippedOnFailure instance.
func NewRuleGateJobSkippedOnFailure() *RuleGateJobSkippedOnFailure {
	return &RuleGateJobSkippedOnFailure{
		RuleBase: RuleBase{
			name: "gate-job-skipped-on-failure",
			desc: "Checks that a job which reads the result of the jobs it needs runs when they fail",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleGateJobSkippedOnFailure) VisitWorkflowPre(n *Workflow) error {
	rule.jobs = n.Jobs
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleGateJobSkippedOnFailure) VisitJobPre(n *Job) error {
	if len(n.Needs) == 0 || !rule.Config().RuleEnabled("gate-job-skipped-on-failure") {
		return nil
	}
	cond, ok := conditionExprs(n.If)
	if !ok || callsStatusFunction(cond, false) {
		return nil
	}
	// A job that needs a job with continue-on-error may see a failure as a success or the other way
	// round. Do not judge those.
	for _, need := range n.Needs {
		if j, ok := rule.jobs[strings.ToLower(need.Value)]; ok && j.ContinueOnError != nil && (j.ContinueOnError.Expression != nil || j.ContinueOnError.Value) {
			return nil
		}
	}

	rs := jobRefs(n)
	if rs.unknown {
		return nil
	}
	// A gate reports what happened to the jobs it needs, so it reads their results in its steps or in the values it
	// passes on. A job that only tests the results in its own "if:" is a job that runs when they went well (a publish
	// or a cleanup that needs `result == 'success' || result == 'skipped'`): GitHub skipping it on a failure is what its
	// author wants, so only a condition that expects a failure or a cancellation is a gate.
	body := *n
	body.If = nil
	read := firstResultRead(jobRefs(&body).refs, false)
	if read == "" && n.If != nil {
		var ifRefs []exprRef
		for _, e := range cond {
			collectExprRefs(e, nil, &ifRefs)
		}
		read = firstResultRead(ifRefs, true)
	}
	if read == "" {
		return nil
	}
	pos := n.Pos
	if n.If != nil && n.If.Pos != nil {
		pos = n.If.Pos
	}
	if n.If != nil {
		rule.ReportIDf(
			"gate-job-skipped-on-failure",
			pos,
			"job %q reads %q but its \"if\" has no status check function, so GitHub skips the job when a job it needs fails or is skipped, and a skipped job counts as passing for a required check. add \"!cancelled()\" or \"always()\" to the condition",
			n.ID.Value, read,
		)
		return nil
	}
	rule.ReportIDf(
		"gate-job-skipped-on-failure",
		pos,
		"job %q reads %q but has no \"if\" with a status check function, so GitHub skips the job when a job it needs fails or is skipped, and a skipped job counts as passing for a required check. add \"if: ${{ !cancelled() }}\" (or \"always()\") to the job",
		n.ID.Value, read,
	)
	return nil
}

// firstResultRead returns the first reference to the result of a needed job that expects to see a failure, "" if
// there is none. In a job condition (inCondition) a comparison that selects the jobs which went well is not one.
func firstResultRead(refs []exprRef, inCondition bool) string {
	for _, r := range refs {
		if !refCovers(r.chain, []string{"needs", "*", "result"}) && !refCovers(r.chain, []string{"needs", "*", "outcome"}) {
			continue
		}
		if isSuccessComparison(r) || inCondition && selectsGoodResult(r) {
			continue
		}
		return r.String()
	}
	return ""
}

// selectsGoodResult reports whether the reference is compared with == to "success" or "skipped", or with != to
// "skipped", "failure" or "cancelled": the comparisons of a job that runs after its needs went well.
func selectsGoodResult(r exprRef) bool {
	c, ok := r.parent.(*CompareOpNode)
	if !ok || r.negated || !c.Kind.IsEqualityOp() {
		return false
	}
	other := c.Right
	if c.Right == r.node {
		other = c.Left
	}
	s, ok := other.(*StringNode)
	if !ok {
		return false
	}
	v := strings.ToLower(s.Value)
	if c.Kind == CompareOpNodeKindEq {
		return v == "success" || v == "skipped"
	}
	return v == "skipped" || v == "failure" || v == "cancelled"
}

// isSuccessComparison reports whether the reference is only compared with 'success' with ==. Such a
// check is redundant in a job that is skipped unless its needs succeeded, but it does not expect
// to see a failure.
func isSuccessComparison(r exprRef) bool {
	c, ok := r.parent.(*CompareOpNode)
	if !ok || r.negated || c.Kind != CompareOpNodeKindEq {
		return false
	}
	other := c.Right
	if c.Right == r.node {
		other = c.Left
	}
	s, ok := other.(*StringNode)
	return ok && strings.EqualFold(s.Value, "success")
}

func init() {
	registerRules(
		RuleInfo{ID: "gate-job-skipped-on-failure", Group: RuleGroupCorrectness, Summary: "A job that reads the results of the jobs it needs is skipped when one of them fails.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-gate-job-skipped-on-failure"},
	)
	registerRuleFactory("gate-job-skipped-on-failure", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleGateJobSkippedOnFailure()}
	})
}
