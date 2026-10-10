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
	return newLineIndex(src).startOf(line)
}

// startOf is srcLineStart for the source of the index.
func (x *lineIndex) startOf(line int) int {
	if line < 1 || line > len(x.starts) {
		return -1
	}
	return x.starts[line-1]
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
	// valueOff is the byte offset in Str.Value where the text of the expression starts, and text is
	// the rest of the value from there. They give the exact position of a token when the string
	// knows where it is written in the file.
	valueOff int
	text     string
}

// PosOf returns the position in the source of the node.
func (o *exprOccurrence) PosOf(n ExprNode) *Pos {
	t := n.Token()
	if t == nil {
		return o.Str.Pos
	}
	if l, c, ok := o.Str.valueAt(o.valueOff + exprTextOffset(o.text, t.Line, t.Column)); ok {
		return &Pos{Line: l, Col: c}
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
		expr, err := s.condExpr()
		if err != nil || expr == nil {
			return
		}
		o := &exprOccurrence{Root: expr, Str: s, Cond: true, baseLine: s.Pos.Line, baseCol: s.Pos.Col, text: s.Value}
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
	for _, p := range s.placeholders().list {
		offset := p.after
		l, c := line, col+offset
		if s.Indent > 0 {
			before := full[:offset]
			l = s.Pos.Line + 1 + strings.Count(before, "\n")
			c = s.Indent + 1 + offset - (strings.LastIndexByte(before, '\n') + 1)
		}
		f(&exprOccurrence{Root: p.node, Str: s, Cond: cond, baseLine: l, baseCol: c, valueOff: offset, text: full[offset:]})
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

// workflowExprSites calls f for every string of the workflow which can hold an expression. It
// visits every field of the AST which the parser reads as a string, a boolean, a number or a
// matrix value, so a rule which looks at expressions does not miss one in "strategy:", the timeouts
// or "continue-on-error:". TestWorkflowExprSitesVisitEveryExpression keeps it in sync with ast.go.
func workflowExprSites(w *Workflow, f func(site exprSite)) {
	emit := func(j *Job, s *String, cond bool) {
		if s != nil {
			f(exprSite{Job: j, Str: s, Cond: cond})
		}
	}
	emitAll := func(j *Job, ss []*String) {
		for _, s := range ss {
			emit(j, s, false)
		}
	}
	// The expression of a boolean or a number which is written as "${{ }}"
	emitBool := func(j *Job, b *Bool) {
		if b != nil {
			emit(j, b.Expression, false)
		}
	}
	emitInt := func(j *Job, i *Int) {
		if i != nil {
			emit(j, i.Expression, false)
		}
	}
	emitFloat := func(j *Job, x *Float) {
		if x != nil {
			emit(j, x.Expression, false)
		}
	}
	// A value of a matrix is any YAML value
	var emitRaw func(j *Job, v RawYAMLValue)
	emitRaw = func(j *Job, v RawYAMLValue) {
		switch v := v.(type) {
		case *RawYAMLString:
			if v == nil {
				return
			}
			if v.str != nil {
				emit(j, v.str, false)
			} else if v.pos != nil {
				emit(j, &String{Value: v.Value, Pos: v.pos}, false)
			}
		case *RawYAMLArray:
			if v != nil {
				for _, e := range v.Elems {
					emitRaw(j, e)
				}
			}
		case *RawYAMLObject:
			if v != nil {
				for _, e := range v.Props {
					emitRaw(j, e)
				}
			}
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
			emit(j, c.Queue, false)
			emitBool(j, c.CancelInProgress)
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
		emitAll(j, c.Ports)
		emitAll(j, c.Volumes)
		emit(j, c.Options, false)
		emit(j, c.Command, false)
		emit(j, c.Entrypoint, false)
	}
	combinations := func(j *Job, cs *MatrixCombinations) {
		if cs == nil {
			return
		}
		emit(j, cs.Expression, false)
		for _, c := range cs.Combinations {
			if c == nil {
				continue
			}
			emit(j, c.Expression, false)
			for _, a := range c.Assigns {
				if a != nil {
					emitRaw(j, a.Value)
				}
			}
		}
	}

	permissions := func(j *Job, p *Permissions) {
		if p == nil {
			return
		}
		emit(j, p.All, false)
		for _, sc := range p.Scopes {
			if sc != nil {
				emit(j, sc.Name, false)
				emit(j, sc.Value, false)
			}
		}
	}

	emit(nil, w.Name, false)
	emit(nil, w.RunName, false)
	permissions(nil, w.Permissions)
	emit(nil, w.CacheMode, false)
	env(nil, w.Env)
	defaults(nil, w.Defaults)
	conc(nil, w.Concurrency)
	for _, e := range w.On {
		// The value of an output of a reusable workflow is an expression
		if c, ok := e.(*WorkflowCallEvent); ok && c != nil {
			for _, o := range c.Outputs {
				if o != nil {
					emit(nil, o.Value, false)
				}
			}
			// The default of an input is evaluated with the github, inputs and vars contexts
			for _, in := range c.Inputs {
				if in != nil {
					emit(nil, in.Default, false)
				}
			}
		}
	}

	for _, j := range w.Jobs {
		if j == nil {
			continue
		}
		emit(j, j.Name, false)
		emit(j, j.If, true)
		emit(j, j.CacheMode, false)
		permissions(j, j.Permissions)
		if j.RunsOn != nil {
			emit(j, j.RunsOn.LabelsExpr, false)
			emit(j, j.RunsOn.Group, false)
			emitAll(j, j.RunsOn.Labels)
		}
		env(j, j.Env)
		defaults(j, j.Defaults)
		conc(j, j.Concurrency)
		if j.Environment != nil {
			emit(j, j.Environment.Name, false)
			emit(j, j.Environment.URL, false)
			emitBool(j, j.Environment.Deployment)
		}
		for _, o := range j.Outputs {
			if o != nil {
				emit(j, o.Value, false)
			}
		}
		emitFloat(j, j.TimeoutMinutes)
		emitBool(j, j.ContinueOnError)
		container(j, j.Container)
		if j.Services != nil {
			emit(j, j.Services.Expression, false)
			for _, s := range j.Services.Value {
				if s != nil {
					container(j, s.Container)
				}
			}
		}
		if st := j.Strategy; st != nil {
			emitBool(j, st.FailFast)
			emitInt(j, st.MaxParallel)
			if m := st.Matrix; m != nil {
				emit(j, m.Expression, false)
				for _, r := range m.Rows {
					if r == nil {
						continue
					}
					emit(j, r.Expression, false)
					for _, v := range r.Values {
						emitRaw(j, v)
					}
				}
				combinations(j, m.Include)
				combinations(j, m.Exclude)
			}
		}
		if c := j.WorkflowCall; c != nil {
			emit(j, c.Uses, false)
			for _, i := range c.Inputs {
				if i != nil {
					emit(j, i.Value, false)
				}
			}
			for _, s := range c.Secrets {
				if s != nil {
					emit(j, s.Value, false)
				}
			}
		}
		if sn := j.Snapshot; sn != nil {
			emit(j, sn.ImageName, false)
			emit(j, sn.Version, false)
			emit(j, sn.If, true)
		}
		walkSteps(j.Steps, func(s *Step) {
			emit(j, s.Name, false)
			emit(j, s.If, true)
			env(j, s.Env)
			emitBool(j, s.ContinueOnError)
			emitFloat(j, s.TimeoutMinutes)
			emitBool(j, s.Background)
			switch e := s.Exec.(type) {
			case *ExecRun:
				emit(j, e.Run, false)
				emit(j, e.Shell, false)
				emit(j, e.WorkingDirectory, false)
			case *ExecAction:
				emit(j, e.Uses, false)
				for _, in := range e.Inputs {
					if in != nil {
						emit(j, in.Value, false)
					}
				}
				emit(j, e.Entrypoint, false)
				emit(j, e.Args, false)
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
