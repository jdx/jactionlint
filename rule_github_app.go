package jactionlint

import "strings"

// createGitHubAppTokenAction is the action which exchanges the credentials of a GitHub App for a
// short-lived installation token.
const createGitHubAppTokenAction = "actions/create-github-app-token"

// RuleGitHubApp is a rule checker which reports the ways of using actions/create-github-app-token
// that issue a token which is more powerful or lives longer than the job needs:
//
//   - skip-token-revoke: true keeps the token valid after the job
//   - owner without repositories gives the token access to every repository of the owner that has the
//     app installed
//   - no permission-* input gives the token every permission the app was granted
type RuleGitHubApp struct {
	RuleBase
}

// NewRuleGitHubApp creates a new RuleGitHubApp instance.
func NewRuleGitHubApp() *RuleGitHubApp {
	return &RuleGitHubApp{
		RuleBase: RuleBase{
			name: "github-app",
			desc: "Checks the use of actions/create-github-app-token for tokens which live longer or can do more than needed",
		},
	}
}

// VisitStep is callback when visiting Step node.
func (rule *RuleGitHubApp) VisitStep(n *Step) error {
	if !rule.Config().RuleEnabled("github-app") {
		return nil
	}
	a, ref := stepAction(n)
	if a == nil || !ref.isRepoAction(createGitHubAppTokenAction) {
		return nil
	}

	if v, ok := a.input("skip-token-revoke"); ok && isTrueLiteral(v) {
		rule.ReportID("github-app", a.Inputs["skip-token-revoke"].Value.Pos, "\"skip-token-revoke: true\" keeps the GitHub App token valid after the job ends. remove it so that the token is revoked in the post step")
	}

	_, hasOwner := a.input("owner")
	repos, hasRepos := a.input("repositories")
	if hasOwner && (!hasRepos || repos == "") {
		rule.ReportID("github-app", a.Inputs["owner"].Value.Pos, "\"owner\" without \"repositories\" issues a token with access to every repository of the owner where the app is installed. list the repositories the token is for in \"repositories\"")
	}

	hasPermission := false
	for name := range a.Inputs {
		if strings.HasPrefix(name, "permission-") {
			hasPermission = true
			break
		}
	}
	if !hasPermission {
		rule.ReportID("github-app", a.Uses.Pos, "no \"permission-*\" input is set, so the token gets every permission granted to the GitHub App. request only what the job needs, for instance \"permission-contents: read\"")
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "github-app", Group: RuleGroupSecurity, Summary: "A GitHub App token is issued with more access or a longer life than needed.", DefaultLevel: SeverityWarning, Profile: ProfileStrict, DocsAnchor: "check-github-app"},
	)
	registerRuleFactory("github-app", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleGitHubApp()}
	})
}
