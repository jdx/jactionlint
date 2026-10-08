package jactionlint

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// attachPinFixes gives the "unpinned-uses" errors of actions and reusable workflows referenced by a
// tag the fix that pins them to the commit the tag points to, with the tag name as a comment:
//
//	uses: actions/checkout@v4
//	uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4
//
// It needs GitHub to resolve the tag, so it exists only with the online checks. The fix is safe, and
// so offered, only when the result is the same code that runs today: the ref is the name of a tag
// of the repository (annotated tags are followed to the commit) and no branch has the same name,
// which would make it ambiguous. A ref which is a branch, an abbreviated SHA or unknown to GitHub
// gets no fix, and neither does a line this cannot edit without guessing (a comment which does
// not mention the ref, or other content after the value).
func (l *Linter) attachPinFixes(sess *onlineSession, src []byte, errs []*Error) {
	var starts []int // Byte offset of each line
	for _, e := range errs {
		if e.ID != "unpinned-uses" || e.Fix != nil || e.Line <= 0 || e.Column <= 0 {
			continue
		}
		if starts == nil {
			starts = lineStarts(src)
		}
		if e.Line > len(starts) {
			continue
		}
		fix, err := pinFix(sess, src, starts[e.Line-1], e.Column)
		if err != nil {
			l.debug("No fix for the error at line %d: %v", e.Line, err)
			continue
		}
		e.Fix = fix
	}
}

// lineStarts returns the byte offset where each line of the source starts.
func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// pinFix builds the fix for the `uses:` value which starts at the column (1-based, in characters) of
// the line that starts at lineStart.
func pinFix(sess *onlineSession, src []byte, lineStart, col int) (*Fix, error) {
	lineEnd := len(src)
	if i := bytes.IndexByte(src[lineStart:], '\n'); i >= 0 {
		lineEnd = lineStart + i
	}
	line := src[lineStart:lineEnd]
	line = bytes.TrimSuffix(line, []byte("\r"))

	// Byte offset of the value in the line
	off := 0
	for n := 1; n < col; n++ {
		if off >= len(line) {
			return nil, fmt.Errorf("the column is outside the line")
		}
		_, w := utf8.DecodeRune(line[off:])
		off += w
	}

	// The value: quoted or plain, on this line
	var spec string
	valueStart, valueEnd, tokenEnd := off, 0, 0
	quote := byte(0)
	if off < len(line) && (line[off] == '"' || line[off] == '\'') {
		quote = line[off]
		valueStart = off + 1
		i := bytes.IndexByte(line[valueStart:], quote)
		if i < 0 {
			return nil, fmt.Errorf("the quoted value is not closed on its line")
		}
		valueEnd = valueStart + i
		tokenEnd = valueEnd + 1
	} else {
		valueEnd = off
		for valueEnd < len(line) && line[valueEnd] != ' ' && line[valueEnd] != '\t' && line[valueEnd] != '#' {
			valueEnd++
		}
		tokenEnd = valueEnd
	}
	spec = string(line[valueStart:valueEnd])
	if strings.ContainsAny(spec, "\\{}[],") {
		return nil, fmt.Errorf("the value %q is not a plain action reference", spec)
	}

	ref := ParseUses(spec)
	if ref.Dynamic || (ref.Kind != UsesAction && ref.Kind != UsesReusableWorkflow) || (ref.RefKind != RefSemverTag && ref.RefKind != RefOther) {
		return nil, fmt.Errorf("%q is not an action or workflow referenced by a symbolic ref", spec)
	}
	if !reSafeRepoPart.MatchString(ref.Owner) || !reSafeRepoPart.MatchString(ref.Repo) {
		return nil, fmt.Errorf("%q is not a GitHub repository", spec)
	}

	sha, isTag, err := sess.TagCommit(ref.Owner, ref.Repo, ref.Ref)
	if err != nil {
		return nil, err
	}
	if !isTag {
		return nil, fmt.Errorf("%q is not a tag of %s/%s, so what it points to can change", ref.Ref, ref.Owner, ref.Repo)
	}
	if _, isBranch, err := sess.BranchCommit(ref.Owner, ref.Repo, ref.Ref); err != nil {
		return nil, err
	} else if isBranch {
		return nil, fmt.Errorf("%q is both a branch and a tag of %s/%s", ref.Ref, ref.Owner, ref.Repo)
	}
	if len(sha) != 40 || classifyRef(sha, true, false) != RefFullSHA {
		return nil, fmt.Errorf("%q does not resolve to a commit SHA", ref.Ref)
	}

	// What follows the value on the line
	rest := strings.TrimSpace(string(line[tokenEnd:]))
	refStart := lineStart + valueStart + strings.IndexByte(spec, '@') + 1
	refEnd := lineStart + valueEnd
	newText := sha
	switch {
	case rest == "":
		// No comment: name the tag in one, as Dependabot and Renovate do. The edit also covers the
		// closing quote, so that the comment lands after it, and the white space at the end of the line.
		refEnd = lineStart + len(line)
		if quote != 0 {
			newText += string(quote)
		}
		newText += " # " + ref.Ref
	case strings.HasPrefix(rest, "#") && commentNamesRef(rest[1:], ref.Ref):
		// The comment already names the version
	default:
		return nil, fmt.Errorf("the line has other content after the value")
	}
	return &Fix{
		Description: fmt.Sprintf("Pin %s/%s@%s to commit %s", ref.Owner, ref.Repo, ref.Ref, shortSHA(sha)),
		Edits:       []TextEdit{{Start: refStart, End: refEnd, NewText: newText}},
	}, nil
}

// commentNamesRef reports whether the comment names the ref as its version.
func commentNamesRef(comment, ref string) bool {
	v, ok := commentVersion(comment)
	return ok && v == ref
}
