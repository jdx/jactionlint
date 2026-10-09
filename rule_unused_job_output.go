package jactionlint

import (
	"sort"
)

// RuleUnusedJobOutput reports job outputs that nothing reads: no job reads them through the needs
// context and no output of a reusable workflow uses them through the jobs context.
type RuleUnusedJobOutput struct {
	RuleBase
}

// NewRuleUnusedJobOutput creates a new RuleUnusedJobOutput instance.
func NewRuleUnusedJobOutput() *RuleUnusedJobOutput {
	return &RuleUnusedJobOutput{
		RuleBase: RuleBase{
			name: "unused-job-output",
			desc: "Checks that every output of a job is read by another job or by a workflow_call output",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnusedJobOutput) VisitWorkflowPre(n *Workflow) error {
	if !rule.Config().RuleEnabled("unused-job-output") {
		return nil
	}
	any := false
	for _, j := range n.Jobs {
		if len(j.Outputs) > 0 {
			any = true
		}
	}
	if !any {
		return nil
	}
	rs := workflowRefs(n)
	if rs.unknown {
		return nil // a reference may be in the expression that does not parse
	}
	_, callable := n.FindWorkflowCallEvent()
	for _, id := range jobIDsInOrder(n) {
		j := n.Jobs[id]
		names := make([]string, 0, len(j.Outputs))
		for name := range j.Outputs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			o := j.Outputs[name]
			if o == nil || o.Name == nil {
				continue
			}
			if rs.reads("needs", id, "outputs", name) || rs.reads("jobs", id, "outputs", name) {
				continue
			}
			where := "no other job reads \"needs." + id + ".outputs." + name + "\""
			if callable {
				where += " and no output of the workflow uses it"
			}
			rule.ReportIDf("unused-job-output", o.Name.Pos, "output %q of job %q is never used: %s. remove it", o.Name.Value, j.ID.Value, where)
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "unused-job-output", Group: RuleGroupPolicy, Summary: "An output of a job is never read by another job or by a workflow_call output.", DefaultLevel: SeverityWarning, Profile: ProfileDefault, DocsAnchor: "check-unused-job-output"},
	)
	registerRuleFactory("unused-job-output", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleUnusedJobOutput()}
	})
}
