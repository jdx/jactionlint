//go:build gofuzz

package jactionlint_fuzz

import (
	"github.com/jdx/jactionlint/v2"
)

func FuzzParseUses(data []byte) int {
	u := jactionlint.ParseUses(string(data))
	if u == nil || u.Raw != string(data) {
		panic("ParseUses must return a reference holding the raw value")
	}
	// None of the helpers may panic whatever the kind is
	_ = u.IsPinned()
	_ = u.CanonicalName()
	_ = u.SameRepo(u)
	if u.Kind == jactionlint.UsesInvalid {
		if u.Problem == "" {
			panic("invalid reference without a problem")
		}
		return 0
	}
	return 1
}
