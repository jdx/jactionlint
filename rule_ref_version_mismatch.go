package jactionlint

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// RuleRefVersionMismatch reports a hash-pinned action whose version comment names another version
// than the pinned commit is. Dependabot and Renovate keep the "# v4.2.2" comment of a pinned
// action up to date; a comment that went stale makes them skip it and misleads the readers.
//
// With the source of the file the finding carries an unsafe fix, which rewrites the version in the
// comment to the tag of the pinned commit. It is unsafe because what the mismatch means is not
// known: the comment may be stale, or the SHA may be wrong (replaced by hand, or by an attacker). The
// commit is what runs, so the fix believes the commit, but a person has to confirm that is the one meant.
type RuleRefVersionMismatch struct {
	onlineUsesRule
	src []byte
	idx *sourceIndex
}

// NewRuleRefVersionMismatch creates a new RuleRefVersionMismatch instance.
// The source of the file is needed to attach the fix, and can be nil.
func NewRuleRefVersionMismatch(sess *onlineSession, src []byte) *RuleRefVersionMismatch {
	return &RuleRefVersionMismatch{onlineUsesRule: newOnlineUsesRule("ref-version-mismatch", "Checks that the version comment of a hash-pinned action matches the pinned commit", sess), src: src}
}

// VisitWorkflowPost implements Pass.
func (r *RuleRefVersionMismatch) VisitWorkflowPost(*Workflow) error {
	for _, s := range r.sites {
		if s.ref.RefKind != RefFullSHA || s.comment == nil {
			continue
		}
		version, _, _, ok := commentVersionAt(s.comment.Text, s.ref)
		if !ok {
			continue // A comment about something else
		}
		sha := strings.ToLower(s.ref.Ref)
		idx, err := r.sess.Tags(s.ref.Owner, s.ref.Repo)
		if err != nil {
			r.skipped(s, "the tags", err)
			continue
		}
		tagged := idx.bySHA[sha]
		if hasVersion(tagged, version) {
			continue
		}

		// The commit is not tagged with the version of the comment. Does the tag of that name exist, and where?
		var at string
		found := false
		failed := false
		for _, name := range versionSpellings(version) {
			c, ok, err := r.sess.TagCommit(s.ref.Owner, s.ref.Repo, name)
			if err != nil {
				r.skipped(s, "the tag "+name, err)
				found, failed = false, true
				at = ""
				break
			}
			if ok {
				at, found = c, true
				version = name
				break
			}
		}
		if failed || r.sess.stopped() != nil {
			continue // The lookup did not work: say nothing rather than guess
		}
		if found && at == sha {
			continue // The list of tags was incomplete
		}

		var problem string
		if found {
			problem = fmt.Sprintf("tag %q points to commit %s", version, shortSHA(at))
		} else {
			problem = fmt.Sprintf("%s has no tag %q", s.repoSlug(), version)
		}
		hint := "the pinned commit has no tag"
		if len(tagged) > 0 {
			hint = "the pinned commit is tagged " + quotes(tagged)
		}
		r.ReportIDf("ref-version-mismatch", s.pos,
			"the version comment %q does not match the commit pinned in %s %q: %s, but %s. either the comment is stale or the SHA is wrong, so check which one is meant: update the comment, or pin the commit of the version you mean",
			"# "+s.comment.Text, s.what(), s.ref.Raw, problem, hint)
		r.attachFix(s, tagged, idx.truncated)
	}
	return nil
}

// attachFix gives the error just reported the fix which rewrites the version of the comment to the tag of the
// pinned commit, or says why there is none.
func (r *RuleRefVersionMismatch) attachFix(s usesSite, tagged []string, truncated bool) {
	if r.src == nil || len(r.errs) == 0 {
		return
	}
	e := r.errs[len(r.errs)-1]
	fix, noFix := r.fixFor(s, tagged, truncated)
	e.Fix, e.NoFix = fix, noFix
}

func (r *RuleRefVersionMismatch) fixFor(s usesSite, tagged []string, truncated bool) (*Fix, *NoFix) {
	shape := func(why string) (*Fix, *NoFix) {
		return nil, &NoFix{Code: NoFixUnsupportedShape, Reason: why}
	}
	judgment := func(why string) (*Fix, *NoFix) {
		return nil, &NoFix{Code: NoFixNeedsJudgment, Reason: why}
	}
	version, vs, ve, ok := commentVersionAt(s.comment.Text, s.ref)
	if !ok {
		return shape("the version in the comment cannot be located")
	}
	switch {
	case truncated:
		return judgment("the repository has more tags than were read, so the tag of the pinned commit may be missing: check the version of the commit and update the comment")
	case len(tagged) == 0:
		return judgment("the pinned commit has no tag, so there is no version to write: pin the commit of a release, or remove the comment")
	}
	tag, ok := pickVersionTag(tagged, "", hasVPrefix(version))
	if !ok {
		if !slices.ContainsFunc(tagged, func(n string) bool { return semverTagRegex.MatchString(n) }) {
			return judgment("the pinned commit has no version tag (it is tagged " + quotes(tagged) + "): name the version in the comment yourself")
		}
		return judgment("the pinned commit has several versions of the same specificity (" + quotes(tagged) + "): choose the one to name in the comment")
	}

	if r.idx == nil {
		r.idx = newSourceIndex(r.src)
	}
	hash, ok := r.idx.offset(s.comment.Line, s.comment.Column)
	if !ok || hash >= len(r.src) || r.src[hash] != '#' {
		return shape("the comment cannot be located in the file")
	}
	lineEnd := r.idx.lineEnd(s.comment.Line)
	raw := string(r.src[hash+1 : lineEnd])
	lead := len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
	if !strings.HasPrefix(raw[lead:], s.comment.Text) || string(r.src[hash+1+lead+vs:hash+1+lead+ve]) != version {
		return shape("the comment cannot be located in the file")
	}
	start := hash + 1 + lead + vs
	return &Fix{
		Description: fmt.Sprintf("Change the version of the comment from %s to %s, the tag of the pinned commit. Unsafe: a mismatch can also mean that the SHA is wrong, and the commit is what runs", version, tag),
		Unsafe:      true,
		Edits:       []TextEdit{{Start: start, End: start + len(version), NewText: tag}},
	}, nil
}

// hasVersion reports whether the version of the comment is the name of one of the tags of the commit,
// ignoring the "v" prefix. A less specific version does not cover a more specific tag: after
// "# v4" was written the tag v4 moved on to a newer release, so the comment says nothing true about
// the pinned commit any more. zizmor draws the line there too.
func hasVersion(tags []string, version string) bool {
	for _, t := range tags {
		if strings.TrimLeft(t, "vV") == strings.TrimLeft(version, "vV") {
			return true
		}
	}
	return false
}

// versionSpellings returns the tag names a version of a comment can stand for: as written, then with
// or without the "v" prefix.
func versionSpellings(v string) []string {
	t := strings.TrimLeft(v, "vV")
	ret := []string{v}
	for _, c := range []string{t, "v" + t} {
		if !slices.Contains(ret, c) {
			ret = append(ret, c)
		}
	}
	return ret
}

func init() {
	registerRules(
		RuleInfo{ID: "ref-version-mismatch", Group: RuleGroupSecurity, Summary: "The version comment of a hash-pinned action does not match the pinned commit.", DefaultLevel: SeverityWarning, Online: true, Fixable: true, DocsAnchor: "check-ref-version-mismatch"},
	)
	registerRuleFactory("ref-version-mismatch", func(env *RuleEnv) []Rule {
		if env.online == nil || !env.config.RuleEnabled("ref-version-mismatch") {
			return nil
		}
		return []Rule{NewRuleRefVersionMismatch(env.online, env.Source())}
	})
}
