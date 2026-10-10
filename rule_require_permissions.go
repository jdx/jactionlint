package jactionlint

// RuleRequirePermissions is an opt-in rule checker which requires explicit `permissions:` configuration
// so that the GITHUB_TOKEN does not silently fall back to the repository/organization default (which
// may be read-write). It does nothing unless "require-permissions" is enabled in the config.
//
// A job is accepted when the workflow has a top-level `permissions:` or the job has its own. Note that
// `permissions: {}` is an explicit configuration and is accepted. Jobs calling reusable workflows
// are checked in the same way since the caller determines the maximum permissions of the callee.
//
// A workflow which runs only on workflow_call (a reusable workflow that nothing else triggers) is not
// checked: the caller decides what the token can do, and a callee that asks for more than its caller
// grants is rejected, so no value written in the callee is right for every caller. A workflow which
// has workflow_call and another event is also run directly, so it is still checked, but its finding has
// no fix (see fixMissingPermissions) and the message says so.
type RuleRequirePermissions struct {
	RuleBase
	workflowHasPermissions bool
	// reusableOnly is true when workflow_call is the only event of the workflow.
	reusableOnly bool
	// alsoReusable is true when the workflow has workflow_call and another event.
	alsoReusable bool
	// fix is the fix adding "permissions:" to the workflow. It is nil when it cannot be made.
	fix *Fix
}

// NewRuleRequirePermissions creates new RuleRequirePermissions instance.
func NewRuleRequirePermissions() *RuleRequirePermissions {
	return &RuleRequirePermissions{
		RuleBase: RuleBase{
			name: "require-permissions",
			desc: "Checks that \"permissions:\" is explicitly set at workflow-level or job-level. This rule is opt-in and enabled by \"require-permissions\" in the config",
		},
	}
}

func (rule *RuleRequirePermissions) enabled() bool {
	return rule.Config().RuleEnabled("missing-permissions")
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleRequirePermissions) VisitWorkflowPre(n *Workflow) error {
	rule.workflowHasPermissions = n.Permissions != nil
	rule.fix = nil
	rule.reusableOnly, rule.alsoReusable = false, false
	if _, ok := n.FindWorkflowCallEvent(); ok {
		rule.reusableOnly = len(n.On) == 1
		rule.alsoReusable = !rule.reusableOnly
	}
	if rule.enabled() && !rule.workflowHasPermissions {
		rule.fix = fixMissingPermissions(n)
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleRequirePermissions) VisitJobPre(n *Job) error {
	if !rule.enabled() || rule.reusableOnly || rule.workflowHasPermissions || n.Permissions != nil {
		return nil
	}
	msg := "neither the workflow nor this job sets \"permissions:\" so the GITHUB_TOKEN gets the default permissions of the repository. set \"permissions:\" at workflow-level or job-level (use \"permissions: {}\" for no permissions) because the \"missing-permissions\" rule is enabled"
	if rule.alsoReusable {
		msg += ". this workflow is also called as a reusable workflow (workflow_call), so the permissions you set must not be more than its callers grant, and there is no automatic fix because the callers are not known here"
	}
	rule.ReportIDf("missing-permissions", n.Pos, "%s", msg)
	// Every job reports the same fix: it is applied once and the findings stay fixable together
	rule.Errs()[len(rule.Errs())-1].Fix = rule.fix
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "missing-permissions", Group: RuleGroupPolicy, Summary: "Neither the workflow nor the job sets permissions:.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "permissions"},
	)
	registerRuleFactory("require-permissions", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleRequirePermissions()}
	})
}
