package jactionlint

// constantNode returns the syntax tree of the string when it is one ${{ }} expression which is a literal, such as
// `${{ 'max' }}` or `${{ true }}`. The value is then known without evaluating anything.
func (s *String) constantNode() ExprNode {
	if s == nil || !s.IsExpressionAssigned() {
		return nil
	}
	ps := s.placeholders()
	if ps.failed || len(ps.list) != 1 {
		return nil
	}
	switch n := ps.list[0].node.(type) {
	case *StringNode, *BoolNode:
		return n
	}
	return nil
}

// literalString returns the value of the string when it is known without evaluating anything: a string without
// expressions, or one expression which is a string literal (`${{ 'max' }}`).
func (s *String) literalString() (string, bool) {
	if s == nil {
		return "", false
	}
	if !s.ContainsExpression() {
		return s.Value, true
	}
	if n, ok := s.constantNode().(*StringNode); ok {
		return n.Value, true
	}
	return "", false
}

// literalTrue reports whether the boolean is known to be true without evaluating anything: `true` or `${{ true }}`.
func (b *Bool) literalTrue() bool {
	if b == nil {
		return false
	}
	if b.Expression == nil {
		return b.Value
	}
	n, ok := b.Expression.constantNode().(*BoolNode)
	return ok && n.Value
}
