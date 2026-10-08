package jactionlint

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// inlineIgnoreRe matches a whole-line comment which suppresses errors such as
// `# jactionlint ignore=<pattern>[,<pattern>...]`. `# actionlint ignore=...` is also accepted.
var inlineIgnoreRe = regexp.MustCompile(`^\s*#\s*j?actionlint\s+ignore=(.*)$`)

// trailingIgnoreRe finds the ignore directive in a comment which follows YAML content on its line,
// e.g. `uses: actions/checkout@abc # v4 # jactionlint ignore=unpinned-uses`. It is given the text from the
// `#` which starts the comment. The directive is the first `#` after white space (or the start of the
// comment) that is followed by `jactionlint ignore=`.
var trailingIgnoreRe = regexp.MustCompile(`(?:^|\s)#\s*j?actionlint\s+ignore=(.*)$`)

// inlineIgnoreEntry is one pattern of an inline ignore comment.
type inlineIgnoreEntry struct {
	pat IgnorePattern
	// line and col are the position of the pattern in the comment.
	line, col int
	// used is whether the pattern suppressed any error.
	used bool
	// comment is the comment line the pattern is written in.
	comment *ignoreComment
}

// ignoreComment is one comment line with `ignore=`. Its offsets let the unused-ignore rule remove the
// patterns which did nothing.
type ignoreComment struct {
	// lineStart and lineEnd are the byte range of the whole line including its line terminator.
	lineStart, lineEnd int
	// valStart and valEnd are the byte range of the text after `ignore=`.
	valStart, valEnd int
	// segs are the comma-separated parts of the text after `ignore=`.
	segs []ignoreSeg
}

// ignoreSeg is one part of the list in a comment. entry is nil when the part is empty or invalid.
type ignoreSeg struct {
	text  string
	entry *inlineIgnoreEntry
}

// inlineIgnore is one set of ignore patterns which is effective for errors reported in the line
// range [start, end] (1-based, inclusive).
type inlineIgnore struct {
	start, end int
	entries    []*inlineIgnoreEntry
}

// splitIgnoreList splits comma-separated patterns. Commas inside (), [] and {} or escaped with a
// backslash do not separate patterns so that regular expressions like `a{1,2}` work.
func splitIgnoreList(s string) []string {
	var ret []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				ret = append(ret, s[start:i])
				start = i + 1
			}
		}
	}
	return append(ret, s[start:])
}

func isCommentOrBlank(line string) (comment bool, blank bool) {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "#"), t == ""
}

// parseInlineIgnores scans the source for `# jactionlint ignore=...` comments. There are two forms.
//
// A comment on its own line applies to the next YAML line which is not a comment nor blank and to the
// lines nested under it. Multiple comment lines can be stacked.
//
// A comment at the end of a line, after YAML content, applies to that line and to the lines nested
// under it. The directive may follow other comment text (`uses: a/b@sha # v1 # jactionlint ignore=x`).
// A '#' inside a quoted string or a block scalar such as a `run: |` script is not a comment.
//
// In both forms, when the target line starts a sequence item ("- "), the whole item is the target:
// a comment on the first line of a step covers the whole step. Invalid patterns are reported as errors.
func parseInlineIgnores(src []byte) ([]inlineIgnore, []*Error) {
	ignores, _, errs := parseInlineIgnoresWithOrphans(src)
	return ignores, errs
}

// parseInlineIgnoresWithOrphans is like parseInlineIgnores but also returns the patterns of the
// comments which have nothing to apply to (e.g. a comment at the end of the file).
func parseInlineIgnoresWithOrphans(src []byte) ([]inlineIgnore, []*inlineIgnoreEntry, []*Error) {
	if !bytes.Contains(src, []byte("actionlint")) {
		return nil, nil, nil
	}
	lines := strings.Split(string(src), "\n")
	starts := make([]int, len(lines)+1) // byte offsets of the lines in the source
	for i, l := range lines {
		starts[i+1] = starts[i] + len(l) + 1
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	comments := NewCommentIndex(src)

	var ret []inlineIgnore
	var errs []*Error
	var pending []*inlineIgnoreEntry

	// addDirective turns the text after `ignore=` into entries. dirStart is the byte offset in the line
	// of the '#' starting the directive, from the byte offset of the list, and the comment owns the
	// bytes [delStart, delEnd) of the file when it is deleted as a whole.
	addDirective := func(i int, line string, dirStart, from int, list string, delStart, delEnd int) {
		col := strings.Index(line[dirStart:], "jactionlint")
		if col < 0 {
			col = strings.Index(line[dirStart:], "actionlint")
		}
		col = utf8.RuneCountInString(line[:dirStart+col]) + 1
		cm := &ignoreComment{lineStart: delStart, lineEnd: delEnd, valStart: starts[i] + from, valEnd: starts[i] + from + len(list)}
		for _, p := range splitIgnoreList(list) {
			raw := p
			p = strings.TrimSpace(p)
			if p == "" {
				from += len(raw) + 1
				cm.segs = append(cm.segs, ignoreSeg{})
				continue
			}
			patCol := utf8.RuneCountInString(line[:from]) + 1 + utf8.RuneCountInString(raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))])
			from += len(raw) + 1
			r, err := ParseIgnorePattern(p)
			if err != nil {
				errs = append(errs, &Error{
					Message: fmt.Sprintf("invalid regular expression %q in inline ignore comment: %s", p, err.Error()),
					Line:    i + 1,
					Column:  col,
					Kind:    "syntax-check",
					ID:      "invalid-ignore-comment",
				})
				cm.segs = append(cm.segs, ignoreSeg{text: p})
				continue
			}
			e := &inlineIgnoreEntry{pat: r, line: i + 1, col: patCol, comment: cm}
			cm.segs = append(cm.segs, ignoreSeg{text: p, entry: e})
			pending = append(pending, e)
		}
	}

	for i, line := range lines {
		comment, blank := isCommentOrBlank(line)
		if blank {
			continue
		}
		if comment {
			m := inlineIgnoreRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			// Patterns are located after "ignore="
			from := strings.Index(line, "ignore=") + len("ignore=")
			dirStart := strings.Index(line, "#")
			addDirective(i, line, dirStart, from, m[1], starts[i], min(starts[i+1], len(src)))
			continue
		}
		// A comment at the end of a line with content applies to this line, together with the comments above
		if c := comments.Inline(i + 1); c != nil && strings.Contains(c.Text, "actionlint") {
			hash := byteOffsetOfColumn(line, c.Column)
			rest := line[hash:]
			if loc := trailingIgnoreRe.FindStringSubmatchIndex(rest); loc != nil {
				dirStart := hash + loc[0]
				if !strings.HasPrefix(rest[loc[0]:], "#") {
					dirStart++ // the match starts with the white space before '#'
				}
				// Deleting the directive takes the white space before it, but not the comment before it
				delStart := dirStart
				for delStart > 0 && (line[delStart-1] == ' ' || line[delStart-1] == '\t') {
					delStart--
				}
				addDirective(i, line, dirStart, hash+loc[2], rest[loc[2]:loc[3]], starts[i]+delStart, starts[i]+len(line))
			}
		}
		if len(pending) == 0 {
			continue
		}

		end := blockEnd(lines, i)
		ret = append(ret, inlineIgnore{start: i + 1, end: end, entries: pending})
		pending = nil
	}
	return ret, pending, errs
}

// byteOffsetOfColumn returns the byte offset in the line of the 1-based column counted in characters.
func byteOffsetOfColumn(line string, col int) int {
	n := 0
	for i := range line {
		n++
		if n == col {
			return i
		}
	}
	return len(line)
}

// blockEnd returns the 1-based number of the last line of the block which starts at the 0-based line i:
// the line itself and the lines nested under it. When the line starts a sequence item ("- "), the whole
// item is the block.
func blockEnd(lines []string, i int) int {
	line := lines[i]
	// This line is the target. Compute the indentation which the nested lines must exceed.
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	threshold := indent // for a sequence item, this makes the whole item the target
	// YAML allows the items of a sequence to sit at the same column as the key holding it:
	//   key:
	//   - a
	// When the target is a key, such items belong to it.
	isKey := !isSequenceItem(line)
	end := i + 1
	for j := i + 1; j < len(lines); j++ {
		// Comments are skipped like blank lines so that a comment does not cut a nested block short
		if c, b := isCommentOrBlank(lines[j]); b || c {
			continue
		}
		ind := len(lines[j]) - len(strings.TrimLeft(lines[j], " \t"))
		if ind < threshold || (ind == threshold && !(isKey && isSequenceItem(lines[j]))) {
			break
		}
		end = j + 1
	}
	return end
}

// filterInlineIgnores removes errors suppressed by inline ignore comments and marks the patterns which
// suppressed any error as used.
func (l *Linter) filterInlineIgnores(errs []*Error, ignores []inlineIgnore) []*Error {
	if len(ignores) == 0 {
		return errs
	}
	filtered := make([]*Error, 0, len(errs))
	for _, err := range errs {
		ignored := false
		for _, ig := range ignores {
			if err.Line < ig.start || ig.end < err.Line {
				continue
			}
			for _, e := range ig.entries {
				if e.pat.Match(err) {
					e.used = true
					ignored = true
				}
			}
		}
		if ignored {
			l.debug("Error %q is ignored due to the inline ignore comment", err.Message)
			continue
		}
		filtered = append(filtered, err)
	}
	return filtered
}

// unusedInlineIgnores returns an error for each pattern of the inline ignore comments which did not
// suppress any error. A pattern for a rule which is off is not reported because the rule could not
// report anything.
func unusedInlineIgnores(ignores []inlineIgnore, orphans []*inlineIgnoreEntry, cfg *Config) []*Error {
	var errs []*Error
	// stale tells whether the pattern is reported: it did nothing and could have done something
	stale := func(e *inlineIgnoreEntry) bool {
		return !e.used && (e.pat.ID == "" || cfg.RuleEnabled(e.pat.ID))
	}
	report := func(e *inlineIgnoreEntry, what string) {
		if !stale(e) {
			return
		}
		errs = append(errs, &Error{
			Message: fmt.Sprintf("ignore pattern %q %s. remove it", e.pat.String(), what),
			Line:    e.line,
			Column:  e.col,
			Kind:    "ignore",
			ID:      "unused-ignore",
			Fix:     e.comment.removeStale(func(o *inlineIgnoreEntry) bool { return stale(o) && o.comment == e.comment }),
		})
	}
	for _, ig := range ignores {
		for _, e := range ig.entries {
			report(e, "did not suppress any error")
		}
	}
	for _, e := range orphans {
		report(e, "is not followed by any line to apply to")
	}
	return errs
}

// removeStale makes the fix removing the patterns of the comment for which stale is true. The
// comment line is deleted when nothing else is left in it. Otherwise the list is rewritten with the
// remaining patterns. All the stale patterns of a comment get the same edit, so applying them
// together never conflicts.
func (c *ignoreComment) removeStale(stale func(*inlineIgnoreEntry) bool) *Fix {
	if c == nil {
		return nil
	}
	var kept []string
	for _, s := range c.segs {
		if s.text == "" || (s.entry != nil && stale(s.entry)) {
			continue
		}
		kept = append(kept, s.text)
	}
	if len(kept) == 0 {
		return &Fix{Description: "Remove the unused ignore comment", Edits: []TextEdit{{Start: c.lineStart, End: c.lineEnd}}}
	}
	return &Fix{
		Description: "Remove the unused ignore patterns",
		Edits:       []TextEdit{{Start: c.valStart, End: c.valEnd, NewText: strings.Join(kept, ",")}},
	}
}

// dropFixesChangingYAML removes the fixes which would change what the YAML file means. Removing a
// comment does not, except in odd places such as the middle of a multi-line plain scalar, where the
// comment ends the scalar. The fix is checked by parsing the file with and without it.
func dropFixesChangingYAML(src []byte, errs []*Error) {
	var before any
	parsed := false
	for _, e := range errs {
		if e.Fix == nil {
			continue
		}
		if !parsed {
			parsed = true
			if yaml.Unmarshal(src, &before) != nil {
				before = nil
			}
		}
		out, n := applyFixes(src, []*Error{e}, FixModeUnsafe)
		var after any
		if n != 1 || before == nil || yaml.Unmarshal(out, &after) != nil || !reflect.DeepEqual(before, after) {
			e.Fix = nil
		}
	}
}

// isSequenceItem returns true when the line is a block sequence item ("- ..." or a bare "-").
func isSequenceItem(line string) bool {
	t := strings.TrimLeft(line, " \t")
	return t == "-" || strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "-\t")
}

func init() {
	registerRules(
		RuleInfo{ID: "invalid-ignore-comment", Group: RuleGroupCorrectness, Summary: "An inline ignore comment is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault},
		RuleInfo{ID: "unused-ignore", Group: RuleGroupPolicy, Summary: "An ignore comment or an entry of \"ignores\" in the config file did not suppress anything.", DefaultLevel: SeverityError, Profile: ProfileStrict},
	)
}
