package runscript

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Word is one shell word: a command name, an argument or a redirect target.
type Word struct {
	Loc
	// Raw is the word as written in the script.
	Raw string
	// Value is the word with quotes and backslashes removed. Expansions are kept as written (`$FOO`,
	// `${FOO:-x}`, `$(cmd)`) and `${{ }}` expressions are restored as written, so a word is never "half
	// replaced": `--version=${{ matrix.v }}` has that exact value.
	Value string
	// Exprs are the inner texts of the `${{ }}` expressions in the word, in order.
	Exprs []string
	// Vars are the names of the shell parameters expanded in the word, in order (`$A`, `${B:-x}`, `"$C"`).
	Vars []string
	// Quoted is whether any part of the word is quoted.
	Quoted bool
	// Glob is whether an unquoted part may expand to several files or to the home directory (`*`, `?`, `[`, `~`).
	Glob bool
	// Subst is whether the word contains a command substitution, process substitution or arithmetic expansion.
	Subst bool
	// ProcSubst is whether the word starts with a process substitution (`<(cmd)`).
	ProcSubst bool
	// Subs are the commands inside command and process substitutions of the word, at any depth.
	Subs []*Command

	plainVar string
	whole    bool // the whole word is exactly one ${{ }} expression
}

// HasExpr returns whether the word contains a `${{ }}` expression.
func (w *Word) HasExpr() bool { return len(w.Exprs) > 0 }

// Literal returns whether the word is a constant: no expression, no expansion, no glob.
func (w *Word) Literal() bool {
	return len(w.Exprs) == 0 && len(w.Vars) == 0 && !w.Subst && !w.Glob && w.plainVar == "" && !strings.Contains(w.Raw, "$")
}

// Dynamic returns whether the value is not known statically (expression, variable or substitution).
func (w *Word) Dynamic() bool {
	return len(w.Exprs) > 0 || len(w.Vars) > 0 || w.Subst || strings.Contains(w.Raw, "$")
}

// Var returns the name of the parameter if the word is exactly one plain parameter expansion: `$NAME`,
// `${NAME}` and their double quoted forms. It is false for everything else: `$NAME_x` is the parameter NAME_x,
// `${NAME:-x}` and `$NAME/y` are not plain.
func (w *Word) Var() (string, bool) { return w.plainVar, w.plainVar != "" }

// IsExpr returns whether the whole word is a single `${{ }}` expression (and nothing else).
func (w *Word) IsExpr() bool { return w.whole }

func (w *Word) String() string { return w.Value }

// word converts a syntax word. It returns nil for a nil word.
func (b *builder) word(w *syntax.Word) *Word {
	if w == nil {
		return nil
	}
	start, end := int(w.Pos().Offset()), int(w.End().Offset())
	out := &Word{Loc: b.s.loc(start, end), Raw: b.src(start, end)}
	_, out.whole = b.s.exprSpans[start]
	out.whole = out.whole && b.s.exprSpans[start] == end
	var sb strings.Builder
	b.parts(out, w.Parts, false, &sb)
	value, exprs := b.s.restore(sb.String())
	out.Value, out.Exprs = value, exprs
	if len(w.Parts) == 1 {
		if _, ok := w.Parts[0].(*syntax.CmdSubst); ok {
			out.Subst = true
		}
		if _, ok := w.Parts[0].(*syntax.ProcSubst); ok {
			out.Subst, out.ProcSubst = true, true
		}
	}
	out.plainVar = plainVar(w)
	return out
}

func plainVar(w *syntax.Word) string {
	parts := w.Parts
	if len(parts) == 1 {
		if dq, ok := parts[0].(*syntax.DblQuoted); ok {
			parts = dq.Parts
		}
	}
	if len(parts) != 1 {
		return ""
	}
	p, ok := parts[0].(*syntax.ParamExp)
	if !ok || p.Param == nil || p.Excl || p.Length || p.Width || p.Index != nil || p.Slice != nil || p.Repl != nil || p.Exp != nil || p.Flags != nil {
		return ""
	}
	return p.Param.Value
}

// parts renders the parts of a word into sb (placeholders still in place) and records its properties.
func (b *builder) parts(w *Word, parts []syntax.WordPart, inDQ bool, sb *strings.Builder) {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if !inDQ && strings.ContainsAny(p.Value, "*?[") {
				w.Glob = true
			}
			if !inDQ && strings.HasPrefix(p.Value, "~") && sb.Len() == 0 {
				w.Glob = true
			}
			sb.WriteString(unescape(p.Value, inDQ))
		case *syntax.SglQuoted:
			w.Quoted = true
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			w.Quoted = true
			b.parts(w, p.Parts, true, sb)
		case *syntax.ParamExp:
			if p.Param != nil {
				w.Vars = append(w.Vars, p.Param.Value)
			}
			sb.WriteString(b.src(int(p.Pos().Offset()), int(p.End().Offset())))
			// nested expansions: ${A:-$(cmd)}
			b.collectSubs(w, p)
		case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp:
			w.Subst = true
			sb.WriteString(b.src(int(part.Pos().Offset()), int(part.End().Offset())))
		default:
			// extended globs, brace expansions, ...
			sb.WriteString(b.src(int(part.Pos().Offset()), int(part.End().Offset())))
			if _, ok := part.(*syntax.ExtGlob); ok {
				w.Glob = true
			}
		}
	}
}

func (b *builder) collectSubs(w *Word, n syntax.Node) {
	syntax.Walk(n, func(n syntax.Node) bool {
		switch n.(type) {
		case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp:
			w.Subst = true
		}
		return true
	})
}

// unescape removes backslashes. Inside double quotes only \$ \` \" \\ and a line continuation are escapes.
func unescape(s string, inDQ bool) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			n := s[i+1]
			switch {
			case n == '\n':
				i++
				continue
			case !inDQ || n == '$' || n == '`' || n == '"' || n == '\\':
				i++
				sb.WriteByte(n)
				continue
			}
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}
