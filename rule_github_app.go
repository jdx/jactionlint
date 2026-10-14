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
		// The version 1 of the action has "repository" (the current one by default) and no "repositories", no "revoke"
		v := withCommentVersion(rule.wf, a, ref)
		major, known := majorOf(v)
		rule.checkTibdex(a, tibdexLayout{v1: known && major < 2, unknown: !known && v.RefKind == RefFullSHA})
		return nil
	case ref.isRepoAction(peterMurrayGitHubAppAction):
		v := withCommentVersion(rule.wf, a, ref)
		major, known := majorOf(v)
		rule.checkPeterMurray(a, peterMurrayVersion{major: major, known: known, unknown: !known && v.RefKind == RefFullSHA})
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

// tibdexLayout says which inputs the used version of tibdex/github-app-token has: the version 1 has "permissions" and
// "repository" (the current repository by default) and no "repositories" and no "revoke"; the version 2 has them. A commit
// whose version is not known (no "# v1.2.3" comment) may be of either, so what is not in both is not checked for it.
type tibdexLayout struct {
	v1      bool
	unknown bool
}

// checkTibdex checks a use of tibdex/github-app-token.
func (rule *RuleGitHubApp) checkTibdex(a *ExecAction, layout tibdexLayout) {
	skip := layout.v1 || layout.unknown // "revoke" and "repositories" are not in the version 1, and not known to be in the commit
	if v, ok := a.input("revoke"); !skip && ok && isFalseLiteral(v) {
		rule.ReportID("github-app", a.Inputs["revoke"].Value.Pos, "\"revoke: false\" keeps the GitHub App token valid after the job ends. remove it so that the token is revoked in the post step")
	}
	if a.InputsUnknown() {
		return // the inputs may have what is missing
	}
	if v, ok := a.input("repositories"); !skip && (!ok || strings.TrimSpace(v) == "") {
		rule.ReportID("github-app", a.Uses.Pos, "no \"repositories\" input is set, so the token has access to every repository that the installation of the GitHub App can access. list the repositories the token is for, as a JSON array of their names without the owner, like '[\"repo\"]'")
	}
	if v, ok := a.input("permissions"); !ok || strings.TrimSpace(v) == "" {
		rule.ReportID("github-app", a.Uses.Pos, "no \"permissions\" input is set, so the token gets every permission granted to the GitHub App. request only what the job needs, as JSON like '{\"contents\": \"read\"}'")
	}
}

// peterMurrayVersion is the version of peter-murray/workflow-application-token-action that a step uses: its major version
// when the reference is a version tag (or a commit with a "# v1.2.3" comment), unknown for a commit without one. The
// "revoke_token" input and the post step that revokes the token are in the version 3 and later; the versions 1 and 2 have
// neither.
type peterMurrayVersion struct {
	major   int
	known   bool
	unknown bool
}

// checkPeterMurray checks a use of peter-murray/workflow-application-token-action.
func (rule *RuleGitHubApp) checkPeterMurray(a *ExecAction, ver peterMurrayVersion) {
	if ver.known && ver.major < 3 {
		// no input can fix this: the versions 1 and 2 ignore "revoke_token" and have no post step
		rule.ReportID("github-app", a.Uses.Pos, "the GitHub App token is not revoked at the end of the job: the versions 1 and 2 of the action have no \"revoke_token\" input and no post step. use the version 3 or later and set \"revoke_token: true\" so that the token is revoked in the post step")
	} else {
		msg := "the GitHub App token is not revoked at the end of the job: \"revoke_token\" is false unless it is set to true. set \"revoke_token: true\" so that the token is revoked in the post step"
		if ver.unknown {
			msg += " (an input of the version 3 and later, the earlier versions cannot revoke the token: note the version in a comment after the commit)"
		}
		// An explicit false is reported whatever else the inputs hold, like "revoke: false" of tibdex's action
		if v, ok := a.input("revoke_token"); ok && isFalseLiteral(v) {
			rule.ReportID("github-app", a.Inputs["revoke_token"].Value.Pos, msg)
		}
		if !a.InputsUnknown() {
			if v, ok := a.input("revoke_token"); !ok || strings.TrimSpace(v) == "" {
				rule.ReportID("github-app", a.Uses.Pos, msg)
			}
		}
	}
	if a.InputsUnknown() {
		return // the inputs may have what is missing
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
