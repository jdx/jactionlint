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
