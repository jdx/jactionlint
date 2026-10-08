package jactionlint

import "strings"

// UsesPattern is a pattern which matches the repository references of `uses:` keys, i.e. actions and
// reusable workflows. It is the matching logic of the forbidden-uses rule and is meant to be shared by
// the rules which select actions by name.
//
// A pattern is "{name}" or "{name}@{ref}". Names are compared case-insensitively and refs
// case-sensitively. In a name:
//
//   - "*" matches every action
//   - "owner/*" matches every action of the owner, including the ones in sub-directories
//   - "owner/repo/*" matches the repository itself and everything in it
//   - "owner/repo" matches only the root action of the repository. "owner/repo/sub" matches only that
//     sub-directory
//   - any other '*' matches any run of characters, "/" included
//
// Without "@{ref}" any ref matches.
type UsesPattern struct {
	raw    string
	name   string // lower-cased
	ref    string
	hasRef bool
}

// ParseUsesPattern parses a pattern.
func ParseUsesPattern(s string) UsesPattern {
	p := UsesPattern{raw: s}
	name := strings.TrimSpace(s)
	if i := strings.IndexByte(name, '@'); i >= 0 {
		p.ref, p.hasRef = name[i+1:], true
		name = name[:i]
	}
	p.name = strings.ToLower(strings.TrimRight(name, "/"))
	return p
}

// String returns the pattern as it was written.
func (p UsesPattern) String() string { return p.raw }

// Match reports whether the reference matches the pattern. Only actions and reusable workflows can
// match: local paths, Docker images and invalid values never do.
func (p UsesPattern) Match(u *UsesRef) bool {
	if u == nil || !u.IsRepo() {
		return false
	}
	if p.hasRef && !wildcardMatch(p.ref, u.Ref) {
		return false
	}
	name := strings.ToLower(u.CanonicalName())
	switch {
	case p.name == "*":
		return true
	case strings.HasSuffix(p.name, "/*") && strings.Count(p.name, "*") == 1:
		prefix := strings.TrimSuffix(p.name, "/*")
		return name == prefix || strings.HasPrefix(name, prefix+"/")
	case strings.Contains(p.name, "*"):
		return wildcardMatch(p.name, name)
	default:
		return name == p.name
	}
}

// wildcardMatch matches s against a pattern where '*' stands for any run of characters.
func wildcardMatch(pattern, s string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(s, mid)
		if i < 0 {
			return false
		}
		s = s[i+len(mid):]
	}
	return strings.HasSuffix(s, last) && len(s) >= len(last)
}
