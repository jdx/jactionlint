package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jdx/jactionlint/v2"
)

// Env is what an expression can see.
type Env struct {
	// Contexts maps the context names (github, env, vars, secrets, matrix, needs, steps, job, runner, inputs,
	// strategy) to their values. Names are matched without regard to case.
	Contexts map[string]any
	// Status answers the status functions. Name is one of success, failure, always, cancelled. When nil, success()
	// and always() are true and failure() and cancelled() are false.
	Status func(name string) bool
	// HashFiles implements hashFiles(). When nil the function is an error.
	HashFiles func(patterns []string) (string, error)
}

// Error is an error of evaluating an expression.
type Error struct {
	Expr string
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("expression %q: %s", e.Expr, e.Msg) }

// Parse parses the text of an expression (without ${{ }}).
func Parse(expr string) (jactionlint.ExprNode, error) {
	n, err := jactionlint.NewExprParser().Parse(jactionlint.NewExprLexer(expr + "}}"))
	if err != nil {
		return nil, &Error{expr, err.Message}
	}
	return n, nil
}

// Eval evaluates the text of an expression.
func (e *Env) Eval(expr string) (any, error) {
	n, err := Parse(expr)
	if err != nil {
		return nil, err
	}
	return e.EvalNode(n, expr)
}

// EvalNode evaluates a parsed expression. src is used in error messages.
func (e *Env) EvalNode(n jactionlint.ExprNode, src string) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ee, ok := r.(*Error); ok {
				v, err = nil, ee
				return
			}
			panic(r)
		}
	}()
	return e.eval(n, src), nil
}

func fail(src, format string, a ...any) {
	panic(&Error{src, fmt.Sprintf(format, a...)})
}

// filtered is the result of a.*: the values selected so far, which a following .prop or [i] maps over.
type filtered []any

func (e *Env) eval(n jactionlint.ExprNode, src string) any {
	switch n := n.(type) {
	case *jactionlint.NullNode:
		return nil
	case *jactionlint.BoolNode:
		return n.Value
	case *jactionlint.IntNode:
		return float64(n.Value)
	case *jactionlint.FloatNode:
		return n.Value
	case *jactionlint.StringNode:
		return n.Value
	case *jactionlint.VariableNode:
		return e.context(n.Name)
	case *jactionlint.NotOpNode:
		return !Truthy(e.value(n.Operand, src))
	case *jactionlint.CompareOpNode:
		l, r := e.value(n.Left, src), e.value(n.Right, src)
		switch n.Kind {
		case jactionlint.CompareOpNodeKindEq:
			return Equal(l, r)
		case jactionlint.CompareOpNodeKindNotEq:
			return !Equal(l, r)
		}
		c, ok := Compare(l, r)
		if !ok {
			return false
		}
		switch n.Kind {
		case jactionlint.CompareOpNodeKindLess:
			return c < 0
		case jactionlint.CompareOpNodeKindLessEq:
			return c <= 0
		case jactionlint.CompareOpNodeKindGreater:
			return c > 0
		default:
			return c >= 0
		}
	case *jactionlint.LogicalOpNode:
		l := e.value(n.Left, src)
		if n.Kind == jactionlint.LogicalOpNodeKindAnd {
			if !Truthy(l) {
				return l
			}
		} else if Truthy(l) {
			return l
		}
		return e.value(n.Right, src)
	case *jactionlint.ObjectDerefNode:
		return property(e.eval(n.Receiver, src), n.Property)
	case jactionlint.ObjectDerefNode:
		return property(e.eval(n.Receiver, src), n.Property)
	case *jactionlint.ArrayDerefNode:
		return star(e.value(n.Receiver, src))
	case jactionlint.ArrayDerefNode:
		return star(e.value(n.Receiver, src))
	case *jactionlint.IndexAccessNode:
		return index(e.eval(n.Operand, src), e.value(n.Index, src))
	case *jactionlint.FuncCallNode:
		return e.call(n, src)
	}
	fail(src, "unsupported expression %T", n)
	return nil
}

// value evaluates and turns a filtered array into a plain array.
func (e *Env) value(n jactionlint.ExprNode, src string) any {
	v := e.eval(n, src)
	if f, ok := v.(filtered); ok {
		return []any(f)
	}
	return v
}

func (e *Env) context(name string) any {
	for k, v := range e.Contexts {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}

func lookup(m map[string]any, key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

func property(recv any, name string) any {
	switch r := recv.(type) {
	case map[string]any:
		v, _ := lookup(r, name)
		return v
	case filtered:
		out := filtered{}
		for _, item := range r {
			if m, ok := item.(map[string]any); ok {
				if v, ok := lookup(m, name); ok {
					out = append(out, v)
				}
			}
		}
		return out
	}
	return nil
}

func star(v any) any {
	switch v := v.(type) {
	case []any:
		return filtered(append([]any(nil), v...))
	case map[string]any:
		// GitHub orders object values by key for a stable result
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sortStrings(keys)
		out := filtered{}
		for _, k := range keys {
			out = append(out, v[k])
		}
		return out
	case filtered:
		return v
	}
	return filtered{}
}

func index(recv, idx any) any {
	switch r := recv.(type) {
	case []any:
		f, ok := idx.(float64)
		if !ok {
			if s, isStr := idx.(string); isStr {
				f = parseNumber(s)
				ok = !math.IsNaN(f)
			}
		}
		if !ok || f != math.Trunc(f) || f < 0 || f >= float64(len(r)) {
			return nil
		}
		if i := int(f); i >= 0 && i < len(r) {
			return r[i]
		}
		return nil
	case map[string]any:
		if s, ok := idx.(string); ok {
			v, _ := lookup(r, s)
			return v
		}
		return nil
	case filtered:
		out := filtered{}
		for _, item := range r {
			if v := index(item, idx); v != nil {
				out = append(out, v)
			}
		}
		return out
	}
	return nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func (e *Env) status(name string) bool {
	if e.Status != nil {
		return e.Status(name)
	}
	return name == "success" || name == "always"
}

func (e *Env) call(n *jactionlint.FuncCallNode, src string) any {
	name := strings.ToLower(n.Callee)
	args := make([]any, len(n.Args))
	for i, a := range n.Args {
		args[i] = e.value(a, src)
	}
	need := func(min, max int) {
		if len(args) < min || (max >= 0 && len(args) > max) {
			fail(src, "wrong number of arguments for %s(): got %d", n.Callee, len(args))
		}
	}
	switch name {
	case "success", "failure", "always", "cancelled":
		need(0, 0)
		return e.status(name)
	case "contains":
		need(2, 2)
		switch s := args[0].(type) {
		case []any:
			for _, el := range s {
				if Equal(el, args[1]) {
					return true
				}
			}
			return false
		case map[string]any:
			return false
		default:
			return strings.Contains(strings.ToLower(ToString(args[0])), strings.ToLower(ToString(args[1])))
		}
	case "startswith":
		need(2, 2)
		return strings.HasPrefix(strings.ToLower(ToString(args[0])), strings.ToLower(ToString(args[1])))
	case "endswith":
		need(2, 2)
		return strings.HasSuffix(strings.ToLower(ToString(args[0])), strings.ToLower(ToString(args[1])))
	case "format":
		need(1, -1)
		s, err := Format(ToString(args[0]), args[1:])
		if err != nil {
			fail(src, "%v", err)
		}
		return s
	case "join":
		need(1, 2)
		sep := ","
		if len(args) == 2 {
			sep = ToString(args[1])
		}
		switch a := args[0].(type) {
		case []any:
			parts := make([]string, len(a))
			for i, el := range a {
				parts[i] = ToString(el)
			}
			return strings.Join(parts, sep)
		case map[string]any:
			return ""
		}
		return ToString(args[0])
	case "tojson":
		need(1, 1)
		return ToJSON(args[0])
	case "fromjson":
		need(1, 1)
		s := ToString(args[0])
		if strings.TrimSpace(s) == "" {
			fail(src, "fromJSON: empty input")
		}
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			fail(src, "fromJSON: invalid JSON: %v", err)
		}
		if dec.More() {
			fail(src, "fromJSON: invalid JSON: unexpected data after the value")
		}
		return Normalize(v)
	case "hashfiles":
		need(1, -1)
		if e.HashFiles == nil {
			fail(src, "hashFiles() is not available here")
		}
		pats := make([]string, len(args))
		for i, a := range args {
			pats[i] = ToString(a)
		}
		h, err := e.HashFiles(pats)
		if err != nil {
			fail(src, "hashFiles: %v", err)
		}
		return h
	}
	fail(src, "unknown function %q", n.Callee)
	return nil
}

// Format implements format(): {N} takes argument N, {{ and }} are literal braces.
func Format(f string, args []any) (string, error) {
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		c := f[i]
		switch c {
		case '{':
			if i+1 < len(f) && f[i+1] == '{' {
				b.WriteByte('{')
				i++
				continue
			}
			end := strings.IndexByte(f[i:], '}')
			if end < 0 {
				return "", errors.New("format: unterminated { in the format string")
			}
			spec := f[i+1 : i+end]
			n, err := strconv.Atoi(spec)
			if err != nil || n < 0 {
				return "", fmt.Errorf("format: invalid placeholder {%s}", spec)
			}
			if n >= len(args) {
				return "", fmt.Errorf("format: the format string uses {%d} but has only %d argument(s)", n, len(args))
			}
			b.WriteString(ToString(args[n]))
			i += end
		case '}':
			if i+1 < len(f) && f[i+1] == '}' {
				b.WriteByte('}')
				i++
				continue
			}
			return "", errors.New("format: unmatched } in the format string")
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// CanEval reports whether every context that the string's expressions read is defined in the environment, so that
// evaluating it would not silently read null from a context that exists only at run time.
func (e *Env) CanEval(s string) bool {
	names, err := StringContexts(s)
	if err != nil {
		return false
	}
	for _, n := range names {
		found := false
		for k := range e.Contexts {
			if strings.EqualFold(k, n) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
