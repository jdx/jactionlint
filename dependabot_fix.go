package jactionlint

import (
	"bytes"
	"regexp"
	"strconv"
)

// dependabotSource maps the positions of a Dependabot configuration to byte offsets so that rules can
// build text edits. The syntax tree has no end positions, so every helper verifies the text it is
// about to edit and gives up (returns false) when the source does not look as expected, e.g. when the
// YAML is written in flow style. A rule then reports the finding without a fix.
type dependabotSource struct {
	src    []byte
	starts []int // byte offsets where lines start
}

func newDependabotSource(src []byte) *dependabotSource {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &dependabotSource{src, starts}
}

// line returns the content of the 1-based line without the line terminator, the offset where the
// content starts, and the terminator ("\n" or "\r\n"; "\n" for the last line without one, which is
// what the file most likely uses).
func (s *dependabotSource) line(n int) (content []byte, start int, eol string, ok bool) {
	if s == nil || n < 1 || n > len(s.starts) {
		return nil, 0, "", false
	}
	start = s.starts[n-1]
	end := len(s.src)
	if n < len(s.starts) {
		end = s.starts[n] - 1 // the '\n'
	}
	eol = "\n"
	if end > start && s.src[end-1] == '\r' {
		end--
		eol = "\r\n"
	}
	return s.src[start:end], start, eol, true
}

var (
	// dependabotKeyPrefix matches what precedes a key in a block mapping: the indentation, optionally
	// with the "- " of a sequence item. A flow mapping ("{ key:") does not match.
	dependabotKeyPrefix = regexp.MustCompile(`^( *(?:- +)?)$`)
	// dependabotLineRest matches what may follow a value on its line: nothing but a comment.
	dependabotLineRest = regexp.MustCompile(`^[ \t]*(#.*)?$`)
)

// keyAt checks that the block mapping key `name` (written plain or quoted) starts at pos. It returns
// the indent of the key, the offset of the end of the content of its line, the line terminator and
// the text after the colon.
func (s *dependabotSource) keyAt(pos *Pos, name string) (keyIndent int, lineEnd int, eol string, rest []byte, ok bool) {
	if pos == nil {
		return
	}
	content, start, eol, found := s.line(pos.Line)
	if !found || pos.Col < 1 || pos.Col-1 > len(content) {
		return
	}
	col := pos.Col - 1
	// The key starts at pos (a key node), possibly quoted.
	for _, q := range []string{"", `"`, `'`} {
		k := q + name + q
		if !bytes.HasPrefix(content[col:], []byte(k)) {
			continue
		}
		prefix := content[:col]
		if !dependabotKeyPrefix.Match(prefix) {
			continue
		}
		after := bytes.TrimLeft(content[col+len(k):], " \t")
		if len(after) == 0 || after[0] != ':' {
			continue
		}
		return len(prefix), start + len(content), eol, after[1:], true
	}
	return
}

// keyOfValueAt is like keyAt for a pos which points at the value of the key. It returns the text
// from the value to the end of the line.
func (s *dependabotSource) keyOfValueAt(pos *Pos, name string) (keyIndent int, lineEnd int, eol string, after []byte, ok bool) {
	if pos == nil {
		return
	}
	content, start, eol, found := s.line(pos.Line)
	if !found || pos.Col < 1 || pos.Col-1 > len(content) {
		return
	}
	col := pos.Col - 1
	before := content[:col]
	re := regexp.MustCompile(`^( *(?:- +)?)(["']?)` + regexp.QuoteMeta(name) + `(["']?)[ \t]*:[ \t]*$`)
	m := re.FindSubmatch(before)
	if m == nil || !bytes.Equal(m[2], m[3]) {
		return
	}
	return len(m[1]), start + len(content), eol, content[col:], true
}

// scalarToken returns the length of the scalar written at the start of text when it is exactly value
// (in quotes when quoted is set) and nothing but a comment follows it.
func scalarToken(text []byte, value string, quoted bool) (int, bool) {
	for _, q := range []string{`"`, `'`} {
		if quoted && bytes.HasPrefix(text, []byte(q+value+q)) {
			n := len(value) + 2
			return n, dependabotLineRest.Match(text[n:])
		}
	}
	if !quoted && bytes.HasPrefix(text, []byte(value)) {
		n := len(value)
		return n, dependabotLineRest.Match(text[n:])
	}
	return 0, false
}

// replaceScalar returns the edit replacing the scalar value written at pos with repl, quoted like the
// original.
func (s *dependabotSource) replaceScalar(pos *Pos, value string, quoted bool, repl string) (TextEdit, bool) {
	content, start, _, ok := s.line(pos.Line)
	if !ok || pos.Col < 1 || pos.Col-1 > len(content) {
		return TextEdit{}, false
	}
	text := content[pos.Col-1:]
	n, ok := scalarToken(text, value, quoted)
	if !ok {
		return TextEdit{}, false
	}
	begin := start + pos.Col - 1
	if quoted {
		q := string(text[0])
		repl = q + repl + q
	}
	return TextEdit{Start: begin, End: begin + n, NewText: repl}, true
}

// replaceInt returns the edit replacing the integer at pos with n. The integer must be written as
// plain decimal digits so that the edit cannot misread another notation (0o3, 1_0, +3).
func (s *dependabotSource) replaceInt(pos *Pos, old int, n int) (TextEdit, bool) {
	text := strconv.Itoa(old)
	return s.replaceScalar(pos, text, false, strconv.Itoa(n))
}

// insertAfter returns the edit adding lines after the line whose content ends at lineEnd. Each line
// is indented by indent columns.
func (s *dependabotSource) insertAfter(lineEnd int, eol string, indent int, lines ...string) TextEdit {
	var b bytes.Buffer
	for _, l := range lines {
		b.WriteString(eol)
		for i := 0; i < indent; i++ {
			b.WriteByte(' ')
		}
		b.WriteString(l)
	}
	return TextEdit{Start: lineEnd, End: lineEnd, NewText: b.String()}
}

// childIndent returns the indent of the first content line after the line n whose indent is greater
// than parentIndent, i.e. how the existing entries of a block mapping are indented, or false when
// the mapping has no entries on the following lines. Blank and comment lines are skipped.
func (s *dependabotSource) childIndent(n int, parentIndent int) (int, bool) {
	for i := n + 1; ; i++ {
		content, _, _, ok := s.line(i)
		if !ok {
			return 0, false
		}
		trimmed := bytes.TrimLeft(content, " ")
		if len(bytes.TrimSpace(trimmed)) == 0 || trimmed[0] == '#' {
			continue
		}
		ind := len(content) - len(trimmed)
		if ind > parentIndent {
			return ind, true
		}
		return 0, false
	}
}
