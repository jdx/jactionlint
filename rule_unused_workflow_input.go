package jactionlint

import (
	"sort"
	"strings"
)

// RuleUnusedWorkflowInput reports inputs of workflow_dispatch and workflow_call that the workflow
// never reads.
type RuleUnusedWorkflowInput struct {
	RuleBase
}

// NewRuleUnusedWorkflowInput creates a new RuleUnusedWorkflowInput instance.
func NewRuleUnusedWorkflowInput() *RuleUnusedWorkflowInput {
	return &RuleUnusedWorkflowInput{
		RuleBase: RuleBase{
			name: "unused-workflow-input",
			desc: "Checks that every input of workflow_dispatch and workflow_call is used",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnusedWorkflowInput) VisitWorkflowPre(n *Workflow) error {
	if !rule.Config().RuleEnabled("unused-workflow-input") {
		return nil
	}
	var dispatch *WorkflowDispatchEvent
	var call *WorkflowCallEvent
	for _, e := range n.On {
		switch e := e.(type) {
		case *WorkflowDispatchEvent:
			dispatch = e
		case *WorkflowCallEvent:
			call = e
		}
	}
	if (dispatch == nil || len(dispatch.Inputs) == 0) && (call == nil || len(call.Inputs) == 0) {
		return nil
	}
	rs := workflowRefs(n)
	if rs.unknown {
		return nil
	}
	// A script can read the inputs of a manual run from the event payload
	readsPayload := false
	walkStrings(n, func(s *String) {
		if strings.Contains(s.Value, "GITHUB_EVENT_PATH") {
			readsPayload = true
		}
	})

	if dispatch != nil && !readsPayload {
		names := make([]string, 0, len(dispatch.Inputs))
		for name := range dispatch.Inputs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			in := dispatch.Inputs[name]
			if in == nil || in.Name == nil {
				continue
			}
			if rs.reads("inputs", name) || rs.reads("github", "event", "inputs", name) {
				continue
			}
			rule.ReportIDf("unused-workflow-input", in.Name.Pos, "input %q of \"workflow_dispatch\" is never used: no expression reads \"inputs.%s\". remove it or use it", in.Name.Value, name)
		}
	}
	if call != nil {
		for _, in := range call.Inputs {
			if in == nil || in.Name == nil || rs.reads("inputs", in.ID) {
				continue
			}
			rule.ReportIDf("unused-workflow-input", in.Name.Pos, "input %q of \"workflow_call\" is never used: no expression reads \"inputs.%s\". remove it from the workflow and from the callers, or use it", in.Name.Value, in.ID)
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "unused-workflow-input", Group: RuleGroupPolicy, Summary: "An input of workflow_dispatch or workflow_call is never used.", DefaultLevel: SeverityWarning, Profile: ProfileStrict, DocsAnchor: "check-unused-workflow-input"},
	)
	registerRuleFactory("unused-workflow-input", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleUnusedWorkflowInput()}
	})
}
