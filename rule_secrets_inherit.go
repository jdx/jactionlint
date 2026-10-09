package jactionlint

// RuleSecretsInherit is a rule to detect "secrets: inherit" at a job which calls a reusable
// workflow. It passes every secret of the caller to the called workflow, which breaks the principle
// of least privilege and makes it impossible to tell which secrets the called workflow receives.
// https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#jobsjob_idsecretsinherit
type RuleSecretsInherit struct {
	RuleBase
}

// NewRuleSecretsInherit creates a new RuleSecretsInherit instance.
func NewRuleSecretsInherit() *RuleSecretsInherit {
	return &RuleSecretsInherit{
		RuleBase: RuleBase{
			name: "secrets-inherit",
			desc: "Checks that reusable workflows are not called with \"secrets: inherit\"",
		},
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleSecretsInherit) VisitJobPre(n *Job) error {
	c := n.WorkflowCall
	if c == nil || !c.InheritSecrets {
		return nil
	}
	pos := c.InheritSecretsPos
	callee := ""
	if c.Uses != nil {
		callee = c.Uses.Value
		pos = c.Uses.Pos // Where the workflow which receives the secrets is named
	}
	if pos == nil {
		pos = n.Pos
	}
	rule.ReportIDf("secrets-inherit", pos, "\"secrets: inherit\" passes every secret of this workflow to the reusable workflow %q. list only the secrets it needs in a \"secrets:\" mapping, e.g. \"NAME: ${{ secrets.NAME }}\"", callee)
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "secrets-inherit", Group: RuleGroupSecurity, Summary: "A reusable workflow is called with secrets: inherit.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-secrets-inherit"},
	)
	registerRuleFactory("secrets-inherit", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("secrets-inherit") {
			return nil
		}
		return []Rule{NewRuleSecretsInherit()}
	})
}
