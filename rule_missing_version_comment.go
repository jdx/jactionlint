package jactionlint

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// RuleMissingVersionComment reports an action or reusable workflow pinned to a commit SHA without a
// comment that names its version ("uses: actions/checkout@<sha> # v4.2.2"). Dependabot and Renovate
// read the comment to know which release the pin is and to keep it up to date, and a reviewer cannot
// tell what a bare SHA is. This is the part of the zizmor audit ref-version-mismatch which is about
// a missing comment; the comment that does not match the commit is reported by the online rule of the same name.
//
// It needs no network. With the online checks on, the finding carries a fix which adds the comment: the
// most specific version tag that points to the commit. The fix is safe because it only adds a comment.
type RuleMissingVersionComment struct {
	onlineUsesRule
	src []byte
}

// NewRuleMissingVersionComment creates a new RuleMissingVersionComment instance. sess (nil without
// the online checks) is used to find the version for the fix, and src (which can be nil) to attach it.
func NewRuleMissingVersionComment(sess *onlineSession, src []byte) *RuleMissingVersionComment {
	return &RuleMissingVersionComment{
		onlineUsesRule: newOnlineUsesRule("missing-version-comment", "Checks that actions pinned to a commit SHA have a comment which names their version", sess),
		src:            src,
	}
}

// VisitWorkflowPost implements Pass.
func (r *RuleMissingVersionComment) VisitWorkflowPost(*Workflow) error {
	var starts []int
	for _, s := range r.sites {
		if s.ref.RefKind != RefFullSHA {
			continue
		}
		if s.comment != nil {
			if _, _, _, ok := commentVersionAt(s.comment.Text, s.ref); ok || mentionsVersion(s.comment.Text) || namesBranch(s.comment.Text) {
				continue
			}
		}
		var msg string
		if s.comment == nil {
			msg = fmt.Sprintf("%s %q is pinned to a commit but has no comment which names its version. add one like \"# v1.2.3\" after it: Dependabot and Renovate read it to update the pin, and it tells a reader what the commit is",
				s.what(), s.ref.Raw)
		} else {
			msg = fmt.Sprintf("%s %q is pinned to a commit but its comment %q names no version. write the version at its start like \"# v1.2.3\": Dependabot and Renovate read it to update the pin, and it tells a reader what the commit is",
				s.what(), s.ref.Raw, "# "+s.comment.Text)
		}
		r.ReportIDf("missing-version-comment", s.pos, "%s", msg)
		if r.src == nil {
			continue
		}
		if starts == nil {
			starts = lineStarts(r.src)
		}
		e := r.errs[len(r.errs)-1]
		if fix, err := r.fixFor(s, starts); err != nil {
			r.Debug("No fix for %s: %v", s.ref.Raw, err)
			e.NoFix = noFixOf(err)
		} else {
			e.Fix = fix
		}
	}
	return nil
}

// fixFor builds the edit which adds the comment with the version of the commit at the end of the line of
// the `uses:` value.
func (r *RuleMissingVersionComment) fixFor(s usesSite, starts []int) (*Fix, error) {
	if s.comment != nil {
		return nil, judgmentErr("the comment has other text: write the version at its start")
	}
	if r.sess == nil {
		return nil, &pinFixError{code: NoFixOnlineRequired, msg: "adding the version needs GitHub to read the tags of the repository, run with --online to enable the fix"}
	}
	if s.pos.Line < 1 || s.pos.Line > len(starts) {
		return nil, shapeErr("the position of the value is outside the file")
	}
	lineStart := starts[s.pos.Line-1]
	v, err := scanUsesValue(r.src, lineStart, s.pos.Col)
	if err != nil {
		return nil, err
	}
	if rest := bytes.TrimSpace(v.line[v.tokenEnd:]); len(rest) > 0 {
		return nil, shapeErr("the line has other content after the value")
	}
	if v.spec != s.ref.Raw {
		return nil, shapeErr("the value in the file is not the one which was parsed")
	}

	idx, err := r.sess.Tags(s.ref.Owner, s.ref.Repo)
	if err != nil {
		return nil, &pinFixError{code: NoFixLookupFailed, err: err,
			msg: fmt.Sprintf("adding the version needs the tags of %s and the lookup failed: %v", s.repoSlug(), err)}
	}
	tagged := idx.bySHA[strings.ToLower(s.ref.Ref)]
	switch {
	case idx.truncated:
		return nil, judgmentErr("%s has more tags than were read, so the version of the commit cannot be told for sure: write it yourself", s.repoSlug())
	case len(tagged) == 0:
		return nil, judgmentErr("no tag of %s points to the commit, so it has no version to name: pin the commit of a release", s.repoSlug())
	}
	tag, ok := pickVersionTag(tagged, "", true)
	if !ok {
		if len(versionTags(tagged)) == 0 {
			return nil, judgmentErr("the commit is tagged %s, and none is a version: name the version yourself", quotes(tagged))
		}
		return nil, judgmentErr("the commit has several versions of the same specificity (%s): choose the one to name", quotes(tagged))
	}
	end := lineStart + len(v.line)
	return &Fix{
		Description: fmt.Sprintf("Add the comment \"# %s\", the tag of the pinned commit %s", tag, shortSHA(s.ref.Ref)),
		Edits:       []TextEdit{{Start: lineStart + v.tokenEnd, End: end, NewText: " # " + tag}},
	}, nil
}

// trackedBranches are the names of the branches which a comment gives to a pin that follows one
// ("# main"): the commit is not a release, so there is no version to name.
var trackedBranches = []string{"main", "master", "trunk", "develop", "development", "dev", "next", "latest", "stable", "nightly", "head"}

// versionWordRe finds a version in the text of a comment: "1.2", "v1.2.3" or "v4", not as a part of a word ("v4abc", "pinv4").
var versionWordRe = regexp.MustCompile(`(?:^|[^0-9A-Za-z.])(?:[vV]?[0-9]+(?:\.[0-9]+)+|[vV][0-9]+)(?:$|[^0-9A-Za-z])`)

// mentionsVersion reports whether the comment has a version anywhere, such as "pin@v7.0.1" or "Release v4.4.1". Such a
// comment says which version the pin is even when it is not written the way the bots write it. Whether the
// comment is the version of the commit is the business of ref-version-mismatch.
func mentionsVersion(text string) bool { return versionWordRe.MatchString(text) }

// namesBranch reports whether the comment starts with the name of a branch which is usually followed, such as
// "main" or "master (updated by a bot)": the pin follows the branch on purpose.
func namesBranch(text string) bool {
	w := strings.TrimSpace(text)
	for _, p := range []string{"branch=", "branch:"} {
		if len(w) >= len(p) && strings.EqualFold(w[:len(p)], p) {
			w = strings.TrimSpace(w[len(p):])
		}
	}
	if i := strings.IndexAny(w, " \t"); i >= 0 {
		w = w[:i]
	}
	w = strings.ToLower(strings.Trim(w, "`'\"(),;"))
	return slices.Contains(trackedBranches, w)
}

// versionTags returns the names which are version tags.
func versionTags(names []string) []string {
	var ret []string
	for _, n := range names {
		if semverTagRegex.MatchString(n) && !strings.Contains(n, "+") {
			ret = append(ret, n)
		}
	}
	return ret
}

func init() {
	registerRules(
		RuleInfo{
			ID: "missing-version-comment", Group: RuleGroupPolicy, Summary: "A hash-pinned action has no comment which names its version.",
			DefaultLevel: SeverityWarning, Profile: ProfilePedantic, Fixable: true, DocsAnchor: "check-missing-version-comment",
		},
	)
	registerRuleFactory("missing-version-comment", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("missing-version-comment") {
			return nil
		}
		return []Rule{NewRuleMissingVersionComment(env.online, env.Source())}
	})
}
