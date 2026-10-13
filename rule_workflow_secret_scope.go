package jactionlint

import (
	"sort"
	"strings"
)

// RuleWorkflowSecretScope is a rule to detect a secret which is assigned to the workflow-level
// "env:" and so is passed to the steps of every job, although only one job or step needs it.
// https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions
type RuleWorkflowSecretScope struct {
	RuleBase
}

// NewRuleWorkflowSecretScope creates a new RuleWorkflowSecretScope instance.
func NewRuleWorkflowSecretScope() *RuleWorkflowSecretScope {
	return &RuleWorkflowSecretScope{
		RuleBase: RuleBase{
			name: "workflow-secret-scope",
			desc: "Checks for secrets which are assigned to the workflow-level \"env:\" and reach multiple jobs",
		},
	}
}

// envOverrides reports whether env may set the variable. A whole-env expression is unknown, so it is
// assumed to override.
func envOverrides(env *Env, name string) bool {
	if env == nil {
		return false
	}
	if env.Dynamic() {
		return true
	}
	for _, v := range env.Vars {
		if v != nil && v.Name != nil && strings.EqualFold(v.Name.Value, name) {
			return true
		}
	}
	return false
}

// jobSeesWorkflowEnv reports whether some step of the job runs with the workflow-level value of the
// variable. A job which calls a reusable workflow does not pass "env:" on, and a job or step which
// sets the variable itself replaces the workflow-level value.
func jobSeesWorkflowEnv(j *Job, name string) bool {
	if j == nil || j.Composite || j.WorkflowCall != nil || envOverrides(j.Env, name) {
		return false
	}
	for _, s := range j.Steps {
		if s != nil && !envOverrides(s.Env, name) {
			return true
		}
	}
	return false
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleWorkflowSecretScope) VisitWorkflowPre(n *Workflow) error {
	if n.Env == nil || n.Env.Expression != nil || len(n.Jobs) < 2 {
		return nil
	}
	names := make([]string, 0, len(n.Env.Vars))
	for k := range n.Env.Vars {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		v := n.Env.Vars[k]
		if v == nil || v.Name == nil {
			continue
		}
		reach := 0
		for _, j := range n.Jobs {
			if jobSeesWorkflowEnv(j, v.Name.Value) {
				reach++
			}
		}
		if reach < 2 {
			continue
		}
		scanExpressions(v.Value, false, func(o *exprOccurrence) {
			VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
				if !entering {
					return
				}
				secret, ok := secretNameOf(node)
				if !ok || strings.EqualFold(secret, "github_token") {
					return // Its permissions are decided by the workflow
				}
				rule.ReportIDf("workflow-secret-scope", o.PosOf(node), "secret %q is assigned to the workflow-level env %q, so it reaches %d jobs. set it at the job or the step that needs it instead", strings.ToUpper(secret), v.Name.Value, reach)
			})
		})
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children. With the option "job-level" it
// reports a secret in the "env:" of a job, which every step of the job gets, third-party actions included
// (ghalint job_secrets, zizmor#1816).
func (rule *RuleWorkflowSecretScope) VisitJobPre(n *Job) error {
	if on, _ := rule.Config().ruleOptionBool("workflow-secret-scope", "job-level"); !on {
		return nil
	}
	if n == nil || n.Composite || n.Env == nil || n.Env.Expression != nil {
		return nil
	}
	steps := flattenSteps(n.Steps) // the steps of a parallel group get the env of the job too
	if len(steps) < 2 {
		return nil
	}
	names := make([]string, 0, len(n.Env.Vars))
	for k := range n.Env.Vars {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		v := n.Env.Vars[k]
		if v == nil || v.Name == nil || v.Value == nil {
			continue
		}
		reach := 0
		for _, st := range steps {
			if st != nil && !envOverrides(st.Env, v.Name.Value) {
				reach++
			}
		}
		if reach < 2 {
			continue
		}
		scanExpressions(v.Value, false, func(o *exprOccurrence) {
			VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
				if !entering {
					return
				}
				secret, ok := secretNameOf(node)
				if !ok || strings.EqualFold(secret, "github_token") {
					return // Its permissions are decided by the workflow
				}
				rule.ReportIDf("workflow-secret-scope", o.PosOf(node), "secret %q is assigned to the env %q of the job, so it reaches all %d steps of the job, third-party actions included. set it at the step that needs it instead", strings.ToUpper(secret), v.Name.Value, reach)
			})
		})
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "workflow-secret-scope", Group: RuleGroupSecurity, Summary: "A secret is assigned to the workflow-level env and reaches multiple jobs. With the option job-level, also one of the env of a job that reaches its steps.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-workflow-secret-scope",
			Options: []RuleOption{{Name: "job-level", Kind: RuleOptionBool, Default: false, Summary: "Also report a secret assigned to the env of a job with two or more steps, which reaches all of them."}}},
	)
	registerRuleFactory("workflow-secret-scope", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("workflow-secret-scope") {
			return nil
		}
		return []Rule{NewRuleWorkflowSecretScope()}
	})
}
