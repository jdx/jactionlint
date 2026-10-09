package jactionlint

import (
	"fmt"
	"slices"
)

// scopeWriteImpact says what `write` on a permission scope lets the GITHUB_TOKEN do. It is used in the
// messages of the excessive-permissions rule to make the risk of a scope concrete.
var scopeWriteImpact = map[string]string{
	"actions":              "cancel and re-run workflow runs and manage caches and artifacts",
	"artifact-metadata":    "write artifact metadata records",
	"attestations":         "create artifact attestations",
	"checks":               "create and modify check runs",
	"code-quality":         "write code quality results",
	"contents":             "push commits and tags and create or delete releases",
	"copilot-requests":     "make Copilot requests",
	"deployments":          "create and modify deployments",
	"discussions":          "create and modify discussions",
	"id-token":             "request OIDC tokens to authenticate to cloud providers and package registries",
	"issues":               "create and modify issues and comments",
	"packages":             "publish and delete packages",
	"pages":                "deploy GitHub Pages",
	"pull-requests":        "create and modify pull requests, comments and reviews",
	"repository-projects":  "modify projects",
	"security-events":      "upload and dismiss code scanning alerts",
	"statuses":             "set commit statuses",
	"vulnerability-alerts": "modify Dependabot alerts",
}

// RuleExcessivePermissions is a rule checker which reports write access that the GITHUB_TOKEN gets
// without a narrow need: `write-all`, `read-all`, and write scopes set at the workflow level, where
// every job inherits them. Write scopes on a job are the recommended way to grant access, so they
// are not reported, except `write-all` and `read-all`. When the workflow runs on a privileged trigger (pull_request_target,
// workflow_run and issue_comment) the message of a workflow-level write scope says so, since people
// without write access can start the workflow.
//
// A job without any `permissions:` in a workflow without any is the business of the missing-permissions
// rule. The "require-workflow-permissions" option also reports a workflow without a top-level
// `permissions:`, even when its jobs set their own, so that the default permissions of the repository
// apply to no job added later.
type RuleExcessivePermissions struct {
	RuleBase
	// trigger is the name of the privileged trigger of the workflow, or "".
	trigger string
	// singleJob is true when the workflow has exactly one job.
	singleJob bool
}

// NewRuleExcessivePermissions creates a new RuleExcessivePermissions instance.
func NewRuleExcessivePermissions() *RuleExcessivePermissions {
	return &RuleExcessivePermissions{
		RuleBase: RuleBase{
			name: "excessive-permissions",
			desc: "Checks for write permissions of the GITHUB_TOKEN which are broader than needed: \"write-all\" and write scopes at the workflow level",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleExcessivePermissions) VisitWorkflowPre(n *Workflow) error {
	rule.trigger = ""
	rule.singleJob = len(n.Jobs) == 1
	for _, e := range n.On {
		if slices.Contains(privilegedTriggers, e.EventName()) {
			rule.trigger = e.EventName()
			break
		}
	}
	if !rule.Config().RuleEnabled("excessive-permissions") {
		return nil
	}
	if n.Permissions == nil {
		if on, _ := rule.Config().ruleOptionBool("excessive-permissions", "require-workflow-permissions"); on {
			rule.ReportID(
				"excessive-permissions",
				&Pos{Line: 1, Col: 1},
				"the workflow has no top-level \"permissions:\", so every job without its own gets the default permissions of the repository. set \"permissions: {}\" at the workflow level and grant the scopes to the jobs which need them",
			)
		}
		return nil
	}
	rule.check(n.Permissions, false)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleExcessivePermissions) VisitJobPre(n *Job) error {
	if !rule.Config().RuleEnabled("excessive-permissions") {
		return nil
	}
	rule.check(n.Permissions, true)
	return nil
}

func (rule *RuleExcessivePermissions) check(p *Permissions, job bool) {
	if p == nil {
		return
	}

	if p.All != nil {
		where := "workflow"
		if job {
			where = "job"
		}
		switch p.All.Value {
		case "write-all":
			rule.ReportIDf(
				"excessive-permissions",
				p.All.Pos,
				"\"write-all\" gives the GITHUB_TOKEN write access to every scope for the whole %s. list only the scopes which are needed and set the others to \"read\" or \"none\"",
				where,
			)
		case "read-all":
			rule.ReportIDf(
				"excessive-permissions",
				p.All.Pos,
				"\"read-all\" gives the GITHUB_TOKEN read access to every scope for the whole %s, including ones it never uses. list only the scopes which are needed",
				where,
			)
		}
		return
	}

	scopes := make([]string, 0, len(p.Scopes))
	for name := range p.Scopes {
		scopes = append(scopes, name)
	}
	slices.Sort(scopes)

	for _, name := range scopes {
		s := p.Scopes[name]
		if s == nil || s.Name == nil || s.Value == nil || s.Value.Value != "write" {
			continue
		}
		if _, ok := allPermissionScopes[name]; !ok {
			continue // reported by the permissions rule
		}
		impact := scopeWriteImpact[name]
		if job {
			continue
		}
		if rule.singleJob {
			// There is no other job to keep it from: moving the scope to the job would change nothing, so the advice
			// would be a no-op (zizmor does not report this either). It stays a finding only where people without write
			// access can start the workflow, with a message that does not ask for the move.
			if rule.trigger != "" {
				rule.ReportIDf(
					"excessive-permissions",
					s.Name.Pos,
					"%q lets the job %s, and the workflow runs on %q which people without write access can trigger. check that the job needs it and cannot run untrusted code",
					name+": write", impact, rule.trigger,
				)
			}
			continue
		}
		note := ""
		if rule.trigger != "" {
			note = fmt.Sprintf(", and the workflow runs on %q which people without write access can trigger", rule.trigger)
		}
		rule.ReportIDf(
			"excessive-permissions",
			s.Name.Pos,
			"%q is granted to every job of the workflow, which lets any of them %s%s. set \"permissions: {}\" at the workflow level and grant %q only to the job which needs it",
			name+": write", impact, note, name+": write",
		)
	}
}

func init() {
	registerRules(
		RuleInfo{
			ID: "excessive-permissions", Group: RuleGroupSecurity, Summary: "The GITHUB_TOKEN gets write access that is broader than needed.",
			DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-excessive-permissions",
			Options: []RuleOption{{Name: "require-workflow-permissions", Kind: RuleOptionBool, Default: false, Summary: "Also report a workflow which has no top-level permissions, even when its jobs set their own."}},
		},
	)
	registerRuleFactory("excessive-permissions", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleExcessivePermissions()}
	})
}
