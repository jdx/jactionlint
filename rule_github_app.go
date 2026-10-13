package jactionlint

import "strings"

// createGitHubAppTokenAction is the action which exchanges the credentials of a GitHub App for a
// short-lived installation token.
const createGitHubAppTokenAction = "actions/create-github-app-token"

// The other actions which exchange the credentials of a GitHub App for a token. Their inputs are not the ones of
// actions/create-github-app-token:
//
//   - tibdex/github-app-token (archived): "permissions" and "repositories" are JSON, both default to everything the
//     installation has, and "revoke" is true unless set to false
//   - peter-murray/workflow-application-token-action: "permissions" ("issues:read,packages:read") defaults to everything,
//     "organization" picks the installation, there is no list of repositories, and "revoke_token" is false unless set to true
const (
	tibdexGitHubAppAction      = "tibdex/github-app-token"
	peterMurrayGitHubAppAction = "peter-murray/workflow-application-token-action"
)

// RuleGitHubApp is a rule checker which reports the ways of using actions/create-github-app-token
// that issue a token which is more powerful or lives longer than the job needs:
//
//   - skip-token-revoke: true keeps the token valid after the job
//   - owner without repositories gives the token access to every repository of the owner that has the
//     app installed
//   - no permission-* input gives the token every permission the app was granted
//
// The same is checked, with their own inputs, for tibdex/github-app-token and
// peter-murray/workflow-application-token-action.
type RuleGitHubApp struct {
	RuleBase
	wf *Workflow
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleGitHubApp) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	return nil
}

// orgOnlyPermissions are the permission inputs which affect the organization and not the repositories. A token that
// has these and nothing else needs no list of repositories, so "owner" alone is fine for it. The list is the one of
// actions/create-github-app-token/action.yml (zizmor#2219).
var orgOnlyPermissions = map[string]bool{
	"permission-custom-properties-for-organizations":            true,
	"permission-enterprise-custom-properties-for-organizations": true,
	"permission-members":                                     true,
	"permission-organization-administration":                 true,
	"permission-organization-announcement-banners":           true,
	"permission-organization-copilot-seat-management":        true,
	"permission-organization-custom-org-roles":               true,
	"permission-organization-custom-properties":              true,
	"permission-organization-custom-roles":                   true,
	"permission-organization-events":                         true,
	"permission-organization-hooks":                          true,
	"permission-organization-packages":                       true,
	"permission-organization-personal-access-token-requests": true,
	"permission-organization-personal-access-tokens":         true,
	"permission-organization-plan":                           true,
	"permission-organization-projects":                       true,
	"permission-organization-secrets":                        true,
	"permission-organization-self-hosted-runners":            true,
	"permission-organization-user-blocking":                  true,
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
	if a == nil {
		return nil
	}
	switch {
	case ref.isRepoAction(tibdexGitHubAppAction):
		// The version 1 of the action has "repository" (the current one by default) and no "repositories"
		v1 := false
		if major, ok := majorOf(withCommentVersion(rule.wf, a, ref)); ok && major < 2 {
			v1 = true
		}
		rule.checkTibdex(a, v1)
		return nil
	case ref.isRepoAction(peterMurrayGitHubAppAction):
		rule.checkPeterMurray(a)
		return nil
	case !ref.isRepoAction(createGitHubAppTokenAction):
		return nil
	}

	if v, ok := a.input("skip-token-revoke"); ok && isTrueLiteral(v) {
		rule.ReportID("github-app", a.Inputs["skip-token-revoke"].Value.Pos, "\"skip-token-revoke: true\" keeps the GitHub App token valid after the job ends. remove it so that the token is revoked in the post step")
	}

	if a.InputsUnknown() {
		return nil // the inputs may have what is missing
	}
	_, hasOwner := a.input("owner")
	repos, hasRepos := a.input("repositories")
	if hasOwner && (!hasRepos || repos == "") && !orgOnly(a) {
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

// checkTibdex checks a use of tibdex/github-app-token. The version 1 has no "repositories" and no "revoke" to check (its 1.x of today has "permissions").
func (rule *RuleGitHubApp) checkTibdex(a *ExecAction, v1 bool) {
	// the version 1 has no "revoke" (it does not revoke the token) and no "repositories"
	if v, ok := a.input("revoke"); !v1 && ok && isFalseLiteral(v) {
		rule.ReportID("github-app", a.Inputs["revoke"].Value.Pos, "\"revoke: false\" keeps the GitHub App token valid after the job ends. remove it so that the token is revoked in the post step")
	}
	if a.InputsUnknown() {
		return // the inputs may have what is missing
	}
	if v, ok := a.input("repositories"); !v1 && (!ok || strings.TrimSpace(v) == "") {
		rule.ReportID("github-app", a.Uses.Pos, "no \"repositories\" input is set, so the token has access to every repository that the installation of the GitHub App can access. list the repositories the token is for, as a JSON array like '[\"owner/repo\"]'")
	}
	if v, ok := a.input("permissions"); !ok || strings.TrimSpace(v) == "" {
		rule.ReportID("github-app", a.Uses.Pos, "no \"permissions\" input is set, so the token gets every permission granted to the GitHub App. request only what the job needs, as JSON like '{\"contents\": \"read\"}'")
	}
}

// checkPeterMurray checks a use of peter-murray/workflow-application-token-action.
func (rule *RuleGitHubApp) checkPeterMurray(a *ExecAction) {
	const notRevoked = "the GitHub App token is not revoked at the end of the job: \"revoke_token\" is false unless it is set to true. set \"revoke_token: true\" so that the token is revoked in the post step"
	// An explicit false is reported whatever else the inputs hold, like "revoke: false" of tibdex's action
	if v, ok := a.input("revoke_token"); ok && isFalseLiteral(v) {
		rule.ReportID("github-app", a.Inputs["revoke_token"].Value.Pos, notRevoked)
	}
	if a.InputsUnknown() {
		return // the inputs may have what is missing
	}
	if v, ok := a.input("revoke_token"); !ok || strings.TrimSpace(v) == "" {
		rule.ReportID("github-app", a.Uses.Pos, notRevoked)
	}
	if v, ok := a.input("permissions"); !ok || strings.TrimSpace(v) == "" {
		rule.ReportID("github-app", a.Uses.Pos, "no \"permissions\" input is set, so the token gets every permission granted to the GitHub App. request only what the job needs, for instance \"permissions: contents:read\"")
	}
}

// orgOnly reports whether the step asks for permissions and all of them are organization permissions.
func orgOnly(a *ExecAction) bool {
	n := 0
	for name := range a.Inputs {
		if !strings.HasPrefix(name, "permission-") {
			continue
		}
		if !orgOnlyPermissions[name] {
			return false
		}
		n++
	}
	return n > 0 // without permissions the token has everything the app was granted
}

func init() {
	registerRules(
		RuleInfo{ID: "github-app", Group: RuleGroupSecurity, Summary: "A GitHub App token (actions/create-github-app-token, tibdex/github-app-token, peter-murray/workflow-application-token-action) is issued with more access or a longer life than needed.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-github-app"},
	)
	registerRuleFactory("github-app", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleGitHubApp()}
	})
}
