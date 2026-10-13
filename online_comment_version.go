package jactionlint

import (
	"strings"
	"unicode"
)

// ratchetPrefix starts the version comment of a pin written by ratchet (https://github.com/sethvargo/ratchet):
//
//	uses: actions/checkout@<sha> # ratchet:actions/checkout@v4
const ratchetPrefix = "ratchet:"

// commentVersionAt finds the version which the text of a comment after a pinned `uses:` starts with, like
// commentVersion, and also tells where it is: text[start:end] is the version. With ref (the action the
// comment is about) it also reads the form of ratchet, "ratchet:owner/repo@v4", when the repository it
// names is the one of ref. A ratchet comment about another action says nothing about this one, and
// neither does one which is not a version ("ratchet:exclude", "ratchet:owner/repo@main").
func commentVersionAt(text string, ref *UsesRef) (version string, start, end int, ok bool) {
	base := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
	rest := text[base:]

	if len(rest) >= len(ratchetPrefix) && strings.EqualFold(rest[:len(ratchetPrefix)], ratchetPrefix) {
		if ref == nil {
			return "", 0, 0, false
		}
		word := rest[len(ratchetPrefix):]
		if i := strings.IndexAny(word, " \t"); i >= 0 {
			word = word[:i]
		}
		at := strings.LastIndexByte(word, '@')
		if at < 0 || !strings.EqualFold(word[:at], ratchetSlug(ref)) {
			return "", 0, 0, false
		}
		v := word[at+1:]
		if !reVersionToken.MatchString(v) {
			return "", 0, 0, false
		}
		start = base + len(ratchetPrefix) + at + 1
		return v, start, start + len(v), true
	}

	for _, p := range []string{"tag=", "tag:", "version=", "version:", "ver:", "ver="} {
		if len(rest) >= len(p) && strings.EqualFold(rest[:len(p)], p) {
			after := rest[len(p):]
			trimmed := strings.TrimLeftFunc(after, unicode.IsSpace)
			base += len(p) + len(after) - len(trimmed)
			rest = trimmed
			break
		}
	}
	word := rest
	if i := strings.IndexAny(word, " \t"); i >= 0 {
		word = word[:i]
	}
	const cutset = "`'\"(),;"
	lead := len(word) - len(strings.TrimLeft(word, cutset))
	w := strings.Trim(word, cutset)
	if !reVersionToken.MatchString(w) {
		return "", 0, 0, false
	}
	start = base + lead
	return w, start, start + len(w), true
}

// ratchetSlug is how ratchet names the action in its comment: "owner/repo" and the path inside it.
func ratchetSlug(ref *UsesRef) string {
	slug := ref.Owner + "/" + ref.Repo
	if sub := strings.Trim(ref.Subpath, "/"); sub != "" {
		slug += "/" + sub
	}
	return slug
}
