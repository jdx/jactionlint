package jactionlint

import (
	"bytes"
	"strings"
)

// This file has the helpers shared by the rules of batch A (anonymous-definition,
// concurrency-limits, secrets-inherit, insecure-commands, dangerous-triggers, self-hosted-runner,
// unsound-contains, overprovisioned-secrets, unredacted-secrets, secrets-outside-env,
// typosquat-uses and forbidden-uses).

// reportWithFix reports an error like RuleBase.ReportID and attaches an automatic fix to it.
func reportWithFix(r *RuleBase, id string, pos *Pos, msg string, fix *Fix) {
	r.ReportID(id, pos, msg)
	r.errs[len(r.errs)-1].Fix = fix
}

// srcLineStart returns the byte offset of the first byte of the 1-based line, or -1.
func srcLineStart(src []byte, line int) int {
	if line < 1 {
		return -1
	}
	off := 0
	for i := 1; i < line; i++ {
		j := bytes.IndexByte(src[off:], '\n')
		if j < 0 {
			return -1
		}
		off += j + 1
	}
	return off
}

// srcLineEnd returns the byte offset just after the last byte of the line, excluding its line
// break ("\n" or "\r\n").
func srcLineEnd(src []byte, start int) int {
	j := bytes.IndexByte(src[start:], '\n')
	if j < 0 {
		return len(src)
	}
	end := start + j
	if end > start && src[end-1] == '\r' {
		end--
	}
	return end
}

// srcLineBreak returns the line break used by the source: "\r\n" when the first line ends with it.
func srcLineBreak(src []byte) string {
	if i := bytes.IndexByte(src, '\n'); i > 0 && src[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// exprOccurrence is one parsed ${{ }} expression (or a bare expression in an "if:" condition).
type exprOccurrence struct {
	// Root is the root of the expression syntax tree.
	Root ExprNode
	// Str is the string value which contains the expression.
	Str *String
	// Cond is true when the string is an "if:" condition.
	Cond bool

	baseLine, baseCol int
}

// PosOf returns the position in the source of the node.
func (o *exprOccurrence) PosOf(n ExprNode) *Pos {
	t := n.Token()
	if t == nil {
		return o.Str.Pos
	}
	p := &Pos{Line: t.Line - 1 + o.baseLine, Col: t.Column - 1 + o.baseCol}
	if t.Line > 1 && o.Str.Indent > 0 {
		p.Col = o.Str.Indent + t.Column
	}
	return p
}

// scanExpressions calls f for every expression in the string. Strings which do not parse are
// skipped: the expression rule reports them.
func scanExpressions(s *String, cond bool, f func(o *exprOccurrence)) {
	if s == nil || s.Pos == nil || s.Value == "" {
		return
	}
	if cond && !s.ContainsExpression() {
		l := NewExprLexer(s.Value + "}}") // }} is necessary since lexer lexes it as end of tokens
		expr, err := NewExprParser().Parse(l)
		if err != nil || expr == nil {
			return
		}
		o := &exprOccurrence{Root: expr, Str: s, Cond: true, baseLine: s.Pos.Line, baseCol: s.Pos.Col}
		if s.Indent > 0 {
			// The content of a literal block starts on the line after the "|"
			o.baseLine, o.baseCol = s.Pos.Line+1, s.Indent+1
		}
		f(o)
		return
	}

	line, col := s.Pos.Line, s.Pos.Col
	if s.Quoted {
		col++
	}
	full := s.Value
	rest := full
	offset := 0
	for {
		idx := strings.Index(rest, "${{")
		if idx < 0 {
			return
		}
		start := idx + 3
		rest = rest[start:]
		offset += start
		l, c := line, col+offset
		if s.Indent > 0 {
			before := full[:offset]
			l = s.Pos.Line + 1 + strings.Count(before, "\n")
			c = s.Indent + 1 + offset - (strings.LastIndexByte(before, '\n') + 1)
		}
		lex := NewExprLexer(rest)
		expr, err := NewExprParser().Parse(lex)
		if err != nil || expr == nil {
			return
		}
		f(&exprOccurrence{Root: expr, Str: s, Cond: cond, baseLine: l, baseCol: c})
		n := lex.Offset()
		if n == 0 {
			return
		}
		rest = rest[n:]
		offset += n
	}
}

// exprSite is a string of a workflow which can contain expressions.
type exprSite struct {
	// Job is the job which contains the string. It is nil for the strings of the workflow itself.
	Job *Job
	Str *String
	// Cond is true for "if:" conditions.
	Cond bool
}

// workflowExprSites calls f for every string of the workflow which can hold expressions.
func workflowExprSites(w *Workflow, f func(site exprSite)) {
	emit := func(j *Job, s *String, cond bool) {
		if s != nil {
			f(exprSite{Job: j, Str: s, Cond: cond})
		}
	}
	env := func(j *Job, e *Env) {
		if e == nil {
			return
		}
		emit(j, e.Expression, false)
		for _, v := range e.Vars {
			emit(j, v.Value, false)
		}
	}
	defaults := func(j *Job, d *Defaults) {
		if d != nil && d.Run != nil {
			emit(j, d.Run.Shell, false)
			emit(j, d.Run.WorkingDirectory, false)
		}
	}
	conc := func(j *Job, c *Concurrency) {
		if c != nil {
			emit(j, c.Group, false)
		}
	}
	container := func(j *Job, c *Container) {
		if c == nil {
			return
		}
		emit(j, c.Image, false)
		if c.Credentials != nil {
			emit(j, c.Credentials.Expression, false)
			emit(j, c.Credentials.Username, false)
			emit(j, c.Credentials.Password, false)
		}
		env(j, c.Env)
		emit(j, c.Options, false)
	}

	emit(nil, w.Name, false)
	emit(nil, w.RunName, false)
	env(nil, w.Env)
	defaults(nil, w.Defaults)
	conc(nil, w.Concurrency)

	for _, j := range w.Jobs {
		if j == nil {
			continue
		}
		emit(j, j.Name, false)
		emit(j, j.If, true)
		if j.RunsOn != nil {
			emit(j, j.RunsOn.LabelsExpr, false)
			emit(j, j.RunsOn.Group, false)
			for _, l := range j.RunsOn.Labels {
				emit(j, l, false)
			}
		}
		env(j, j.Env)
		defaults(j, j.Defaults)
		conc(j, j.Concurrency)
		if j.Environment != nil {
			emit(j, j.Environment.Name, false)
			emit(j, j.Environment.URL, false)
		}
		for _, o := range j.Outputs {
			emit(j, o.Value, false)
		}
		container(j, j.Container)
		if j.Services != nil {
			emit(j, j.Services.Expression, false)
			for _, s := range j.Services.Value {
				if s != nil {
					container(j, s.Container)
				}
			}
		}
		if j.Strategy != nil && j.Strategy.Matrix != nil {
			emit(j, j.Strategy.Matrix.Expression, false)
		}
		if c := j.WorkflowCall; c != nil {
			emit(j, c.Uses, false)
			for _, i := range c.Inputs {
				emit(j, i.Value, false)
			}
			for _, s := range c.Secrets {
				emit(j, s.Value, false)
			}
		}
		walkSteps(j.Steps, func(s *Step) {
			emit(j, s.Name, false)
			emit(j, s.If, true)
			env(j, s.Env)
			switch e := s.Exec.(type) {
			case *ExecRun:
				emit(j, e.Run, false)
				emit(j, e.Shell, false)
				emit(j, e.WorkingDirectory, false)
			case *ExecAction:
				emit(j, e.Uses, false)
				for _, in := range e.Inputs {
					emit(j, in.Value, false)
				}
			}
		})
	}
}

// walkSteps calls f for every step, including the steps nested in "parallel:" groups.
func walkSteps(steps []*Step, f func(*Step)) {
	for _, s := range steps {
		if s == nil {
			continue
		}
		f(s)
		if p, ok := s.Exec.(*ExecParallel); ok && p != nil {
			walkSteps(p.Steps, f)
		}
	}
}

// workflowExprs calls f for every expression of the workflow.
func workflowExprs(w *Workflow, f func(site exprSite, o *exprOccurrence)) {
	workflowExprSites(w, func(site exprSite) {
		scanExpressions(site.Str, site.Cond, func(o *exprOccurrence) { f(site, o) })
	})
}

// isContextVariable reports whether the node is a reference to the named context, e.g. "secrets".
func isContextVariable(n ExprNode, name string) bool {
	v, ok := n.(*VariableNode)
	return ok && strings.EqualFold(v.Name, name)
}

// secretNameOf returns the name of the secret when the node is a direct access to one
// ("secrets.NAME" or "secrets['NAME']").
func secretNameOf(n ExprNode) (string, bool) {
	switch n := n.(type) {
	case *ObjectDerefNode:
		if isContextVariable(n.Receiver, "secrets") {
			return n.Property, true
		}
	case *IndexAccessNode:
		if s, ok := n.Index.(*StringNode); ok && isContextVariable(n.Operand, "secrets") {
			return s.Value, true
		}
	}
	return "", false
}
