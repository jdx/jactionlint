package jactionlint

import (
	"sort"
)

// RuleUnusedWorkflowSecret reports secrets that a reusable workflow declares in "on.workflow_call.secrets" and never
// reads: the sibling of unused-workflow-input. Every caller has to pass a required secret, and an optional one is
// passed for nothing, so an unused one hands a credential to a workflow that does not need it.
type RuleUnusedWorkflowSecret struct {
	RuleBase
}

// NewRuleUnusedWorkflowSecret creates a new RuleUnusedWorkflowSecret instance.
func NewRuleUnusedWorkflowSecret() *RuleUnusedWorkflowSecret {
	return &RuleUnusedWorkflowSecret{
		RuleBase: RuleBase{
			name: "unused-workflow-secret",
			desc: "Checks that every secret of workflow_call is used",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnusedWorkflowSecret) VisitWorkflowPre(n *Workflow) error {
	if !rule.Config().RuleEnabled("unused-workflow-secret") {
		return nil
	}
	var call *WorkflowCallEvent
	for _, e := range n.On {
		if e, ok := e.(*WorkflowCallEvent); ok {
			call = e
		}
	}
	if call == nil || len(call.Secrets) == 0 {
		return nil
	}
	// "secrets: inherit" of a job which calls another workflow passes every secret on
	for _, j := range n.Jobs {
		if j != nil && j.WorkflowCall != nil && j.WorkflowCall.InheritSecrets {
			return nil
		}
	}
	rs := workflowRefs(n)
	if rs.unknown {
		return nil
	}
	names := make([]string, 0, len(call.Secrets))
	for name := range call.Secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := call.Secrets[name]
		if s == nil || s.Name == nil || rs.reads("secrets", name) {
			continue
		}
		rule.ReportIDf("unused-workflow-secret", s.Name.Pos, "secret %q of \"workflow_call\" is never used: no expression reads \"secrets.%s\". remove it from the workflow and from the callers, so that nothing passes a credential which is not needed, or use it", s.Name.Value, name)
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "unused-workflow-secret", Group: RuleGroupPolicy, Summary: "A secret of workflow_call is never used.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-unused-workflow-secret"},
	)
	registerRuleFactory("unused-workflow-secret", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleUnusedWorkflowSecret()}
	})
}
