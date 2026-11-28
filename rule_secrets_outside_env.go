package jactionlint

import "strings"

// RuleSecretsOutsideEnv is a rule to detect secrets which are used by a job without an
// "environment:". Secrets of a repository or an organization are exposed to every job that asks for
// them, while the secrets of an environment are protected by the rules of the environment.
// https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-deployments/managing-environments-for-deployment
type RuleSecretsOutsideEnv struct {
	RuleBase
	cache *LocalReusableWorkflowCache // maybe nil
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

// calleeReadsInEnvOnly reports whether the local reusable workflow reads the secret and all reads
// are in jobs with an "environment:". A same-name environment secret takes precedence in those jobs.
// https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows#using-inputs-and-secrets-in-a-reusable-workflow
func (rule *RuleSecretsOutsideEnv) calleeReadsInEnvOnly(call *WorkflowCall, name string) bool {
	if rule.cache == nil || call.Uses == nil {
		return false
	}
	u := rule.cache.secretUse(call.Uses.Value)
	name = strings.ToLower(name)
	return u != nil && !u.unknown && u.inEnv[name] && !u.outsideEnv[name]
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleSecretsOutsideEnv) VisitWorkflowPre(n *Workflow) error {
	if onlyWorkflowCall(n) {
		// A reusable workflow gets its secrets from the caller, which reports a passed secret that no environment
		// protects. A workflow which also has another trigger runs on its own too, so it is checked.
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
		if j == nil || j.Environment != nil {
			return
		}
		if j.WorkflowCall != nil {
			// Only expressions in the values under "secrets:" pass secrets to the callee.
			key := ""
			for k, sec := range j.WorkflowCall.Secrets {
				if sec != nil && sec.Value == site.Str {
					key = k
					break
				}
			}
			if key == "" {
				return
			}
			// Environment secrets take precedence for a direct, same-name mapping.
			// A string with literal text or additional placeholders can transform the passed value.
			if name, ok := secretNameOf(o.Root); ok && strings.EqualFold(key, name) {
				p := o.Str.placeholders()
				if !p.failed && len(p.list) == 1 && p.list[0].after == len("${{") &&
					p.list[0].after+p.list[0].n == len(o.Str.Value) && rule.calleeReadsInEnvOnly(j.WorkflowCall, key) {
					return
				}
			}
			VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
				if !entering {
					return
				}
				name, ok := secretNameOf(node)
				if !ok || isAllowed(name) {
					return
				}
				rule.ReportIDf("secrets-outside-env", o.PosOf(node), "secret %q is passed to a reusable workflow by a job which cannot have an \"environment:\", so a repository or organization secret of this name may be available outside an environment. pass a token that the called workflow mints, let a job of the called workflow use an environment of its own, or inline the workflow in a job with an environment", strings.ToUpper(name))
			})
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
			ID: "secrets-outside-env", Group: RuleGroupSecurity, Summary: "A job uses a secret but has no environment, or passes one to a reusable workflow.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic,
			DocsAnchor: "check-secrets-outside-env",
			Options:    []RuleOption{{Name: "allow", Kind: RuleOptionStrings, Summary: "Names of secrets which may be used outside of an environment. GITHUB_TOKEN is always allowed."}},
		},
	)
	registerRuleFactory("secrets-outside-env", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("secrets-outside-env") {
			return nil
		}
		rule := NewRuleSecretsOutsideEnv()
		rule.cache = env.localReusableWorkflows
		return []Rule{rule}
	})
}
