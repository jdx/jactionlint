package jactionlint

import (
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"
)

// Comment is one YAML comment of a source file.
type Comment struct {
	// Line is the 1-based line of the comment.
	Line int
	// Column is the 1-based column of the '#', counted in characters like the positions of the syntax tree.
	Column int
	// Text is the comment without the leading '#' and the surrounding white space.
	Text string
	// Inline is true for a comment following YAML content on its line ("key: value # comment") and
	// false for a comment which has the line to itself.
	Inline bool
}

// CommentIndex finds the comments of a YAML file by line. The YAML library does not tell where a
// comment is, only which node it is loosely attached to (and attaches the comment at the end of
// the file to an unrelated key), so the index is built by scanning the source instead. It
// understands what is not a comment: '#' inside quoted scalars, block scalars ("|" and ">") and
// plain scalars ("a#b").
//
// Workflow.Comments is the index of a parsed workflow. A syntax node is located by Pos.Line, so
// a rule asks about the comments of a node with Inline(n.Pos.Line), Before(n.Pos.Line) or
// Documented(n.Pos.Line). All methods accept a nil index, which has no comments.
//
// Known limit: a continuation line of a multi-line plain scalar that starts with a quote is read
// as the start of a quoted scalar.
type CommentIndex struct {
	comments []Comment // sorted by line; at most one comment per line
}

// NewCommentIndex scans src for comments.
func NewCommentIndex(src []byte) *CommentIndex {
	idx := &CommentIndex{}
	if bytes.IndexByte(src, '#') < 0 {
		return idx
	}
	var s commentScanner
	for i, line := range strings.Split(string(src), "\n") {
		s.scanLine(idx, i+1, strings.TrimSuffix(line, "\r"))
	}
	return idx
}

// All returns every comment in the order of the source.
func (idx *CommentIndex) All() []Comment {
	if idx == nil {
		return nil
	}
	return idx.comments
}

// At returns the comment on the line, inline or not, or nil.
func (idx *CommentIndex) At(line int) *Comment {
	if idx == nil {
		return nil
	}
	i := sort.Search(len(idx.comments), func(i int) bool { return idx.comments[i].Line >= line })
	if i < len(idx.comments) && idx.comments[i].Line == line {
		return &idx.comments[i]
	}
	return nil
}

// Inline returns the comment which follows content on the line ("key: value # comment"), or nil.
// In a flow collection, or after an anchor, it is the comment at the end of the line which holds
// the node.
func (idx *CommentIndex) Inline(line int) *Comment {
	if c := idx.At(line); c != nil && c.Inline {
		return c
	}
	return nil
}

// Before returns the comments which sit on the lines directly above the line, in source order. The
// block ends at the first blank line or line of content going up, so a comment separated by a blank
// line belongs to nobody.
func (idx *CommentIndex) Before(line int) []Comment {
	n := 0
	for l := line - 1; l >= 1; l-- {
		if c := idx.At(l); c == nil || c.Inline {
			break
		}
		n++
	}
	if n == 0 {
		return nil
	}
	return idx.between(line-n, line-1)
}

// After returns the comments which sit on the lines directly below the line, in source order, up to
// the first blank line or line of content. Those comments are also what Before reports for the
// node which follows. The syntax tree has no end position, so for a node spanning several lines
// the caller must pass its last line.
func (idx *CommentIndex) After(line int) []Comment {
	n := 0
	for l := line + 1; ; l++ {
		if c := idx.At(l); c == nil || c.Inline {
			break
		}
		n++
	}
	if n == 0 {
		return nil
	}
	return idx.between(line+1, line+n)
}

// Documented reports whether the line is explained by a comment: one trailing the line or a block
// directly above it.
func (idx *CommentIndex) Documented(line int) bool {
	return idx.Inline(line) != nil || len(idx.Before(line)) > 0
}

func (idx *CommentIndex) between(from, to int) []Comment {
	i := sort.Search(len(idx.comments), func(i int) bool { return idx.comments[i].Line >= from })
	j := sort.Search(len(idx.comments), func(i int) bool { return idx.comments[i].Line > to })
	return idx.comments[i:j]
}

// commentScanner is the state carried from one line to the next.
type commentScanner struct {
	// quote is the quote character of a quoted scalar which continues on the next line, or 0.
	quote byte
	// inBlock is true while the lines belong to a block scalar. They must be indented deeper than blockParent.
	inBlock     bool
	blockParent int
}

func isSpaceByte(b byte) bool { return b == ' ' || b == '\t' }

func (s *commentScanner) scanLine(idx *CommentIndex, lineNo int, line string) {
	i := 0

	if s.inBlock {
		ind := indentOf(line)
		if ind == len(line) || ind > s.blockParent {
			return // blank or content of the scalar
		}
		s.inBlock = false
	}

	add := func(at int, inline bool) {
		idx.comments = append(idx.comments, Comment{
			Line:   lineNo,
			Column: utf8.RuneCountInString(line[:at]) + 1,
			Text:   strings.TrimSpace(line[at+1:]),
			Inline: inline,
		})
	}

	if s.quote != 0 {
		i = s.skipQuoted(line, 0, s.quote)
		if s.quote != 0 {
			return
		}
	}

	// tokenStart is true where a new node can begin; only there a quote starts a quoted scalar and
	// "|" or ">" a block scalar.
	tokenStart := i == 0
	content := i > 0
	entryCol := indentOf(line) // the column of the key or the sequence item holding the node
	tokCol := entryCol
	for ; i < len(line); i++ {
		c := line[i]
		switch {
		case isSpaceByte(c):
			continue
		case c == '#':
			if i == 0 || isSpaceByte(line[i-1]) {
				add(i, content)
				return
			}
			tokenStart = false
		case (c == '"' || c == '\'') && tokenStart:
			tokCol = i
			content = true
			s.quote = c
			i = s.skipQuoted(line, i+1, c) - 1
			if s.quote != 0 {
				return
			}
			tokenStart = false
		case (c == '|' || c == '>') && tokenStart:
			// Block scalar header. Only an indentation/chomping indicator and a comment may follow.
			rest := strings.TrimLeft(line[i+1:], "+-0123456789")
			if t := strings.TrimLeft(rest, " \t"); t == "" || t[0] == '#' {
				if t != "" && len(t) < len(rest) {
					add(len(line)-len(t), true)
				}
				s.inBlock, s.blockParent = true, entryCol
				return
			}
			content = true
			tokenStart = false
		case (c == '-' || c == '?') && tokenStart && (i+1 == len(line) || isSpaceByte(line[i+1])):
			entryCol = i
			content = true
		case c == '-' && i == 0 && strings.HasPrefix(line, "---") || c == '.' && i == 0 && strings.HasPrefix(line, "..."):
			i += 2
			tokenStart = true
			content = true
		case c == ':' && (i+1 == len(line) || isSpaceByte(line[i+1]) || strings.ContainsRune(",]}", rune(line[i+1]))):
			entryCol = tokCol
			tokenStart = true
			content = true
		case c == '[' || c == '{' || c == ',':
			tokenStart = true
			content = true
		case (c == '&' || c == '!' || c == '*') && tokenStart:
			// Anchor, tag or alias: a token of its own. A node may still follow an anchor or a tag.
			for i+1 < len(line) && !isSpaceByte(line[i+1]) {
				i++
			}
			content = true
		default:
			if tokenStart {
				tokCol = i
			}
			content = true
			tokenStart = false
		}
	}
}

// skipQuoted returns the index after the closing quote at or after from. When the quote is not
// closed on the line, it records that in s.quote and returns len(line).
func (s *commentScanner) skipQuoted(line string, from int, q byte) int {
	for i := from; i < len(line); i++ {
		switch {
		case q == '"' && line[i] == '\\':
			i++
		case line[i] == q:
			if q == '\'' && i+1 < len(line) && line[i+1] == '\'' {
				i++ // escaped quote
				continue
			}
			s.quote = 0
			return i + 1
		}
	}
	s.quote = q
	return len(line)
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}
