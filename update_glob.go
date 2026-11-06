package jactionlint

import (
	"fmt"
	"regexp"
	"strings"
)

// tagPattern is a compiled glob pattern for tag names. `*` matches any characters, `?` one character, `[abc]` and
// `[a-z]` one of a set, and `{a,b}` is expanded into the patterns `a` and `b` first (they nest).
type tagPattern struct {
	res []*regexp.Regexp
}

// compileTagPattern compiles a glob pattern for the tag filters of `jactionlint update`.
func compileTagPattern(p string) (*tagPattern, error) {
	if p == "" {
		return nil, fmt.Errorf("the pattern is empty")
	}
	alts, err := expandBraces(p)
	if err != nil {
		return nil, err
	}
	tp := &tagPattern{}
	for _, a := range alts {
		re, err := globToRegexp(a)
		if err != nil {
			return nil, err
		}
		tp.res = append(tp.res, re)
	}
	return tp, nil
}

func (t *tagPattern) match(name string) bool {
	for _, re := range t.res {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// maxTagPatternAlternatives bounds the patterns that braces expand to, so that a hostile or careless setting cannot
// allocate without limit.
const maxTagPatternAlternatives = 256

// expandBraces expands the first group of braces and then the rest, so `v{1,2}.{0,1}` gives four patterns. A backslash
// makes the next character literal, braces included.
func expandBraces(p string) ([]string, error) {
	open := -1
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '{':
			open = i
		case '}':
			return nil, fmt.Errorf("unmatched } in the pattern")
		}
		if open >= 0 {
			break
		}
	}
	if open < 0 {
		return []string{p}, nil
	}
	depth, last := 0, open+1
	var parts []string
	for i := open; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				parts = append(parts, p[last:i])
				rest, err := expandBraces(p[i+1:])
				if err != nil {
					return nil, err
				}
				var subs [][]string
				total := 0
				for _, part := range parts {
					sub, err := expandBraces(part)
					if err != nil {
						return nil, err
					}
					subs = append(subs, sub)
					total += len(sub)
				}
				if total*len(rest) > maxTagPatternAlternatives {
					return nil, fmt.Errorf("the braces expand to more than %d patterns", maxTagPatternAlternatives)
				}
				out := make([]string, 0, total*len(rest))
				for _, sub := range subs {
					for _, s := range sub {
						for _, r := range rest {
							out = append(out, p[:open]+s+r)
						}
					}
				}
				return out, nil
			}
		case ',':
			if depth == 1 {
				parts = append(parts, p[last:i])
				last = i + 1
			}
		}
	}
	return nil, fmt.Errorf("unmatched { in the pattern")
}

func globToRegexp(g string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(g); i++ {
		c := g[i]
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '[':
			end := strings.IndexByte(g[i+1:], ']')
			if end < 0 {
				return nil, fmt.Errorf("missing ] in the pattern")
			}
			set := g[i+1 : i+1+end]
			if strings.HasPrefix(set, "!") {
				set = "^" + set[1:]
			}
			b.WriteString("[" + set + "]")
			i += end + 1
		case '\\':
			if i+1 >= len(g) {
				return nil, fmt.Errorf("the pattern ends with a backslash")
			}
			i++
			b.WriteString(regexp.QuoteMeta(string(g[i])))
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}
	return re, nil
}
