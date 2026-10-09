package jactionlint

import "strings"

// RuleUnsoundContains is a rule to detect conditions which use contains() with a string literal as
// the haystack. contains('refs/heads/main refs/heads/develop', github.ref) is also true for
// substrings such as a branch named "mai", so it cannot be used to check membership in a list.
// https://docs.github.com/en/actions/reference/workflows-and-actions/expressions#example-matching-an-array-of-strings
type RuleUnsoundContains struct {
	RuleBase
}

// NewRuleUnsoundContains creates a new RuleUnsoundContains instance.
func NewRuleUnsoundContains() *RuleUnsoundContains {
	return &RuleUnsoundContains{
		RuleBase: RuleBase{
			name: "unsound-contains",
			desc: "Checks conditions which use contains() on a string literal to test membership in a list",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnsoundContains) VisitWorkflowPre(n *Workflow) error {
	workflowExprSites(n, func(site exprSite) {
		if !site.Cond {
			return
		}
		scanExpressions(site.Str, true, func(o *exprOccurrence) {
			VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
				if !entering {
					return
				}
				call, ok := node.(*FuncCallNode)
				if !ok || !strings.EqualFold(call.Callee, "contains") || len(call.Args) != 2 {
					return
				}
				lit, ok := call.Args[0].(*StringNode)
				if !ok {
					return
				}
				if _, ok := call.Args[1].(*StringNode); ok {
					return // Both are constant
				}
				rule.ReportIDf("unsound-contains", o.PosOf(call), "contains() with the string literal %q as its first argument is true for any substring of it, not only for its words. to check membership in a list pass an array instead, e.g. contains(fromJSON('[\"a\", \"b\"]'), value), or compare each value with ==", lit.Value)
			})
		})
	})
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "unsound-contains", Group: RuleGroupSecurity, Summary: "A condition uses contains() on a string literal, which also matches substrings.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-unsound-contains"},
	)
	registerRuleFactory("unsound-contains", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("unsound-contains") {
			return nil
		}
		return []Rule{NewRuleUnsoundContains()}
	})
}
