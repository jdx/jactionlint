package eval

import (
	"strings"
)

// segment is a piece of a template: literal text or the source of one ${{ }} expression.
type segment struct {
	text string
	expr bool
}

// split cuts a string at its ${{ }} placeholders. The end of a placeholder is the first }} outside a string literal.
func split(s string) ([]segment, error) {
	var segs []segment
	for {
		i := strings.Index(s, "${{")
		if i < 0 {
			if s != "" {
				segs = append(segs, segment{text: s})
			}
			return segs, nil
		}
		if i > 0 {
			segs = append(segs, segment{text: s[:i]})
		}
		s = s[i+3:]
		end := -1
		inStr := false
		for j := 0; j < len(s); j++ {
			switch {
			case s[j] == '\'':
				if inStr && j+1 < len(s) && s[j+1] == '\'' {
					j++
				} else {
					inStr = !inStr
				}
			case !inStr && s[j] == '}' && j+1 < len(s) && s[j+1] == '}':
				end = j
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return nil, &Error{s, "unterminated ${{ in the string"}
		}
		segs = append(segs, segment{text: s[:end], expr: true})
		s = s[end+2:]
	}
}

// HasExpr reports whether the string contains a ${{ }} placeholder.
func HasExpr(s string) bool { return strings.Contains(s, "${{") }

// Interpolate replaces each ${{ }} of the string by its value converted to a string.
func (e *Env) Interpolate(s string) (string, error) {
	if !HasExpr(s) {
		return s, nil
	}
	segs, err := split(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, sg := range segs {
		if !sg.expr {
			b.WriteString(sg.text)
			continue
		}
		v, err := e.Eval(sg.text)
		if err != nil {
			return "", err
		}
		b.WriteString(ToString(v))
	}
	return b.String(), nil
}

// Value evaluates a string that is exactly one ${{ }} to its typed value, and any other string to itself with its
// placeholders interpolated. This is what the fields that take a typed value (a matrix, fromJSON results) do.
func (e *Env) Value(s string) (any, error) {
	segs, err := split(s)
	if err == nil && len(segs) == 1 && segs[0].expr {
		return e.Eval(segs[0].text)
	}
	if err == nil && len(segs) == 3 && segs[1].expr && strings.TrimSpace(segs[0].text) == "" && strings.TrimSpace(segs[2].text) == "" {
		return e.Eval(segs[1].text)
	}
	return e.Interpolate(s)
}

// Condition evaluates the `if:` of a job or step. A condition without a status function gets an implicit success()
// in front, like on GitHub; a condition that is not wrapped in ${{ }} is the expression itself.
func (e *Env) Condition(s string) (bool, error) {
	s = strings.TrimSpace(s)
	expr := s
	if segs, err := split(s); err == nil && len(segs) == 1 && segs[0].expr {
		expr = strings.TrimSpace(segs[0].text)
	} else if HasExpr(s) {
		// text around placeholders: the whole string is interpolated and the result tested
		out, err := e.Interpolate(s)
		if err != nil {
			return false, err
		}
		return e.statusAnd(Truthy(strings.TrimSpace(out)), false), nil
	}
	if expr == "" {
		return e.status("success"), nil
	}
	n, err := Parse(expr)
	if err != nil {
		return false, err
	}
	v, err := e.EvalNode(n, expr)
	if err != nil {
		return false, err
	}
	return e.statusAnd(Truthy(v), hasStatusCall(n)), nil
}

func (e *Env) statusAnd(v, hasStatus bool) bool {
	if hasStatus {
		return v
	}
	return v && e.status("success")
}
