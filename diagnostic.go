package jactionlint

import (
	"bytes"
	"unicode/utf8"
)

// sourceLines splits the source into lines without line terminators.
func sourceLines(src []byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	lines := bytes.Split(src, []byte("\n"))
	for i, l := range lines {
		lines[i] = bytes.TrimSuffix(l, []byte("\r"))
	}
	return lines
}

// tokenEndColumn returns the column just after the run of non-space characters which starts at the
// column in the line (up to the "}}" of an expression). The column is 1-based and counts Unicode code points. When there is no
// character at the column, the column itself is returned.
func tokenEndColumn(line []byte, col int) int {
	if col <= 0 {
		return col
	}
	i := 0
	for n := 1; n < col; n++ {
		if i >= len(line) {
			return col
		}
		_, w := utf8.DecodeRune(line[i:])
		i += w
	}
	end := col
	for i < len(line) {
		r, w := utf8.DecodeRune(line[i:])
		if r == ' ' || r == '\t' {
			break
		}
		if r == '}' && end > col && bytes.HasPrefix(line[i:], []byte("}}")) {
			break // the end of a ${{ }} written without a space before it is not part of the token
		}
		i += w
		end++
	}
	return end
}

// fillRegion sets the end position of the error if the rule did not report it. The region is the
// token starting at the error position, which is what the source indicator ^~~~ underlines.
func (e *Error) fillRegion(lines [][]byte) {
	if e.EndLine > 0 && e.EndColumn > 0 {
		return
	}
	if e.Line <= 0 || e.Column <= 0 {
		return
	}
	e.EndLine = e.Line
	e.EndColumn = e.Column
	if e.Line <= len(lines) {
		e.EndColumn = tokenEndColumn(lines[e.Line-1], e.Column)
	}
}
