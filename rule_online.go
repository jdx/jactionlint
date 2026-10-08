package jactionlint

import (
	"regexp"
	"strings"
)

// This file has what the rules which query GitHub (the "online" rules) have in common: they look at
// every `uses:` of a repository action or reusable workflow, and ask GitHub about the repository
// and the ref of each. They exist only when the online checks are on (see LinterOptions.Online),
// which gives them the onlineSession through RuleEnv.

// usesSite is one `uses:` value of a workflow which names a GitHub repository.
type usesSite struct {
	ref *UsesRef
	pos *Pos
	// comment is the YAML comment at the end of the line of the value, or nil.
	comment *Comment
}

// what names the kind of the referenced thing for messages.
func (s usesSite) what() string {
	if s.ref.Kind == UsesReusableWorkflow {
		return "reusable workflow"
	}
	return "action"
}

// repoSlug returns "owner/repo".
func (s usesSite) repoSlug() string { return s.ref.Owner + "/" + s.ref.Repo }

// onlineUsesRule collects the repository `uses:` values of a workflow for its rules. A concrete rule
// embeds it and checks the sites in VisitWorkflowPost, so that the lookups of the whole file
// are made together and the answers are shared with the other rules through the session.
type onlineUsesRule struct {
	RuleBase
	sess  *onlineSession
	wf    *Workflow
	sites []usesSite
}

func newOnlineUsesRule(name, desc string, sess *onlineSession) onlineUsesRule {
	return onlineUsesRule{RuleBase: NewRuleBase(name, desc), sess: sess}
}

// VisitWorkflowPre implements Pass.
func (r *onlineUsesRule) VisitWorkflowPre(w *Workflow) error {
	r.wf = w
	return nil
}

// VisitStep implements Pass.
func (r *onlineUsesRule) VisitStep(n *Step) error {
	if a, ok := n.Exec.(*ExecAction); ok && a.Uses != nil {
		r.add(a.Uses)
	}
	return nil
}

// VisitJobPre implements Pass.
func (r *onlineUsesRule) VisitJobPre(n *Job) error {
	if n.WorkflowCall != nil && n.WorkflowCall.Uses != nil {
		r.add(n.WorkflowCall.Uses)
	}
	return nil
}

// reSafeRepoPart matches what a GitHub owner or repository name is made of. Anything else is not
// looked up, so a value can never change the path of the API request.
var reSafeRepoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func (r *onlineUsesRule) add(uses *String) {
	if uses.Pos == nil {
		return
	}
	ref := ParseUses(uses.Value)
	if ref.Dynamic || (ref.Kind != UsesAction && ref.Kind != UsesReusableWorkflow) {
		return
	}
	if !reSafeRepoPart.MatchString(ref.Owner) || !reSafeRepoPart.MatchString(ref.Repo) || ref.Ref == "" {
		return
	}
	r.sites = append(r.sites, usesSite{ref: ref, pos: uses.Pos, comment: r.wf.Comments.Inline(uses.Pos.Line)})
}

// skipped explains a lookup that did not work in the debug log. Nothing is reported to the user: the
// session already warned once if the API cannot be used, and a repository which does not exist
// or is not visible has nothing to say.
func (r *onlineUsesRule) skipped(s usesSite, what string, err error) {
	r.Debug("%s: skipped %s: %v", s.ref.Raw, what, err)
}

// shortSHA abbreviates a commit SHA for messages.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

var reVersionToken = regexp.MustCompile(`^[vV]?\d+(?:\.\d+)*(?:-[0-9A-Za-z.-]+)?$`)

// commentVersion extracts the version a comment after a pinned `uses:` starts with, the way tools such as
// Dependabot and Renovate write it: "# v4.2.2", "# tag=v4", "# v4.2.2 (some note)". A version in the
// middle of the text ("# keep in sync with v4") is not what the comment is about, and neither is
// a comment that starts with another directive ("# zizmor: ignore[cache-poisoning] v2"). It returns
// false when the comment does not start with a version.
func commentVersion(text string) (string, bool) {
	text = strings.TrimSpace(text)
	for _, p := range []string{"tag=", "tag:", "version=", "version:", "ver:", "ver="} {
		if len(text) >= len(p) && strings.EqualFold(text[:len(p)], p) {
			text = strings.TrimSpace(text[len(p):])
			break
		}
	}
	w, _, _ := strings.Cut(text, " ")
	w, _, _ = strings.Cut(w, "\t")
	w = strings.Trim(w, "`'\"(),;")
	if reVersionToken.MatchString(w) {
		return w, true
	}
	return "", false
}
