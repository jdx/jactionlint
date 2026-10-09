package jactionlint

// RuleRefConfusion reports an action pinned to a name which is both a branch and a tag of the
// repository. GitHub resolves such a name by its own rules, and whoever can create the second ref
// can change what runs in every workflow which uses the first.
type RuleRefConfusion struct {
	onlineUsesRule
}

// NewRuleRefConfusion creates a new RuleRefConfusion instance.
func NewRuleRefConfusion(sess *onlineSession) *RuleRefConfusion {
	return &RuleRefConfusion{newOnlineUsesRule("ref-confusion", "Checks that the ref of an action is not both a branch and a tag", sess)}
}

// VisitWorkflowPost implements Pass.
func (r *RuleRefConfusion) VisitWorkflowPost(*Workflow) error {
	for _, s := range r.sites {
		if s.ref.RefKind != RefSemverTag && s.ref.RefKind != RefOther {
			continue // A commit SHA is not a name. A short SHA could be, but it is not a name anybody means.
		}
		_, isTag, err := r.sess.TagCommit(s.ref.Owner, s.ref.Repo, s.ref.Ref)
		if err != nil {
			r.skipped(s, "the tag", err)
			continue
		}
		if !isTag {
			continue
		}
		_, isBranch, err := r.sess.BranchCommit(s.ref.Owner, s.ref.Repo, s.ref.Ref)
		if err != nil {
			r.skipped(s, "the branch", err)
			continue
		}
		if isBranch {
			r.ReportIDf("ref-confusion", s.pos,
				"ref %q of %s %q is both a branch and a tag of %s, so it is ambiguous what runs and whoever controls the other ref can change it. pin the action to a full-length commit SHA",
				s.ref.Ref, s.what(), s.ref.Raw, s.repoSlug())
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "ref-confusion", Group: RuleGroupSecurity, Summary: "The ref of an action is both a branch and a tag of its repository.", DefaultLevel: SeverityWarning, Online: true, DocsAnchor: "check-ref-confusion"},
	)
	registerRuleFactory("ref-confusion", func(env *RuleEnv) []Rule {
		if env.online == nil || !env.config.RuleEnabled("ref-confusion") {
			return nil
		}
		return []Rule{NewRuleRefConfusion(env.online)}
	})
}
