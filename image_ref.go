package jactionlint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// This file has what the rules about container images share: how an image reference is normalized, and the glob
// patterns of the policies of unpinned-images and of the allow and deny lists of forbidden-uses.

// imageRefParts splits an image reference into its name (with the registry, if written), its tag and its digest.
// The name keeps a registry host with a port ("localhost:5000/img").
func imageRefParts(ref string) (name, tag, digest string) {
	name = ref
	if i := strings.IndexByte(name, '@'); i >= 0 {
		name, digest = name[:i], name[i+1:]
	}
	if i := strings.LastIndexByte(name, ':'); i >= 0 && !strings.Contains(name[i+1:], "/") {
		name, tag = name[:i], name[i+1:]
	}
	return name, tag, digest
}

// normalizedImageName returns the name of the image with the registry written out: "node" is
// "docker.io/library/node" and "grafana/grafana" is "docker.io/grafana/grafana". The case is lowered, since
// registries do not know upper case names.
func normalizedImageName(name string) string {
	name = strings.ToLower(name)
	first, rest, hasSlash := strings.Cut(name, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		if first == "index.docker.io" || first == "registry-1.docker.io" {
			first = "docker.io"
		}
		if first == "docker.io" && !strings.Contains(rest, "/") {
			return "docker.io/library/" + rest
		}
		return first + "/" + rest
	}
	if !hasSlash {
		return "docker.io/library/" + name
	}
	return "docker.io/" + name
}

// imagePattern is a glob pattern for the images of a policy or a list: "*" matches any characters, "/" included.
// A pattern without a tag matches the name of the image, written as in the workflow or with the registry written
// out ("node" and "docker.io/library/node" are the same). A pattern with a tag ("node:20", "ghcr.io/o/*:v1")
// matches the name and the tag, and a tag of "*" matches any (also none).
type imagePattern struct {
	raw string
	// res are the expressions of the name as written and, when it differs, of the name with the registry written out
	// ("node" and "docker.io/library/node"): a pattern matches an image which either form of it matches.
	res    []*regexp.Regexp
	hasTag bool
	// prefix is the number of literal characters before the first "*" of the normalized form of the name (the form with the
	// registry written out, the longer one when both have a prefix), and wildcards the number of "*" of that form.
	prefix, wildcards int
}

// moreSpecific reports whether pattern a is more specific than pattern b, in this order:
//
//  1. more literal characters before the first wildcard, in the normalized pattern ("node" is "docker.io/library/node");
//  2. fewer wildcards;
//  3. a pattern with a tag beats the same name without one;
//  4. the longer pattern as written.
//
// Patterns that tie on all of these are equally specific: the caller keeps the first one in lexical order.
func moreSpecific(a, b imagePattern) bool {
	switch {
	case a.prefix != b.prefix:
		return a.prefix > b.prefix
	case a.wildcards != b.wildcards:
		return a.wildcards < b.wildcards
	case a.hasTag != b.hasTag:
		return a.hasTag
	}
	return len(a.raw) > len(b.raw)
}

// globRegexp turns a name with "*" (which matches any characters) into an anchored expression, with the tag when it has one.
func globRegexp(name, tag string, hasTag bool) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	write := func(s string) {
		for i, seg := range strings.Split(regexp.QuoteMeta(s), `\*`) {
			if i > 0 {
				b.WriteString(".*")
			}
			b.WriteString(seg)
		}
	}
	write(name)
	if hasTag {
		b.WriteString(":")
		write(tag)
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func parseImagePattern(s string) (imagePattern, error) {
	p := imagePattern{raw: s}
	if s == "" || strings.ContainsAny(s, " \t@") || strings.Contains(s, "://") {
		return p, fmt.Errorf("pattern %q is invalid. it must look like \"*\", \"ghcr.io/owner/*\", \"registry.example.com/team/image\" or \"node:20\"", s)
	}
	name, tag, _ := imageRefParts(strings.ToLower(s))
	p.hasTag = tag != ""
	forms := []string{name}
	if name != "" && name != "*" {
		if n := normalizedImageName(name); n != name {
			forms = append(forms, n)
		}
	}
	for _, f := range forms {
		re, err := globRegexp(f, tag, p.hasTag)
		if err != nil {
			return p, fmt.Errorf("pattern %q is invalid: %v", s, err)
		}
		p.res = append(p.res, re)
	}
	for k, f := range forms {
		prefix := len(f)
		if i := strings.IndexByte(f, '*'); i >= 0 {
			prefix = i
		}
		if k == 0 || prefix > p.prefix {
			p.prefix, p.wildcards = prefix, strings.Count(f, "*")
		}
	}
	return p, nil
}

// matches reports whether the image reference (a name with an optional tag and digest) matches: some form of the pattern
// matches some form of the image.
func (p imagePattern) matches(ref string) bool {
	name, tag, _ := imageRefParts(ref)
	for _, n := range []string{strings.ToLower(name), normalizedImageName(name)} {
		cand := n
		if p.hasTag {
			cand = n + ":" + tag
		}
		for _, re := range p.res {
			if re.MatchString(strings.ToLower(cand)) {
				return true
			}
		}
	}
	return false
}

// imagePatternMatches reports whether the image matches one of the patterns. Invalid patterns do not match (the
// configuration is validated before).
func imagePatternMatches(patterns []string, ref string) (string, bool) {
	for _, s := range patterns {
		if p, err := parseImagePattern(s); err == nil && p.matches(ref) {
			return s, true
		}
	}
	return "", false
}

func validateImagePatterns(v any) error {
	ss, _ := v.([]string)
	for _, s := range ss {
		if _, err := parseImagePattern(s); err != nil {
			return err
		}
	}
	return nil
}

// imagePolicy says how strongly an image must be pinned.
type imagePolicy int

const (
	// imagePolicyHashPin requires a content digest.
	imagePolicyHashPin imagePolicy = iota
	// imagePolicyTagPin requires a tag other than "latest", or a digest.
	imagePolicyTagPin
	// imagePolicyAny requires nothing.
	imagePolicyAny
)

func parseImagePolicy(s string) (imagePolicy, bool) {
	switch s {
	case "hash-pin":
		return imagePolicyHashPin, true
	case "tag-pin", "ref-pin":
		return imagePolicyTagPin, true
	case "any":
		return imagePolicyAny, true
	}
	return 0, false
}

func validateImagePolicies(v any) error {
	m, _ := v.(map[string]string)
	for pat, pol := range m {
		if _, err := parseImagePattern(pat); err != nil {
			return err
		}
		if _, ok := parseImagePolicy(pol); !ok {
			return fmt.Errorf("policy %q of %q is invalid. available policies are \"hash-pin\", \"tag-pin\" and \"any\"", pol, pat)
		}
	}
	return nil
}

// imagePolicyFor returns the policy of the most specific pattern that matches the image (see moreSpecific), or the fallback
// when no pattern does. Patterns are tried in lexical order, so two that are equally specific give the same answer every run.
func imagePolicyFor(policies map[string]string, ref string, fallback imagePolicy) imagePolicy {
	pats := make([]string, 0, len(policies))
	for p := range policies {
		pats = append(pats, p)
	}
	sort.Strings(pats)
	best, found := fallback, false
	var bestPat imagePattern
	for _, raw := range pats {
		p, err := parseImagePattern(raw)
		if err != nil || !p.matches(ref) {
			continue
		}
		pol, ok := parseImagePolicy(policies[raw])
		if !ok || (found && !moreSpecific(p, bestPat)) {
			continue
		}
		best, bestPat, found = pol, p, true
	}
	return best
}
