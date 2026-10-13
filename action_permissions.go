package jactionlint

import (
	"regexp"
	"strings"
)

// This file is the table of what well-known actions need from the GITHUB_TOKEN. The rule
// insufficient-permissions reads it to report a job whose explicit "permissions:" are too narrow for a
// step, and the fix of missing-permissions reads it to write the permissions of a job.
//
// Every entry was written from the documentation of the action itself (its README, action.yml or the
// workflow in its own documentation), which is cited in the comment of the entry. The data of other
// projects' knowledge bases was not used. An entry says only what the documentation says the token must
// be able to do: the table is read as "at least these scopes", never as "only these". A condition that the
// documentation ties to an input (no scope is needed when the action is given another token, a dry run needs
// none) is part of the entry, and a step whose configuration the entry cannot judge (an input that is an
// expression, another token) has no need, so the rule stays silent.

// permNeed is a scope of the GITHUB_TOKEN at a level (permLevelRead or permLevelWrite).
type permNeed struct {
	Scope string
	Level int
}

func (n permNeed) String() string {
	return n.Scope + ": " + permissionLevelName(n.Level)
}

// permNeeds is what one step needs from the token.
type permNeeds struct {
	// All lists scopes the job must grant, each at the level or above.
	All []permNeed
	// Any lists scopes of which the job must grant at least one. The documentation of the action says that
	// the action works with one of them (the REST endpoints it calls accept either).
	Any []permNeed
	// Exact says that the documentation lists these scopes as everything the step needs for its configuration, so
	// that they are the minimal permissions of a job (the fix of missing-permissions only writes such needs).
	// An action which has more needs in some repositories (private ones, say) or whose needs depend on options the
	// table does not look at is not exact.
	Exact bool
	// Pedantic says that the need is reported only with the option pedantic of the rule, because it holds in some
	// repositories only.
	Pedantic bool
}

// actionPermission is the entry of an action in the table.
type actionPermission struct {
	// Source is the documentation the entry comes from.
	Source string
	// Needs says what the step needs. It returns no need for a configuration it does not judge.
	Needs func(s actionStep) permNeeds
}

// actionStep is a step which runs an action, with the context the entries need.
type actionStep struct {
	step *Step
	act  *ExecAction
	ctx  *runContext
}

// set reports whether the input is given with a value.
func (s actionStep) set(name string) bool {
	v, ok := s.act.input(name)
	return ok && v != ""
}

// literal returns the value of the input when it is written out: it is false for an input that is not given
// and for one that has an expression in it.
func (s actionStep) literal(name string) (string, bool) {
	i, ok := s.act.Inputs[strings.ToLower(name)]
	if !ok || i == nil || i.Value == nil || i.Value.ContainsExpression() {
		return "", false
	}
	return strings.TrimSpace(i.Value.Value), true
}

// flagMayBeOn reports whether a boolean input may be true: it is a literal true, or a value which cannot be told (an
// expression, a spelling that is not a boolean). A literal false and an input which is not given or is empty are off.
// An entry which drops a need because of such an input drops it only when the input may be on.
func (s actionStep) flagMayBeOn(name string) bool {
	i, ok := s.act.Inputs[strings.ToLower(name)]
	if !ok || i == nil || i.Value == nil {
		return false
	}
	v := strings.TrimSpace(i.Value.Value)
	if v == "" || (!i.Value.ContainsExpression() && isFalseLiteral(v)) {
		return false
	}
	return true
}

// anyFlagMayBeOn is flagMayBeOn for several inputs.
func (s actionStep) anyFlagMayBeOn(names ...string) bool {
	for _, n := range names {
		if s.flagMayBeOn(n) {
			return true
		}
	}
	return false
}

// tokenIsGitHubToken reports whether the token of the action is the GITHUB_TOKEN of the job: the input is not
// given (the default of such an input is github.token), or it is `${{ secrets.GITHUB_TOKEN }}` or
// `${{ github.token }}`. Anything else (a secret, an expression, an empty value) is another credential, or one
// that cannot be told.
func (s actionStep) tokenIsGitHubToken(input string) bool {
	i, ok := s.act.Inputs[strings.ToLower(input)]
	if !ok || i == nil || i.Value == nil {
		return true
	}
	return isGitHubTokenValue(i.Value.Value)
}

var githubTokenValueRe = regexp.MustCompile(`(?i)^\$\{\{\s*(secrets\.github_token|github\.token)\s*\}\}$`)

// isGitHubTokenValue reports whether the value is the GITHUB_TOKEN of the job as an expression.
func isGitHubTokenValue(v string) bool {
	return githubTokenValueRe.MatchString(strings.TrimSpace(v))
}

func writes(scopes ...string) []permNeed {
	out := make([]permNeed, len(scopes))
	for i, s := range scopes {
		out[i] = permNeed{Scope: s, Level: permLevelWrite}
	}
	return out
}

// githubTokenNeeds is an entry for an action which uses the token of the input `input` for what the scopes
// say, unless it is given another token.
func githubTokenNeeds(source, input string, exact bool, scopes ...string) actionPermission {
	return actionPermission{Source: source, Needs: func(s actionStep) permNeeds {
		if !s.tokenIsGitHubToken(input) {
			return permNeeds{}
		}
		return permNeeds{All: writes(scopes...), Exact: exact}
	}}
}

// attestNeeds is the entry of an attestation action: "id-token: write" is the OIDC token of the signing certificate and
// does not depend on the input github-token, which only carries the "attestations: write" that persists the attestation.
func attestNeeds(source string) actionPermission {
	return actionPermission{Source: source, Needs: func(s actionStep) permNeeds {
		if !s.tokenIsGitHubToken("github-token") {
			return permNeeds{All: writes("id-token")}
		}
		return permNeeds{All: writes("id-token", "attestations")}
	}}
}

// anyTokenNeeds is an entry for an action which works with one of the scopes.
func anyTokenNeeds(source, input string, scopes ...string) actionPermission {
	return actionPermission{Source: source, Needs: func(s actionStep) permNeeds {
		if !s.tokenIsGitHubToken(input) {
			return permNeeds{}
		}
		return permNeeds{Any: writes(scopes...)}
	}}
}

// actionPermissionTable maps the canonical name (lower case, "owner/repo[/path]") of an action to what it
// needs. See the comments of the entries for where each one comes from.
var actionPermissionTable = map[string]actionPermission{
	// actions/checkout: the README of the action says that the token needs "contents: read" to clone a private
	// repository. A public repository can be cloned without the scope, so the need is pedantic, and it is not
	// judged when the step clones another repository, uses an SSH key or has a token of its own.
	// https://github.com/actions/checkout#recommended-permissions
	"actions/checkout": {
		Source: "https://github.com/actions/checkout#recommended-permissions",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("token") || s.set("ssh-key") || s.set("repository") {
				return permNeeds{}
			}
			return permNeeds{All: []permNeed{{"contents", permLevelRead}}, Pedantic: true}
		},
	},

	// actions/deploy-pages: "The job that executes the deployment must at minimum have the following
	// permissions: pages: write, id-token: write". The id-token is the OIDC token of the deployment and does not
	// depend on the `token` input.
	// https://github.com/actions/deploy-pages#usage
	"actions/deploy-pages": {
		Source: "https://github.com/actions/deploy-pages#usage",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("token") {
				return permNeeds{All: writes("id-token")}
			}
			return permNeeds{All: writes("pages", "id-token"), Exact: true}
		},
	},

	// github/codeql-action/upload-sarif: "the workflow must have the `security-events: write` permission" (the
	// description of the input `token` in action.yml). The token must be the built-in one.
	// https://github.com/github/codeql-action/blob/main/upload-sarif/action.yml
	"github/codeql-action/upload-sarif": githubTokenNeeds("https://github.com/github/codeql-action/blob/main/upload-sarif/action.yml", "token", false, "security-events"),

	// github/codeql-action/analyze: "All advanced setup code scanning workflows must have the
	// `security-events: write` permission." It uploads the results unless `upload` is `never`.
	// https://github.com/github/codeql-action#workflow-permissions
	"github/codeql-action/analyze": {
		Source: "https://github.com/github/codeql-action#workflow-permissions",
		Needs: func(s actionStep) permNeeds {
			if s.set("upload") {
				v, ok := s.literal("upload")
				if !ok || strings.EqualFold(v, "never") || isFalseLiteral(v) {
					return permNeeds{}
				}
			}
			return permNeeds{All: writes("security-events")}
		},
	},

	// actions/attest-build-provenance and actions/attest: "Ensure that the following permissions are set:
	// id-token: write, attestations: write" (+ artifact-metadata: write for the storage record, which the table
	// does not require).
	// https://github.com/actions/attest#usage
	"actions/attest":                  attestNeeds("https://github.com/actions/attest#usage"),
	"actions/attest-build-provenance": attestNeeds("https://github.com/actions/attest-build-provenance#usage"),

	// actions/stale: the README lists "issues: write" and "pull-requests: write" for what the options do (labels,
	// comments, closing). The action fails when the token can write neither, unless it only logs (`debug-only`).
	// https://github.com/actions/stale#recommended-permissions
	"actions/stale": {
		Source: "https://github.com/actions/stale#recommended-permissions",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("repo-token") || s.flagMayBeOn("debug-only") {
				return permNeeds{}
			}
			return permNeeds{Any: writes("issues", "pull-requests")}
		},
	},

	// actions/labeler: "Adding Existing Labels: Requires pull-requests: write", "Creating New Labels: Requires
	// issues: write".
	// https://github.com/actions/labeler#recommended-permissions
	"actions/labeler": anyTokenNeeds("https://github.com/actions/labeler#recommended-permissions", "repo-token", "pull-requests", "issues"),

	// actions/dependency-review-action: `comment-summary-in-pr` "If enabled, you must give the workflow or job the
	// `pull-requests: write` permission".
	// https://github.com/actions/dependency-review-action#configuration-options
	"actions/dependency-review-action": {
		Source: "https://github.com/actions/dependency-review-action#configuration-options",
		Needs: func(s actionStep) permNeeds {
			v, ok := s.literal("comment-summary-in-pr")
			if !ok || !s.tokenIsGitHubToken("repo-token") || (!strings.EqualFold(v, "always") && !strings.EqualFold(v, "on-failure")) {
				return permNeeds{}
			}
			return permNeeds{All: writes("pull-requests")}
		},
	},

	// peter-evans/create-pull-request: "If using the repository's GITHUB_TOKEN ... Elevate the permissions in your
	// workflow: contents: write, pull-requests: write". The input `token` opens the pull request, so it needs
	// "pull-requests: write" while it is the GITHUB_TOKEN. `branch-token` "The token that the action will use to create
	// and update the branch" (default: `token`) carries the push: with another one the GITHUB_TOKEN needs no
	// "contents: write". `push-to-fork` needs a token of its own.
	// https://github.com/peter-evans/create-pull-request#token
	"peter-evans/create-pull-request": {
		Source: "https://github.com/peter-evans/create-pull-request#token",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("token") || s.set("push-to-fork") {
				return permNeeds{}
			}
			if !s.tokenIsGitHubToken("branch-token") {
				return permNeeds{All: writes("pull-requests")}
			}
			return permNeeds{All: writes("contents", "pull-requests"), Exact: true}
		},
	},

	// softprops/action-gh-release: "This Action requires the following permissions on the GitHub integration
	// token: contents: write", and "discussions: write" with `discussion_category_name`. The token is the input
	// `token` (default github.token) or the environment variable GITHUB_TOKEN of older versions.
	// https://github.com/softprops/action-gh-release#permissions
	"softprops/action-gh-release": {
		Source: "https://github.com/softprops/action-gh-release#permissions",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("token") || !s.envTokenIsGitHubToken("GITHUB_TOKEN") {
				return permNeeds{}
			}
			scopes := []string{"contents"}
			if s.set("discussion_category_name") {
				scopes = append(scopes, "discussions")
			}
			return permNeeds{All: writes(scopes...), Exact: true}
		},
	},

	// ncipollo/release-action: creates and updates releases, which the REST API allows with "contents: write"; the
	// example of its README grants it.
	// https://github.com/ncipollo/release-action#example-workflow
	"ncipollo/release-action": githubTokenNeeds("https://github.com/ncipollo/release-action#example-workflow", "token", true, "contents"),

	// JamesIves/github-pages-deploy-action: "permissions: contents: write" (the section about the token of its
	// README). It is not judged with an SSH key or another repository, which need a credential of their own.
	// https://github.com/JamesIves/github-pages-deploy-action#getting-started-airplane
	"jamesives/github-pages-deploy-action": {
		Source: "https://github.com/JamesIves/github-pages-deploy-action#getting-started-airplane",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("token") || s.set("ssh-key") || s.set("repository-name") {
				return permNeeds{}
			}
			return permNeeds{All: writes("contents"), Exact: true}
		},
	},

	// peaceiris/actions-gh-pages: with `github_token` the job needs "permissions: contents: write". A deploy key, a
	// personal token or an external repository are other credentials.
	// https://github.com/peaceiris/actions-gh-pages#%EF%B8%8F-first-deployment-with-github_token
	"peaceiris/actions-gh-pages": {
		Source: "https://github.com/peaceiris/actions-gh-pages#%EF%B8%8F-first-deployment-with-github_token",
		Needs: func(s actionStep) permNeeds {
			if !s.set("github_token") || !s.tokenIsGitHubToken("github_token") || s.set("deploy_key") || s.set("personal_token") || s.set("external_repository") {
				return permNeeds{}
			}
			return permNeeds{All: writes("contents"), Exact: true}
		},
	},

	// googleapis/release-please-action: "permissions: contents: write, issues: write, pull-requests: write". The
	// table asks for the first and the last, which every mode that opens a release pull request needs; the
	// options that skip the pull request or the release change what is needed, so they are not judged.
	// https://github.com/googleapis/release-please-action#basic-configuration
	"googleapis/release-please-action": {
		Source: "https://github.com/googleapis/release-please-action#basic-configuration",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("token") || s.anyFlagMayBeOn("skip-github-release", "skip-github-pull-request", "fork") {
				return permNeeds{}
			}
			return permNeeds{All: writes("contents", "pull-requests")}
		},
	},

	// release-drafter/release-drafter: "permissions: contents: write # write permission is required to create a
	// github release". With `disable-releaser` it only labels.
	// https://github.com/release-drafter/release-drafter#usage
	"release-drafter/release-drafter": {
		Source: "https://github.com/release-drafter/release-drafter#usage",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("token") || s.anyFlagMayBeOn("disable-releaser", "dry-run") {
				return permNeeds{}
			}
			return permNeeds{All: writes("contents")}
		},
	},

	// release-drafter/release-drafter/autolabeler: "permissions: pull-requests: write".
	// https://github.com/release-drafter/release-drafter#autolabeler
	"release-drafter/release-drafter/autolabeler": githubTokenNeeds("https://github.com/release-drafter/release-drafter#autolabeler", "token", false, "pull-requests"),

	// dorny/test-reporter: "permissions: contents: read, actions: read, checks: write".
	// https://github.com/dorny/test-reporter#recommended-setup
	"dorny/test-reporter": githubTokenNeeds("https://github.com/dorny/test-reporter#recommended-setup", "token", false, "checks"),

	// EnricoMi/publish-unit-test-result-action: "permissions: checks: write, pull-requests: write" (the latter
	// only with a comment); `check_run: false` creates no check run.
	// https://github.com/EnricoMi/publish-unit-test-result-action#permissions
	"enricomi/publish-unit-test-result-action": {
		Source: "https://github.com/EnricoMi/publish-unit-test-result-action#permissions",
		Needs: func(s actionStep) permNeeds {
			if !s.tokenIsGitHubToken("github_token") {
				return permNeeds{}
			}
			// Only a literal false creates no check run; true, or a value which cannot be told, still may
			if v, ok := s.literal("check_run"); s.set("check_run") && (!ok || isFalseLiteral(v)) {
				return permNeeds{}
			}
			return permNeeds{All: writes("checks")}
		},
	},

	// marocchino/sticky-pull-request-comment: "permissions: pull-requests: write". A comment is written with the
	// issues API, which takes either scope.
	// https://github.com/marocchino/sticky-pull-request-comment#usage
	"marocchino/sticky-pull-request-comment": anyTokenNeeds("https://github.com/marocchino/sticky-pull-request-comment#usage", "github_token", "pull-requests", "issues"),

	// aws-actions/configure-aws-credentials: "Need ID token write permission to use OIDC: id-token: write". The
	// action uses OIDC when it has a role and no access key, web identity token file or role chaining (the
	// source of the action decides it in useGitHubOIDCProvider). A job which exports credentials by another way
	// is not judged.
	// https://github.com/aws-actions/configure-aws-credentials#oidc
	"aws-actions/configure-aws-credentials": {
		Source: "https://github.com/aws-actions/configure-aws-credentials#oidc",
		Needs: func(s actionStep) permNeeds {
			if !s.set("role-to-assume") || s.set("aws-access-key-id") || s.set("web-identity-token-file") || s.anyFlagMayBeOn("force-skip-oidc", "role-chaining") ||
				s.ctx.jobMentions("aws_access_key_id", "aws-access-key-id", "web-identity-token", "aws_web_identity_token_file") {
				return permNeeds{}
			}
			return permNeeds{All: writes("id-token"), Exact: true}
		},
	},

	// google-github-actions/auth: "Add id-token with the intended permissions: id-token: write" for the
	// workload identity federation (`workload_identity_provider`); a `credentials_json` is a key and needs none.
	// https://github.com/google-github-actions/auth#usage
	"google-github-actions/auth": {
		Source: "https://github.com/google-github-actions/auth#usage",
		Needs: func(s actionStep) permNeeds {
			if !s.set("workload_identity_provider") || s.set("credentials_json") {
				return permNeeds{}
			}
			return permNeeds{All: writes("id-token"), Exact: true}
		},
	},

	// azure/login: "you should set permissions: with id-token: write" for the OIDC login with a client-id (and
	// no `creds`, which is a service principal secret; `auth-type: IDENTITY` is a managed identity).
	// https://github.com/Azure/login#login-with-openid-connect-oidc-recommended
	"azure/login": {
		Source: "https://github.com/Azure/login#login-with-openid-connect-oidc-recommended",
		Needs: func(s actionStep) permNeeds {
			if !s.set("client-id") || s.set("creds") {
				return permNeeds{}
			}
			// SERVICE_PRINCIPAL (the default) with a client-id is the OIDC login; IDENTITY is a managed identity
			if v, ok := s.literal("auth-type"); s.set("auth-type") && (!ok || !strings.EqualFold(v, "SERVICE_PRINCIPAL")) {
				return permNeeds{}
			}
			return permNeeds{All: writes("id-token"), Exact: true}
		},
	},

	// pypa/gh-action-pypi-publish: "To enter the trusted publishing flow, configure this action's job with the
	// `id-token: write` permission and without an explicit username or password".
	// https://github.com/pypa/gh-action-pypi-publish#trusted-publishing
	"pypa/gh-action-pypi-publish": {
		Source: "https://github.com/pypa/gh-action-pypi-publish#trusted-publishing",
		Needs: func(s actionStep) permNeeds {
			if s.set("password") || s.set("user") {
				return permNeeds{}
			}
			return permNeeds{All: writes("id-token"), Exact: true}
		},
	},

	// rubygems/configure-rubygems-credentials: "permissions: id-token: write" for the trusted publisher (a
	// `role-to-assume`); an `api-token` is a secret.
	// https://github.com/rubygems/configure-rubygems-credentials#trusted-publisher-recommended
	"rubygems/configure-rubygems-credentials": {
		Source: "https://github.com/rubygems/configure-rubygems-credentials#trusted-publisher-recommended",
		Needs: func(s actionStep) permNeeds {
			if !s.set("role-to-assume") || s.set("api-token") {
				return permNeeds{}
			}
			return permNeeds{All: writes("id-token"), Exact: true}
		},
	},

	// NuGet/login: "permissions: id-token: write", the action exchanges the OIDC token for an API key.
	// https://github.com/NuGet/login#usage
	"nuget/login": {
		Source: "https://github.com/NuGet/login#usage",
		Needs: func(s actionStep) permNeeds {
			return permNeeds{All: writes("id-token"), Exact: true}
		},
	},
}

// envTokenIsGitHubToken reports whether the environment variable of the step is not set to another credential:
// it is not set, or it is the GITHUB_TOKEN of the job.
func (s actionStep) envTokenIsGitHubToken(name string) bool {
	v, ok := s.ctx.envValue(s.step, name)
	if !ok {
		return true
	}
	return v != nil && isGitHubTokenValue(v.Value)
}

// jobMentions reports whether the text of the job, its steps and the environment of the workflow has one of the
// words (lower case): an input, an environment variable or a script. It is the guard of the entries whose
// condition can be met by a step before the action which this file does not show.
func (c *runContext) jobMentions(words ...string) bool {
	has := func(s string) bool {
		s = strings.ToLower(s)
		// The input of actions/checkout which keeps the token is not a sign that the job sets up its own credentials
		s = strings.ReplaceAll(s, "persist-credentials", "")
		for _, w := range words {
			if strings.Contains(s, w) {
				return true
			}
		}
		return false
	}
	envHas := func(e *Env) bool {
		if e == nil {
			return false
		}
		if e.Expression != nil {
			return true // a whole mapping from an expression is not known
		}
		for k, v := range e.Vars {
			if has(k) || (v != nil && v.Value != nil && has(v.Value.Value)) {
				return true
			}
		}
		return false
	}
	if envHas(c.workflowEnv()) || envHas(c.jobEnv()) {
		return true
	}
	if c.job == nil {
		return false
	}
	for _, s := range flattenSteps(c.job.Steps) {
		if envHas(s.Env) {
			return true
		}
		switch e := s.Exec.(type) {
		case *ExecRun:
			if e.Run != nil && has(e.Run.Value) {
				return true
			}
		case *ExecAction:
			for k, in := range e.Inputs {
				if has(k) || (in != nil && in.Value != nil && has(in.Value.Value)) {
					return true
				}
			}
		}
	}
	return false
}
