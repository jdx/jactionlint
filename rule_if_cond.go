package jactionlint

import (
	"strings"
)

// RuleIfCond is a rule to check if: conditions.
type RuleIfCond struct {
	RuleBase
}

// NewRuleIfCond creates new RuleIfCond instance.
func NewRuleIfCond() *RuleIfCond {
	return &RuleIfCond{
		RuleBase: RuleBase{
			name: "if-cond",
			desc: "Checks for if: conditions which are always true/false",
		},
	}
}

// VisitStep is callback when visiting Step node.
func (rule *RuleIfCond) VisitStep(n *Step) error {
	rule.checkIfCond(n.If)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleIfCond) VisitJobPre(n *Job) error {
	rule.checkIfCond(n.If)
	if n.Snapshot != nil {
		rule.checkIfCond(n.Snapshot.If)
	}
	return nil
}

func (rule *RuleIfCond) checkIfCond(n *String) {
	if n == nil {
		return
	}
	s, e := strings.Index(n.Value, "${{"), strings.Index(n.Value, "}}")
	if s >= 0 && e >= 0 {
		rule.checkPlaceholder(n, s, e)
	} else {
		if rule.Config().RuleEnabled("require-expression-wrapping") && strings.TrimSpace(n.Value) != "" {
			rule.ReportIDf("require-expression-wrapping", n.Pos, "if: condition %q must be wrapped in ${{ }} because \"require-expression-wrapping\" is enabled", n.Value)
		}
		rule.checkBare(n)
	}
}

func (rule *RuleIfCond) checkPlaceholder(n *String, start, end int) {
	// Check number of ${{ }} for conditions like `${{ false }} || ${{ true }}` which are always evaluated to true.
	// Newlines at the end are not extra characters: a block scalar (`if: |`) ends with one and GitHub trims it
	// (changelog 2026-01-29), which the runs of a job with `if: |` and a false expression confirm.
	if start > 0 || end+len("}}") < len(strings.TrimRight(n.Value, "\r\n")) || strings.Count(n.Value, "${{") > 1 {
		rule.ReportIDf(
			"if-always-true",
			n.Pos,
			"if: condition %q is always evaluated to true because extra characters are around ${{ }}",
			n.Value,
		)
		return
	}
	// The string has parsed its expression for the other rules. It is the text which is checked here
	// when it ends at the first "}}"; a "}}" inside a string literal ends the text here earlier.
	if c := n.placeholders(); !c.failed && len(c.list) == 1 && c.list[0].after == start+len("${{") && c.list[0].after+c.list[0].n == end+len("}}") {
		rule.checkConstant(n.Pos, strings.TrimSpace(n.Value[start+len("${{"):end]), c.list[0].node)
		return
	}
	rule.checkExpression(n.Pos, n.Value[start+len("${{"):end])
}

// checkBare checks an "if:" condition without ${{ }}.
func (rule *RuleIfCond) checkBare(n *String) {
	if strings.TrimSpace(n.Value) == n.Value {
		// Nothing is trimmed, so the cached tree is the one of the text.
		if e, err := n.condExpr(); err == nil {
			rule.checkConstant(n.Pos, strings.TrimSpace(n.Value), e)
		}
		return
	}
	rule.checkExpression(n.Pos, n.Value)
}

func (rule *RuleIfCond) checkExpression(pos *Pos, input string) {
	i := strings.TrimSpace(input)
	l := NewExprLexer(i + "}}")
	if e, err := NewExprParser().Parse(l); err == nil {
		rule.checkConstant(pos, i, e)
	}
}

func (rule *RuleIfCond) checkConstant(pos *Pos, text string, e ExprNode) {
	if NewExprSemanticsChecker(false, nil, nil).IsConstant(e) {
		rule.ReportIDf("constant-condition", pos, "constant expression %q in condition. remove the if: section", text)
	}
}

func init() {
	registerRules(
		RuleInfo{ID: "constant-condition", Group: RuleGroupCorrectness, Summary: "An if: condition is a constant expression.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "if-cond-constant"},
		RuleInfo{ID: "if-always-true", Group: RuleGroupCorrectness, Summary: "An if: condition is always true because of the characters around ${{ }}.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "if-cond-constant"},
		RuleInfo{ID: "require-expression-wrapping", Group: RuleGroupStyle, Summary: "An if: condition is not wrapped in ${{ }}.", DefaultLevel: SeverityError, Profile: ProfilePedantic, DocsAnchor: "check-require-expression-wrapping"},
	)
	registerRuleFactory("if-cond", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleIfCond()}
	})
}
