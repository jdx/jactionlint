package jactionlint

import "github.com/jdx/jactionlint/v2/internal/runscript"

// stringScript is the result of analyzing a string as a shell script. It is kept on the string so that every rule which looks
// at the script of a `run:` shares one parse of it: a run script was parsed by a dozen rules, each building the same
// tree. The result is never changed after it is built.
type stringScript struct {
	// value is the value which was analyzed. The result is dropped when the value is not the same.
	value  string
	script *runscript.Script
	err    error
}

// analyzeScript is runscript.Analyze of the value of the string. The analysis does not depend on the shell but on whether
// the shell is supported, so a script is analyzed once whatever the shell was named. Like the parsed expressions of the string
// it is not safe for concurrent use: the strings of a workflow are checked by one goroutine.
func (s *String) analyzeScript(shell string) (*runscript.Script, error) {
	if !runscript.SupportsShell(shell) {
		return nil, runscript.ErrUnsupportedShell
	}
	if s.script == nil || s.script.value != s.Value {
		script, err := runscript.Analyze(s.Value, shell)
		s.script = &stringScript{value: s.Value, script: script, err: err}
	}
	return s.script.script, s.script.err
}
