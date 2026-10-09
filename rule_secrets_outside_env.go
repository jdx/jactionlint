package jactionlint

import "strings"

// RuleSecretsOutsideEnv is a rule to detect secrets which are used by a job without an
// "environment:". Secrets of a repository or an organization are exposed to every job that asks for
// them, while the secrets of an environment are protected by the rules of the environment.
// https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-deployments/managing-environments-for-deployment
type RuleSecretsOutsideEnv struct {
	RuleBase
}

// NewRuleSecretsOutsideEnv creates a new RuleSecretsOutsideEnv instance.
func NewRuleSecretsOutsideEnv() *RuleSecretsOutsideEnv {
	return &RuleSecretsOutsideEnv{
		RuleBase: RuleBase{
			name: "secrets-outside-env",
			desc: "Checks that jobs which use secrets run in an environment",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleSecretsOutsideEnv) VisitWorkflowPre(n *Workflow) error {
	if onlyWorkflowCall(n) {
		// A reusable workflow gets its secrets from the caller. Environment secrets do not reach it
		// unless the caller inherits all secrets, so there is nothing the workflow itself can fix.
		// A workflow which also has another trigger runs on its own too, so it is checked.
		return nil
	}
	allow := rule.Config().ruleOptionStrings("secrets-outside-env", "allow")
	isAllowed := func(name string) bool {
		if strings.EqualFold(name, "github_token") {
			return true // Its permissions are decided by the workflow
		}
		for _, a := range allow {
			if strings.EqualFold(a, name) {
				return true
			}
		}
		return false
	}

	workflowExprs(n, func(site exprSite, o *exprOccurrence) {
		j := site.Job
		if j == nil || j.Environment != nil || j.WorkflowCall != nil {
			return
		}
		VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
			if !entering {
				return
			}
			name, ok := secretNameOf(node)
			if !ok || isAllowed(name) {
				return
			}
			rule.ReportIDf("secrets-outside-env", o.PosOf(node), "secret %q is used by a job which has no \"environment:\", so it is a repository or organization secret exposed to every job. move it to an environment with protection rules and set \"environment:\" at the job", strings.ToUpper(name))
		})
	})
	return nil
}

func init() {
	registerRules(
		RuleInfo{
			ID: "secrets-outside-env", Group: RuleGroupSecurity, Summary: "A job uses a secret but has no environment.", DefaultLevel: SeverityWarning, Profile: ProfileAll,
			DocsAnchor: "check-secrets-outside-env",
			Options:    []RuleOption{{Name: "allow", Kind: RuleOptionStrings, Summary: "Names of secrets which may be used outside of an environment. GITHUB_TOKEN is always allowed."}},
		},
	)
	registerRuleFactory("secrets-outside-env", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("secrets-outside-env") {
			return nil
		}
		return []Rule{NewRuleSecretsOutsideEnv()}
	})
}
