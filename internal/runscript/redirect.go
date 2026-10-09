package runscript

import (
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Redirect is a redirection (`>> file`, `2>&1`, `<<EOF`) or, with Tee set, a file `tee` writes to.
type Redirect struct {
	Loc
	// Op is the operator as written: > >> >| &> &>> < <> <& >& << <<- <<<. It is "tee" for Tee redirects.
	Op string
	// Fd is the file descriptor in front of the operator (`2` of `2>`), "" if there is none.
	Fd string
	// Target is the file, descriptor, here-string or the heredoc delimiter. It is nil for nothing.
	Target *Word
	// Append is whether the redirect appends (`>>`, `&>>`, `tee -a`).
	Append bool
	// Write is whether the redirect writes to a file named by Target (not to a descriptor like `>&2`).
	Write bool
	// Heredoc is the here document of `<<` and `<<-`.
	Heredoc *Heredoc
	// Cmd is the simple command the redirect is attached to, nil if it is attached to a group or to nothing.
	Cmd *Command
	// Group is whether the redirect is attached to a compound command (`{ ...; } >> f`, `( ... ) > f`,
	// `if ...; fi > f`, `while ...; done > f`). Inner are the simple commands inside it.
	Group bool
	Inner []*Command
	// Tee is whether this is not a real redirect but a file argument of the tee command (Cmd). The data comes
	// from the previous stages of the pipeline of Cmd.
	Tee bool
}

// Heredoc is a here document.
type Heredoc struct {
	Loc
	// Delim is the delimiter without quotes.
	Delim string
	// Quoted is whether the delimiter is quoted (`<<'EOF'`), which means the body is not expanded.
	Quoted bool
	// Tabs is whether leading tabs are stripped (`<<-`).
	Tabs bool
	// Body is the body as written, without the delimiter line.
	Body string
	// Exprs are the inner texts of the `${{ }}` expressions in the body.
	Exprs []string
}

type groupRedirect struct {
	r          *Redirect
	start, end int
}

func isPipe(op syntax.BinCmdOperator) bool { return op == syntax.Pipe || op == syntax.PipeAll }

// stmt attaches the redirections of a statement to the command or group they belong to.
func (b *builder) stmt(st *syntax.Stmt) {
	var owner *Command
	group := false
	switch c := st.Cmd.(type) {
	case *syntax.CallExpr:
		b.call(c)
		owner = b.cmds[c]
	case *syntax.DeclClause:
		b.decl(c)
		owner = b.cmds[c]
	case *syntax.BinaryCmd:
		if st.Negated && isPipe(c.Op) {
			b.negated[c] = true
		}
		group = true
	case nil:
	default:
		group = true
	}
	for _, r := range st.Redirs {
		red := b.redirect(r, owner)
		if group && owner == nil {
			red.Group = true
			b.groups = append(b.groups, groupRedirect{red, b.off(st.Cmd.Pos()), b.offEnd(st.Cmd.End())})
		}
	}
}

func (b *builder) redirect(r *syntax.Redirect, owner *Command) *Redirect {
	red := &Redirect{
		Loc:    b.s.loc(b.off(r.Pos()), b.offEnd(r.End())),
		Op:     r.Op.String(),
		Target: b.word(r.Word),
		Cmd:    owner,
	}
	if r.N != nil {
		red.Fd = r.N.Value
	}
	switch r.Op {
	case syntax.AppOut, syntax.AppAll:
		red.Append, red.Write = true, true
	case syntax.RdrOut, syntax.RdrClob, syntax.RdrAll, syntax.RdrInOut:
		red.Write = true
	case syntax.DplOut:
		// `>&2` and `>&-` duplicate descriptors, `>&file` writes stdout and stderr to the file
		if t := red.Target; t != nil && t.Value != "-" && !isDigits(t.Value) {
			red.Write = true
		}
	case syntax.Hdoc, syntax.DashHdoc:
		h := &Heredoc{Loc: red.Loc, Tabs: r.Op == syntax.DashHdoc}
		if red.Target != nil {
			h.Delim = red.Target.Value
			h.Quoted = red.Target.Quoted
		}
		if r.Hdoc != nil {
			start, end := b.off(r.Hdoc.Pos()), b.offEnd(r.Hdoc.End())
			h.Body = b.src(start, end)
			if i := strings.LastIndexByte(h.Body, '\n'); i >= 0 && strings.TrimLeft(h.Body[i+1:], "\t ") == h.Delim {
				h.Body = h.Body[:i+1] // the line with the closing delimiter belongs to the here document in the tree
			}
			h.Exprs = b.exprsIn(start, end)
			h.Loc = b.s.loc(start, end)
		}
		red.Heredoc = h
	}
	if owner != nil {
		owner.Redirects = append(owner.Redirects, red)
	}
	b.s.Redirects = append(b.s.Redirects, red)
	return red
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// exprsIn returns the inner texts of the expressions between the offsets.
func (b *builder) exprsIn(start, end int) []string {
	var out []string
	for _, e := range b.s.Exprs {
		if e.Loc.Offset >= start && e.Loc.End <= end {
			out = append(out, e.Text)
		}
	}
	return out
}

// directCommands returns the commands in [start, end) which are not inside a word (a command substitution) of
// another command. b.sorted must be set.
func (b *builder) directCommands(start, end int) []*Command {
	i := sort.Search(len(b.sorted), func(i int) bool { return b.sorted[i].Offset >= start })
	var in []*Command
	for ; i < len(b.sorted) && b.sorted[i].Offset < end; i++ {
		if b.sorted[i].End <= end {
			in = append(in, b.sorted[i])
		}
	}
	// union of the substitution words, as sorted disjoint intervals
	var iv [][2]int
	for _, c := range in {
		for _, w := range c.Words {
			if w.Subst {
				iv = append(iv, [2]int{w.Offset, w.End})
			}
		}
		for _, a := range c.Assigns {
			if a.Value != nil && a.Value.Subst {
				iv = append(iv, [2]int{a.Value.Offset, a.Value.End})
			}
		}
	}
	if len(iv) == 0 {
		return in
	}
	sort.Slice(iv, func(i, j int) bool { return iv[i][0] < iv[j][0] })
	merged := iv[:1]
	for _, x := range iv[1:] {
		if last := &merged[len(merged)-1]; x[0] <= last[1] {
			last[1] = max(last[1], x[1])
		} else {
			merged = append(merged, x)
		}
	}
	var out []*Command
	for _, c := range in {
		j := sort.Search(len(merged), func(j int) bool { return merged[j][1] > c.Offset })
		if j < len(merged) && merged[j][0] <= c.Offset && c.End <= merged[j][1] {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (b *builder) finishGroups() {
	for _, g := range b.groups {
		g.r.Inner = b.directCommands(g.start, g.end)
	}
	// tee writes to its file arguments
	for _, c := range b.s.Commands {
		if c.Tool != "tee" {
			continue
		}
		app := c.HasFlag("-a", "--append")
		for _, w := range c.Positional {
			b.s.Redirects = append(b.s.Redirects, &Redirect{Loc: w.Loc, Op: "tee", Target: w, Append: app, Write: true, Cmd: c, Tee: true})
		}
	}
}

func sortRedirects(rs []*Redirect) {
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].Offset < rs[j].Offset })
}

// Pipeline is a sequence of two or more commands connected with `|` or `|&`.
type Pipeline struct {
	Loc
	Stages  []*Stage
	Negated bool
	// Tested is whether the script looks at the exit status of the pipeline: it is the condition of `if`,
	// `elif`, `while` or `until`, or an operand of `&&` or `||` which is not the last of the list. A failure is
	// then not hidden but handled.
	Tested bool
}

// Stage is one element of a pipeline. It is a single command or a compound command, so it can contain several
// simple commands.
type Stage struct {
	Loc
	// Commands are the simple commands of the stage, without the ones in command substitutions.
	Commands []*Command
}

type pendingPipeline struct {
	p      *Pipeline
	stages []*syntax.Stmt
}

func (b *builder) pipeline(n *syntax.BinaryCmd) {
	var stmts []*syntax.Stmt
	var flatten func(bc *syntax.BinaryCmd)
	flatten = func(bc *syntax.BinaryCmd) {
		b.seenPipe[bc] = true
		if l, ok := bc.X.Cmd.(*syntax.BinaryCmd); ok && isPipe(l.Op) && len(bc.X.Redirs) == 0 && !bc.X.Negated {
			flatten(l)
		} else {
			stmts = append(stmts, bc.X)
		}
		stmts = append(stmts, bc.Y)
	}
	flatten(n)
	p := &Pipeline{Loc: b.s.loc(b.off(n.Pos()), b.offEnd(n.End())), Negated: b.negated[n], Tested: b.tested[n]}
	b.s.Pipelines = append(b.s.Pipelines, p)
	b.pending = append(b.pending, pendingPipeline{p, stmts})
}

func (b *builder) finishPipelines() {
	for _, pp := range b.pending {
		for _, st := range pp.stages {
			start, end := b.off(st.Pos()), b.offEnd(st.End())
			cs, ce := start, end
			if st.Cmd != nil { // a here document makes the statement extend over its body
				cs, ce = b.off(st.Cmd.Pos()), b.offEnd(st.Cmd.End())
			}
			stage := &Stage{Loc: b.s.loc(start, end), Commands: b.directCommands(cs, ce)}
			pp.p.Stages = append(pp.p.Stages, stage)
			idx := len(pp.p.Stages) - 1
			for _, c := range stage.Commands {
				c.Pipeline, c.Stage = pp.p, idx
			}
		}
	}
}

// aliases records `NAME=$VAR` assignments so that `>> $NAME` can be resolved to VAR.
func (b *builder) aliases() {
	b.s.aliases = map[string]string{}
	for _, a := range b.s.Assignments {
		if a.Value == nil || a.Append || a.Array {
			continue
		}
		if v, ok := a.Value.Var(); ok {
			b.s.aliases[a.Name] = v
		}
	}
}

// VarName resolves the word to the name of a shell variable, following `NAME=$VAR` assignments of the script.
// It returns false if the word is not exactly a plain parameter expansion.
func (s *Script) VarName(w *Word) (string, bool) {
	if w == nil {
		return "", false
	}
	v, ok := w.Var()
	if !ok {
		return "", false
	}
	for i := 0; i < 4; i++ {
		a, ok := s.aliases[v]
		if !ok || a == v {
			break
		}
		v = a
	}
	return v, true
}

// Write is a place where data is written to a file named by a shell variable, found by WritesTo.
type Write struct {
	// Var is the name of the variable, as asked for (not an alias).
	Var string
	// Redirect is the redirect or tee argument which writes the file.
	Redirect *Redirect
	// Append is whether the file is appended to.
	Append bool
	// Producers are the simple commands whose output goes into the file: the command itself or the commands of
	// the group, and the earlier stages of its pipeline.
	Producers []*Command
	// Heredoc is a here document which feeds the command, nil if there is none.
	Heredoc *Heredoc
	// Exprs are the inner texts of all `${{ }}` expressions in the producers' words and the heredoc.
	Exprs []string
}

// WritesTo returns all writes to files named by one of the variables (without `$`): `>> $GITHUB_ENV`,
// `> "${GITHUB_ENV}"`, `tee -a "$GITHUB_PATH"`, `{ ...; } >> $GITHUB_OUTPUT` and heredocs feeding them. The
// names must match exactly: `$GITHUB_ENV_x` and `$GITHUB_ENVIRONMENT` are other variables. Variables which are
// assigned from the variable (`F=$GITHUB_ENV; echo x >> "$F"`) are resolved. Targets which are only built from
// the variable (`$GITHUB_ENV.bak`, `$RUNNER_TEMP/$GITHUB_ENV`) do not match.
func (s *Script) WritesTo(varNames ...string) []*Write {
	var out []*Write
	for _, r := range s.Redirects {
		if !r.Write {
			continue
		}
		v, ok := s.VarName(r.Target)
		if !ok {
			continue
		}
		matched := false
		for _, n := range varNames {
			if n == v {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		w := &Write{Var: v, Redirect: r, Append: r.Append}
		switch {
		case r.Group:
			w.Producers = append(w.Producers, r.Inner...)
		case r.Cmd != nil:
			w.Producers = append(w.Producers, r.Cmd)
		}
		if r.Cmd != nil && r.Cmd.Pipeline != nil {
			for _, st := range r.Cmd.Pipeline.Stages[:r.Cmd.Stage] {
				w.Producers = append(w.Producers, st.Commands...)
			}
		}
		for _, c := range w.Producers {
			for _, o := range c.Redirects {
				if o.Heredoc != nil {
					w.Heredoc = o.Heredoc
				}
			}
			for _, o := range c.Redirects {
				if o.Op == "<<<" && o.Target != nil { // here-string
					w.Exprs = append(w.Exprs, o.Target.Exprs...)
				}
			}
			for _, word := range c.Words {
				w.Exprs = append(w.Exprs, word.Exprs...)
			}
			for _, a := range c.Assigns {
				if a.Value != nil {
					w.Exprs = append(w.Exprs, a.Value.Exprs...)
				}
			}
		}
		if w.Heredoc != nil {
			w.Exprs = append(w.Exprs, w.Heredoc.Exprs...)
		}
		out = append(out, w)
	}
	return out
}

// markTested records that the script handles the exit status of the statements: the pipelines and commands in
// them are Tested. It follows the places where the shell ignores `set -e`: the operands of `&&` and `||` (all but
// the last one of the list, which the caller selects), and the commands of groups and subshells there.
func (b *builder) markTested(stmts ...*syntax.Stmt) {
	for _, st := range stmts {
		if st == nil {
			continue
		}
		switch c := st.Cmd.(type) {
		case *syntax.BinaryCmd:
			if isPipe(c.Op) {
				b.tested[c] = true
			} else {
				b.markTested(c.X, c.Y)
			}
		case *syntax.CallExpr:
			b.testedCalls[c] = true
		case *syntax.Block:
			b.markTested(c.Stmts...)
		case *syntax.Subshell:
			b.markTested(c.Stmts...)
		}
	}
}
