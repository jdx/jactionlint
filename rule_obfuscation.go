package jactionlint

import (
	"strconv"
	"strings"
)

// RuleObfuscation is a rule to find obfuscated usages of GitHub Actions features, which work but are
// written in a way that makes other tools (and readers) miss what they do. It checks the path of
// uses: here and the expressions in RuleExpression.checkObfuscation.
type RuleObfuscation struct {
	RuleBase
	src *sourceIndex
}

// NewRuleObfuscation creates a new RuleObfuscation instance. The source of the file lets the rule
// attach fixes to its findings. It can be nil.
func NewRuleObfuscation(src []byte) *RuleObfuscation {
	r := &RuleObfuscation{
		RuleBase: RuleBase{
			name: "obfuscation",
			desc: "Checks for obfuscated paths at \"uses:\"",
		},
	}
	if src != nil {
		r.src = newSourceIndex(src)
	}
	return r
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleObfuscation) VisitJobPre(n *Job) error {
	if n.WorkflowCall != nil {
		rule.checkUses(n.WorkflowCall.Uses)
	}
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleObfuscation) VisitStep(n *Step) error {
	if e, ok := n.Exec.(*ExecAction); ok {
		rule.checkUses(e.Uses)
	}
	return nil
}

func (rule *RuleObfuscation) checkUses(s *String) {
	if s == nil {
		return
	}
	path, clean, ok := obfuscatedUsesPath(s.Value)
	if !ok {
		return
	}
	if clean == "" {
		rule.ReportIDf("obfuscation", s.Pos, "path of %q has redundant or empty segments (\"//\", \".\" or \"..\"). write the path in its plain form", s.Value)
		return
	}
	rule.ReportIDf("obfuscation", s.Pos, "path of %q has redundant or empty segments (\"//\", \".\" or \"..\"). write it as %q", s.Value, clean+s.Value[len(path):])
	// GitHub resolves the path on its own, so the plain form is expected but not guaranteed to be
	// the same reference. That is why the fix is unsafe.
	if rule.src == nil {
		return
	}
	off, ok := rule.src.valueOffset(s, 0)
	if !ok || !rule.src.matches(off, path) {
		return
	}
	rule.errs[len(rule.errs)-1].Fix = &Fix{
		Description: "Write the path of uses: in its plain form",
		Unsafe:      true,
		Edits:       []TextEdit{{off, off + len(path), clean}},
	}
}

// obfuscatedUsesPath tells whether the path of the uses: value has empty, "." or ".." segments. It
// returns the path (the value up to the ref) and its plain form. The plain form is empty when it
// cannot be computed (the path climbs out of the repository).
func obfuscatedUsesPath(v string) (path, clean string, ok bool) {
	if strings.Contains(v, "${{") || strings.HasPrefix(v, "docker://") {
		return "", "", false
	}
	path = v
	if i := strings.IndexByte(path, '@'); i >= 0 {
		path = path[:i]
	}
	if u := ParseUses(v); u.Kind == UsesInvalid {
		return "", "", false // reported by the action rule
	}
	prefix := ""
	rest := path
	switch {
	case strings.HasPrefix(path, "./"):
		prefix, rest = "./", path[2:]
	case strings.HasPrefix(path, selfRepositoryUsesPrefix):
		prefix, rest = selfRepositoryUsesPrefix, path[len(selfRepositoryUsesPrefix):]
	}
	if rest == "" {
		return "", "", false // the root of the repository
	}
	segs := strings.Split(rest, "/")
	trailing := len(segs) > 1 && segs[len(segs)-1] == ""
	if trailing {
		segs = segs[:len(segs)-1]
	}
	obfuscated := false
	var out []string
	climbs := false
	for _, sg := range segs {
		switch sg {
		case "", ".":
			obfuscated = true
		case "..":
			switch {
			case len(out) == 0 && prefix != "":
				// ./../other-checkout/action refers to a repository checked out next to this one
				out = append(out, sg)
			case len(out) == 0 || (prefix == "" && len(out) <= 2):
				obfuscated = true
				climbs = true
			case out[len(out)-1] == "..":
				out = append(out, sg)
			default:
				obfuscated = true
				out = out[:len(out)-1]
			}
		default:
			out = append(out, sg)
		}
	}
	if !obfuscated {
		return "", "", false
	}
	if climbs || len(out) == 0 {
		return path, "", true
	}
	clean = prefix + strings.Join(out, "/")
	if trailing {
		clean += "/"
	}
	return path, clean, true
}

// checkObfuscation reports expressions which are written in a roundabout way: fromJSON(toJSON(x))
// returns x, and format() with literal arguments is a constant string.
func (rule *RuleExpression) checkObfuscation(expr ExprNode, line, col int, workflowKey string) {
	if !strings.HasSuffix(workflowKey, ".if") && NewExprSemanticsChecker(false, nil, nil).IsConstant(expr) {
		// The if: conditions are reported by the constant-condition rule.
		tok := expr.Token()
		if call, ok := expr.(*FuncCallNode); ok && strings.EqualFold(call.Callee, "format") {
			if res, ok := formatOfLiterals(call.Args); ok {
				rule.ReportIDf("obfuscation", rule.exprPos(tok.Line, tok.Column, line, col), "format() is called with literal arguments only so its result is the constant %q. write the string itself", res)
				return
			}
		}
		rule.ReportIDf("obfuscation", rule.exprPos(tok.Line, tok.Column, line, col), "the expression is constant so it can be replaced by its value. write the value itself")
		return
	}
	isIf := strings.HasSuffix(workflowKey, ".if")
	VisitExprNode(expr, func(n, _ ExprNode, entering bool) {
		if !entering {
			return
		}
		if idx, ok := n.(*IndexAccessNode); ok {
			switch idx.Index.(type) {
			case *StringNode, *IntNode:
			default:
				if v := chainRoot(idx.Index); v != nil && strings.EqualFold(v.Name, "matrix") {
					return // the matrix is how a workflow selects a value by name, `fromJSON(x)[matrix.tool]`
				}
				tok := idx.Index.Token()
				rule.ReportIDf("obfuscation", rule.exprPos(tok.Line, tok.Column, line, col), "the index is computed, which hides which property is read from tools that look for it. use a literal property name or a matrix to select the value")
			}
			return
		}
		call, ok := n.(*FuncCallNode)
		if !ok {
			return
		}
		tok := call.Token()
		switch strings.ToLower(call.Callee) {
		case "fromjson":
			if len(call.Args) != 1 {
				return
			}
			if inner, ok := call.Args[0].(*FuncCallNode); ok && strings.EqualFold(inner.Callee, "toJSON") && len(inner.Args) == 1 && !isContextChain(inner.Args[0]) {
				rule.ReportIDf("obfuscation", rule.exprPos(tok.Line, tok.Column, line, col), "fromJSON(toJSON(...)) returns its argument unchanged. remove both calls")
			}
		case "format":
			if isIf && ExprNode(call) == expr {
				return // a constant condition is reported by the constant-condition rule
			}
			if res, ok := formatOfLiterals(call.Args); ok {
				rule.ReportIDf("obfuscation", rule.exprPos(tok.Line, tok.Column, line, col), "format() is called with literal arguments only so its result is the constant %q. write the string itself", res)
			}
		}
	})
}

// formatOfLiterals computes format() for arguments which are all literals.
func formatOfLiterals(args []ExprNode) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	f, ok := args[0].(*StringNode)
	if !ok {
		return "", false
	}
	vals := make([]string, 0, len(args)-1)
	for _, a := range args[1:] {
		switch a := a.(type) {
		case *StringNode:
			vals = append(vals, a.Value)
		case *IntNode:
			vals = append(vals, strconv.Itoa(a.Value))
		case *BoolNode:
			vals = append(vals, strconv.FormatBool(a.Value))
		case *NullNode:
			vals = append(vals, "")
		default:
			return "", false // a float is formatted by GitHub in a way which is not reproduced here
		}
	}
	var b strings.Builder
	s := f.Value
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '{':
			if i+1 < len(s) && s[i+1] == '{' {
				b.WriteByte('{')
				i++
				continue
			}
			j := strings.IndexByte(s[i:], '}')
			if j < 0 {
				return "", false
			}
			n, err := strconv.Atoi(s[i+1 : i+j])
			if err != nil || n < 0 || n >= len(vals) {
				return "", false
			}
			b.WriteString(vals[n])
			i += j
		case '}':
			if i+1 < len(s) && s[i+1] == '}' {
				b.WriteByte('}')
				i++
				continue
			}
			return "", false
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), true
}

func init() {
	registerRules(
		RuleInfo{ID: "obfuscation", Group: RuleGroupSecurity, Summary: "A path at uses: or an expression is written in an obfuscated way.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-obfuscation"},
	)
	registerRuleFactory("obfuscation", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleObfuscation(env.src)}
	})
}

// isContextChain reports whether the node is a property access like matrix.container. The round trip
// fromJSON(toJSON(matrix.container)) is a known workaround to make a value of a context an object, so
// it is not reported.
func isContextChain(n ExprNode) bool {
	switch n := n.(type) {
	case *VariableNode:
		return true
	case *ObjectDerefNode:
		return isContextChain(n.Receiver)
	case *ArrayDerefNode:
		return isContextChain(n.Receiver)
	case *IndexAccessNode:
		return isContextChain(n.Operand)
	}
	return false
}
