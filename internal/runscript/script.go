// Package runscript analyzes the shell scripts of `run:` steps for the security rules of jactionlint.
//
// The scripts are parsed with mvdan.cc/sh (pure Go, no cgo). Analyze never panics and never needs a shell: a
// script which is not bash/sh, or which does not parse, yields an error that callers are expected to swallow
// (no findings, nothing shown to the user). The analyzer only describes the script. All judgement lives in the
// rules which use it.
//
// # Template expressions
//
// `${{ }}` expressions are not shell syntax. Before parsing, each expression is replaced by a placeholder token
// of exactly the same byte length (`X0___`), so every byte offset of the parsed text is also an offset of the
// original script. Words are always reported with the original expression text restored, see [Word].
//
// # Positions
//
// Every node has a [Loc] with byte offsets and a line/column inside the script. [Script.Position] maps that to
// a position in the YAML file given the [Origin] of the `run:` string:
//
//   - when the origin has a Locate function (the jactionlint AST gives one for every scalar it parsed from a file),
//     the position is exact for every style of scalar: escapes, folded lines and block scalars are followed;
//   - otherwise literal blocks (`run: |`, `|-`, `|+`) map exactly when the indentation is known (Origin.Indent > 0),
//     and folded blocks (`>`), plain and quoted scalars map approximately.
package runscript

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"
)

// ErrUnsupportedShell is returned by Analyze for shells other than bash and sh (pwsh, cmd, python, ...).
var ErrUnsupportedShell = errors.New("runscript: unsupported shell")

// ParseError is returned when the script could not be parsed. Callers must treat it as "no findings".
type ParseError struct {
	Err error
}

func (e *ParseError) Error() string { return "runscript: cannot parse script: " + e.Err.Error() }

// Unwrap returns the underlying parser error.
func (e *ParseError) Unwrap() error { return e.Err }

// SupportsShell returns whether scripts run with the shell (the value of `shell:`, empty for the default) are
// analyzed. Only bash and sh are. Note that the default shell of Windows runners is pwsh: callers which know
// that the job runs on Windows should skip scripts without `shell:` themselves.
func SupportsShell(shell string) bool {
	f := strings.Fields(shell)
	if len(f) == 0 {
		return true
	}
	name := f[0]
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".exe")
	return name == "bash" || name == "sh"
}

// InSplitList reports whether the byte offset is inside a word of the list of a `for` or `select` loop, or of the
// elements of an array assignment (`a=(x y)`, `declare -a a=(x)`, `a+=(x)`). The shell splits an unquoted
// expansion there into the items of the list on purpose, so quoting it would change the program.
func (s *Script) InSplitList(offset int) bool {
	for _, r := range s.splitWords {
		if offset >= r[0] && offset < r[1] {
			return true
		}
	}
	return false
}

// InAssignValue reports whether the byte offset is inside the value of a scalar assignment (`A=x`, `A=1 cmd`,
// `A+=x`), where the shell does not split the value into words. The arguments of a declaration builtin
// (`export A=x`, `local A=x`) are not counted, and the value of an array assignment is not one: see
// [Script.InSplitList].
func (s *Script) InAssignValue(offset int) bool {
	for _, a := range s.Assignments {
		if !a.Array && (a.Cmd == nil || !a.Cmd.Decl) && a.Value != nil && offset >= a.Value.Offset && offset < a.Value.End {
			return true
		}
	}
	return false
}

// Loc is the location of a node: byte offsets into the script and a 1-based line and column (in runes)
// inside the script, before the YAML indentation is considered. See [Script.Position].
type Loc struct {
	// Offset is the byte offset of the start. End is the byte offset right after the node.
	Offset, End int
	Line, Col   int
}

// Expr is a `${{ }}` expression of the script.
type Expr struct {
	// Text is the expression without `${{`, `}}` and surrounding spaces.
	Text string
	// Raw is the expression as written, including `${{ }}`.
	Raw string
	Loc Loc
}

// Script is the result of analyzing one script.
type Script struct {
	// Source is the analyzed script, unchanged.
	Source string
	// Exprs are all `${{ }}` expressions in the script, in source order, wherever they appear (comments and
	// heredocs included).
	Exprs []*Expr
	// Commands are all simple commands in source order, including the ones in pipelines, groups, control
	// structures, functions and command substitutions.
	Commands []*Command
	// Pipelines are the pipelines of two or more stages.
	Pipelines []*Pipeline
	// Redirects are all redirections in source order, including the ones on groups, heredocs and the files
	// `tee` writes to (Redirect.Tee).
	Redirects []*Redirect
	// Assignments are all variable assignments (`A=1`, `A=1 cmd`, `export A=1`).
	Assignments []*Assignment
	// Guards are the tests at the top level that end the script unless a variable has a checked shape.
	Guards []*Guard
	// Totals are the variables that an `if` or `case` at the top level sets on every path.
	Totals []*Total
	// ForVars are the variables of `for` loops.
	ForVars []*ForVar
	// Funcs are the names of the functions the script defines. A prefix assignment of a call to one of them
	// (`V=x fn`) is visible in its body.
	Funcs []string

	splitWords [][2]int // byte ranges of the words of for/select lists and array assignments

	lineStarts []int
	tokens     []string    // placeholder token of the expression with the same index; "" if none
	exprSpans  map[int]int // start offset -> end offset of each expression
	aliases    map[string]string
}

// Analyze parses a script. shell is the value of `shell:` (empty for the default). It returns
// ErrUnsupportedShell for shells other than bash and sh, and a *ParseError if the script does not parse. The
// returned Script is nil in both cases.
func Analyze(script, shell string) (s *Script, err error) {
	if !SupportsShell(shell) {
		return nil, ErrUnsupportedShell
	}
	defer func() {
		if r := recover(); r != nil {
			s, err = nil, &ParseError{fmt.Errorf("internal error: %v", r)}
		}
	}()

	s = &Script{Source: script}
	s.indexLines()
	sub := s.substitute()
	// The parser sees the script with the "\r" of each CRLF removed, so that a "\<newline>" continuation and a
	// heredoc delimiter keep their meaning. omap turns its offsets back into offsets of the original script.
	parsed, omap := stripCR(sub)
	f, perr := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(parsed), "")
	if perr != nil {
		return nil, &ParseError{perr}
	}
	b := &builder{s: s, sub: sub, omap: omap, cmds: map[syntax.Command]*Command{}}
	b.build(f)
	return s, nil
}

// stripCR removes the "\r" of every "\r\n". It returns the text and, when something was removed, a table which maps
// each offset of the text (and its end) to the offset in src; the table is nil when src has no CRLF.
func stripCR(src string) (string, []int) {
	if !strings.Contains(src, "\r\n") {
		return src, nil
	}
	out := make([]byte, 0, len(src))
	omap := make([]int, 0, len(src)+1)
	for i := 0; i < len(src); i++ {
		if src[i] == '\r' && i+1 < len(src) && src[i+1] == '\n' {
			continue
		}
		out = append(out, src[i])
		omap = append(omap, i)
	}
	omap = append(omap, len(src))
	return string(out), omap
}

func (s *Script) indexLines() {
	s.lineStarts = []int{0}
	for i := 0; i < len(s.Source); i++ {
		if s.Source[i] == '\n' {
			s.lineStarts = append(s.lineStarts, i+1)
		}
	}
}

// loc builds a Loc from byte offsets.
func (s *Script) loc(off, end int) Loc {
	if off < 0 {
		off = 0
	}
	if end < off {
		end = off
	}
	if off > len(s.Source) {
		off = len(s.Source)
	}
	if end > len(s.Source) {
		end = len(s.Source)
	}
	line := sort.Search(len(s.lineStarts), func(i int) bool { return s.lineStarts[i] > off }) // first start after off
	start := s.lineStarts[line-1]
	return Loc{off, end, line, utf8.RuneCountInString(s.Source[start:off]) + 1}
}

// substitute replaces each `${{ }}` with a same-length placeholder, collects s.Exprs and returns the result.
//
// The placeholder is `X`, the decimal index of the expression and underscores up to the length of the
// expression, so it is a valid word on its own, unique, and recognizable again by restore. Newlines in
// multi-line expressions become underscores as well so that the script stays well formed; positions are
// computed from the original script.
func (s *Script) substitute() string {
	src := s.Source
	if !strings.Contains(src, "${{") {
		return src
	}
	b := []byte(src)
	s.exprSpans = map[int]int{}
	for i := 0; i < len(src); {
		j := strings.Index(src[i:], "${{")
		if j < 0 {
			break
		}
		start := i + j
		end := exprEnd(src, start+3)
		if end < 0 {
			break // unterminated, GitHub rejects this. leave it alone
		}
		id := len(s.Exprs)
		n := end - start
		idStr := fmt.Sprint(id)
		token := ""
		if n >= 2+len(idStr) { // at least one underscore so the end of the token is unambiguous
			token = "X" + idStr + strings.Repeat("_", n-1-len(idStr))
		} else {
			token = "X" + strings.Repeat("_", n-1)
		}
		copy(b[start:end], token)
		if n < 2+len(idStr) {
			token = "" // not recoverable by restore, only listed in Exprs
		}
		raw := s.Source[start:end]
		s.Exprs = append(s.Exprs, &Expr{Text: strings.TrimSpace(raw[3 : len(raw)-2]), Raw: raw, Loc: s.loc(start, end)})
		s.tokens = append(s.tokens, token)
		s.exprSpans[start] = end
		i = end
	}
	return string(b)
}

// exprEnd returns the offset after the `}}` closing an expression whose body starts at i, or -1. String literals
// ('...' with ” as the escape) may contain `}}`.
func exprEnd(src string, i int) int {
	for i < len(src) {
		switch {
		case src[i] == '\'':
			i++
			for i < len(src) {
				if src[i] == '\'' {
					if i+1 < len(src) && src[i+1] == '\'' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			i++
		case strings.HasPrefix(src[i:], "}}"):
			return i + 2
		default:
			i++
		}
	}
	return -1
}

// restore puts the original expression text back into text derived from the placeholder-substituted source and
// returns the expressions (inner text) which were found.
func (s *Script) restore(text string) (string, []string) {
	if len(s.Exprs) == 0 || !strings.Contains(text, "X") {
		return text, nil
	}
	var sb strings.Builder
	var found []string
	last := 0
	for i := 0; i < len(text); i++ {
		if text[i] != 'X' {
			continue
		}
		j := i + 1
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		if j == i+1 || j-i > 7 {
			continue
		}
		id := 0
		for _, c := range text[i+1 : j] {
			id = id*10 + int(c-'0')
		}
		if id >= len(s.tokens) || s.tokens[id] == "" || !strings.HasPrefix(text[i:], s.tokens[id]) {
			continue
		}
		sb.WriteString(text[last:i])
		sb.WriteString(s.Exprs[id].Raw)
		found = append(found, s.Exprs[id].Text)
		i += len(s.tokens[id]) - 1
		last = i + 1
	}
	if last == 0 {
		return text, nil
	}
	sb.WriteString(text[last:])
	return sb.String(), found
}

// Origin describes where the analyzed string is in the YAML file.
type Origin struct {
	// Line and Col are the 1-based position of the YAML node: for a literal block the line of the `|` header.
	Line, Col int
	// Literal is whether the string is a literal block scalar (`|`).
	Literal bool
	// Indent is the number of spaces the YAML parser stripped from each content line of the literal block, 0 if
	// unknown. It is String.Indent of the jactionlint AST.
	Indent int
	// Quoted is whether the string is a quoted scalar.
	Quoted bool
	// Locate, when set, maps a byte offset of the script to its line and column (in code points) in the
	// YAML file, following the escapes, the folded lines and the indentation of the scalar. It is exact,
	// so it is used instead of the estimate below whenever it knows the offset.
	Locate func(offset int) (line, col int, ok bool)
}

// Position is a position in the YAML file.
type Position struct {
	Line, Col int
	// Exact is true when the position is guaranteed to be the position of the text in the file: literal blocks
	// with a known indentation, or whatever Origin.Locate gave. Otherwise it is a best effort which can be off.
	Exact bool
}

// Position maps a byte offset of the script to a position in the YAML file.
func (s *Script) Position(o Origin, offset int) Position {
	if o.Locate != nil {
		if line, col, ok := o.Locate(offset); ok {
			return Position{line, col, true}
		}
	}
	l := s.loc(offset, offset)
	return o.Map(l.Line, l.Col)
}

// Map maps a line and column inside the script to a position in the YAML file.
func (o Origin) Map(line, col int) Position {
	if o.Literal && o.Indent > 0 {
		return Position{o.Line + line, o.Indent + col, true}
	}
	if o.Literal { // unknown indentation: the line is right, the column is not
		return Position{o.Line + line, col, false}
	}
	p := Position{Line: o.Line + line - 1, Col: col}
	if line == 1 {
		p.Col = o.Col + col - 1
		if o.Quoted {
			p.Col++
		}
	}
	return p
}
