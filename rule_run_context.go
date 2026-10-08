package jactionlint

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// runContext is what the rules which analyze `run:` scripts (github-env, adhoc-packages, unpinned-tools,
// use-trusted-publishing and unlocked-install) need to know about the place of a step: its workflow, its job, the
// shell that runs it and the environment variables that reach it. Rules embed it and forward the Visit* methods.
type runContext struct {
	wf  *Workflow
	job *Job
}

func (c *runContext) enterWorkflow(w *Workflow) {
	c.wf, c.job = w, nil
}

func (c *runContext) enterJob(j *Job) {
	c.job = j
}

func (c *runContext) leaveJob() {
	c.job = nil
}

// hasTrigger reports whether the workflow is started by one of the events.
func (c *runContext) hasTrigger(events ...string) bool {
	if c.wf == nil {
		return false
	}
	for _, e := range c.wf.On {
		w, ok := e.(*WebhookEvent)
		if !ok || w.Hook == nil {
			continue
		}
		for _, name := range events {
			if w.Hook.Value == name {
				return true
			}
		}
	}
	return false
}

// windowsRunner reports whether the job certainly runs on Windows: one of its `runs-on` labels is a Windows
// runner. A runner chosen with an expression is not known.
func (c *runContext) windowsRunner() bool {
	if c.job == nil || c.job.RunsOn == nil {
		return false
	}
	for _, l := range c.job.RunsOn.Labels {
		if l != nil && strings.Contains(strings.ToLower(l.Value), "windows") {
			return true
		}
	}
	return false
}

// shellName returns the lower case name of the shell that runs the script of the step: the `shell:` of the step,
// else the default of the job, else the default of the workflow, else the default of the runner. It is "" when the
// shell is chosen with an expression or is a custom command.
func (c *runContext) shellName(run *ExecRun) string {
	var s *String
	switch {
	case run.Shell != nil:
		s = run.Shell
	case c.job != nil && c.job.Defaults != nil && c.job.Defaults.Run != nil && c.job.Defaults.Run.Shell != nil:
		s = c.job.Defaults.Run.Shell
	case c.wf != nil && c.wf.Defaults != nil && c.wf.Defaults.Run != nil && c.wf.Defaults.Run.Shell != nil:
		s = c.wf.Defaults.Run.Shell
	}
	if s == nil {
		if c.windowsRunner() {
			return "pwsh"
		}
		return "bash"
	}
	if s.ContainsExpression() {
		return ""
	}
	f := strings.Fields(s.Value)
	if len(f) == 0 {
		return ""
	}
	name := f[0]
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(strings.ToLower(name), ".exe")
}

// script analyzes the `run:` script of the step with the shell that runs it. It returns nil when the shell is not
// bash or sh or when the script does not parse; such a script yields no findings.
func (c *runContext) script(run *ExecRun) (*runscript.Script, runscript.Origin) {
	if run == nil || run.Run == nil {
		return nil, runscript.Origin{}
	}
	shell := c.shellName(run)
	if shell != "bash" && shell != "sh" {
		return nil, runscript.Origin{}
	}
	s, err := runscript.Analyze(run.Run.Value, shell)
	if err != nil {
		return nil, runscript.Origin{}
	}
	return s, run.Run.scriptOrigin()
}

// envValue returns the value of the environment variable as seen by the step: the step overrides the job and the
// job overrides the workflow. It returns false when the variable is not set in the workflow file or when an
// `env:` is given as an expression.
func (c *runContext) envValue(step *Step, name string) (*String, bool) {
	key := strings.ToLower(name)
	for _, env := range []*Env{step.Env, c.jobEnv(), c.workflowEnv()} {
		if env == nil || env.Vars == nil {
			continue
		}
		if v, ok := env.Vars[key]; ok && v != nil && v.Value != nil {
			return v.Value, true
		}
	}
	return nil, false
}

func (c *runContext) jobEnv() *Env {
	if c.job == nil {
		return nil
	}
	return c.job.Env
}

func (c *runContext) workflowEnv() *Env {
	if c.wf == nil {
		return nil
	}
	return c.wf.Env
}

// grantsIDToken reports whether the job can request an OIDC token: it (or, when it has no permissions, the
// workflow) grants `id-token: write`.
func (c *runContext) grantsIDToken() bool {
	var p *Permissions
	switch {
	case c.job != nil && c.job.Permissions != nil:
		p = c.job.Permissions
	case c.wf != nil:
		p = c.wf.Permissions
	}
	if p == nil {
		return false
	}
	if p.All != nil {
		return strings.EqualFold(p.All.Value, "write-all")
	}
	s, ok := p.Scopes["id-token"]
	return ok && s != nil && s.Value != nil && strings.EqualFold(s.Value.Value, "write")
}

// permissionsComeFromCaller reports whether the permissions of the job are not decided by this file: a reusable
// workflow without `permissions:` runs with what its caller grants.
func (c *runContext) permissionsComeFromCaller() bool {
	if c.wf == nil || c.job == nil || c.job.Permissions != nil || c.wf.Permissions != nil {
		return false
	}
	_, ok := c.wf.FindWorkflowCallEvent()
	return ok
}

// scriptPos maps an offset of the analyzed script to a position in the workflow file.
func scriptPos(s *runscript.Script, o runscript.Origin, offset int) *Pos {
	p := s.Position(o, offset)
	return &Pos{Line: p.Line, Col: p.Col}
}

// commandRange returns the positions of the start and the end of a command of a script in the workflow file.
func commandRange(s *runscript.Script, o runscript.Origin, c *runscript.Command) (start, end *Pos) {
	return scriptPos(s, o, c.Offset), scriptPos(s, o, c.End)
}

// errorIDAt reports a finding at a position. It returns the reported error so that the caller can attach a fix or
// an end position. The name matches what TestRuleIDsInSourceMatchRegistry looks for: the first argument must be
// the literal ID.
//
// The error shows its ID instead of the name of the rule as its label. The rules of this batch report several IDs,
// and the label is what people copy into the "rules" section of the configuration.
func (r *RuleBase) errorIDAt(id string, pos *Pos, msg string) *Error {
	r.ReportID(id, pos, msg)
	e := r.errs[len(r.errs)-1]
	e.Kind = id
	return e
}

// endAt sets the end of the region of the error to the position just after it.
func (e *Error) endAt(end *Pos) *Error {
	e.EndLine, e.EndColumn = end.Line, end.Col
	return e
}

// fileOffsets converts the positions of the syntax tree (a line and a column counted in code points, both 1-based)
// into the byte offsets which the edits of a Fix use.
type fileOffsets struct {
	src    []byte
	starts []int // byte offset of the start of each line
	// valid is false when the source uses a line break other than "\n" or "\r\n", which the YAML parser counts as a
	// line break and this index does not, so positions cannot be converted reliably.
	valid bool
}

func newFileOffsets(src []byte) *fileOffsets {
	f := &fileOffsets{src: src, starts: []int{0}, valid: true}
	for i, b := range src {
		switch {
		case b == '\n':
			f.starts = append(f.starts, i+1)
		case b == '\r' && (i+1 >= len(src) || src[i+1] != '\n'):
			f.valid = false
		}
	}
	// NEL, LS and PS are line breaks for the YAML parser
	if bytes.Contains(src, []byte("\u0085")) || bytes.Contains(src, []byte("\u2028")) || bytes.Contains(src, []byte("\u2029")) {
		f.valid = false
	}
	return f
}

// offset returns the byte offset of a position. It returns false when the position is outside of the source.
func (f *fileOffsets) offset(line, col int) (int, bool) {
	if f == nil || !f.valid || line < 1 || line > len(f.starts) || col < 1 {
		return 0, false
	}
	off := f.starts[line-1]
	end := len(f.src)
	if line < len(f.starts) {
		end = f.starts[line] - 1
	}
	if end > f.starts[line-1] && f.src[end-1] == '\r' {
		end--
	}
	for i := 1; i < col; i++ {
		if off >= end {
			return 0, false
		}
		_, n := utf8.DecodeRune(f.src[off:])
		off += n
	}
	return off, true
}
