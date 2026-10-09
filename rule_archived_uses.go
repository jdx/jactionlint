package jactionlint

// RuleArchivedUses reports actions and reusable workflows whose repository is archived. An archived
// repository is read-only and unmaintained: its vulnerabilities, and those of what it vendors,
// will not be fixed.
type RuleArchivedUses struct {
	onlineUsesRule
}

// NewRuleArchivedUses creates a new RuleArchivedUses instance.
func NewRuleArchivedUses(sess *onlineSession) *RuleArchivedUses {
	return &RuleArchivedUses{newOnlineUsesRule("archived-uses", "Checks that actions and reusable workflows are not in archived repositories", sess)}
}

// VisitWorkflowPost implements Pass.
func (r *RuleArchivedUses) VisitWorkflowPost(*Workflow) error {
	for _, s := range r.sites {
		repo, err := r.sess.Repository(s.ref.Owner, s.ref.Repo)
		if err != nil {
			r.skipped(s, "the repository", err)
			continue
		}
		if repo.Archived {
			r.ReportIDf("archived-uses", s.pos,
				"%s %q is in the archived repository %s, which is read-only and no longer maintained, so problems in it will not be fixed. replace it with a maintained alternative, or run the commands yourself in a \"run:\" step",
				s.what(), s.ref.Raw, s.repoSlug())
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "archived-uses", Group: RuleGroupSecurity, Summary: "An action or reusable workflow is in an archived repository.", DefaultLevel: SeverityWarning, Online: true, DocsAnchor: "check-archived-uses"},
	)
	registerRuleFactory("archived-uses", func(env *RuleEnv) []Rule {
		if env.online == nil || !env.config.RuleEnabled("archived-uses") {
			return nil
		}
		return []Rule{NewRuleArchivedUses(env.online)}
	})
}
