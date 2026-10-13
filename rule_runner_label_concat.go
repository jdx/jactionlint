package jactionlint

import (
	"strconv"
	"strings"
)

// concatenatedRunnerLabels resolves a label of `runs-on` which is text and one matrix value written together, such as
// `${{ matrix.os }}-latest` or `ubuntu-${{ matrix.version }}`, into the labels that the jobs of the matrix get
// (`linux-latest` for `os: linux`). The labels are checked like the labels of a plain `${{ matrix.os }}`.
//
// It is conservative: it returns nothing unless every `${{ }}` of the label is the same plain `matrix.<name>`. A label
// made of several matrix values (`${{ matrix.os }}-${{ matrix.arch }}`) is not resolved because `exclude` and `include`
// decide which pairs run and the product of the values has pairs that do not. A value that is not a string without an
// expression, or a number that GitHub prints differently from the text of the file (`20.10` is `20.1`), is skipped. A
// value of the row is used only when some job certainly has it, after `exclude` (survivingRowValues); a value that
// `include` adds always has a job. Wherever an expression makes the effect unknown nothing is reported.
func concatenatedRunnerLabels(label *String, m *Matrix) []*String {
	if m == nil || m.Expression != nil || !strings.Contains(label.Value, "${{") {
		return nil
	}

	// Cut the label into text and expressions
	type piece struct {
		text string // text, or "" for the matrix value
		expr bool
	}
	var pieces []piece
	name := ""
	rest := label.Value
	for rest != "" {
		s := strings.Index(rest, "${{")
		if s < 0 {
			pieces = append(pieces, piece{text: rest})
			break
		}
		if s > 0 {
			pieces = append(pieces, piece{text: rest[:s]})
		}
		e := strings.Index(rest[s:], "}}")
		if e < 0 {
			return nil
		}
		e += s
		n, ok := matrixPropertyOfExpression(rest[s+len("${{") : e+len("}}")])
		if !ok || (name != "" && name != n) {
			return nil
		}
		name = n
		pieces = append(pieces, piece{expr: true})
		rest = rest[e+len("}}"):]
	}
	if name == "" {
		return nil
	}

	// The values of the row that certainly have a job (see matrix_survival.go), and the ones that `include` adds, which
	// are a job whatever `exclude` removed.
	survive := survivingRowValues(m, name)
	var values []*RawYAMLString
	if row, ok := m.Rows[name]; ok && row.Expression == nil {
		for _, v := range row.Values {
			if s, ok := v.(*RawYAMLString); ok && survive[s] {
				values = append(values, s)
			}
		}
	}
	if m.Include != nil {
		for _, c := range m.Include.Combinations {
			if a, ok := c.Assigns[name]; ok {
				if s, ok := a.Value.(*RawYAMLString); ok {
					values = append(values, s)
				}
			}
		}
	}

	var labels []*String
	seen := map[string]bool{}
	for _, v := range values {
		if !isStableMatrixString(v) {
			continue
		}
		var b strings.Builder
		for _, p := range pieces {
			if p.expr {
				b.WriteString(v.Value)
			} else {
				b.WriteString(p.text)
			}
		}
		l := b.String()
		if seen[l] {
			continue
		}
		seen[l] = true
		labels = append(labels, &String{Value: l, Pos: v.Pos()})
	}
	return labels
}

// matrixPropertyOfExpression returns the name in a `matrix.<name>` expression. text is what follows the "${{" of the
// expression, including its "}}".
func matrixPropertyOfExpression(text string) (string, bool) {
	expr, err := NewExprParser().Parse(NewExprLexer(text))
	if err != nil {
		return "", false
	}
	deref, ok := expr.(*ObjectDerefNode)
	if !ok {
		return "", false
	}
	recv, ok := deref.Receiver.(*VariableNode)
	if !ok || recv.Name != "matrix" {
		return "", false
	}
	return strings.ToLower(deref.Property), true
}

// isStableMatrixString reports whether the text of a matrix value is the string that GitHub makes of it when it is put
// into another string: a scalar without an expression which is a string, or a number that is printed the same way.
func isStableMatrixString(v *RawYAMLString) bool {
	if v.Value == "" || ContainsExpression(v.Value) {
		return false
	}
	if v.StringTag {
		return true
	}
	switch strings.ToLower(v.Value) {
	case "null", "~":
		return false // an empty string when it is put into a string
	case "true", "false":
		// A boolean (YAML 1.2 core: true, True and TRUE) is printed in lower case. yes, no, on and off are strings
		// for GitHub, which does not read YAML 1.1.
		return v.Value == strings.ToLower(v.Value)
	}
	if f, err := strconv.ParseFloat(v.Value, 64); err == nil {
		return strconv.FormatFloat(f, 'g', -1, 64) == v.Value
	}
	// A number in another form (0x1F, 1_000, 010, .5, .inf) is not looked at; a text such as 22.04-arm is a string
	if c := v.Value[0]; c >= '0' && c <= '9' || c == '-' || c == '+' || c == '.' {
		return strings.Trim(strings.ToLower(v.Value), "0123456789+-._abcdefxoin") != ""
	}
	return true
}
