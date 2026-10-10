package runscript

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// This file has the facts about the order and the conditions of execution that a data-flow over the script needs.
// The rest of the package lists commands and assignments in source order without their surroundings; here the
// builder adds
//
//   - Cond on assignments and commands that may not run, or may not run in the shell that holds the variables:
//     the branches of if and case, loops, the right operand of && and ||, functions, subshells, substitutions,
//     pipelines and background jobs;
//   - [Guard]s: tests at the top level of the script after which the script only goes on when a variable has a
//     checked shape;
//   - [ForVar]s: the variable of a for loop.

// Guard is a statement at the top level of the script which ends the script unless Var has the shape that the
// test demands. After End the variable matches Regex (an extended regular expression, from `[[ v =~ re ]]`) or
// is one of Literals (from `[[ v == lit ]]` or a `case` whose other branches leave).
//
// A guard is only recorded when the proof is complete: the test is on exactly one variable, the failing outcome
// leaves the script with `exit` or `return` in the same list (or, for a bare `[[ ]]`, the default `-e` of the shell
// does it), and the polarity is right (`[[ ! v =~ re ]] || exit` is not a guard).
type Guard struct {
	Loc
	Var      string
	Regex    string
	Literals []string
	// Bare is whether the guard is a test on its own, which stops the script only when errexit is on (the shell
	// of the step runs with -e, or the script ran `set -e` before it).
	Bare bool
	// ScopeEnd is the offset where the proof ends, 0 for the end of the script. A guard inside a branch of an `if`
	// holds for the rest of that branch only.
	ScopeEnd int
}

// Total is an `if` (with an `else`) or a `case` (with a `*` branch) at the top level of the script that, on every
// path through it, either assigns the variable (`=`, not `+=`) in an unconditional statement of the branch or leaves
// the script. After it the variable has been set by one of the assignments inside, whichever branch ran.
type Total struct {
	Loc
	Name string
}

// ForVar is the variable of a `for NAME in ITEMS` loop.
type ForVar struct {
	Loc
	Name  string
	Items []*Word
}

type span struct{ from, to int }

func (b *builder) spanOf(n syntax.Node) span { return span{b.off(n.Pos()), b.offEnd(n.End())} }

func stmtsSpan(b *builder, list []*syntax.Stmt) (span, bool) {
	if len(list) == 0 {
		return span{}, false
	}
	return span{b.off(list[0].Pos()), b.offEnd(list[len(list)-1].End())}, true
}

// flow computes Cond, Guards and ForVars.
func (b *builder) flow(f *syntax.File) {
	var conds []span
	add := func(sp span, ok bool) {
		if ok {
			conds = append(conds, sp)
		}
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.IfClause:
			add(stmtsSpan(b, n.Then))
			if n.Else != nil {
				conds = append(conds, b.spanOf(n.Else))
			}
		case *syntax.WhileClause:
			conds = append(conds, b.spanOf(n))
		case *syntax.ForClause:
			conds = append(conds, b.spanOf(n))
			if it, ok := n.Loop.(*syntax.WordIter); ok && it.Name != nil {
				fv := &ForVar{Loc: b.s.loc(b.off(n.Pos()), b.offEnd(n.End())), Name: it.Name.Value}
				for _, w := range it.Items {
					fv.Items = append(fv.Items, b.word(w))
				}
				b.s.ForVars = append(b.s.ForVars, fv)
			}
		case *syntax.CaseClause:
			for _, it := range n.Items {
				add(stmtsSpan(b, it.Stmts))
			}
		case *syntax.BinaryCmd:
			switch n.Op {
			case syntax.AndStmt, syntax.OrStmt:
				conds = append(conds, b.spanOf(n.Y))
			case syntax.Pipe, syntax.PipeAll:
				conds = append(conds, b.spanOf(n))
			}
		case *syntax.FuncDecl:
			conds = append(conds, b.spanOf(n.Body))
		case *syntax.Subshell, *syntax.CmdSubst, *syntax.ProcSubst:
			conds = append(conds, b.spanOf(n))
		case *syntax.Stmt:
			if n.Background || n.Coprocess {
				conds = append(conds, b.spanOf(n))
			}
		}
		return true
	})
	in := func(off int) bool {
		for _, c := range conds {
			if off >= c.from && off < c.to {
				return true
			}
		}
		return false
	}
	for _, a := range b.s.Assignments {
		a.Cond = in(a.Offset)
	}
	for _, c := range b.s.Commands {
		c.Cond = in(c.Offset)
	}
	b.guards(f)
	b.totals(f)
}

// totals records the branch constructs at the top level that set a variable on every path.
func (b *builder) totals(f *syntax.File) {
	for _, st := range f.Stmts {
		if st.Background || st.Negated {
			continue
		}
		var names []string
		switch st.Cmd.(type) {
		case *syntax.IfClause, *syntax.CaseClause:
			syntax.Walk(st, func(n syntax.Node) bool {
				if a, ok := n.(*syntax.Assign); ok && a.Name != nil && !a.Append && a.Array == nil {
					names = append(names, a.Name.Value)
				}
				return true
			})
		default:
			continue
		}
		seen := map[string]bool{}
		for _, name := range names {
			if !seen[name] && b.defines(st, name) {
				b.s.Totals = append(b.s.Totals, &Total{Loc: b.s.loc(b.off(st.Pos()), b.offEnd(st.End())), Name: name})
			}
			seen[name] = true
		}
	}
}

// defines reports whether the statement sets the variable on every path or leaves the script.
func (b *builder) defines(st *syntax.Stmt, name string) bool {
	if st.Background || st.Negated {
		return false
	}
	switch c := st.Cmd.(type) {
	case *syntax.CallExpr:
		if len(c.Args) == 0 {
			for _, a := range c.Assigns {
				if a.Name != nil && a.Name.Value == name && !a.Append && a.Array == nil {
					return true
				}
			}
			return false
		}
		return b.leaves([]*syntax.Stmt{st})
	case *syntax.Block:
		return b.definesList(c.Stmts, name)
	case *syntax.IfClause:
		return b.definesList(c.Then, name) && c.Else != nil && b.definesElse(c.Else, name)
	case *syntax.CaseClause:
		star := false
		for _, it := range c.Items {
			if it.Op != syntax.Break || !b.definesList(it.Stmts, name) {
				return false
			}
			for _, p := range it.Patterns {
				if b.src(b.off(p.Pos()), b.offEnd(p.End())) == "*" {
					star = true
				}
			}
		}
		return star && len(c.Items) > 0
	}
	return false
}

func (b *builder) definesElse(e *syntax.IfClause, name string) bool {
	if len(e.Cond) == 0 { // else
		return b.definesList(e.Then, name)
	}
	return b.definesList(e.Then, name) && e.Else != nil && b.definesElse(e.Else, name)
}

func (b *builder) definesList(list []*syntax.Stmt, name string) bool {
	for _, st := range list {
		if b.defines(st, name) {
			return true
		}
	}
	return false
}

// leaves reports whether a list of statements ends the script by one of its own statements: `exit`, `return`.
func (b *builder) leaves(list []*syntax.Stmt) bool {
	for _, st := range list {
		if st.Background {
			continue
		}
		switch c := st.Cmd.(type) {
		case *syntax.CallExpr:
			if len(c.Args) > 0 {
				if v := b.word(c.Args[0]); !v.Dynamic() && (v.Value == "exit" || v.Value == "return") {
					return true
				}
			}
		case *syntax.Block:
			if b.leaves(c.Stmts) {
				return true
			}
		}
	}
	return false
}

// testShape is what a `[[ ]]` on one variable says. holds is the outcome of the test when the value has the shape.
type testShape struct {
	name     string
	regex    string
	literals []string
	holds    bool
}

// testOf reads a `[[ ]]` statement: one test on a plain variable, optionally negated.
func (b *builder) testOf(st *syntax.Stmt) (testShape, bool) {
	tc, ok := st.Cmd.(*syntax.TestClause)
	if !ok || st.Background {
		return testShape{}, false
	}
	holds := !st.Negated
	x := tc.X
	for {
		switch t := x.(type) {
		case *syntax.ParenTest:
			x = t.X
			continue
		case *syntax.UnaryTest:
			if t.Op != syntax.TsNot {
				return testShape{}, false
			}
			holds = !holds
			x = t.X
			continue
		}
		break
	}
	bt, ok := x.(*syntax.BinaryTest)
	if !ok {
		return testShape{}, false
	}
	l, ok := bt.X.(*syntax.Word)
	r, ok2 := bt.Y.(*syntax.Word)
	if !ok || !ok2 {
		return testShape{}, false
	}
	name, ok := b.word(l).Var()
	if !ok {
		return testShape{}, false
	}
	sh := testShape{name: name}
	switch bt.Op {
	case syntax.TsReMatch:
		if !allLit(r) {
			return testShape{}, false // a quoted pattern is a plain string there, not a regular expression
		}
		sh.regex = b.src(b.off(r.Pos()), b.offEnd(r.End()))
		sh.holds = holds
	case syntax.TsMatch, syntax.TsMatchShort, syntax.TsNoMatch:
		lit, ok := b.literalWord(r)
		if !ok {
			return testShape{}, false
		}
		sh.literals = []string{lit}
		sh.holds = holds
		if bt.Op == syntax.TsNoMatch {
			sh.holds = !holds
		}
	default:
		return testShape{}, false
	}
	return sh, true
}

func allLit(w *syntax.Word) bool {
	for _, p := range w.Parts {
		if _, ok := p.(*syntax.Lit); !ok {
			return false
		}
	}
	return len(w.Parts) > 0
}

// literalWord returns the value of a word that is a constant string and, if unquoted, has no glob character.
func (b *builder) literalWord(w *syntax.Word) (string, bool) {
	cw := b.word(w)
	if cw.Dynamic() || cw.Glob {
		return "", false
	}
	return cw.Value, true
}

// guards records the guards of the statements at the top level of the script.
func (b *builder) guards(f *syntax.File) {
	errexit := !strings.Contains(b.s.Source, "set +e") && !strings.Contains(b.s.Source, "set +o errexit")
	curScope := 0
	emit := func(st *syntax.Stmt, sh testShape) {
		_, bare := st.Cmd.(*syntax.TestClause)
		b.s.Guards = append(b.s.Guards, &Guard{Loc: b.s.loc(b.off(st.Pos()), b.offEnd(st.End())), Var: sh.name, Regex: sh.regex, Literals: sh.literals, Bare: bare, ScopeEnd: curScope})
	}
	var list func(stmts []*syntax.Stmt, scopeEnd int)
	// branches records the guards inside the bodies of an if: a guard there holds up to the end of its body.
	branches := func(c *syntax.IfClause) {
		for e := c; e != nil; e = e.Else {
			if end, ok := stmtsSpan(b, e.Then); ok {
				list(e.Then, end.to)
			}
		}
	}
	list = func(stmts []*syntax.Stmt, scopeEnd int) {
		prev := curScope
		curScope = scopeEnd
		defer func() { curScope = prev }()
		for _, st := range stmts {
			if st.Background {
				continue
			}
			switch c := st.Cmd.(type) {
			case *syntax.TestClause:
				// a bare test fails the script under -e: going on means the test held. Inside a branch the errexit
				// of the `if` may be suspended by its context, so only the top level counts
				if sh, ok := b.testOf(st); ok && errexit && sh.holds && scopeEnd == 0 {
					emit(st, sh)
				}
			case *syntax.BinaryCmd:
				if st.Negated || (c.Op != syntax.OrStmt && c.Op != syntax.AndStmt) {
					continue
				}
				sh, ok := b.testOf(c.X)
				if !ok || !b.leaves([]*syntax.Stmt{c.Y}) {
					continue
				}
				// `T || exit` goes on when T held, `T && exit` when it did not
				if (c.Op == syntax.OrStmt) == sh.holds {
					emit(st, sh)
				}
			case *syntax.IfClause:
				if st.Negated {
					continue
				}
				branches(c)
				if len(c.Cond) != 1 {
					continue
				}
				sh, ok := b.testOf(c.Cond[0])
				if !ok {
					continue
				}
				thenLeaves := b.leaves(c.Then)
				switch {
				case c.Else == nil && thenLeaves:
					// the body runs when the test held, going on means it did not
					if !sh.holds {
						emit(st, sh)
					}
				case c.Else != nil && len(c.Else.Cond) == 0 && !c.Else.ThenPos.IsValid() && !thenLeaves && b.leaves(c.Else.Then):
					if sh.holds {
						emit(st, sh)
					}
				}
			case *syntax.CaseClause:
				if g := b.caseGuard(st, c); g != nil {
					g.ScopeEnd = scopeEnd
					b.s.Guards = append(b.s.Guards, g)
				}
			}
		}
	}
	list(f.Stmts, 0)
}

// caseGuard reads `case "$v" in a|b) ;; *) exit 1 ;; esac`: the last branch is `*` and leaves, and every other
// branch has only constant patterns.
func (b *builder) caseGuard(st *syntax.Stmt, c *syntax.CaseClause) *Guard {
	name, ok := b.word(c.Word).Var()
	if !ok || len(c.Items) < 2 {
		return nil
	}
	var lits []string
	for i, it := range c.Items {
		if it.Op != syntax.Break {
			return nil
		}
		last := i == len(c.Items)-1
		if last {
			if len(it.Patterns) != 1 || b.src(b.off(it.Patterns[0].Pos()), b.offEnd(it.Patterns[0].End())) != "*" || !b.leaves(it.Stmts) {
				return nil
			}
			continue
		}
		if b.leaves(it.Stmts) {
			continue
		}
		for _, p := range it.Patterns {
			lit, ok := b.literalWord(p)
			if !ok {
				return nil
			}
			lits = append(lits, lit)
		}
	}
	if len(lits) == 0 {
		return nil
	}
	return &Guard{Loc: b.s.loc(b.off(st.Pos()), b.offEnd(st.End())), Var: name, Literals: lits}
}
