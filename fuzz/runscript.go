//go:build gofuzz

package jactionlint_fuzz

import (
	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// FuzzRunScript feeds arbitrary text as a `run:` script to the shell analyzer. It must never panic, and scripts
// which do parse must satisfy the structural guarantees of the analyzer.
func FuzzRunScript(data []byte) int {
	s, err := runscript.Analyze(string(data), "bash")
	if err != nil {
		return 0
	}
	if err := runscript.CheckInvariants(s); err != nil {
		panic(err)
	}
	return 1
}
