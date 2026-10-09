package jactionlint

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// exprSpan is one ${{ }} placeholder inside a String. Rules which analyze the expressions of a string
// by themselves (template injection, obfuscation, bot conditions) use scanExprs instead of the
// machinery of the expression type checker.
type exprSpan struct {
	// Start is the byte offset in String.Value of the "${{".
	Start int
	// End is the byte offset in String.Value just after the "}}".
	End int
	// Src is the text between "${{" and "}}".
	Src string
	// Node is the syntax tree of Src.
	Node ExprNode
	// Line and Col are the source position of the first character after "${{". Tokens of Node are
	// relative to it.
	Line, Col int

	str *String
	idx *sourceIndex
	// base is the offset in str.Value where Src starts.
	base int
}

// Text returns the whole placeholder including "${{" and "}}", given the string it was scanned from.
func (s *exprSpan) Text(str *String) string {
	return str.Value[s.Start:s.End]
}

// TokPos returns the source position of a token of Node.
func (s *exprSpan) TokPos(t *Token) *Pos {
	return tokenPos(s.idx, s.str, s.base, t)
}

// tokenPos returns the source position of a token of an expression which starts at the offset base
// of the value of the string.
func tokenPos(idx *sourceIndex, str *String, base int, t *Token) *Pos {
	line, col := idx.valuePosition(str, base+t.Offset)
	return &Pos{line, col}
}

// valuePos returns the source position of the byte at offset off of the value of the string. For a
// literal block scalar it is the position in the block. For the other strings it assumes that the
// value is on one line, which holds for the first line of a multi-line scalar only.
func valuePos(s *String, off int) (line, col int) {
	if l, c, ok := s.valueAt(off); ok {
		return l, c
	}
	if s.Literal && s.Indent > 0 {
		before := s.Value[:off]
		nl := strings.Count(before, "\n")
		return s.Pos.Line + 1 + nl, s.Indent + 1 + utf8.RuneCountInString(before[strings.LastIndexByte(before, '\n')+1:])
	}
	col = s.Pos.Col + utf8.RuneCountInString(s.Value[:off])
	if s.Quoted {
		col++
	}
	return s.Pos.Line, col
}

// scanExprs finds the ${{ }} placeholders of the string. It stops at the first placeholder which
// does not parse; the expression rule reports such an error.
func scanExprs(s *String) []exprSpan {
	return (*sourceIndex)(nil).scanExprs(s)
}

// scanExprs is like the function of the same name. The source lets the positions of the tokens be
// exact in a string which spans several lines. It can be nil.
func (x *sourceIndex) scanExprs(s *String) []exprSpan {
	if s == nil {
		return nil
	}
	var ret []exprSpan
	value := s.Value
	pos := 0
	for {
		i := strings.Index(value[pos:], "${{")
		if i < 0 {
			return ret
		}
		start := pos + i
		after := start + len("${{")
		rest := value[after:]
		l := NewExprLexer(rest)
		node, err := NewExprParser().Parse(l)
		if err != nil {
			return ret
		}
		n := l.Offset()
		if n < 2 || rest[n-2:n] != "}}" {
			return ret
		}
		line, col := valuePos(s, after)
		ret = append(ret, exprSpan{
			Start: start,
			End:   after + n,
			Src:   rest[:n-2],
			Node:  node,
			Line:  line,
			Col:   col,
			str:   s,
			idx:   x,
			base:  after,
		})
		pos = after + n
	}
}

// parseExprString parses a condition or another value which is a whole expression, wrapped in ${{ }}
// or not. It returns the syntax tree and the byte offset in value where the expression text starts.
// The boolean is false when the value is not exactly one expression.
func parseWholeExpr(value string) (ExprNode, string, int, bool) {
	trimmed := strings.TrimSpace(value)
	lead := len(value) - len(strings.TrimLeft(value, " \t\r\n"))
	if strings.HasPrefix(trimmed, "${{") {
		if !isExprAssigned(value) {
			return nil, "", 0, false
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "${{"), "}}")
		node, err := NewExprParser().Parse(NewExprLexer(inner + "}}"))
		if err != nil {
			return nil, "", 0, false
		}
		return node, inner, lead + len("${{"), true
	}
	if strings.Contains(value, "${{") {
		return nil, "", 0, false
	}
	node, err := NewExprParser().Parse(NewExprLexer(trimmed + "}}"))
	if err != nil {
		return nil, "", 0, false
	}
	return node, trimmed, lead, true
}

// lineStart returns the offset of the first byte of the line.
func (x *sourceIndex) lineStart(line int) int {
	return x.lineStarts[line-1]
}

// valueOffset converts a byte offset in the value of the string to a byte offset in the source. The
// result is only a guess for strings with escapes or folded lines, so callers must compare the
// source at the offset with what they expect to find there.
func (x *sourceIndex) valueOffset(s *String, off int) (int, bool) {
	if x == nil || s == nil || off < 0 || off > len(s.Value) {
		return 0, false
	}
	if line, bcol, ok := s.valueByteAt(off); ok && line <= len(x.lineStarts) {
		return x.lineStarts[line-1] + bcol, true
	}
	if s.Literal && s.Indent > 0 {
		before := s.Value[:off]
		line := s.Pos.Line + 1 + strings.Count(before, "\n")
		if line > len(x.lineStarts) {
			return 0, false
		}
		return x.lineStarts[line-1] + s.Indent + off - (strings.LastIndexByte(before, '\n') + 1), true
	}
	if base, indent, ok := x.foldedBlockBase(s); ok {
		return x.walkFolded(s, base, off, indent)
	}
	base, ok := x.offset(s.Pos.Line, s.Pos.Col)
	if !ok {
		return 0, false
	}
	if s.Quoted {
		base++
	}
	if strings.Contains(s.Value[:off], "\n") {
		return 0, false
	}
	if x.multiline(s) {
		return x.walkFolded(s, base, off, -1)
	}
	return base + off, true
}

// walkFolded finds the source offset of the byte at off of the value of a scalar which is not a
// block scalar. The scalar starts at the source offset base. A line break inside such a scalar is
// folded into one space in the value. The function fails for anything else that makes the source
// differ from the value, like escapes.
func (x *sourceIndex) walkFolded(s *String, base, off int, blockIndent int) (int, bool) {
	i := base
	for j := 0; j < off; {
		if i >= len(x.src) {
			return 0, false
		}
		c := x.src[i]
		switch {
		case c == '\r' && i+1 < len(x.src) && x.src[i+1] == '\n':
			i++
		case c == s.Value[j] && c != '\n':
			i++
			j++
		case c == '\n' && (s.Value[j] == ' ' || (s.Value[j] == '\n' && blockIndent >= 0)):
			// A line break is folded into a space; in a block scalar a more indented line keeps it.
			i++
			for k := 0; i < len(x.src) && (x.src[i] == ' ' || x.src[i] == '\t') && (blockIndent < 0 || k < blockIndent); k++ {
				i++
			}
			j++
		default:
			return 0, false
		}
	}
	return i, true
}

// valuePosition returns the source position of the byte at off of the value of the string. Unlike
// valuePos it follows the line breaks of a scalar which spans several lines when the source is
// known. The index can be nil.
func (x *sourceIndex) valuePosition(s *String, off int) (line, col int) {
	if l, c, ok := s.valueAt(off); ok {
		return l, c
	}
	if x != nil && s.Literal && s.Indent > 0 {
		return x.literalPosition(s, off)
	}
	if x != nil {
		if base, indent, ok := x.foldedBlockBase(s); ok {
			if o, ok := x.walkFolded(s, base, off, indent); ok {
				return x.lineCol(o)
			}
		}
	}
	if x != nil && !(s.Literal && s.Indent > 0) && x.multiline(s) {
		if base, ok := x.offset(s.Pos.Line, s.Pos.Col); ok {
			if s.Quoted {
				base++
			}
			if o, ok := x.walkFolded(s, base, off, -1); ok {
				return x.lineCol(o)
			}
		}
	}
	return valuePos(s, off)
}

// literalPosition is valuePos for a literal block scalar, for many offsets of one string: the line
// breaks of the value are found once.
func (x *sourceIndex) literalPosition(s *String, off int) (line, col int) {
	if x.nlOf != s {
		x.nlOf, x.nl = s, x.nl[:0]
		for i := 0; i < len(s.Value); i++ {
			if s.Value[i] == '\n' {
				x.nl = append(x.nl, i)
			}
		}
	}
	off = min(max(off, 0), len(s.Value))
	n := sort.SearchInts(x.nl, off) // the line breaks before the offset
	last := -1
	if n > 0 {
		last = x.nl[n-1]
	}
	return s.Pos.Line + 1 + n, s.Indent + 1 + utf8.RuneCountInString(s.Value[last+1:off])
}

// foldedBlockBase returns the source offset where the content of a folded block scalar (">") starts,
// or false when the string is not one. The header is on the line of the position of the string.
func (x *sourceIndex) foldedBlockBase(s *String) (base, indent int, ok bool) {
	hdr, ok := x.offset(s.Pos.Line, s.Pos.Col)
	if !ok || hdr >= len(x.src) || x.src[hdr] != '>' || s.Quoted {
		return 0, 0, false
	}
	for l := s.Pos.Line + 1; l <= len(x.lineStarts); l++ {
		line := x.src[x.lineStarts[l-1]:x.lineEnd(l)]
		n := len(line) - len(strings.TrimLeft(string(line), " "))
		if n < len(line) {
			return x.lineStarts[l-1] + n, n, true
		}
	}
	return 0, 0, false
}

// multiline reports whether the scalar spans several lines in the source.
func (x *sourceIndex) multiline(s *String) bool {
	base, ok := x.offset(s.Pos.Line, s.Pos.Col)
	if !ok {
		return false
	}
	end := x.lineEnd(s.Pos.Line)
	if base > end {
		return false
	}
	// A scalar that is longer than the rest of its first line continues on the next line.
	n := len(s.Value)
	if s.Quoted {
		n += 2
	}
	return base+n > end
}

// lineCol converts a byte offset to a 1-based line and a column counted in characters.
func (x *sourceIndex) lineCol(off int) (int, int) {
	line := sort.Search(len(x.lineStarts), func(i int) bool { return x.lineStarts[i] > off })
	return line, utf8.RuneCount(x.src[x.lineStarts[line-1]:off]) + 1
}

// matches reports whether the source at the offset starts with the text.
func (x *sourceIndex) matches(off int, text string) bool {
	return x != nil && off >= 0 && off+len(text) <= len(x.src) && string(x.src[off:off+len(text)]) == text
}

// isExprIdentChar reports whether the byte can be in a property access such as github.event.issue-number.
func isExprIdentChar(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
