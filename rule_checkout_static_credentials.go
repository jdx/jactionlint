package jactionlint

import (
	"strings"
)

// RuleCheckoutStaticCredentials is a rule checker which reports actions/checkout steps that are given a
// credential that does not expire: the "ssh-key" input, or a "token" input written in the workflow. With
// the "secret-tokens" option it also reports a "token" taken from a secret other than GITHUB_TOKEN (a
// personal access token, in most cases). That is off by default because workflows that have to push
// something that triggers other workflows, or reach other repositories, use one on purpose. The default
// GITHUB_TOKEN lives as long as the job and is limited to the repository, and a token created by a GitHub
// App lives for an hour at most. A static credential is valid until somebody rotates it, can usually
// reach much more than the job needs, and checkout writes it to the git config of the workspace (unless
// "persist-credentials" is false) where every later step can read it.
//
// The rule is conservative: a "token" taken from an expression that is not made only of secrets (the
// output of a step, "github.token", an input of a reusable workflow, a fallback to GITHUB_TOKEN) is not
// reported, because what it holds is not known.
type RuleCheckoutStaticCredentials struct {
	RuleBase
}

// NewRuleCheckoutStaticCredentials creates a new RuleCheckoutStaticCredentials instance.
func NewRuleCheckoutStaticCredentials() *RuleCheckoutStaticCredentials {
	return &RuleCheckoutStaticCredentials{
		RuleBase: RuleBase{
			name: "checkout-static-credentials",
			desc: "Checks that actions/checkout is not given a static credential",
		},
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleCheckoutStaticCredentials) VisitJobPre(n *Job) error {
	allow := rule.Config().ruleOptionStrings("checkout-static-credentials", "allow")
	secretTokens, _ := rule.Config().ruleOptionBool("checkout-static-credentials", "secret-tokens")
	for _, s := range flattenSteps(n.Steps) {
		a, ref := stepAction(s)
		if a == nil || !ref.isRepoAction("actions/checkout") {
			continue
		}
		for _, name := range []string{"ssh-key", "token"} {
			in := a.Inputs[name]
			if in == nil || in.Value == nil || strings.TrimSpace(in.Value.Value) == "" {
				continue
			}
			src, ok := staticCredentialSource(in.Value, allow, name == "ssh-key" || secretTokens)
			if !ok {
				continue
			}
			pos := in.Name.Pos
			if pos == nil {
				pos = in.Value.Pos
			}
			rule.ReportIDf("checkout-static-credentials", pos, "actions/checkout is given %s as %q. this credential does not expire and can reach more than this job needs, and unless \"persist-credentials: false\" is set it is also left in the git config of the workspace for every later step to read. use the default GITHUB_TOKEN for this repository, or a short-lived token from a GitHub App (actions/create-github-app-token) to reach other repositories", src, name)
		}
	}
	return nil
}

// staticCredentialSource describes where the value of an input gets a static credential from: a literal
// written in the workflow or secrets. It returns false when the value is not (known to be) one, when
// every secret is allowed, or when secrets are not to be reported (withSecrets is false).
func staticCredentialSource(v *String, allow []string, withSecrets bool) (string, bool) {
	if !v.ContainsExpression() {
		return "a value written in the workflow", true
	}
	if !withSecrets {
		return "", false
	}
	var secrets []string
	other := false
	scanExpressions(v, false, func(o *exprOccurrence) {
		static, total := 0, 0
		VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
			if !entering {
				return
			}
			if name, ok := secretNameOf(node); ok {
				static++
				if strings.EqualFold(name, "GITHUB_TOKEN") {
					other = true
				} else {
					secrets = append(secrets, name)
				}
				return
			}
			vn, ok := node.(*VariableNode)
			if !ok {
				return
			}
			if strings.EqualFold(vn.Name, "secrets") {
				total++
			} else {
				other = true // github.token, steps, inputs, vars...: not known to be static
			}
		})
		if total != static {
			other = true // a secret picked by a computed name
		}
	})
	if other || len(secrets) == 0 {
		return "", false
	}
	for _, name := range secrets {
		allowed := false
		for _, a := range allow {
			if strings.EqualFold(a, name) {
				allowed = true
			}
		}
		if !allowed {
			return "secret \"" + strings.ToUpper(name) + "\"", true
		}
	}
	return "", false
}

func init() {
	registerRules(RuleInfo{
		ID: "checkout-static-credentials", Group: RuleGroupSecurity, Summary: "actions/checkout is given an SSH key or a token that does not expire.",
		DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-checkout-static-credentials",
		Options: []RuleOption{
			{Name: "secret-tokens", Kind: RuleOptionBool, Default: false, Summary: "Also report a token input taken from a secret other than GITHUB_TOKEN (a personal access token). The ssh-key input is always reported."},
			{Name: "allow", Kind: RuleOptionStrings, Summary: "Names of secrets which may be given to actions/checkout (for example a deploy key)."},
		},
	})
	registerRuleFactory("checkout-static-credentials", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("checkout-static-credentials") {
			return nil
		}
		return []Rule{NewRuleCheckoutStaticCredentials()}
	})
}
