package jactionlint

import (
	"strings"
)

// RuleUnusedNeeds reports entries of `needs` that do nothing: the job never reads the outputs or the
// result of the needed job and another needed job already depends on it, so the entry changes
// neither the order nor whether the job runs.
//
// An entry that is only there for the order is not reported, because the rule cannot tell it from
// an entry that was forgotten.
type RuleUnusedNeeds struct {
	RuleBase
	jobs map[string]*Job
}

// NewRuleUnusedNeeds creates a new RuleUnusedNeeds instance.
func NewRuleUnusedNeeds() *RuleUnusedNeeds {
	return &RuleUnusedNeeds{
		RuleBase: RuleBase{
			name: "unused-needs",
			desc: "Checks for needs entries that are neither read nor needed for the order of jobs",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnusedNeeds) VisitWorkflowPre(n *Workflow) error {
	rule.jobs = n.Jobs
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUnusedNeeds) VisitJobPre(n *Job) error {
	if len(n.Needs) < 2 || !rule.Config().RuleEnabled("unused-needs") {
		return nil
	}
	rs := jobRefs(n)
	if rs.unknown {
		return nil
	}
	for _, need := range n.Needs {
		a := strings.ToLower(need.Value)
		if a == "" || rs.reads("needs", a) {
			continue
		}
		for _, other := range n.Needs {
			b := strings.ToLower(other.Value)
			if b == a || b == "" {
				continue
			}
			if rule.impliesSuccess(b, a, map[string]bool{}) {
				rule.ReportIDf(
					"unused-needs",
					need.Pos,
					"job %q needs %q but never reads its outputs or result, and it already needs %q which waits for %q. this entry changes nothing and can be removed",
					n.ID.Value, need.Value, other.Value, need.Value,
				)
				break
			}
		}
	}
	return nil
}

// impliesSuccess reports whether the job b only runs after the job a succeeded: a is a need of b,
// directly or through jobs that run only after their needs succeeded.
func (rule *RuleUnusedNeeds) impliesSuccess(b, a string, seen map[string]bool) bool {
	if seen[b] {
		return false
	}
	seen[b] = true
	j, ok := rule.jobs[b]
	if !ok {
		return false
	}
	cond, ok := conditionExprs(j.If)
	if !ok || callsStatusFunction(cond, false) {
		return false // the job runs even if its needs failed
	}
	for _, n := range j.Needs {
		id := strings.ToLower(n.Value)
		if id == a || rule.impliesSuccess(id, a, seen) {
			return true
		}
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "unused-needs", Group: RuleGroupStyle, Summary: "A needs entry is neither read by the job nor needed for the order of jobs.", DefaultLevel: SeverityInfo, Profile: ProfileStrict, DocsAnchor: "check-unused-needs"},
	)
	registerRuleFactory("unused-needs", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleUnusedNeeds()}
	})
}
