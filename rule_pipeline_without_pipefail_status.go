package jactionlint

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// reStatusAssign matches the whole statement `rc=${PIPESTATUS[0]}` (also `local`, `declare`, `export` and
// `readonly` in front, and `$PIPESTATUS`). Arrays and other expressions are not followed.
var reStatusAssign = regexp.MustCompile(`^(?:(?:local|declare|export|readonly)\s+)?([A-Za-z_][A-Za-z0-9_]*)=["']?\$\{?PIPESTATUS(?:\[(-?\d+)\])?\}?["']?\s*$`)

// reCheckLine matches a line that tests or returns a status: a test (`[`, `[[`, `test`, `((`), `exit` or `return`,
// optionally behind `if`, `elif`, `while`, `until`, `!` or `then`.
var reCheckLine = regexp.MustCompile(`^\s*(?:(?:if|elif|while|until|then|!)\s+)*(?:\[\[?\s|test\s|\(\(|exit\b|return\b)`)

// reTrailingComment matches a comment that ends a statement (`rc=${PIPESTATUS[0]} # keep`).
var reTrailingComment = regexp.MustCompile(`\s#.*$`)

// followsPipestatus looks at the statement right after the pipeline. simple is whether it copies the status of the
// stage that hides the failure into a plain variable (`rc=${PIPESTATUS[0]}`), and then checked is whether the script
// checks the variable later: in a test, an arithmetic condition, `exit` or `return`. It is conservative: the
// variable must not be assigned again before the check.
func followsPipestatus(s *runscript.Script, p *runscript.Pipeline, stage int) (simple, checked bool) {
	if stage < 0 {
		return false, false
	}
	src := s.Source
	pos := p.End
	// skip what ends the pipeline, blank lines and comments
	for {
		for pos < len(src) && strings.IndexByte(" \t\r\n;", src[pos]) >= 0 {
			pos++
		}
		if pos < len(src) && src[pos] == '#' {
			i := strings.IndexByte(src[pos:], '\n')
			if i < 0 {
				return false, false
			}
			pos += i
			continue
		}
		break
	}
	end := pos + strings.IndexAny(src[pos:]+"\n", ";\n")
	stmt := src[pos:end]
	if i := reTrailingComment.FindStringIndex(stmt); i != nil {
		stmt = stmt[:i[0]]
	}
	m := reStatusAssign.FindStringSubmatch(strings.TrimSpace(stmt))
	if m == nil {
		return false, false
	}
	if m[2] != "" {
		n, _ := strconv.Atoi(m[2])
		if n != stage && !(n < 0 && len(p.Stages)+n == stage) {
			return false, false
		}
	} else if stage != 0 {
		return false, false
	}
	name := m[1]
	reassign := -1
	for _, a := range s.Assignments {
		if a.Name == name && a.Offset >= end && (reassign < 0 || a.Offset < reassign) {
			reassign = a.Offset
		}
	}
	reUse := regexp.MustCompile(`\$\{?` + name + `\b|\(\(.*\b` + name + `\b`)
	for _, loc := range reUse.FindAllStringIndex(src[end:], -1) {
		off := end + loc[0]
		if reassign >= 0 && off > reassign {
			return true, false
		}
		ls := strings.LastIndexByte(src[:off], '\n') + 1
		le := off + strings.IndexByte(src[off:]+"\n", '\n')
		// the check may follow the use on the same line (`rc=${PIPESTATUS[0]}; exit $rc`): only the statement that
		// holds the use counts, so cut at the last separator before it
		start := ls + statementStart(src[ls:off])
		if reCheckLine.MatchString(src[start:le]) {
			return true, true
		}
	}
	return true, false
}

// statusHandled reports whether the script looks at the status of the stage itself. A copy into a plain variable
// counts only when the variable is checked later, any other read of PIPESTATUS right after the pipeline counts.
func statusHandled(s *runscript.Script, p *runscript.Pipeline, stage int) bool {
	if simple, checked := followsPipestatus(s, p, stage); simple {
		return checked
	}
	return readsPipestatus(s, p, stage)
}

// statementStart returns where the last statement of a line prefix begins: after the last `;`, `&&` or `||` that is
// not inside a test (`[[ a && b ]]`, `[ a ] ...`, `(( a || b ))`).
func statementStart(seg string) int {
	start, depth := 0, 0
	for i := 0; i < len(seg); i++ {
		rest := seg[i:]
		switch {
		case strings.HasPrefix(rest, "[["), strings.HasPrefix(rest, "(("):
			depth++
			i++
		case strings.HasPrefix(rest, "]]"), strings.HasPrefix(rest, "))"):
			if depth > 0 {
				depth--
			}
			i++
		case rest[0] == '[' && (i == 0 || strings.IndexByte(" \t;&|", seg[i-1]) >= 0) && len(rest) > 1 && (rest[1] == ' ' || rest[1] == '\t'):
			depth++
		case rest[0] == ']' && depth > 0 && i > 0 && (seg[i-1] == ' ' || seg[i-1] == '\t'):
			depth--
		case depth == 0 && rest[0] == ';':
			start = i + 1
		case depth == 0 && (strings.HasPrefix(rest, "&&") || strings.HasPrefix(rest, "||")):
			start = i + 2
			i++
		}
	}
	return start
}
