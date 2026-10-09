package jactionlint

// RuleDependabotExecution is a rule to check that updates of a Dependabot configuration do not allow
// the package managers to run code of the dependencies.
// https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference#insecure-external-code-execution--
type RuleDependabotExecution struct {
	DependabotRuleBase
	src *dependabotSource
}

// NewRuleDependabotExecution creates a new RuleDependabotExecution instance. src is the content of
// the file, which is used to build the automatic fix. It can be nil.
func NewRuleDependabotExecution(src []byte) *RuleDependabotExecution {
	r := &RuleDependabotExecution{
		DependabotRuleBase: NewDependabotRuleBase("dependabot-execution", "Checks that updates of dependabot.yml do not set insecure-external-code-execution to allow"),
	}
	if src != nil {
		r.src = newDependabotSource(src)
	}
	return r
}

// VisitDependabotUpdate is callback when visiting an item of "updates".
func (r *RuleDependabotExecution) VisitDependabotUpdate(u *DependabotUpdate) error {
	v := u.InsecureExternalCodeExecution
	if v == nil || v.Pos == nil || v.Value != "allow" {
		return nil
	}
	r.ReportID("dependabot-execution", v.Pos, "\"insecure-external-code-execution: allow\" lets Dependabot run code from the dependencies it updates, which can expose the credentials Dependabot uses. remove it or set it to \"deny\"")
	if r.src != nil {
		if edit, ok := r.src.replaceScalar(v.Pos, v.Value, v.Quoted, "deny"); ok {
			// Dependabot may fail to update dependencies which need the code to be executed
			r.errs[len(r.errs)-1].Fix = &Fix{Description: "Set insecure-external-code-execution to deny", Unsafe: true, Edits: []TextEdit{edit}}
		}
	}
	return nil
}

func init() {
	registerRules(RuleInfo{
		ID: "dependabot-execution", Group: RuleGroupSecurity,
		Summary:      "An update in dependabot.yml allows insecure external code execution.",
		DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-dependabot-execution",
	})
	dependabotRuleFactories = append(dependabotRuleFactories, dependabotRuleFactory{
		kind: "dependabot-execution",
		new: func(ctx *dependabotRuleContext) (DependabotRule, error) {
			if !ctx.config.RuleEnabled("dependabot-execution") {
				return nil, nil
			}
			return NewRuleDependabotExecution(ctx.src), nil
		},
	})
}
