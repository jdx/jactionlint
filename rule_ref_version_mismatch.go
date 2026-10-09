package jactionlint

import (
	"fmt"
	"slices"
	"strings"
)

// RuleRefVersionMismatch reports a hash-pinned action whose version comment names another version
// than the pinned commit is. Dependabot and Renovate keep the "# v4.2.2" comment of a pinned
// action up to date; a comment that went stale makes them skip it and misleads the readers.
type RuleRefVersionMismatch struct {
	onlineUsesRule
}

// NewRuleRefVersionMismatch creates a new RuleRefVersionMismatch instance.
func NewRuleRefVersionMismatch(sess *onlineSession) *RuleRefVersionMismatch {
	return &RuleRefVersionMismatch{newOnlineUsesRule("ref-version-mismatch", "Checks that the version comment of a hash-pinned action matches the pinned commit", sess)}
}

// VisitWorkflowPost implements Pass.
func (r *RuleRefVersionMismatch) VisitWorkflowPost(*Workflow) error {
	for _, s := range r.sites {
		if s.ref.RefKind != RefFullSHA || s.comment == nil {
			continue
		}
		version, ok := commentVersion(s.comment.Text)
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
		found, failed := false, false
		for _, name := range versionSpellings(version) {
			c, ok, err := r.sess.TagCommit(s.ref.Owner, s.ref.Repo, name)
			if err != nil {
				r.skipped(s, "the tag "+name, err)
				failed = true
				break
			}
			if ok {
				at, found = c, true
				version = name
				break
			}
		}
		if failed {
			continue // The tag could not be looked up, so there is nothing to say about it
		}
		if err := r.sess.stopped(); err != nil {
			continue
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
			"the version comment %q does not match the commit pinned in %s %q: %s, but %s. update the comment, or pin the commit of the version you mean",
			"# "+s.comment.Text, s.what(), s.ref.Raw, problem, hint)
	}
	return nil
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
		RuleInfo{ID: "ref-version-mismatch", Group: RuleGroupSecurity, Summary: "The version comment of a hash-pinned action does not match the pinned commit.", DefaultLevel: SeverityWarning, Online: true, DocsAnchor: "check-ref-version-mismatch"},
	)
	registerRuleFactory("ref-version-mismatch", func(env *RuleEnv) []Rule {
		if env.online == nil || !env.config.RuleEnabled("ref-version-mismatch") {
			return nil
		}
		return []Rule{NewRuleRefVersionMismatch(env.online)}
	})
}
