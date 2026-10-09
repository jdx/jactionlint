package jactionlint

import (
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// This file has what github-env needs to tell a value that was made safe before it is written to
// $GITHUB_ENV or $GITHUB_PATH. Two things make an outsider's value harmless there:
//
//   - it cannot carry a newline, so it cannot add a variable: `tr -d '\n'`, `${v//$'\n'/}`, `head -n 1`;
//   - it was cut down to a character set, or validated against one, that holds no newline, slash or dot:
//     `sed 's/[^a-zA-Z0-9-]/-/g'`, `${v//[^a-zA-Z0-9]/}`, `[[ "$v" =~ ^[a-z0-9]+$ ]] || exit 1`.
//
// A directory added to $GITHUB_PATH is dangerous without a newline, so only the second kind makes it safe.

// sanitizeKind tells what a command or an expansion guarantees about its output.
type sanitizeKind int

const (
	sanitizeNone sanitizeKind = iota
	// sanitizeNewline: the output has no newline.
	sanitizeNewline
	// sanitizeCharset: the output holds only letters, digits and a few harmless characters.
	sanitizeCharset
)

// satisfies reports whether the guarantee is enough for a write to the destination ("GITHUB_ENV" or "GITHUB_PATH").
func (k sanitizeKind) satisfies(dest string) bool {
	if dest == "GITHUB_PATH" {
		return k == sanitizeCharset
	}
	return k != sanitizeNone
}

var reSetClass = regexp.MustCompile(`\[:(alnum|alpha|digit|lower|upper|xdigit):\]`)

// charsetSafe reports whether a set of characters (the inside of a bracket expression or the argument of tr)
// holds only letters, digits, ranges of them and a few punctuation marks that are harmless in a value. For a
// path the dot, slash and colon are not.
func charsetSafe(set string, forPath bool) bool {
	set = reSetClass.ReplaceAllString(set, "")
	for _, r := range set {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		case !forPath && strings.ContainsRune("./:+@,=", r):
		default:
			return false
		}
	}
	return true
}

// setHasNewline reports whether a tr set or a sed/bash pattern names the newline.
func setHasNewline(set string) bool {
	return strings.Contains(set, `\n`) || strings.Contains(set, `\012`) || strings.Contains(set, "[:space:]") || strings.Contains(set, "[:cntrl:]")
}

// commandSanitizes tells what the output of the command guarantees whatever its input is.
func commandSanitizes(c *runscript.Command, dest string) sanitizeKind {
	if c == nil {
		return sanitizeNone
	}
	switch c.Name {
	case "tr":
		del, comp := c.HasFlag("-d"), c.HasFlag("-c", "-C")
		if len(c.Positional) == 0 || c.Positional[0].Dynamic() {
			return sanitizeNone
		}
		set1 := c.Positional[0].Value
		switch {
		case del && comp:
			if !setHasNewline(set1) && charsetSafe(set1, dest == "GITHUB_PATH") {
				return sanitizeCharset
			}
		case del:
			if setHasNewline(set1) {
				return sanitizeNewline
			}
		case len(c.Positional) == 2 && !c.Positional[1].Dynamic():
			set2 := c.Positional[1].Value
			if comp && !setHasNewline(set1) && charsetSafe(set1, dest == "GITHUB_PATH") && !setHasNewline(set2) {
				return sanitizeCharset
			}
			if !comp && setHasNewline(set1) && !setHasNewline(set2) {
				return sanitizeNewline
			}
		}
	case "head":
		if f := firstLineOnly(c); f {
			return sanitizeNewline
		}
	case "sed":
		var scripts []*runscript.Word
		scripts = append(scripts, c.FlagValues("-e", "--expression")...)
		if len(scripts) == 0 && len(c.Positional) > 0 {
			scripts = append(scripts, c.Positional[0])
		}
		best := sanitizeNone
		for _, w := range scripts {
			if !staticWord(w) {
				continue
			}
			if k := sedSanitizes(w.Value, c.HasFlag("-z"), dest); k > best {
				best = k
			}
		}
		return best
	}
	return sanitizeNone
}

// staticWord reports whether the word has no expression, variable or substitution; a single quoted word with a `$`
// in it (a sed script) is static.
func staticWord(w *runscript.Word) bool {
	return len(w.Exprs) == 0 && len(w.Vars) == 0 && !w.Subst
}

// firstLineOnly reports whether `head` prints one line: `head -1`, `head -n 1`, `head -n1`.
func firstLineOnly(c *runscript.Command) bool {
	for _, f := range c.Flags {
		if f.Name == "-1" || f.Name == "-n1" {
			return true
		}
		if (f.Name == "-n" || f.Name == "--lines") && f.Value != nil && !f.Value.Dynamic() && f.Value.Value == "1" {
			return true
		}
	}
	if c.HasFlag("-n") && len(c.Positional) > 0 && c.Positional[0].Value == "1" {
		return true
	}
	return false
}

var (
	reSedWhitelist = regexp.MustCompile(`s/\[\^([^\]]+)\]/([^/\n]*)/[0-9I]*g`)
	reSedNewline   = regexp.MustCompile(`s/(\\n|\\r\\n|\\s|\[\[:space:\]\])/`)
)

// sedSanitizes judges the script of sed. sed works on one line at a time, so a whitelist replaces every character
// of a line but never its newline (unless -z puts the whole input in one line): a set that holds `=` still lets a
// value carry a second line `NAME=value`.
func sedSanitizes(script string, nulSeparated bool, dest string) sanitizeKind {
	if m := reSedWhitelist.FindStringSubmatch(script); m != nil && charsetSafe(m[1], dest == "GITHUB_PATH") && !setHasNewline(m[2]) && !strings.Contains(m[2], "&") && (nulSeparated || !strings.Contains(m[1], "=")) {
		return sanitizeCharset
	}
	if reSedNewline.MatchString(script) && (nulSeparated || strings.Contains(script, "N")) {
		return sanitizeNewline
	}
	return sanitizeNone
}

// keepsSanitized reports whether the stage of a pipeline, which comes after the sanitizing one, cannot bring back
// what the sanitizer took out. Only stages that select, cut or reorder the lines they get qualify, and the
// translations that are proven not to write a newline (or, for a path, anything but a deletion). sed, tr, awk,
// printf, xargs and the like can write a newline or a dot whatever they read, so unless one of them is proven
// harmless below the pipeline is not trusted.
func keepsSanitized(c *runscript.Command, dest string) bool {
	switch c.Name {
	case "head", "tail", "sort", "uniq", "wc", "rev", "tee":
		return true
	case "cut":
		return !c.HasFlag("--output-delimiter")
	case "tr":
		return trKeeps(c, dest)
	case "sed":
		return dest != "GITHUB_PATH" && sedKeeps(c)
	}
	return false
}

var reTrPlain = regexp.MustCompile(`^[A-Za-z0-9 _.,:;@/+=-]*$`)

// trKeeps: `tr -d SET`, `tr -s SET` and, for the environment, `tr SET1 SET2` with plain sets: none of them can
// write a newline that was not in the input.
func trKeeps(c *runscript.Command, dest string) bool {
	for _, f := range c.Flags {
		if f.Name != "-d" && f.Name != "-s" && f.Name != "-c" && f.Name != "-C" && f.Name != "-ds" && f.Name != "-sd" {
			return false
		}
	}
	if len(c.Positional) == 0 || len(c.Positional) > 2 {
		return false
	}
	for _, p := range c.Positional {
		if p.Dynamic() || !reTrPlain.MatchString(p.Value) {
			return false
		}
	}
	if len(c.Positional) == 2 && (dest == "GITHUB_PATH" || c.HasFlag("-d")) {
		return dest != "GITHUB_PATH" && !c.HasFlag("-d")
	}
	return true
}

var reSedFlags = regexp.MustCompile(`^[gIi0-9]*$`)

// sedPlainSubstitution reports whether the script is one `s/old/new/flags` (any of / | # , as the delimiter) whose
// parts write no newline: no `\n`, `\r`, `\t` or numbered escape, and no newline.
func sedPlainSubstitution(script string) bool {
	if len(script) < 4 || script[0] != 's' || !strings.ContainsRune("/|#,", rune(script[1])) {
		return false
	}
	parts := strings.Split(script[2:], script[1:2])
	if len(parts) != 3 || !reSedFlags.MatchString(parts[2]) {
		return false
	}
	return !strings.Contains(script, "\n") && !reSedEscape.MatchString(script)
}

// reSedEscape matches the escapes of sed that stand for a control character or a character by number: a newline
// (or a tab, a return) in the replacement, or one `\x0a` away.
var reSedEscape = regexp.MustCompile(`\\[nrtfvaxocd0-9]`)

// sedKeeps: only `s/old/new/flags` scripts whose parts hold no backslash and no newline; a newline in the input
// is not possible (it was removed), so & cannot bring one either.
func sedKeeps(c *runscript.Command) bool {
	if c.HasFlag("-z", "-s", "-i", "-n", "--null-data", "--in-place") {
		return false
	}
	scripts := c.FlagValues("-e", "--expression")
	if len(scripts) == 0 && len(c.Positional) > 0 {
		scripts = c.Positional[:1]
	}
	if len(scripts) == 0 {
		return false
	}
	for _, w := range scripts {
		if !staticWord(w) || !sedPlainSubstitution(w.Value) {
			return false
		}
	}
	return true
}

// sanitizedCommands returns the commands of the substitutions of the word whose output is made safe for the
// destination: a pipeline with a sanitizing stage followed by commands that keep it, or a sanitizing command
// on its own. The earlier stages of the pipeline and the commands inside them are included, so what they read
// does not matter.
func sanitizedCommands(w *runscript.Word, dest string) map[*runscript.Command]bool {
	if len(w.Subs) == 0 {
		return nil
	}
	type span struct{ from, to int }
	var spans []span
	seen := map[*runscript.Pipeline]bool{}
	for _, c := range w.Subs {
		if p := c.Pipeline; p != nil {
			if seen[p] {
				continue
			}
			seen[p] = true
			found := -1
			for i, st := range p.Stages {
				for _, sc := range st.Commands {
					if commandSanitizes(sc, dest).satisfies(dest) {
						found = i
					}
				}
			}
			if found < 0 {
				continue
			}
			ok := true
			for _, st := range p.Stages[found+1:] {
				for _, sc := range st.Commands {
					if !keepsSanitized(sc, dest) {
						ok = false
					}
				}
			}
			if ok {
				spans = append(spans, span{p.Offset, p.End})
			}
			continue
		}
		if commandSanitizes(c, dest).satisfies(dest) {
			spans = append(spans, span{c.Offset, c.End})
		}
	}
	if len(spans) == 0 {
		return nil
	}
	out := map[*runscript.Command]bool{}
	for _, c := range w.Subs {
		for _, sp := range spans {
			if c.Offset >= sp.from && c.End <= sp.to {
				out[c] = true
			}
		}
	}
	return out
}

var reSanitizedExpansion = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)//([^/]+)/([^}]*)\}`)

// sanitizedExpansions returns the names of the variables which the word expands with all occurrences of
// something replaced, so no newline (or nothing but a harmless set) is left: `${v//[^a-zA-Z0-9]/}`, `${v//$'\n'/}`.
func sanitizedExpansions(raw, dest string) []string {
	var names []string
	for _, m := range reSanitizedExpansion.FindAllStringSubmatch(raw, -1) {
		pat, repl := m[2], m[3]
		if setHasNewline(repl) || strings.Contains(repl, "$") {
			continue
		}
		kind := sanitizeNone
		switch {
		case strings.HasPrefix(pat, "[^") && strings.HasSuffix(pat, "]") && charsetSafe(pat[2:len(pat)-1], dest == "GITHUB_PATH") && !setHasNewline(pat):
			kind = sanitizeCharset
		case strings.Contains(pat, `$'\n'`) || strings.Contains(pat, `$'\r'`) || strings.Contains(pat, `$'\r\n'`) || strings.Contains(pat, "[[:space:]]") || strings.Contains(pat, "[[:cntrl:]]"):
			if !strings.HasPrefix(pat, "[^") {
				kind = sanitizeNewline
			}
		}
		if kind.satisfies(dest) {
			names = append(names, m[1])
		}
	}
	return names
}

// subtractOnce removes one occurrence of each of the items from the list.
func subtractOnce(list, items []string) []string {
	if len(items) == 0 || len(list) == 0 {
		return list
	}
	counts := map[string]int{}
	for _, it := range items {
		counts[it]++
	}
	out := make([]string, 0, len(list))
	for _, s := range list {
		if counts[s] > 0 {
			counts[s]--
			continue
		}
		out = append(out, s)
	}
	return out
}

// reSafeRegexAtom matches what a validating regular expression may hold between its anchors: bracket
// expressions of harmless characters, escaped punctuation, groups, alternation, quantifiers and literals.
var reSafeRegexAtom = regexp.MustCompile(`^(?:\[[^\]^]+\]|\\[.+*?()\[\]{}|\-/_]|\\[wd]|[A-Za-z0-9_\-]|[()|+*?]|\{[0-9]+(?:,[0-9]*)?\})+$`)

// validatingRegex reports whether the regular expression of a `[[ x =~ re ]]` test holds an anchored
// pattern that no newline can match, and the character set it allows is harmless for the destination.
func validatingRegex(re, dest string) bool {
	if !strings.HasPrefix(re, "^") || !strings.HasSuffix(re, "$") || strings.HasSuffix(re, `\$`) {
		return false
	}
	inner := re[1 : len(re)-1]
	if !reSafeRegexAtom.MatchString(inner) {
		return false
	}
	for _, br := range regexp.MustCompile(`\[[^\]]*\]`).FindAllString(inner, -1) {
		if setHasNewline(br) || !charsetSafe(br[1:len(br)-1], dest == "GITHUB_PATH") {
			return false
		}
	}
	if dest == "GITHUB_PATH" && (strings.ContainsAny(inner, "./") || strings.Contains(inner, `\w`)) {
		return false
	}
	return !strings.Contains(inner, `\n`)
}
