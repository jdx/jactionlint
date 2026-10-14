package jactionlint

import "strings"

// RuleDependabotRegistryCredentials is a rule to check that the registries of a Dependabot
// configuration do not hold their credentials in plain text.
//
// Dependabot reads a secret from the "Dependabot secrets" of the repository when a value is written as
// ${{secrets.NAME}}. A literal "password", "token", "key" or "auth-key" is a credential committed to the
// repository. It is the counterpart of hardcoded-container-credentials for dependabot.yml
// (https://github.com/zizmorcore/zizmor/issues/1222, which is not an audit of zizmor yet).
//
// https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference#registries--
type RuleDependabotRegistryCredentials struct {
	DependabotRuleBase
}

// NewRuleDependabotRegistryCredentials creates a new RuleDependabotRegistryCredentials instance.
func NewRuleDependabotRegistryCredentials() *RuleDependabotRegistryCredentials {
	return &RuleDependabotRegistryCredentials{
		DependabotRuleBase: NewDependabotRuleBase("hardcoded-registry-credentials", "Checks that the registries of dependabot.yml do not hold their credentials in plain text"),
	}
}

// VisitDependabotPre is callback when visiting the root node before visiting its children.
func (r *RuleDependabotRegistryCredentials) VisitDependabotPre(d *Dependabot) error {
	for _, reg := range d.Registries {
		// The username is not a secret (the options reference writes a literal one in its example), neither are
		// the other settings: organization, repo, tenant-id, client-id, public-key-fingerprint, and so on.
		r.checkField(reg, "password", reg.Password)
		r.checkField(reg, "token", reg.Token)
		r.checkField(reg, "key", reg.Key)
		for _, s := range reg.Settings {
			if s.Key != nil && s.Key.Value == "auth-key" { // hex-repository
				r.checkField(reg, "auth-key", s.Value)
			}
		}
		r.checkURL(reg)
	}
	return nil
}

func (r *RuleDependabotRegistryCredentials) checkField(reg *DependabotRegistry, key string, v *String) {
	if v == nil || v.Pos == nil || !dependabotIsLiteral(v.Value) {
		return
	}
	// The message does not repeat the value.
	r.ReportIDf("hardcoded-registry-credentials", v.Pos, "%q of registry %q is written in plain text. store it as a Dependabot secret and write ${{secrets.NAME}} instead, and rotate the credential because it is in the history of the repository", key, reg.Name.Value)
}

// checkURL reports a password in the user information of the URL (https://user:password@host/).
func (r *RuleDependabotRegistryCredentials) checkURL(reg *DependabotRegistry) {
	if reg.URL == nil || reg.URL.Pos == nil || !dependabotURLHasPassword(reg.URL.Value) {
		return
	}
	r.ReportIDf("hardcoded-registry-credentials", reg.URL.Pos, "\"url\" of registry %q has a password in plain text. remove it from the URL, set \"username\" and \"password\" with ${{secrets.NAME}} instead, and rotate the credential because it is in the history of the repository", reg.Name.Value)
}

// dependabotIsLiteral returns true when a credential value is neither empty nor a reference to a Dependabot
// secret. Any value which has an expression (${{secrets.NAME}} is the form Dependabot knows) is left alone: it
// is not a credential written in plain text, whether or not Dependabot accepts the expression.
func dependabotIsLiteral(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && !strings.Contains(v, "${{")
}

// dependabotURLHasPassword returns true when the URL has user information with a non-empty password
// that is not an expression. A URL without a scheme has none.
func dependabotURLHasPassword(u string) bool {
	_, rest, ok := strings.Cut(u, "://")
	if !ok {
		return false
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	at := strings.LastIndexByte(rest, '@')
	if at < 0 {
		return false
	}
	_, pass, ok := strings.Cut(rest[:at], ":")
	return ok && dependabotIsLiteral(pass)
}

func init() {
	registerRules(RuleInfo{
		ID: "hardcoded-registry-credentials", Group: RuleGroupSecurity,
		Summary:      "A registry in dependabot.yml has a password, token or key written in plain text.",
		DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-hardcoded-registry-credentials",
	})
	dependabotRuleFactories = append(dependabotRuleFactories, dependabotRuleFactory{
		kind: "hardcoded-registry-credentials",
		new: func(ctx *dependabotRuleContext) (DependabotRule, error) {
			if !ctx.config.RuleEnabled("hardcoded-registry-credentials") {
				return nil, nil
			}
			return NewRuleDependabotRegistryCredentials(), nil
		},
	})
}
