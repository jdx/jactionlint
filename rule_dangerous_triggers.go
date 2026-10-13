package jactionlint

import (
	"fmt"
	"sort"
	"strings"
)

// dangerousTriggers are the events which run with the privileges of the target repository (its token and secrets) while a
// fork or any commenter controls part of the event. Before is what the message says up to the privileges ("with ..."), After
// what follows them.
var dangerousTriggers = map[string]struct{ Before, After string }{
	"pull_request_target": {"runs in the context of the base repository", ", even when the pull request comes from a fork. use \"pull_request\" unless write access is required, and never check out or run code of the pull request"},
	"workflow_run":        {"runs in the context of the default branch", ", but processes artifacts and metadata produced by a run that a fork may control. consider a reusable workflow called with \"workflow_call\" instead, and never trust the artifacts or the event payload"},
	"issue_comment":       {"runs in the context of the default branch", ", and it can be started by anyone who can comment. check the permission of the commenter and never run code of the pull request"},
}

// RuleDangerousTriggers is a rule to detect workflow triggers which are difficult to use securely:
// "pull_request_target", "workflow_run" and "issue_comment". The message says what the workflow can use (the scopes
// its token can write, the secrets it reads), and a workflow that can neither write nor read a secret is a pedantic finding.
// https://securitylab.github.com/resources/github-actions-preventing-pwn-requests/
type RuleDangerousTriggers struct {
	RuleBase
}

// NewRuleDangerousTriggers creates a new RuleDangerousTriggers instance.
func NewRuleDangerousTriggers() *RuleDangerousTriggers {
	return &RuleDangerousTriggers{
		RuleBase: RuleBase{
			name: "dangerous-triggers",
			desc: "Checks for workflow triggers which are difficult to use securely",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleDangerousTriggers) VisitWorkflowPre(n *Workflow) error {
	var priv *triggerPrivileges
	for _, e := range n.On {
		w, ok := e.(*WebhookEvent)
		if !ok || w.Hook == nil {
			continue
		}
		if w.Hook.Value == "pull_request_target" && onlyRunsLabeler(n) {
			continue
		}
		t, ok := dangerousTriggers[w.Hook.Value]
		if !ok {
			continue
		}
		if priv == nil {
			priv = privilegesOf(n)
		}
		if !priv.any() {
			if !rule.pedantic("dangerous-triggers") {
				continue
			}
			rule.ReportIDf("dangerous-triggers", w.Hook.Pos, "trigger %q is dangerous: it %s with %s%s. with no write permission and no secret, what is at stake is the cache, the artifacts and the compute of the runs", w.Hook.Value, t.Before, priv.describe(), t.After)
			continue
		}
		rule.ReportIDf("dangerous-triggers", w.Hook.Pos, "trigger %q is dangerous: it %s with %s%s", w.Hook.Value, t.Before, priv.describe(), t.After)
	}
	return nil
}

// triggerPrivileges is what the jobs of a workflow can do with the token and the secrets of the repository.
type triggerPrivileges struct {
	// writes are the scopes (sorted) that the token of a job can write, "every scope" for write-all.
	writes []string
	// undeclared is true when a job sets no permissions and neither does the workflow, so the default of the repository
	// applies, which is a write token unless the repository restricts it.
	undeclared bool
	// expression is true when the permissions of a job are an expression, or a scope has a value that is one, so what
	// the token can do is not known: it may write.
	expression bool
	// secrets are the names (upper case, sorted) of the secrets which an expression reads, other than GITHUB_TOKEN.
	secrets []string
	// allSecrets is true when the workflow can read secrets whose names it does not write: "secrets: inherit" of a call of a
	// reusable workflow, toJSON(secrets) and secrets[expression].
	allSecrets bool
}

// any reports whether the workflow can write with its token or read a secret.
func (p *triggerPrivileges) any() bool {
	return len(p.writes) > 0 || p.undeclared || p.expression || len(p.secrets) > 0 || p.allSecrets
}

// describe lists what the workflow has, for the message.
func (p *triggerPrivileges) describe() string {
	var parts []string
	// every kind of token that a job of the workflow has
	if len(p.writes) > 0 && p.writes[0] == "every scope" {
		parts = append(parts, "a token that can write every scope")
	} else if len(p.writes) > 0 {
		parts = append(parts, "a write token ("+strings.Join(p.writes, ", ")+")")
	}
	if p.undeclared {
		parts = append(parts, "the default token of the repository (it can write unless the settings of the repository restrict it)")
	}
	if p.expression {
		parts = append(parts, "a token whose permissions are an expression, so it may write")
	}
	switch {
	case len(p.secrets) > 0 && p.allSecrets:
		parts = append(parts, "secrets ("+listNames(p.secrets)+" and others)")
	case len(p.secrets) > 0:
		parts = append(parts, "secrets ("+listNames(p.secrets)+")")
	case p.allSecrets:
		parts = append(parts, "secrets which it passes on without naming them")
	default:
		parts = append(parts, "no secrets")
	}
	if len(p.writes) == 0 && !p.undeclared && !p.expression {
		return "a token that cannot write and " + parts[0]
	}
	// the tokens of the jobs, then the secrets: "a write token (issues) and the default token of the repository (...) and no secrets"
	return strings.Join(parts, " and ")
}

// listNames joins the first names of a list, with the number of the others.
func listNames(names []string) string {
	const max = 3
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + fmt.Sprintf(" and %d more", len(names)-max)
}

// privilegesOf reads the permissions of the jobs and the secrets that the expressions of the workflow use.
func privilegesOf(w *Workflow) *triggerPrivileges {
	p := &triggerPrivileges{}
	writes := map[string]bool{}
	for _, j := range w.Jobs {
		if j == nil {
			continue
		}
		perms := j.Permissions
		if perms == nil {
			perms = w.Permissions
		}
		switch {
		case perms == nil:
			p.undeclared = true
		case perms.All != nil:
			switch strings.ToLower(strings.TrimSpace(perms.All.Value)) {
			case "write-all":
				writes["every scope"] = true
			case "read-all", "":
			default:
				p.expression = true // an expression: not known
			}
		default:
			for name, sc := range perms.Scopes {
				if sc == nil || sc.Value == nil {
					continue
				}
				switch strings.ToLower(strings.TrimSpace(sc.Value.Value)) {
				case "write":
					writes[strings.ToLower(name)] = true
				case "read", "none":
				default:
					p.expression = true
				}
			}
		}
		if j.WorkflowCall != nil && j.WorkflowCall.InheritSecrets {
			p.allSecrets = true
		}
	}
	secrets := map[string]bool{}
	workflowExprs(w, func(_ exprSite, o *exprOccurrence) {
		VisitExprNode(o.Root, func(node, parent ExprNode, entering bool) {
			if !entering {
				return
			}
			if name, ok := secretNameOf(node); ok {
				if !strings.EqualFold(name, "github_token") {
					secrets[strings.ToUpper(name)] = true
				}
				return
			}
			if isContextVariable(node, "secrets") && !isSecretAccess(parent) {
				p.allSecrets = true
			}
		})
	})
	if writes["every scope"] {
		p.writes = []string{"every scope"}
	} else {
		for s := range writes {
			p.writes = append(p.writes, s)
		}
		sort.Strings(p.writes)
	}
	for s := range secrets {
		p.secrets = append(p.secrets, s)
	}
	sort.Strings(p.secrets)
	return p
}

// isSecretAccess reports whether the parent of the secrets context reads one secret by a name written in the expression.
func isSecretAccess(parent ExprNode) bool {
	_, ok := secretNameOf(parent)
	return ok
}

// onlyRunsLabeler reports whether every step of the workflow is actions/labeler, which labels pull
// requests by the paths they change and never runs code from the pull request. That is the one use of
// pull_request_target which is accepted as safe.
func onlyRunsLabeler(n *Workflow) bool {
	steps := 0
	only := true
	for _, j := range n.Jobs {
		if j == nil {
			continue
		}
		if j.WorkflowCall != nil {
			return false
		}
		walkSteps(j.Steps, func(s *Step) {
			steps++
			a, ok := s.Exec.(*ExecAction)
			if !ok || a == nil || a.Uses == nil || ParseUses(a.Uses.Value).CanonicalName() != "actions/labeler" {
				only = false
			}
		})
	}
	return only && steps > 0
}

func init() {
	registerRules(
		RuleInfo{ID: "dangerous-triggers", Group: RuleGroupSecurity, Summary: "A workflow uses pull_request_target, workflow_run or issue_comment. With the option pedantic, also one that cannot write and uses no secret.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-dangerous-triggers", Options: []RuleOption{pedanticOption}},
	)
	registerRuleFactory("dangerous-triggers", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("dangerous-triggers") {
			return nil
		}
		return []Rule{NewRuleDangerousTriggers()}
	})
}
