package jactionlint

import "strings"

// archivedReplacements names the maintained successors of well-known actions whose repository was archived. They are
// only shown for a repository that the online check found archived, so a name in the table that is not archived is
// harmless. The actions that superfluous-actions knows (actions/create-release, actions/upload-release-asset) have their
// replacement there.
var archivedReplacements = map[string]string{
	"actions/setup-ruby":                    "`ruby/setup-ruby`",
	"actions/setup-elixir":                  "`erlef/setup-beam`",
	"actions/setup-haskell":                 "`haskell-actions/setup`",
	"actions-rs/toolchain":                  "`dtolnay/rust-toolchain`, or `rustup` in a script step",
	"actions-rs/cargo":                      "`cargo` in a script step",
	"actions-rs/clippy-check":               "`cargo clippy` in a script step",
	"actions-rs/audit-check":                "`cargo audit` in a script step",
	"actions-rs/install":                    "`cargo install --locked` in a script step",
	"actions-rs/tarpaulin":                  "`cargo tarpaulin` in a script step",
	"actions-rs/grcov":                      "`grcov` in a script step",
	"crazy-max/ghaction-docker-meta":        "`docker/metadata-action`",
	"8398a7/action-slack":                   "`slackapi/slack-github-action`",
	"marvinpinto/action-automatic-releases": "`gh release create` in a script step",
	"nick-invision/retry":                   "`nick-fields/retry`",
}

// archivedReplacement returns the successor of the archived action, or "" when it is not known: from the table above or,
// for the actions that do what the runner can do, the advice of superfluous-actions.
func archivedReplacement(u *UsesRef) string {
	name := strings.ToLower(u.CanonicalName())
	if r, ok := archivedReplacements[name]; ok {
		return r
	}
	for _, s := range superfluousActions {
		if s.action == name && s.source == sourceArchivedGH {
			return "a script step: " + s.instead
		}
	}
	return ""
}

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
			instead := "replace it with a maintained alternative, or run the commands yourself in a \"run:\" step"
			if r := archivedReplacement(s.ref); r != "" {
				instead = "replace it with " + r
			}
			r.ReportIDf("archived-uses", s.pos,
				"%s %q is in the archived repository %s, which is read-only and no longer maintained, so problems in it will not be fixed. %s",
				s.what(), s.ref.Raw, s.repoSlug(), instead)
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
