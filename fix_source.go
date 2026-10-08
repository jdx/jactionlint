package jactionlint

import (
	"bytes"
	"strings"

	"go.yaml.in/yaml/v4"
)

// This file has the helpers fixers use to turn a position in the syntax tree into a byte offset of
// the source and to insert lines in a block-style YAML mapping. The YAML library does not tell where
// a node ends, so a fixer looks at the lines of the source instead. A helper returns false when the
// source has a shape it does not understand (flow style, tabs, anchors, a lone carriage return),
// and the fixer then reports the problem without a fix rather than guess.

// srcLine is one line of a source file.
type srcLine struct {
	// start is the offset of the first byte of the line.
	start int
	// end is the offset just after the last byte of the line, before its terminator ("\n" or "\r\n").
	end int
	// next is the offset of the first byte of the next line, or the length of the file.
	next int
	// crlf is whether the terminator is "\r\n".
	crlf bool
}

// srcDoc gives line based access to the source of a file.
type srcDoc struct {
	src   []byte
	lines []srcLine

	// starts holds the 0-based lines on which a node of the block structure starts. Lines which are not
	// in it continue a node: the rest of a multi-line scalar, the closing bracket of a flow collection.
	// It is nil until blockStarts is called and stays nil if the source does not parse.
	starts   map[int]bool
	startsOK bool
}

// newSrcDoc splits the source into lines. It returns nil when the source has a line break other
// than "\n" and "\r\n", which the YAML parser counts differently from the helpers.
func newSrcDoc(src []byte) *srcDoc {
	// NEL, LS and PS are line breaks for the YAML parser too
	if bytes.Contains(src, []byte("\u0085")) || bytes.Contains(src, []byte("\u2028")) || bytes.Contains(src, []byte("\u2029")) {
		return nil
	}
	d := &srcDoc{src: src}
	start := 0
	for start < len(src) {
		i := bytes.IndexByte(src[start:], '\n')
		var l srcLine
		if i < 0 {
			l = srcLine{start: start, end: len(src), next: len(src)}
		} else {
			l = srcLine{start: start, end: start + i, next: start + i + 1}
			if l.end > start && src[l.end-1] == '\r' {
				l.end--
				l.crlf = true
			}
		}
		if bytes.IndexByte(src[l.start:l.end], '\r') >= 0 {
			return nil
		}
		d.lines = append(d.lines, l)
		start = l.next
	}
	return d
}

func (d *srcDoc) text(i int) string {
	l := d.lines[i]
	return string(d.src[l.start:l.end])
}

// eol is the line terminator to put after the line. It is the one of the line, or of the file when
// the line is the last one and has none.
func (d *srcDoc) eol(i int) string {
	l := d.lines[i]
	if l.next > l.end {
		if l.crlf {
			return "\r\n"
		}
		return "\n"
	}
	if bytes.Contains(d.src, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

type lineKind int

const (
	lineBlank lineKind = iota
	lineComment
	lineContent
)

func (d *srcDoc) kind(i int) lineKind {
	t := strings.TrimLeft(d.text(i), " \t")
	switch {
	case t == "":
		return lineBlank
	case t[0] == '#':
		return lineComment
	}
	return lineContent
}

// indent returns the number of the leading spaces of a line. It returns false for a content line
// which has a tab after the spaces: YAML forbids indenting with tabs, so such a line is a part of
// something the helpers do not understand, e.g. the text of a block scalar. The line is
// deeper than a key when its spaces are, whatever follows them, so callers use the number
// in the false case to tell that.
func (d *srcDoc) indent(i int) (int, bool) {
	t := d.text(i)
	n := len(t) - len(strings.TrimLeft(t, " "))
	if n < len(t) && t[n] == '\t' && d.kind(i) == lineContent {
		return n, false
	}
	return n, true
}

// keyLine parses the line as a block mapping entry "name:" optionally quoted. It returns the indentation
// and the text after the colon without a trailing comment.
func (d *srcDoc) keyLine(i int, name string) (indent int, inline string, ok bool) {
	indent, ok = d.indent(i)
	if !ok || d.kind(i) != lineContent {
		return 0, "", false
	}
	t := d.text(i)[indent:]
	if t != "" && (t[0] == '"' || t[0] == '\'') {
		q := t[:1]
		if !strings.HasPrefix(t, q+name+q) {
			return 0, "", false
		}
		t = t[len(name)+2:]
	} else {
		if !strings.HasPrefix(t, name) {
			return 0, "", false
		}
		t = t[len(name):]
	}
	t = strings.TrimLeft(t, " \t")
	rest, found := strings.CutPrefix(t, ":")
	if !found || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		return 0, "", false
	}
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "#") {
		rest = ""
	}
	return indent, rest, true
}

// blockStarts returns the lines on which a node of the block structure starts, found with the YAML
// parser. A node inside a flow collection ("[a, b]", "{a: 1}") does not count: the collection is
// one node. It returns false when the source does not parse.
func (d *srcDoc) blockStarts() (map[int]bool, bool) {
	if d.startsOK {
		return d.starts, d.starts != nil
	}
	d.startsOK = true
	var root yaml.Node
	if err := yaml.Unmarshal(d.src, &root); err != nil {
		return nil, false
	}
	starts := map[int]bool{}
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if n.Kind != yaml.DocumentNode && n.Line > 0 {
			starts[n.Line-1] = true
		}
		if n.Style&yaml.FlowStyle != 0 {
			return
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&root)
	d.starts = starts
	return starts, true
}

// entryEnd returns the last content line of the block mapping entry which starts at the line i. The
// entry has the indent and the text inline after the colon. Blank and comment lines after the entry
// are not part of it.
//
// The entry goes on until the next line on which a node of the block structure starts with no more
// indentation than the key. The lines between are the value, whatever they look like: the rest of a
// multi-line plain or quoted scalar and the closing bracket of a flow collection can be indented as
// little as the key. It returns false when the extent cannot be determined, so that a fixer offers no
// fix rather than insert a line inside a value.
func (d *srcDoc) entryEnd(i, indent int, inline string) (int, bool) {
	if strings.HasPrefix(inline, "|") || strings.HasPrefix(inline, ">") {
		return 0, false // the lines which look like comments may belong to the scalar
	}
	starts, ok := d.blockStarts()
	if !ok || !starts[i] {
		return 0, false // does not parse, or the line is not where the parser says the entry starts
	}
	last := i
	for j := i + 1; j < len(d.lines); j++ {
		if starts[j] {
			ind, ok := d.indent(j)
			if !ok && ind <= indent {
				return 0, false
			}
			if ind < indent || (ind == indent && !(inline == "" && isSequenceItem(d.text(j)))) {
				break
			}
		}
		if d.kind(j) == lineContent {
			last = j
		}
	}
	return last, true
}

// jobSite is where lines can be added to the block-style mapping of a job.
type jobSite struct {
	// bodyIndent is the indentation of the keys of the job.
	bodyIndent int
	// after is the line after which a new key is added. It is the line of runs-on or name if the job
	// has them in the block style, or else the line of the key of the job.
	after int
}

// locateJob finds the block-style mapping of the job in the source. It returns false when the job is
// written in another style.
func (d *srcDoc) locateJob(j *Job) (jobSite, bool) {
	if j.ID == nil || j.ID.Pos == nil || j.ID.Pos.Line < 1 || j.ID.Pos.Line > len(d.lines) {
		return jobSite{}, false
	}
	key := j.ID.Pos.Line - 1
	keyIndent, inline, ok := d.keyLine(key, j.ID.Value)
	if !ok || inline != "" {
		return jobSite{}, false
	}
	first := -1
	for i := key + 1; i < len(d.lines); i++ {
		if d.kind(i) == lineContent {
			first = i
			break
		}
	}
	if first < 0 {
		return jobSite{}, false
	}
	bodyIndent, ok := d.indent(first)
	if !ok || bodyIndent <= keyIndent {
		return jobSite{}, false
	}
	if !looksLikeKey(d.text(first)[bodyIndent:]) {
		return jobSite{}, false
	}

	site := jobSite{bodyIndent: bodyIndent, after: key}
	runsOn, name := -1, -1
	for i := first; i < len(d.lines); i++ {
		if d.kind(i) != lineContent {
			continue
		}
		ind, ok := d.indent(i)
		if !ok && ind <= bodyIndent {
			return jobSite{}, false
		}
		if ind <= keyIndent {
			break
		}
		if ind != bodyIndent {
			if ind < bodyIndent {
				return jobSite{}, false
			}
			continue
		}
		if _, _, ok := d.keyLine(i, "timeout-minutes"); ok {
			return jobSite{}, false
		}
		if strings.HasPrefix(d.text(i)[ind:], "<<") {
			return jobSite{}, false
		}
		if _, _, ok := d.keyLine(i, "runs-on"); ok && runsOn < 0 {
			runsOn = i
		}
		if _, _, ok := d.keyLine(i, "name"); ok && name < 0 {
			name = i
		}
	}
	for _, c := range []struct {
		line int
		key  string
	}{{runsOn, "runs-on"}, {name, "name"}} {
		if c.line < 0 {
			continue
		}
		ind, inline, _ := d.keyLine(c.line, c.key)
		if end, ok := d.entryEnd(c.line, ind, inline); ok {
			site.after = end
			return site, true
		}
	}
	return site, true
}

// insertAfterLine returns the edit which adds the lines (without terminators) after the line i.
func (d *srcDoc) insertAfterLine(i int, lines ...string) TextEdit {
	eol := d.eol(i)
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(eol)
		b.WriteString(l)
	}
	at := d.lines[i].end
	return TextEdit{Start: at, End: at, NewText: b.String()}
}

// topLevelKey finds the line of a key of the root mapping of the workflow.
func (d *srcDoc) topLevelKey(name string) (line int, indent int, inline string, ok bool) {
	root := -1
	for i := range d.lines {
		if d.kind(i) != lineContent {
			continue
		}
		t := d.text(i)
		if strings.HasPrefix(t, "---") || strings.HasPrefix(t, "%") {
			continue
		}
		ind, okInd := d.indent(i)
		if !okInd {
			if root < 0 || ind <= root {
				return 0, 0, "", false
			}
			continue
		}
		if root < 0 {
			root = ind
		}
		if ind != root {
			continue
		}
		if ind, inline, ok := d.keyLine(i, name); ok {
			return i, ind, inline, true
		}
	}
	return 0, 0, "", false
}

// indentUnit guesses how many spaces one level of nesting is, from the keys of "jobs:".
func (d *srcDoc) indentUnit() int {
	line, indent, inline, ok := d.topLevelKey("jobs")
	if !ok || inline != "" {
		return 2
	}
	for i := line + 1; i < len(d.lines); i++ {
		if d.kind(i) != lineContent {
			continue
		}
		if ind, ok := d.indent(i); ok && ind > indent && ind-indent <= 8 {
			return ind - indent
		}
		break
	}
	return 2
}

// looksLikeKey reports whether the text starts a block mapping entry: a plain or quoted key followed
// by a colon and a space or the end of the line. It is not a YAML parser; it only rules out the
// scalars, sequences, flow collections, aliases and the like that would make an insertion wrong.
func looksLikeKey(t string) bool {
	if t == "" {
		return false
	}
	switch t[0] {
	case '{', '[', '-', '*', '&', '!', '<', '|', '>', '%', '@', '`', '#', ',', ']', '}', '?', ':':
		return false
	case '"', '\'':
		q := t[0]
		for i := 1; i < len(t); i++ {
			switch {
			case t[i] == '\\' && q == '"':
				i++
			case t[i] == q && q == '\'' && i+1 < len(t) && t[i+1] == '\'':
				i++
			case t[i] == q:
				rest := strings.TrimLeft(t[i+1:], " \t")
				return strings.HasPrefix(rest, ":") && (len(rest) == 1 || rest[1] == ' ' || rest[1] == '\t')
			}
		}
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] == ':' && (i+1 == len(t) || t[i+1] == ' ' || t[i+1] == '\t') {
			return true
		}
		if t[i] == '#' && i > 0 && (t[i-1] == ' ' || t[i-1] == '\t') {
			return false
		}
	}
	return false
}
