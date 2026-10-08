package jactionlint

import (
	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// scriptOrigin describes where the string is in the YAML file, for mapping positions in the analyzed script of
// a `run:` back to the file. Literal blocks (`run: |`) map exactly, everything else approximately. See
// runscript.Origin.
func (s *String) scriptOrigin() runscript.Origin {
	o := runscript.Origin{Literal: s.Literal, Indent: s.Indent, Quoted: s.Quoted}
	if s.Pos != nil {
		o.Line, o.Col = s.Pos.Line, s.Pos.Col
	}
	return o
}

// analyzeRun analyzes the script of a `run:` step. It returns nil when there is nothing to report: the shell is not
// bash or sh, or the script does not parse. A parse failure is never an error for users. The returned origin maps
// positions of the script to the file.
//
// The default shell of Windows runners is pwsh. This function treats a missing `shell:` as bash; callers which
// know that the job runs on Windows must not call it for steps without `shell:`.
func analyzeRun(e *ExecRun) (*runscript.Script, runscript.Origin) {
	if e == nil || e.Run == nil {
		return nil, runscript.Origin{}
	}
	shell := ""
	if e.Shell != nil {
		shell = e.Shell.Value
	}
	s, err := runscript.Analyze(e.Run.Value, shell)
	if err != nil {
		return nil, runscript.Origin{}
	}
	return s, e.Run.scriptOrigin()
}
