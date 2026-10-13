package jactionlint

import (
	"errors"
	"strings"
)

// RuleRepojacking reports actions and reusable workflows of an owner whose name anyone can register: the owner does
// not exist (so the repository cannot either), or the repository has moved to another owner and the old name is free,
// so that GitHub only redirects until somebody registers the name and creates the repository again. The workflow
// then runs what that person published, with its token and secrets.
type RuleRepojacking struct {
	onlineUsesRule
}

// NewRuleRepojacking creates a new RuleRepojacking instance.
func NewRuleRepojacking(sess *onlineSession) *RuleRepojacking {
	return &RuleRepojacking{newOnlineUsesRule("repojacking", "Checks that the owners of actions and reusable workflows exist and have not given up the name", sess)}
}

// VisitWorkflowPost implements Pass.
func (r *RuleRepojacking) VisitWorkflowPost(*Workflow) error {
	for _, s := range r.sites {
		repo, err := r.sess.Repository(s.ref.Owner, s.ref.Repo)
		switch {
		case err == nil:
			// GitHub redirected a renamed or transferred repository: the name is of the new owner, spelled as it is now
			newOwner, _, _ := strings.Cut(repo.FullName, "/")
			if repo.FullName == "" || strings.EqualFold(newOwner, s.ref.Owner) {
				continue
			}
			free, err := r.ownerIsFree(s)
			if err != nil || !free {
				continue // the old owner is still there: only it can create the repository again
			}
			r.ReportIDf("repojacking", s.pos,
				"%s %q has moved to %s and the owner %q no longer exists on GitHub, so anyone can register that name and create the repository %s, which GitHub then serves instead of the redirect, and this workflow runs their code. use the owner %q instead",
				s.what(), s.ref.Raw, repo.FullName, s.ref.Owner, s.repoSlug(), newOwner)
		case errors.Is(err, ErrGitHubNotFound):
			// Not visible is what a private repository answers too: only an owner that does not exist is a finding
			free, err := r.ownerIsFree(s)
			if err != nil || !free {
				continue
			}
			r.ReportIDf("repojacking", s.pos,
				"%s %q has the owner %q, which does not exist on GitHub, so anyone can register that name and publish %s under it, and this workflow runs their code. replace it with one of an owner you trust",
				s.what(), s.ref.Raw, s.ref.Owner, s.repoSlug())
		default:
			r.skipped(s, "the repository", err)
		}
	}
	return nil
}

// ownerIsFree tells whether GitHub has no user or organization named like the owner of the site.
func (r *RuleRepojacking) ownerIsFree(s usesSite) (bool, error) {
	_, err := r.sess.Owner(s.ref.Owner)
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, ErrGitHubNotFound):
		return true, nil
	}
	r.skipped(s, "the owner", err)
	return false, err
}

func init() {
	registerRules(
		RuleInfo{ID: "repojacking", Group: RuleGroupSecurity, Summary: "An action or reusable workflow has an owner whose name anyone can register.", DefaultLevel: SeverityWarning, Online: true, DocsAnchor: "check-repojacking"},
	)
	registerRuleFactory("repojacking", func(env *RuleEnv) []Rule {
		if env.online == nil || !env.config.RuleEnabled("repojacking") {
			return nil
		}
		return []Rule{NewRuleRepojacking(env.online)}
	})
}
