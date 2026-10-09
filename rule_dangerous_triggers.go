package jactionlint

// dangerousTriggers are the events which run with the privileges of the target repository (a write
// token and secrets) while a fork or any commenter controls part of the event.
var dangerousTriggers = map[string]string{
	"pull_request_target": "runs in the context of the base repository with a write token and secrets even when the pull request comes from a fork. use \"pull_request\" unless write access is required, and never check out or run code of the pull request",
	"workflow_run":        "runs in the context of the default branch with a write token and secrets, but processes artifacts and metadata produced by a run that a fork may control. consider a reusable workflow called with \"workflow_call\" instead, and never trust the artifacts or the event payload",
	"issue_comment":       "runs in the context of the default branch with a write token and secrets and can be started by anyone who can comment. check the permission of the commenter and never run code of the pull request",
}

// RuleDangerousTriggers is a rule to detect workflow triggers which are difficult to use securely:
// "pull_request_target", "workflow_run" and "issue_comment".
// https://securitylab.github.com/resources/github-actions-preventing-pwn-requests/
type RuleDangerousTriggers struct {
	RuleBase
}

// NewRuleDangerousTriggers creates a new RuleDangerousTriggers instance.
func NewRuleDangerousTriggers() *RuleDangerousTriggers {
	return &RuleDangerousTriggers{
		RuleBase: RuleBase{
			name: "dangerous-triggers",
			desc: "Checks for workflow triggers which are difficult to use securely",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleDangerousTriggers) VisitWorkflowPre(n *Workflow) error {
	for _, e := range n.On {
		w, ok := e.(*WebhookEvent)
		if !ok || w.Hook == nil {
			continue
		}
		if w.Hook.Value == "pull_request_target" && onlyRunsLabeler(n) {
			continue
		}
		if why, ok := dangerousTriggers[w.Hook.Value]; ok {
			rule.ReportIDf("dangerous-triggers", w.Hook.Pos, "trigger %q is dangerous: it %s", w.Hook.Value, why)
		}
	}
	return nil
}

// onlyRunsLabeler reports whether every step of the workflow is actions/labeler, which labels pull
// requests by the paths they change and never runs code from the pull request. That is the one use of
// pull_request_target which is accepted as safe.
func onlyRunsLabeler(n *Workflow) bool {
	steps := 0
	only := true
	for _, j := range n.Jobs {
		if j == nil {
			continue
		}
		if j.WorkflowCall != nil {
			return false
		}
		walkSteps(j.Steps, func(s *Step) {
			steps++
			a, ok := s.Exec.(*ExecAction)
			if !ok || a == nil || a.Uses == nil || ParseUses(a.Uses.Value).CanonicalName() != "actions/labeler" {
				only = false
			}
		})
	}
	return only && steps > 0
}

func init() {
	registerRules(
		RuleInfo{ID: "dangerous-triggers", Group: RuleGroupSecurity, Summary: "A workflow uses pull_request_target, workflow_run or issue_comment.", DefaultLevel: SeverityWarning, Profile: ProfileStrict, DocsAnchor: "check-dangerous-triggers"},
	)
	registerRuleFactory("dangerous-triggers", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("dangerous-triggers") {
			return nil
		}
		return []Rule{NewRuleDangerousTriggers()}
	})
}
