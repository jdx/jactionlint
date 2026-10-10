package jactionlint

import "strings"

// placeholder is one parsed ${{ }} of a string.
type placeholder struct {
	// after is the byte offset in the value of the text after the "${{".
	after int
	// node is the syntax tree of the text.
	node ExprNode
	// n is the number of bytes the expression took, including the closing "}}".
	n int
}

// stringExprs is the result of parsing the expressions of a string. It is kept on the string so that
// every rule which looks at the expressions shares one lex and parse of each of them. The syntax
// trees are never changed after they are parsed.
type stringExprs struct {
	// value is the value which was parsed. The result is dropped when the value is not the same.
	value string

	scanned bool
	list    []placeholder
	// failed is true when the scan stopped at a placeholder which does not parse. err is its error and
	// errAfter the offset of its text.
	failed   bool
	err      *ExprError
	errAfter int

	condDone bool
	cond     ExprNode
	condErr  *ExprError
}

// noPlaceholders is the result for a string without ${{ }}. It is shared and must not be changed.
var noPlaceholders = stringExprs{scanned: true}

func (s *String) exprCache() *stringExprs {
	if s.exprs == nil || s.exprs.value != s.Value {
		s.exprs = &stringExprs{value: s.Value}
	}
	return s.exprs
}

// placeholders returns the parsed ${{ }} placeholders of the string up to the first one which does not
// parse, which is reported by failed and err.
func (s *String) placeholders() *stringExprs {
	if !strings.Contains(s.Value, "${{") {
		return &noPlaceholders // most strings; nothing to cache
	}
	c := s.exprCache()
	if c.scanned {
		return c
	}
	c.scanned = true
	value := s.Value
	pos := 0
	for {
		i := strings.Index(value[pos:], "${{")
		if i < 0 {
			return c
		}
		after := pos + i + len("${{")
		l := NewExprLexer(value[after:])
		node, err := NewExprParser().Parse(l)
		if err != nil || node == nil {
			c.failed, c.err, c.errAfter = true, err, after
			return c
		}
		n := l.Offset()
		c.list = append(c.list, placeholder{after: after, node: node, n: n})
		pos = after + n
	}
}

// condExpr parses the string as an "if:" condition which is not wrapped in ${{ }}.
func (s *String) condExpr() (ExprNode, *ExprError) {
	c := s.exprCache()
	if !c.condDone {
		c.condDone = true
		c.cond, c.condErr = NewExprParser().Parse(NewExprLexer(s.Value + "}}")) // }} is necessary since lexer lexes it as end of tokens
	}
	return c.cond, c.condErr
}

// templateExprs returns the syntax trees of the ${{ }} expressions of the string, and false when one
// of them does not parse. The expressions parsed before the failure are returned either way.
func (s *String) templateExprs() (nodes []ExprNode, ok bool) {
	c := s.placeholders()
	for _, p := range c.list {
		nodes = append(nodes, p.node)
	}
	return nodes, !c.failed
}
