package jactionlint

// RuleRequirePermissions is an opt-in rule checker which requires explicit `permissions:` configuration
// so that the GITHUB_TOKEN does not silently fall back to the repository/organization default (which
// may be read-write). It does nothing unless "require-permissions" is enabled in the config.
//
// A job is accepted when the workflow has a top-level `permissions:` or the job has its own. Note that
// `permissions: {}` is an explicit configuration and is accepted. Jobs calling reusable workflows
// are checked in the same way since the caller determines the maximum permissions of the callee.
type RuleRequirePermissions struct {
	RuleBase
	workflowHasPermissions bool
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
	if rule.enabled() && !rule.workflowHasPermissions {
		rule.fix = fixMissingPermissions(n)
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleRequirePermissions) VisitJobPre(n *Job) error {
	if !rule.enabled() || rule.workflowHasPermissions || n.Permissions != nil {
		return nil
	}
	rule.ReportIDf(
		"missing-permissions",
		n.Pos,
		"neither the workflow nor this job sets \"permissions:\" so the GITHUB_TOKEN gets the default permissions of the repository. set \"permissions:\" at workflow-level or job-level (use \"permissions: {}\" for no permissions) because the \"missing-permissions\" rule is enabled",
	)
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
