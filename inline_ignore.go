package jactionlint

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// inlineIgnoreRe matches a whole-line comment which suppresses errors such as
// `# jactionlint ignore=<pattern>[,<pattern>...]`. `# actionlint ignore=...` is also accepted.
var inlineIgnoreRe = regexp.MustCompile(`^\s*#\s*j?actionlint\s+ignore=(.*)$`)

// inlineIgnore is one set of ignore patterns which is effective for errors reported in the line
// range [start, end] (1-based, inclusive).
type inlineIgnore struct {
	start, end int
	pats       IgnorePatterns
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

// parseInlineIgnores scans the source for `# jactionlint ignore=...` comments. A comment must be
// placed on its own line. It applies to the next YAML line which is not a comment nor blank and to
// the lines nested under it. When the line starts a sequence item ("- "), the whole item is the
// target. Multiple comment lines can be stacked. Invalid patterns are reported as errors.
func parseInlineIgnores(src []byte) ([]inlineIgnore, []*Error) {
	if !bytes.Contains(src, []byte("actionlint")) {
		return nil, nil
	}
	lines := strings.Split(string(src), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}

	var ret []inlineIgnore
	var errs []*Error
	var pending IgnorePatterns
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
			col := strings.Index(line, "jactionlint")
			if col < 0 {
				col = strings.Index(line, "actionlint")
			}
			col++
			for _, p := range splitIgnoreList(m[1]) {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				r, err := ParseIgnorePattern(p)
				if err != nil {
					errs = append(errs, &Error{
						Message: fmt.Sprintf("invalid regular expression %q in inline ignore comment: %s", p, err.Error()),
						Line:    i + 1,
						Column:  col,
						Kind:    "syntax-check",
						ID:      "invalid-ignore-comment",
					})
					continue
				}
				pending = append(pending, r)
			}
			continue
		}
		if len(pending) == 0 {
			continue
		}

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
		ret = append(ret, inlineIgnore{i + 1, end, pending})
		pending = nil
	}
	return ret, errs
}

// filterInlineIgnores removes errors suppressed by inline ignore comments.
func (l *Linter) filterInlineIgnores(errs []*Error, ignores []inlineIgnore) []*Error {
	if len(ignores) == 0 {
		return errs
	}
	filtered := make([]*Error, 0, len(errs))
Loop:
	for _, err := range errs {
		for _, ig := range ignores {
			if ig.start <= err.Line && err.Line <= ig.end && ig.pats.Match(err) {
				l.debug("Error %q is ignored due to the inline ignore comment", err.Message)
				continue Loop
			}
		}
		filtered = append(filtered, err)
	}
	return filtered
}

// isSequenceItem returns true when the line is a block sequence item ("- ..." or a bare "-").
func isSequenceItem(line string) bool {
	t := strings.TrimLeft(line, " \t")
	return t == "-" || strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "-\t")
}
