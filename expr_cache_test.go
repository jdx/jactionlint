package jactionlint

import (
	"reflect"
	"strings"
	"testing"
)

// TestStringSharesParsedExpressions checks that the rules which ask a string for the same expression
// get the one syntax tree, which is parsed once.
func TestStringSharesParsedExpressions(t *testing.T) {
	s := &String{Value: "${{ github.event.pull_request.head.ref }}"}
	nodes, ok := s.templateExprs()
	if !ok || len(nodes) != 1 {
		t.Fatalf("templateExprs() = %v, %v", nodes, ok)
	}
	again, _ := s.templateExprs()
	if again[0] != nodes[0] {
		t.Error("templateExprs parsed the expression twice")
	}
	if c := s.placeholders(); c.list[0].node != nodes[0] {
		t.Error("placeholders did not share the tree of templateExprs")
	}
	whole, _, _, ok := parseWholeExpr(s)
	if !ok || whole != nodes[0] {
		t.Error("parseWholeExpr did not share the tree of templateExprs")
	}
	if refs, ok := s.exprRefs(); !ok || len(refs) == 0 {
		t.Errorf("exprRefs() = %v, %v", refs, ok)
	}
	if _, bad := untrustedString(s); !bad {
		t.Error("untrustedString did not see the head of the pull request")
	}
	if got, _ := s.templateExprs(); got[0] != nodes[0] {
		t.Error("a rule changed the cached tree")
	}

	bare := &String{Value: "github.actor == 'x'"}
	e1, _ := bare.condExpr()
	e2, _, _, ok := parseWholeExpr(bare)
	if !ok || e1 == nil || e1 != e2 {
		t.Error("parseWholeExpr did not share the tree of condExpr")
	}
}

// TestParseWholeExprMatchesAFreshParse compares the shared parse of a whole expression with a parse of
// its text, for the spellings of a condition.
func TestParseWholeExprMatchesAFreshParse(t *testing.T) {
	for _, v := range []string{
		"${{ github.actor == 'x' }}", "  ${{ github.actor == 'x' }}  ", "${{github.actor}}", "${{ '}}' }}",
		"${{ a }} ${{ b }}", "${{ a }} junk", "${{ ( }}", "", "github.actor", "  github.actor ", "!cancelled() ",
		"\n  true", "a }} b", "${{ a ",
		// white space which the lexer does not skip
		"true\u00a0", "\u00a0false", "\u2003true\u2003", "${{ a }}\u00a0", "\u00a0${{ a }}\u00a0", "${{ a\u00a0}}",
		// text after the first placeholder
		"${{ a }} && ${{ b }}", "${{ a }} && b", "${{ a }}x", "${{ a }} }}", "${{ a }}\n${{ b }}",
	} {
		s := &String{Value: v}
		node, text, base, ok := parseWholeExpr(s)
		fresh, ftext, fbase, fok := freshWholeExpr(v)
		if ok != fok || text != ftext || base != fbase || !reflect.DeepEqual(node, fresh) {
			t.Errorf("%q: shared (%v %q %d %v) != fresh (%v %q %d %v)", v, node, text, base, ok, fresh, ftext, fbase, fok)
		}
	}
}

// freshWholeExpr is parseWholeExpr without the shared parse.
func freshWholeExpr(value string) (ExprNode, string, int, bool) {
	trimmed := strings.TrimSpace(value)
	lead := len(value) - len(strings.TrimLeft(value, " \t\r\n"))
	if strings.HasPrefix(trimmed, "${{") {
		if !isExprAssigned(value) {
			return nil, "", 0, false
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "${{"), "}}")
		node, err := NewExprParser().Parse(NewExprLexer(inner + "}}"))
		if err != nil {
			return nil, "", 0, false
		}
		return node, inner, lead + len("${{"), true
	}
	if strings.Contains(value, "${{") {
		return nil, "", 0, false
	}
	node, err := NewExprParser().Parse(NewExprLexer(trimmed + "}}"))
	if err != nil {
		return nil, "", 0, false
	}
	return node, trimmed, lead, true
}
