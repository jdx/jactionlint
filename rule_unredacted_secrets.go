package jactionlint

import "strings"

// RuleUnredactedSecrets is a rule to detect secrets which are parsed as structured data with
// fromJSON(), e.g. fromJSON(secrets.CREDENTIALS).password. The runner redacts the exact value of a
// secret from the logs, but not the fields which are extracted from it.
// https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions#redacting-secrets
type RuleUnredactedSecrets struct {
	RuleBase
}

// NewRuleUnredactedSecrets creates a new RuleUnredactedSecrets instance.
func NewRuleUnredactedSecrets() *RuleUnredactedSecrets {
	return &RuleUnredactedSecrets{
		RuleBase: RuleBase{
			name: "unredacted-secrets",
			desc: "Checks for secrets which are parsed as JSON",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnredactedSecrets) VisitWorkflowPre(n *Workflow) error {
	workflowExprs(n, func(_ exprSite, o *exprOccurrence) {
		VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
			if !entering {
				return
			}
			call, ok := node.(*FuncCallNode)
			if !ok || !strings.EqualFold(call.Callee, "fromjson") || len(call.Args) != 1 {
				return
			}
			name, ok := firstSecretIn(call.Args[0])
			if !ok {
				return
			}
			rule.ReportIDf("unredacted-secrets", o.PosOf(call), "secret %q is parsed with fromJSON(), and the runner does not redact the fields of a parsed secret from the logs. store each field in its own secret and reference it by name", name)
		})
	})
	return nil
}

// firstSecretIn returns the name of the first secret referenced in the expression. A nested fromJSON()
// call is not looked into, since it is reported for its own secret.
func firstSecretIn(root ExprNode) (string, bool) {
	var name string
	found := false
	nested := 0
	isFromJSON := func(n ExprNode) bool {
		c, ok := n.(*FuncCallNode)
		return ok && strings.EqualFold(c.Callee, "fromjson")
	}
	VisitExprNode(root, func(node, _ ExprNode, entering bool) {
		if isFromJSON(node) {
			if entering {
				nested++
			} else {
				nested--
			}
			return
		}
		if found || !entering || nested > 0 {
			return
		}
		if s, ok := secretNameOf(node); ok {
			name, found = strings.ToUpper(s), true
		}
	})
	return name, found
}

func init() {
	registerRules(
		RuleInfo{ID: "unredacted-secrets", Group: RuleGroupSecurity, Summary: "A secret is parsed with fromJSON(), so the fields of it are not redacted in logs.", DefaultLevel: SeverityWarning, Profile: ProfileStrict, DocsAnchor: "check-unredacted-secrets"},
	)
	registerRuleFactory("unredacted-secrets", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("unredacted-secrets") {
			return nil
		}
		return []Rule{NewRuleUnredactedSecrets()}
	})
}
