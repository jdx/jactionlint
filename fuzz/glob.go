//go:build gofuzz

package jactionlint_fuzz

import (
	"github.com/jdx/jactionlint/v2"
)

func FuzzGlobGitRef(data []byte) int {
	errs := jactionlint.ValidateRefGlob(string(data))
	if len(errs) > 0 {
		return 0
	}
	return 1
}

func FuzzGlobFilePath(data []byte) int {
	errs := jactionlint.ValidatePathGlob(string(data))
	if len(errs) > 0 {
		return 0
	}
	return 1
}
