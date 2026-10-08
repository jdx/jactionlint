package jactionlint

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// inlineIgnoreRe matches a whole-line comment which suppresses errors such as
// `# jactionlint ignore=<pattern>[,<pattern>...]`. `# actionlint ignore=...` is also accepted.
var inlineIgnoreRe = regexp.MustCompile(`^\s*#\s*j?actionlint\s+ignore=(.*)$`)

// inlineIgnoreEntry is one pattern of an inline ignore comment.
type inlineIgnoreEntry struct {
	pat IgnorePattern
	// line and col are the position of the pattern in the comment.
	line, col int
	// used is whether the pattern suppressed any error.
	used bool
	// zizmor is the audit name of an entry from a zizmor ignore comment. Then targets lists the
	// diagnostics it stands for and pat is not used.
	zizmor  string
	targets []zizmorAlias
}

func (e *inlineIgnoreEntry) match(err *Error) bool {
	if e.zizmor != "" {
		return e.matchesZizmor(err)
	}
	return e.pat.Match(err)
}

// inlineIgnore is one set of ignore patterns which is effective for errors reported in the line
// range [start, end] (1-based, inclusive).
type inlineIgnore struct {
	start, end int
	entries    []*inlineIgnoreEntry
	// commentLine is the line of a zizmor ignore comment, which also applies to the errors whose region
	// contains that line. It is 0 for the comments of jactionlint.
	commentLine int
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
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}

	var ret []inlineIgnore
	var errs []*Error
	var pending []*inlineIgnoreEntry
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
			// Patterns are located after "ignore="
			from := strings.Index(line, "ignore=") + len("ignore=")
			for _, p := range splitIgnoreList(m[1]) {
				raw := p
				p = strings.TrimSpace(p)
				if p == "" {
					from += len(raw) + 1
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
					continue
				}
				pending = append(pending, &inlineIgnoreEntry{pat: r, line: i + 1, col: patCol})
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
		ret = append(ret, inlineIgnore{start: i + 1, end: end, entries: pending})
		pending = nil
	}
	return ret, pending, errs
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
			if !ig.covers(err) {
				continue
			}
			for _, e := range ig.entries {
				if e.match(err) {
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

// covers returns whether the comment applies to the lines of the error.
func (ig inlineIgnore) covers(err *Error) bool {
	if ig.start <= err.Line && err.Line <= ig.end {
		return true
	}
	return ig.commentLine > 0 && err.Line <= ig.commentLine && ig.commentLine <= max(err.EndLine, err.Line)
}

// unusedInlineIgnores returns an error for each pattern of the inline ignore comments which did not
// suppress any error. A pattern for a rule which is off is not reported because the rule could not
// report anything.
func unusedInlineIgnores(ignores []inlineIgnore, orphans []*inlineIgnoreEntry, cfg *Config) []*Error {
	var errs []*Error
	report := func(e *inlineIgnoreEntry, what string) {
		if e.used {
			return
		}
		msg := fmt.Sprintf("ignore pattern %q %s. remove it", e.pat.String(), what)
		if e.zizmor != "" {
			// Only for an audit which maps onto a rule that is on: a rule which is off cannot report anything
			if !zizmorEntryActive(e, cfg) {
				return
			}
			msg = fmt.Sprintf("zizmor ignore comment for %q %s. remove it", e.zizmor, what)
		} else if e.pat.ID != "" && !cfg.RuleEnabled(e.pat.ID) {
			return
		}
		errs = append(errs, &Error{
			Message: msg,
			Line:    e.line,
			Column:  e.col,
			Kind:    "ignore",
			ID:      "unused-ignore",
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

// isSequenceItem returns true when the line is a block sequence item ("- ..." or a bare "-").
func isSequenceItem(line string) bool {
	t := strings.TrimLeft(line, " \t")
	return t == "-" || strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "-\t")
}

func init() {
	registerRules(
		RuleInfo{ID: "invalid-ignore-comment", Group: RuleGroupCorrectness, Summary: "An inline ignore comment is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault},
		RuleInfo{ID: "unused-ignore", Group: RuleGroupPolicy, Summary: "An inline ignore comment did not suppress anything.", DefaultLevel: SeverityError, Profile: ProfileStrict},
	)
}
