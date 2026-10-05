//go:build gofuzz

package jactionlint_fuzz

import (
	"unicode/utf8"

	"github.com/jdx/jactionlint"
)

func FuzzExprParse(data []byte) int {
	if !utf8.Valid(data) {
		return 0
	}

	l := jactionlint.NewExprLexer(string(data))
	p := jactionlint.NewExprParser()
	e, err := p.Parse(l)
	if err != nil {
		return 0
	}

	c := jactionlint.NewExprSemanticsChecker(true, nil, nil)
	c.Check(e)

	return 1
}
