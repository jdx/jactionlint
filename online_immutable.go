package jactionlint

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// acceptImmutableReleases drops the "unpinned-uses" findings of the actions whose policy is "immutable-release" and
// whose ref is the tag of an immutable GitHub release. GitHub locks the tag of such a release (it cannot be moved or
// deleted) and its assets, so the tag is as good as a commit SHA for as long as the release exists.
//
// It needs GitHub to tell, so it does nothing without the online checks: then the policy is hash-pin. The tag is
// accepted only when
//
//   - the tag has a published release which says "immutable": true, and
//   - no branch has the name of the tag, because `uses: owner/repo@v1.2.3` would be ambiguous: a branch can be pushed
//     by whoever can write to the repository, whatever the release says.
//
// A moving tag (`v4`), a tag of a release which is not immutable (the setting is for the releases published
// after it was turned on), a branch, a ref which is not looked up and a lookup which fails keep the finding. The
// lookups are the release of each tag and the branch of the same name, once for each in the run; the branch is also
// what ref-confusion and the pin fix ask for.
func (l *Linter) acceptImmutableReleases(cfg *Config, src []byte, errs []*Error) []*Error {
	if cfg == nil || !cfg.RuleEnabled("unpinned-uses") {
		return errs
	}
	policies, _ := cfg.ruleOptionStringMap("unpinned-uses", "policies")
	wanted := false
	for _, p := range policies {
		wanted = wanted || p == "immutable-release"
	}
	if !wanted {
		return errs
	}
	sess, _ := l.onlineSession(cfg)
	if sess == nil {
		return errs
	}
	var starts []int
	var kept []*Error
	changed := false
	for _, e := range errs {
		if e.ID == "unpinned-uses" && e.Line > 0 && e.Column > 0 {
			if starts == nil {
				starts = lineStarts(src)
			}
			if e.Line <= len(starts) {
				if ref := usesRefAt(src, starts[e.Line-1], e.Column); ref != nil && usesPolicyFor(policies, ref) == usesPolicyImmutableRelease && l.isImmutableRelease(sess, ref) {
					changed = true
					continue
				}
			}
		}
		kept = append(kept, e)
	}
	if !changed {
		return errs
	}
	return kept
}

// isImmutableRelease reports whether the ref of the `uses:` value is the tag of an immutable release, and not also a branch.
func (l *Linter) isImmutableRelease(sess *onlineSession, ref *UsesRef) bool {
	if ref.Dynamic || (ref.Kind != UsesAction && ref.Kind != UsesReusableWorkflow) || (ref.RefKind != RefSemverTag && ref.RefKind != RefOther) {
		return false
	}
	if !validGitHubOwner(ref.Owner) || !validGitHubRepo(ref.Repo) || !validGitRefName(ref.Ref) {
		return false
	}
	rel, err := sess.Release(ref.Owner, ref.Repo, ref.Ref)
	if err != nil {
		l.debug("%s: not accepted as an immutable release: %v", ref.Raw, err)
		return false
	}
	if !rel.Immutable {
		l.debug("%s: the release is not immutable", ref.Raw)
		return false
	}
	if _, isBranch, err := sess.BranchCommit(ref.Owner, ref.Repo, ref.Ref); err != nil {
		l.debug("%s: not accepted as an immutable release: %v", ref.Raw, err)
		return false
	} else if isBranch {
		l.debug("%s: a branch has the name of the tag", ref.Raw)
		return false
	}
	return true
}

// usesRefAt parses the `uses:` value which starts at the column (1-based, in characters) of the line that starts at lineStart:
// plain or quoted, on that line. It returns nil for anything else.
func usesRefAt(src []byte, lineStart, col int) *UsesRef {
	lineEnd := len(src)
	if i := bytes.IndexByte(src[lineStart:], '\n'); i >= 0 {
		lineEnd = lineStart + i
	}
	line := bytes.TrimSuffix(src[lineStart:lineEnd], []byte("\r"))
	off := 0
	for n := 1; n < col; n++ {
		if off >= len(line) {
			return nil
		}
		_, w := utf8.DecodeRune(line[off:])
		off += w
	}
	start, end := off, off
	if off < len(line) && (line[off] == '"' || line[off] == '\'') {
		start = off + 1
		i := bytes.IndexByte(line[start:], line[off])
		if i < 0 {
			return nil
		}
		end = start + i
	} else {
		for end < len(line) && line[end] != ' ' && line[end] != '\t' && line[end] != '#' {
			end++
		}
	}
	spec := string(line[start:end])
	if spec == "" || strings.ContainsAny(spec, "\\{}[],") {
		return nil
	}
	return ParseUses(spec)
}
