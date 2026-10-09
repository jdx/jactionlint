package jactionlint

// A few audits have the checks of a regular and of a pedantic persona, like zizmor's. They are one rule with one
// ID: the pedantic checks run when the option "pedantic" is true. Unset, it is true under the pedantic
// profile and false under the others, so `profile: pedantic` reports both personas and a plain run only
// the regular one.
const pedanticOptionSummary = "Also report the pedantic checks, which are noisier. Unset, they run under the pedantic profile."

// auditPedantic reports whether the pedantic checks of the audit run.
func (c *Config) auditPedantic(id string) bool {
	if c == nil {
		return false
	}
	if v, ok := c.ruleOptionBool(id, "pedantic"); ok {
		return v
	}
	return c.Profile == ProfilePedantic
}

// pedantic reports whether the pedantic checks of the audit run with the configuration of the rule.
func (r *RuleBase) pedantic(id string) bool {
	return r.Config().auditPedantic(id)
}

// pedanticOption is the option of the audits which have pedantic checks.
var pedanticOption = RuleOption{Name: "pedantic", Kind: RuleOptionBool, Summary: pedanticOptionSummary}
