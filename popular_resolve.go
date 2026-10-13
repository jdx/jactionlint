package jactionlint

import (
	"regexp"
	"strconv"
	"strings"
)

// The data set of the popular actions (PopularActions, OutdatedPopularActionSpecs) is keyed by the major
// version of an action: "actions/cache@v4". Workflows pin actions in other ways as well: a full version
// ("actions/cache@v4.2.4"), or a commit with the version in a comment ("actions/cache@0400d5f... # v4.2.4"),
// which is what the default profile asks for. This file finds the key such a `uses:` value stands for.

// popularRefSource tells what the major version of a popularSpec was taken from.
type popularRefSource int

const (
	// popularExact is a `uses:` value which is a key of the data set as it is.
	popularExact popularRefSource = iota
	// popularFromTag is a version tag which is more specific than the major version: v4.2.4, v4.2, 4.2.4.
	popularFromTag
	// popularFromComment is a commit SHA whose version comment names the version: `# v4.2.4`.
	// The comment is the only evidence: nothing here checks that it is true.
	popularFromComment
)

// popularSpec is the key of the data set which a `uses:` value stands for.
type popularSpec struct {
	// Key is "owner/repo@vN", "owner/repo/path@vN" or the `uses:` value itself.
	Key    string
	Source popularRefSource
	// Comment is the version the comment names, as written, when Source is popularFromComment.
	Comment string
}

// popularMajorRe matches a version tag of any precision and nothing else: a pre-release or build suffix
// ("v4.2.4-beta.1") is not resolved because the metadata of a pre-release can be another one's. Leading
// zeros are not a version number.
var popularMajorRe = regexp.MustCompile(`^([vV]?)(0|[1-9][0-9]*)((?:\.[0-9]+){0,2})$`)

// majorOfVersion returns the major version tag ("v4") of a version such as "v4", "v4.2", "v4.2.4" or "4.2.4".
// A bare number without a dot ("4") is not taken for a version.
func majorOfVersion(v string) (string, bool) {
	m := popularMajorRe.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	if m[1] == "" && m[3] == "" {
		return "", false
	}
	return "v" + m[2], true
}

// versionTokenRe matches one word of a comment which looks like a version.
var versionTokenRe = regexp.MustCompile(`^[vV]?[0-9]+(?:\.[0-9]+)+(?:-[0-9A-Za-z.-]+)?$|^[vV][0-9]+$`)

// unambiguousCommentVersion returns the version a comment names when it names exactly one: "v4.2.4",
// "tag=v4.2.4" and "v4.2.4 (note)" do, "v4 -> v5" and "v4 (was v3)" do not.
func unambiguousCommentVersion(text string) (string, bool) {
	v, ok := commentVersion(text)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	first := true
	for _, w := range strings.Fields(text) {
		w = strings.Trim(w, "`'\"(),;")
		for _, p := range []string{"tag=", "tag:", "version=", "version:", "ver:", "ver="} {
			if len(w) >= len(p) && strings.EqualFold(w[:len(p)], p) {
				w = w[len(p):]
				break
			}
		}
		if versionTokenRe.MatchString(w) {
			if !first {
				return "", false
			}
			first = false
		}
	}
	return v, true
}

// resolvePopularSpec returns the key of the popular actions data set which the `uses:` value of a step
// stands for, or false when it is none of the known forms. The value as written wins; the major version is
// derived only when that is not a key. comments are the comments of the file of the step, which may be nil.
//
// Nothing is guessed: a commit without a version comment, a comment that names several versions, a branch, a
// pre-release and a short SHA stand for nothing.
func resolvePopularSpec(ref *UsesRef, uses *String, comments *CommentIndex, known func(string) bool) (popularSpec, bool) {
	if known(ref.Raw) {
		return popularSpec{Key: ref.Raw}, true
	}
	if ref.Kind != UsesAction || ref.Dynamic || !ref.HasRef {
		return popularSpec{}, false
	}
	base, _, ok := strings.Cut(ref.Raw, "@")
	if !ok {
		return popularSpec{}, false
	}
	switch ref.RefKind {
	case RefSemverTag:
		major, ok := majorOfVersion(ref.Ref)
		if !ok {
			return popularSpec{}, false
		}
		if k := base + "@" + major; known(k) {
			return popularSpec{Key: k, Source: popularFromTag}, true
		}
	case RefFullSHA:
		if comments == nil || uses == nil || uses.Pos == nil {
			return popularSpec{}, false
		}
		c := comments.Inline(uses.Pos.Line)
		if c == nil {
			return popularSpec{}, false
		}
		v, ok := unambiguousCommentVersion(c.Text)
		if !ok {
			return popularSpec{}, false
		}
		major, ok := majorOfVersion(v)
		if !ok {
			return popularSpec{}, false
		}
		if k := base + "@" + major; known(k) {
			return popularSpec{Key: k, Source: popularFromComment, Comment: v}, true
		}
	}
	return popularSpec{}, false
}

// knownPopularAction reports whether the data set has the action: its inputs or an outdated runner.
func knownPopularAction(spec string) bool {
	if _, ok := PopularActions[spec]; ok {
		return true
	}
	_, ok := OutdatedPopularActionSpecs[spec]
	return ok
}

// note says in a message which data was used for a `uses:` value which is not a key of the data set as it is,
// and what that rests on. It is empty for a value which is a key.
func (p popularSpec) note() string {
	switch p.Source {
	case popularFromTag:
		return " (checked as " + strconv.Quote(p.Key) + ", the major version of the tag)"
	case popularFromComment:
		return " (checked as " + strconv.Quote(p.Key) + ": the version comment \"# " + p.Comment + "\" says so, and nothing verifies the comment)"
	}
	return ""
}

// describe names the action in a message: the `uses:` value as written, then the note.
func (p popularSpec) describe(raw string) string {
	return strconv.Quote(raw) + p.note()
}
