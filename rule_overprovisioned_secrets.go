package jactionlint

// RuleOverprovisionedSecrets is a rule to detect expressions which expose the whole "secrets" context
// instead of individual secrets, e.g. toJSON(secrets). Everything in the context is then passed to the
// runner, whether or not it is needed.
// https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions
type RuleOverprovisionedSecrets struct {
	RuleBase
}

// NewRuleOverprovisionedSecrets creates a new RuleOverprovisionedSecrets instance.
func NewRuleOverprovisionedSecrets() *RuleOverprovisionedSecrets {
	return &RuleOverprovisionedSecrets{
		RuleBase: RuleBase{
			name: "overprovisioned-secrets",
			desc: "Checks for expressions which use the whole \"secrets\" context",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleOverprovisionedSecrets) VisitWorkflowPre(n *Workflow) error {
	workflowExprs(n, func(_ exprSite, o *exprOccurrence) {
		VisitExprNode(o.Root, func(node, parent ExprNode, entering bool) {
			if !entering || !isContextVariable(node, "secrets") {
				return
			}
			switch p := parent.(type) {
			case *ObjectDerefNode:
				return // secrets.NAME
			case *IndexAccessNode:
				if p.Operand == node {
					if _, literal := p.Index.(*StringNode); literal {
						return // secrets['NAME']
					}
					rule.ReportID("overprovisioned-secrets", o.PosOf(node), "secrets are looked up by a computed name, which exposes the whole \"secrets\" context to the runner. reference each secret by its name, e.g. \"secrets.NAME\"")
					return
				}
			}
			rule.ReportID("overprovisioned-secrets", o.PosOf(node), "the whole \"secrets\" context is used, which exposes every secret to the runner even if only one is needed. reference each secret by its name, e.g. \"secrets.NAME\"")
		})
	})
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "overprovisioned-secrets", Group: RuleGroupSecurity, Summary: "An expression uses the whole secrets context.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-overprovisioned-secrets"},
	)
	registerRuleFactory("overprovisioned-secrets", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("overprovisioned-secrets") {
			return nil
		}
		return []Rule{NewRuleOverprovisionedSecrets()}
	})
}
