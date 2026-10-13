package jactionlint

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// valueOnLine is a value of the source: plain or quoted, on one line.
type valueOnLine struct {
	line []byte // the line, without its line break
	text string // the value
	// valueEnd is the byte offset in the line of the end of the value, without a closing quote. tokenEnd is
	// the end of the value with it.
	valueEnd, tokenEnd int
}

// scanValue finds the value which starts at the column (1-based, in characters) of the line that starts at lineStart. It
// refuses what an edit could not keep intact: a value which is not closed on its line, or with characters of a flow
// collection or an escape in it.
func scanValue(src []byte, lineStart, col int) (valueOnLine, error) {
	lineEnd := len(src)
	if i := bytes.IndexByte(src[lineStart:], '\n'); i >= 0 {
		lineEnd = lineStart + i
	}
	line := bytes.TrimSuffix(src[lineStart:lineEnd], []byte("\r"))

	off := 0
	for n := 1; n < col; n++ {
		if off >= len(line) {
			return valueOnLine{}, shapeErr("the position of the value is outside its line")
		}
		_, w := utf8.DecodeRune(line[off:])
		off += w
	}
	v := valueOnLine{line: line}
	start := off
	if off < len(line) && (line[off] == '"' || line[off] == '\'') {
		quote := line[off]
		start = off + 1
		i := bytes.IndexByte(line[start:], quote)
		if i < 0 {
			return valueOnLine{}, shapeErr("the quoted value is not closed on its line")
		}
		v.valueEnd = start + i
		v.tokenEnd = v.valueEnd + 1
	} else {
		v.valueEnd = off
		for v.valueEnd < len(line) && line[v.valueEnd] != ' ' && line[v.valueEnd] != '\t' && line[v.valueEnd] != '#' {
			v.valueEnd++
		}
		v.tokenEnd = v.valueEnd
	}
	v.text = string(line[start:v.valueEnd])
	if strings.Contains(v.text, "${{") {
		return valueOnLine{}, shapeErr("the value comes from an expression, which is only known when the workflow runs")
	}
	if strings.ContainsAny(v.text, "\\{}[],") {
		return valueOnLine{}, shapeErr("the value %q is not a plain value", v.text)
	}
	return v, nil
}
