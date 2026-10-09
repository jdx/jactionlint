package jactionlint

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// This file has the helpers fixers use to put text into a YAML file. A fixer must never build YAML by
// concatenating strings: the same text means something else inside a double quoted scalar (where a
// backslash starts an escape), a single quoted one (where a quote must be doubled), a plain one
// (where ": " and " #" end the scalar) and a block scalar (where each line needs the indentation).
//
//   - YAMLSite.Insert renders text which is inserted inside a scalar that already exists.
//   - RenderYAMLValue renders a whole new scalar value, plain when that reads back as the same string.
//   - YAMLSiteAt finds out how the scalar at a byte offset of a source is written.

// YAMLContext is how a scalar is written in the source.
type YAMLContext int

const (
	// YAMLPlain is a plain scalar of the block style: key: value
	YAMLPlain YAMLContext = iota
	// YAMLFlowPlain is a plain scalar inside a flow collection: [a, b] or {a: b}
	YAMLFlowPlain
	// YAMLSingleQuoted is a scalar written with single quotes.
	YAMLSingleQuoted
	// YAMLDoubleQuoted is a scalar written with double quotes.
	YAMLDoubleQuoted
	// YAMLBlockScalar is a literal (|) or folded (>) block scalar.
	YAMLBlockScalar
)

func (c YAMLContext) String() string {
	switch c {
	case YAMLPlain:
		return "plain"
	case YAMLFlowPlain:
		return "flow plain"
	case YAMLSingleQuoted:
		return "single quoted"
	case YAMLDoubleQuoted:
		return "double quoted"
	case YAMLBlockScalar:
		return "block scalar"
	}
	return "unknown"
}

// YAMLSite is a place inside an existing scalar where a fixer inserts text.
type YAMLSite struct {
	Context YAMLContext
	// Indent is the number of spaces in front of each line of the content of a block scalar. Text
	// which has line breaks gets this indentation after each of them.
	Indent int
	// AtStart tells that the text is inserted at the start of a plain scalar. A plain scalar cannot
	// start with an indicator character such as "-", "&", "*", "!", "|", ">", "%" or "@".
	AtStart bool
	// CRLF tells that the line of the site ends with "\r\n": the lines of an inserted block text then do too.
	CRLF bool
}

// Insert renders the text so that it reads back as the same text once it is inserted at the site. It
// returns false when the context cannot hold the text. The caller then offers no fix or writes the
// whole scalar again with RenderYAMLValue. Text which is not escaped by the rules of the context
// is never returned in a form which changes the meaning of the line or the document.
func (s YAMLSite) Insert(text string) (string, bool) {
	if !utf8.ValidString(text) {
		return "", false
	}
	switch s.Context {
	case YAMLDoubleQuoted:
		return escapeDoubleQuoted(text), true
	case YAMLSingleQuoted:
		if strings.ContainsAny(text, "\r\n") || hasControl(text) {
			return "", false // a line break in a single quoted scalar is folded, and a control character is not allowed
		}
		return strings.ReplaceAll(text, "'", "''"), true
	case YAMLBlockScalar:
		if hasControlExceptLineBreak(text) {
			return "", false
		}
		if s.Indent < 0 {
			return "", false
		}
		text = strings.ReplaceAll(text, "\r\n", "\n")
		if strings.Contains(text, "\r") {
			return "", false
		}
		nl := "\n"
		if s.CRLF {
			nl = "\r\n"
		}
		return strings.ReplaceAll(text, "\n", nl+strings.Repeat(" ", s.Indent)), true
	case YAMLPlain, YAMLFlowPlain:
		if !plainInsertable(text, s.Context == YAMLFlowPlain, s.AtStart) {
			return "", false
		}
		return text, true
	}
	return "", false
}

// plainInsertable reports whether the text can be put inside a plain scalar as it is.
func plainInsertable(text string, flow, atStart bool) bool {
	if hasControl(text) {
		return false
	}
	if strings.Contains(text, " #") || strings.Contains(text, "\t#") || strings.Contains(text, ": ") || strings.Contains(text, ":\t") || strings.HasSuffix(text, ":") {
		return false
	}
	// After a space (or at the start) a "#" begins a comment, and the site may be after one
	if strings.HasPrefix(text, "#") {
		return false
	}
	if flow && strings.ContainsAny(text, ",[]{}") {
		return false
	}
	if atStart && text != "" {
		if strings.ContainsRune("-?:,[]{}#&*!|>'\"%@`", rune(text[0])) || text[0] == ' ' || text[0] == '\t' {
			return false
		}
	}
	return true
}

func hasControl(s string) bool {
	for _, r := range s {
		if isYAMLControl(r) {
			return true
		}
	}
	return false
}

func hasControlExceptLineBreak(s string) bool {
	for _, r := range s {
		if r != '\n' && r != '\r' && isYAMLControl(r) {
			return true
		}
	}
	return false
}

func isYAMLControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0xfeff || r == 0x2028 || r == 0x2029
}

// escapeDoubleQuoted escapes the text for the inside of a double quoted YAML scalar.
func escapeDoubleQuoted(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case isYAMLControl(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RenderYAMLValue returns the string as a whole YAML scalar for the value of a mapping entry in
// the block style: plain when that reads back as the same string, or else in double quotes. It
// checks with the YAML parser, so "true", "1.0", "null", "a: b", "# x" and the like are quoted.
func RenderYAMLValue(s string) string {
	if plainRoundTrips(s) {
		return s
	}
	return `"` + escapeDoubleQuoted(s) + `"`
}

// yaml11Special matches the plain scalars which YAML 1.1 parsers (which GitHub's workflow parser and
// many tools are) read as something else than a string: numbers in all their spellings (including
// "1_000", "0x1f", "1:30" and dates) and the special values.
var yaml11Special = regexp.MustCompile(`(?i)^(y|n|yes|no|on|off|true|false|null|~|[-+]?\.(inf|nan)|[-+]?[0-9][0-9_.:eE+-]*|[-+]?0[xob][0-9a-f_]+)$`)

func plainRoundTrips(s string) bool {
	if yaml11Special.MatchString(s) {
		return false
	}
	if s == "" || strings.TrimSpace(s) != s || !utf8.ValidString(s) || hasControl(s) {
		return false
	}
	if !plainInsertable(s, false, true) {
		return false
	}
	var n yaml.Node
	if err := yaml.Unmarshal([]byte("k: "+s+"\n"), &n); err != nil || n.Kind != yaml.DocumentNode || len(n.Content) != 1 {
		return false
	}
	m := n.Content[0]
	if m.Kind != yaml.MappingNode || len(m.Content) != 2 {
		return false
	}
	v := m.Content[1]
	return v.Kind == yaml.ScalarNode && v.ShortTag() == "!!str" && v.Value == s && v.Style == 0
}

// YAMLSiteAt finds out how the scalar which contains the byte offset is written. It returns false
// when the offset is not inside a scalar, when the source does not parse, or when the context is
// not one the helpers handle (for instance, a block scalar whose content is empty).
//
// Every call parses the whole source. Code which asks about many offsets of one source builds a
// yamlSiteIndex once instead (sourceIndex.sites does it lazily).
func YAMLSiteAt(src []byte, off int) (YAMLSite, bool) {
	return newYAMLSiteIndex(src).at(off)
}

// yamlSiteIndex answers YAMLSiteAt for one source without parsing it again for every question.
type yamlSiteIndex struct {
	src     []byte
	t       *nodeTable
	scalars []*yaml.Node // in document order, which is the order of their offsets
}

// newYAMLSiteIndex parses the source. The result answers false for everything when the source does
// not parse.
func newYAMLSiteIndex(src []byte) *yamlSiteIndex {
	x := &yamlSiteIndex{src: src}
	docs, err := parseYAMLDocs(src)
	if err != nil {
		return x
	}
	t := newNodeTable(src, docs)
	if t == nil {
		return x
	}
	x.t = t
	for _, d := range docs {
		t.eachScalar(d, func(n *yaml.Node) { x.scalars = append(x.scalars, n) })
	}
	return x
}

func (x *yamlSiteIndex) at(off int) (YAMLSite, bool) {
	if x == nil || x.t == nil {
		return YAMLSite{}, false
	}
	src, t := x.src, x.t
	// The extent of a scalar ends where the next node starts, so the extents do not overlap and only
	// the last scalar which starts at or before the offset can contain it.
	i := sort.Search(len(x.scalars), func(i int) bool { return t.start[x.scalars[i]] > off }) - 1
	if i < 0 {
		return YAMLSite{}, false
	}
	best := x.scalars[i]
	if s, e := t.extent(best, best); off < s || off >= e {
		return YAMLSite{}, false
	}
	switch {
	case best.Style&yaml.DoubleQuotedStyle != 0:
		return YAMLSite{Context: YAMLDoubleQuoted}, true
	case best.Style&yaml.SingleQuotedStyle != 0:
		return YAMLSite{Context: YAMLSingleQuoted}, true
	case best.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0:
		ind, ok := blockScalarIndent(src, t.start[best])
		if !ok {
			return YAMLSite{}, false
		}
		return YAMLSite{Context: YAMLBlockScalar, Indent: ind, CRLF: lineBreakIsCRLF(src, off)}, true
	}
	site := YAMLSite{Context: YAMLPlain, AtStart: off == t.start[best]}
	if t.inFlow[best] {
		site.Context = YAMLFlowPlain
	}
	return site, true
}

// lineBreakIsCRLF reports whether the line break which ends the line of the offset is "\r\n". The offset may be
// on that break. A last line without a break takes the break of the line before it.
func lineBreakIsCRLF(src []byte, off int) bool {
	if i := bytes.IndexByte(src[off:], '\n'); i >= 0 {
		return off+i > 0 && src[off+i-1] == '\r'
	}
	if i := bytes.LastIndexByte(src[:off], '\n'); i > 0 {
		return src[i-1] == '\r'
	}
	return false
}

// blockScalarIndent returns the indentation of the first content line of the block scalar whose
// indicator is at the offset.
func blockScalarIndent(src []byte, indicator int) (int, bool) {
	i := indicator
	for i < len(src) && src[i] != '\n' {
		i++
	}
	for i < len(src) {
		i++ // the line break
		j := i
		for j < len(src) && src[j] == ' ' {
			j++
		}
		if j < len(src) && src[j] != '\n' && src[j] != '\r' {
			return j - i, j > i
		}
		for j < len(src) && src[j] != '\n' {
			j++
		}
		i = j
	}
	return 0, false
}
