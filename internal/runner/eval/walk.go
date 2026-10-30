package eval

import (
	"strings"

	"github.com/jdx/jactionlint/v2"
)

func hasStatusCall(n jactionlint.ExprNode) bool {
	found := false
	jactionlint.VisitExprNode(n, func(n, _ jactionlint.ExprNode, entering bool) {
		if f, ok := n.(*jactionlint.FuncCallNode); ok && entering {
			switch strings.ToLower(f.Callee) {
			case "success", "failure", "always", "cancelled":
				found = true
			}
		}
	})
	return found
}

// Contexts returns the names of the contexts (lower case) that the expression reads.
func Contexts(expr string) ([]string, error) {
	n, err := Parse(expr)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	jactionlint.VisitExprNode(n, func(n, _ jactionlint.ExprNode, entering bool) {
		if v, ok := n.(*jactionlint.VariableNode); ok && entering && !seen[strings.ToLower(v.Name)] {
			seen[strings.ToLower(v.Name)] = true
			out = append(out, strings.ToLower(v.Name))
		}
	})
	return out, nil
}

// StringContexts returns the contexts read by every ${{ }} of the string.
func StringContexts(s string) ([]string, error) {
	segs, err := split(s)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, sg := range segs {
		if !sg.expr {
			continue
		}
		cs, err := Contexts(sg.text)
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// Refs returns the names that the expressions of the string read directly from a context, such as the NAME of
// secrets.NAME or vars['NAME'], in lower case.
func Refs(s, context string) []string {
	segs, err := split(s)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		n = strings.ToLower(n)
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, sg := range segs {
		if !sg.expr {
			continue
		}
		n, err := Parse(sg.text)
		if err != nil {
			continue
		}
		jactionlint.VisitExprNode(n, func(n, _ jactionlint.ExprNode, entering bool) {
			if !entering {
				return
			}
			isCtx := func(r jactionlint.ExprNode) bool {
				v, ok := r.(*jactionlint.VariableNode)
				return ok && strings.EqualFold(v.Name, context)
			}
			switch n := n.(type) {
			case *jactionlint.ObjectDerefNode:
				if isCtx(n.Receiver) {
					add(n.Property)
				}
			case *jactionlint.IndexAccessNode:
				if s, ok := n.Index.(*jactionlint.StringNode); ok && isCtx(n.Operand) {
					add(s.Value)
				}
			}
		})
	}
	return out
}
