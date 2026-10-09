package jactionlint

import "strings"

// RuleStaleActionRefs reports an action pinned to a commit SHA that no tag of its repository points
// to. Such a commit is between releases: it may carry fixes or bugs which no changelog explains.
type RuleStaleActionRefs struct {
	onlineUsesRule
}

// NewRuleStaleActionRefs creates a new RuleStaleActionRefs instance.
func NewRuleStaleActionRefs(sess *onlineSession) *RuleStaleActionRefs {
	return &RuleStaleActionRefs{newOnlineUsesRule("stale-action-refs", "Checks that actions pinned to a commit SHA use a commit which a tag points to", sess)}
}

// VisitWorkflowPost implements Pass.
func (r *RuleStaleActionRefs) VisitWorkflowPost(*Workflow) error {
	for _, s := range r.sites {
		if s.ref.RefKind != RefFullSHA {
			continue
		}
		sha := strings.ToLower(s.ref.Ref)
		idx, err := r.sess.Tags(s.ref.Owner, s.ref.Repo)
		if err != nil {
			r.skipped(s, "the tags", err)
			continue
		}
		if len(idx.bySHA[sha]) > 0 {
			continue
		}
		if idx.truncated {
			r.Debug("%s: %s has more tags than were read, so the commit may be tagged", s.ref.Raw, s.repoSlug())
			continue
		}
		r.ReportIDf("stale-action-refs", s.pos,
			"%s %q is pinned to commit %s, which no tag of %s points to. the commit may contain changes that no release documents. pin the commit of a tagged release instead",
			s.what(), s.ref.Raw, shortSHA(sha), s.repoSlug())
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "stale-action-refs", Group: RuleGroupSecurity, Summary: "A hash-pinned action uses a commit which no tag of the repository points to.", DefaultLevel: SeverityInfo, Online: true, DocsAnchor: "check-stale-action-refs"},
	)
	registerRuleFactory("stale-action-refs", func(env *RuleEnv) []Rule {
		if env.online == nil || !env.config.RuleEnabled("stale-action-refs") {
			return nil
		}
		return []Rule{NewRuleStaleActionRefs(env.online)}
	})
}
