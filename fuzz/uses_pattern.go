//go:build gofuzz

package jactionlint_fuzz

import (
	"bytes"

	"github.com/jdx/jactionlint/v2"
)

// FuzzUsesPattern fuzzes a pattern of the forbidden-uses rule against a reference. The input is the
// pattern and the reference separated by a NUL byte.
func FuzzUsesPattern(data []byte) int {
	pat, uses, _ := bytes.Cut(data, []byte{0})
	p := jactionlint.ParseUsesPattern(string(pat))
	u := jactionlint.ParseUses(string(uses))
	if p.Match(u) && !u.IsRepo() {
		panic("a pattern matched a reference which is not an action or a reusable workflow")
	}
	_ = p.String()
	return 0
}
