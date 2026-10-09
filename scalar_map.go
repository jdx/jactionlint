package jactionlint

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// This file is the one place which maps a byte offset of the value of a YAML scalar back to the place in
// the file where the text is written. Every rule which reports a finding inside a string (an expression,
// a line of a script, a secret) goes through it. It follows the YAML rules for the five styles: plain,
// single quoted ('' is a quote), double quoted (escapes, "\<newline>"), literal blocks (|) and folded
// blocks (>), with the folding of line breaks of the flow styles and of the folded blocks.
//
// The result is checked against the value the YAML parser produced. When the decoded text differs (a
// construct this file does not know), the map is not used and callers fall back to their estimate.

// scalarSource is the text of the file a String was parsed from. The strings of one file share it. It
// is nil for strings that were not parsed from a file.
type scalarSource struct {
	lines []string // the lines of the file, without "\n" (a "\r" may end a line)

	mu   sync.Mutex
	maps map[*String]*scalarMap
}

// scalarPt is a place in the file: a 1-based line and the byte offset in the line.
type scalarPt struct{ line, col int32 }

// scalarMap holds, for each byte of the value of a scalar and for the end of the value, the place where
// that byte is written in the file. A byte which is made of an escape or of a folded line break is at the
// start of the escape or at the end of the line.
type scalarMap struct {
	pts []scalarPt
}

func newScalarSource(lines []string) *scalarSource {
	if lines == nil {
		return nil
	}
	return &scalarSource{lines: lines, maps: map[*String]*scalarMap{}}
}

// mapOf returns the offset map of the string, or nil when it cannot be built.
func (src *scalarSource) mapOf(s *String) *scalarMap {
	if src == nil || s == nil || s.Pos == nil {
		return nil
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if m, ok := src.maps[s]; ok {
		return m
	}
	m := decodeScalar(src.lines, s)
	src.maps[s] = m
	return m
}

// at returns the line and the byte offset in the line of the byte at off of the value.
func (m *scalarMap) at(off int) (line, col int) {
	off = min(max(off, 0), len(m.pts)-1)
	p := m.pts[off]
	return int(p.line), int(p.col)
}

// valueAt returns the place in the file of the byte at off of the value of the string: a 1-based line and
// a column counted in code points. ok is false when the string has no known source or its text could not
// be followed.
func (s *String) valueAt(off int) (line, col int, ok bool) {
	if s == nil || s.src == nil {
		return 0, 0, false
	}
	m := s.src.mapOf(s)
	if m == nil {
		return 0, 0, false
	}
	line, bcol := m.at(off)
	text := s.src.lines[line-1]
	return line, utf8.RuneCountInString(text[:min(bcol, len(text))]) + 1, true
}

// valueByteAt is like valueAt, but returns the offset in the line in bytes.
func (s *String) valueByteAt(off int) (line, bcol int, ok bool) {
	if s == nil || s.src == nil {
		return 0, 0, false
	}
	m := s.src.mapOf(s)
	if m == nil {
		return 0, 0, false
	}
	line, bcol = m.at(off)
	return line, bcol, true
}

func trimCR(l string) string { return strings.TrimSuffix(l, "\r") }

// byteColOf converts a column counted in code points (1-based) to a byte offset of the line.
func byteColOf(line string, col int) (int, bool) {
	i := 0
	for c := 1; c < col; c++ {
		if i >= len(line) {
			return 0, false
		}
		_, n := utf8.DecodeRuneInString(line[i:])
		i += n
	}
	return i, true
}

func decodeScalar(lines []string, s *String) *scalarMap {
	if s.Pos.Line < 1 || s.Pos.Line > len(lines) {
		return nil
	}
	first := trimCR(lines[s.Pos.Line-1])
	start, ok := byteColOf(first, s.Pos.Col)
	if !ok || start >= len(first) {
		return nil
	}
	d := &scalarDecoder{lines: lines, want: s.Value}
	switch first[start] {
	case '|', '>':
		if !d.block(s, first[start] == '>', start) {
			return nil
		}
		// The chomping indicator decides how much of the final line breaks are in the value.
		if !(strings.HasPrefix(string(d.out), s.Value) || strings.HasPrefix(s.Value, string(d.out))) {
			return nil
		}
		for len(d.pts) <= len(s.Value) {
			d.pts = append(d.pts, d.pts[len(d.pts)-1])
		}
	case '"':
		if !d.flow('"', s.Pos.Line, start+1) || string(d.out) != s.Value {
			return nil
		}
	case '\'':
		if !d.flow('\'', s.Pos.Line, start+1) || string(d.out) != s.Value {
			return nil
		}
	default:
		if !d.flow(0, s.Pos.Line, start) || string(d.out) != s.Value {
			return nil
		}
	}
	return &scalarMap{pts: d.pts}
}

// scalarDecoder decodes the text of a scalar of the file. For each byte it appends to out, it appends
// the place of the byte to pts. The place of the end of the value is appended last.
type scalarDecoder struct {
	lines []string
	want  string
	out   []byte
	pts   []scalarPt
}

func (d *scalarDecoder) emit(b byte, line, col int) {
	d.out = append(d.out, b)
	d.pts = append(d.pts, scalarPt{int32(line), int32(col)})
}

func (d *scalarDecoder) emitRune(r rune, line, col int) {
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], r)
	for _, b := range buf[:n] {
		d.emit(b, line, col)
	}
}

func isBlank(s string) bool { return strings.Trim(s, " \t") == "" }

// flow decodes a plain (quote is 0), single quoted or double quoted scalar which starts at the byte
// col of the line.
func (d *scalarDecoder) flow(quote byte, line, col int) bool {
	l, i := line, col
	for {
		if l > len(d.lines) {
			return false
		}
		text := trimCR(d.lines[l-1])
		lineOut := len(d.out)
		protected := len(d.out) // bytes at the start of d.out[protected:] may be trimmed, not before it
		continued := false      // the line ends with a "\" that escapes the line break
		for i < len(text) {
			c := text[i]
			if quote == 0 && len(d.out) >= len(d.want) {
				break
			}
			switch {
			case quote == '\'' && c == '\'':
				if i+1 < len(text) && text[i+1] == '\'' {
					d.emit('\'', l, i)
					protected = len(d.out)
					i += 2
					continue
				}
				d.pts = append(d.pts, scalarPt{int32(l), int32(i)})
				return true
			case quote == '"' && c == '"':
				d.pts = append(d.pts, scalarPt{int32(l), int32(i)})
				return true
			case quote == '"' && c == '\\':
				if i+1 == len(text) {
					continued = true
					i++
					continue
				}
				n, ok := d.escape(text, i, l)
				if !ok {
					return false
				}
				protected = len(d.out)
				i += n
			default:
				_, w := utf8.DecodeRuneInString(text[i:])
				for k := 0; k < w; k++ {
					d.emit(text[i+k], l, i) // all the bytes of a character are at its start
				}
				i += w
			}
		}
		if quote == 0 && len(d.out) >= len(d.want) {
			d.pts = append(d.pts, scalarPt{int32(l), int32(i)})
			return true
		}
		if i < len(text) && !continued {
			return false
		}
		// The line break. The white space before it is dropped, unless an escape wrote it.
		if !continued {
			for len(d.out) > protected && len(d.out) > lineOut && (d.out[len(d.out)-1] == ' ' || d.out[len(d.out)-1] == '\t') {
				d.out = d.out[:len(d.out)-1]
				d.pts = d.pts[:len(d.pts)-1]
			}
		}
		endCol := len(strings.TrimRight(text, " \t"))
		blanks := 0
		next := l + 1
		for next <= len(d.lines) && isBlank(trimCR(d.lines[next-1])) {
			blanks++
			next++
		}
		if next > len(d.lines) {
			return false
		}
		switch {
		case continued:
			// "\<newline>" joins the lines; blank lines after it are line breaks
			for k := 0; k < blanks; k++ {
				d.emit('\n', l+1+k, 0)
			}
		case blanks == 0:
			d.emit(' ', l, endCol)
		default:
			for k := 0; k < blanks; k++ {
				d.emit('\n', l+1+k, 0)
			}
		}
		l = next
		nt := trimCR(d.lines[l-1])
		i = 0
		for i < len(nt) && (nt[i] == ' ' || nt[i] == '\t') {
			i++
		}
	}
}

var simpleEscapes = map[byte]rune{
	'0': 0, 'a': 7, 'b': 8, 't': 9, '\t': 9, 'n': 10, 'v': 11, 'f': 12, 'r': 13, 'e': 27,
	' ': ' ', '"': '"', '/': '/', '\\': '\\', 'N': 0x85, '_': 0xa0, 'L': 0x2028, 'P': 0x2029,
}

// escape decodes the escape which starts at text[i] == '\\'. It returns the length of the escape.
func (d *scalarDecoder) escape(text string, i, line int) (int, bool) {
	e := text[i+1]
	if r, ok := simpleEscapes[e]; ok {
		d.emitRune(r, line, i)
		return 2, true
	}
	n := 0
	switch e {
	case 'x':
		n = 2
	case 'u':
		n = 4
	case 'U':
		n = 8
	default:
		return 0, false
	}
	if i+2+n > len(text) {
		return 0, false
	}
	v, err := strconv.ParseUint(text[i+2:i+2+n], 16, 32)
	if err != nil {
		return 0, false
	}
	d.emitRune(rune(v), line, i)
	return 2 + n, true
}

type blockLine struct {
	line    int
	content string // without the indentation
	blank   bool
	more    bool // starts with white space: a more indented line of a folded block
}

// block decodes a literal or a folded block scalar whose header is at byte col of the line of the string.
func (d *scalarDecoder) block(s *String, folded bool, col int) bool {
	header := trimCR(d.lines[s.Pos.Line-1])
	explicit := 0
	for _, c := range header[col+1:] {
		if c >= '1' && c <= '9' {
			explicit = int(c - '0')
		}
		if c == ' ' || c == '#' {
			break
		}
	}
	indent := s.Indent
	if indent <= 0 {
		if explicit > 0 {
			return false
		}
		for l := s.Pos.Line + 1; l <= len(d.lines); l++ {
			t := trimCR(d.lines[l-1])
			if strings.Trim(t, " ") == "" {
				continue
			}
			indent = len(t) - len(strings.TrimLeft(t, " "))
			break
		}
	}
	if indent <= 0 {
		return false
	}
	var lns []blockLine
	for l := s.Pos.Line + 1; l <= len(d.lines); l++ {
		t := trimCR(d.lines[l-1])
		blank := strings.Trim(t, " ") == ""
		if !blank && len(t)-len(strings.TrimLeft(t, " ")) < indent {
			break
		}
		content := ""
		if len(t) > indent {
			content = t[indent:]
		}
		more := !blank && (content[0] == ' ' || content[0] == '\t')
		lns = append(lns, blockLine{line: l, content: content, blank: blank, more: more})
	}
	// the empty "line" after the final line break of the file is not a line
	if n := len(lns); n > 0 && lns[n-1].line == len(d.lines) && d.lines[len(d.lines)-1] == "" {
		lns = lns[:n-1]
	}
	endPt := scalarPt{int32(s.Pos.Line), int32(len(header))}
	write := func(b blockLine) {
		for k := 0; k < len(b.content); {
			_, w := utf8.DecodeRuneInString(b.content[k:])
			for j := 0; j < w; j++ {
				d.emit(b.content[k+j], b.line, indent+k)
			}
			k += w
		}
	}
	eol := func(b blockLine) int { return len(trimCR(d.lines[b.line-1])) }
	if !folded {
		for _, b := range lns {
			write(b)
			d.emit('\n', b.line, eol(b))
		}
	} else {
		prev := -1 // index of the previous line which has text
		for i, b := range lns {
			if b.blank {
				continue
			}
			if prev < 0 {
				for k := 0; k < i; k++ {
					d.emit('\n', lns[k].line, 0)
				}
			} else {
				blanks := i - prev - 1
				if !lns[prev].more && !b.more {
					if blanks == 0 {
						d.emit(' ', lns[prev].line, eol(lns[prev]))
					}
				} else {
					d.emit('\n', lns[prev].line, eol(lns[prev]))
				}
				for k := 0; k < blanks; k++ {
					d.emit('\n', lns[prev+1+k].line, 0)
				}
			}
			write(b)
			prev = i
		}
		if prev >= 0 {
			d.emit('\n', lns[prev].line, eol(lns[prev]))
			for k := prev + 1; k < len(lns); k++ {
				d.emit('\n', lns[k].line, 0)
			}
		}
	}
	if n := len(lns); n > 0 {
		endPt = scalarPt{int32(lns[n-1].line), int32(eol(lns[n-1]))}
	}
	d.pts = append(d.pts, endPt)
	return true
}

// endPos returns the position just after the string in the file (after the closing quote of a quoted
// string). It is false when the place of the string is not known.
func (s *String) endPos() (*Pos, bool) {
	line, col, ok := s.valueAt(len(s.Value))
	if !ok {
		return nil, false
	}
	if s.Quoted {
		col++
	}
	return &Pos{Line: line, Col: col}, true
}
