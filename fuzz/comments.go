//go:build gofuzz

package jactionlint_fuzz

import (
	"github.com/jdx/jactionlint/v2"
)

func FuzzCommentIndex(data []byte) int {
	idx := jactionlint.NewCommentIndex(data)
	prev := 0
	for _, c := range idx.All() {
		if c.Line <= prev || c.Column < 1 {
			panic("comments must be in order, one per line")
		}
		prev = c.Line
		_ = idx.Before(c.Line)
		_ = idx.After(c.Line)
		_ = idx.Documented(c.Line)
	}
	if len(idx.All()) == 0 {
		return 0
	}
	return 1
}
