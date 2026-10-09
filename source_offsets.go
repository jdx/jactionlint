package jactionlint

import (
	"bytes"
	"sync"
	"unicode/utf8"
)

// sourceIndex converts the positions of the syntax tree (a line and a column counted in code points)
// into the byte offsets which Fix edits use.
type sourceIndex struct {
	src        []byte
	lineStarts []int // byte offset of the start of each line
	// valid is false when the source uses a line break other than "\n" or "\r\n", which the YAML parser
	// counts as a line break and this index does not, so positions cannot be converted reliably.
	valid bool

	// nl are the offsets of the line breaks in the value of the string nlOf (see literalPosition).
	nlOf *String
	nl   []int

	sitesOnce sync.Once
	sitesIdx  *yamlSiteIndex
}

// sites returns the YAML structure of the source, which is parsed once however many offsets are asked.
func (idx *sourceIndex) sites() *yamlSiteIndex {
	idx.sitesOnce.Do(func() { idx.sitesIdx = newYAMLSiteIndex(idx.src) })
	return idx.sitesIdx
}

// newSourceIndex indexes the lines of the source.
func newSourceIndex(src []byte) *sourceIndex {
	idx := &sourceIndex{src: src, lineStarts: []int{0}, valid: true}
	for i, b := range src {
		switch {
		case b == '\n':
			idx.lineStarts = append(idx.lineStarts, i+1)
		case b == '\r' && (i+1 >= len(src) || src[i+1] != '\n'):
			idx.valid = false
		}
	}
	if hasUnicodeLineBreak(src) {
		idx.valid = false
	}
	return idx
}

// hasUnicodeLineBreak reports whether the source has NEL, LS or PS, which the YAML parser counts as line
// breaks and the helpers that split a source into lines (newSourceIndex, newSrcDoc) do not. Both refuse
// such a document, so nothing is edited by an offset the parser would not agree with.
func hasUnicodeLineBreak(src []byte) bool {
	return bytes.Contains(src, []byte("\u0085")) || bytes.Contains(src, []byte("\u2028")) || bytes.Contains(src, []byte("\u2029"))
}

// offset returns the byte offset of a line and a column (both 1-based, the column in code points).
// It returns false when the position is outside the source.
func (x *sourceIndex) offset(line, col int) (int, bool) {
	if x == nil || !x.valid || line < 1 || line > len(x.lineStarts) || col < 1 {
		return 0, false
	}
	off := x.lineStarts[line-1]
	end := x.lineEnd(line)
	for i := 1; i < col; i++ {
		if off >= end {
			return 0, false
		}
		_, n := utf8.DecodeRune(x.src[off:])
		off += n
	}
	return off, true
}

func (x *sourceIndex) offsetOf(p *Pos) (int, bool) {
	if p == nil {
		return 0, false
	}
	return x.offset(p.Line, p.Col)
}

// lineEnd returns the offset of the end of the line without its line break.
func (x *sourceIndex) lineEnd(line int) int {
	end := len(x.src)
	if line < len(x.lineStarts) {
		end = x.lineStarts[line] - 1
	}
	if end > x.lineStarts[line-1] && x.src[end-1] == '\r' {
		end--
	}
	return end
}

// newline returns the line break the source uses, which is "\r\n" when its first line break is.
func (x *sourceIndex) newline() string {
	if i := bytes.IndexByte(x.src, '\n'); i > 0 && x.src[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// selfRepositoryEdit returns the edit which changes the "./" at the start of the `uses:` value at the
// position into "$/", or false when the value is not written that way (for instance when it is
// quoted in an unexpected way).
func (x *sourceIndex) selfRepositoryEdit(p *Pos) (TextEdit, bool) {
	off, ok := x.offsetOf(p)
	if !ok {
		return TextEdit{}, false
	}
	if off < len(x.src) && (x.src[off] == '"' || x.src[off] == '\'') {
		off++
	}
	if off+2 > len(x.src) || x.src[off] != '.' || x.src[off+1] != '/' {
		return TextEdit{}, false
	}
	return TextEdit{Start: off, End: off + 1, NewText: "$"}, true
}
