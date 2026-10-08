package jactionlint

// RuleForbiddenUses is a rule to restrict the actions and reusable workflows a repository may use. With
// an "allow" list only the listed ones are accepted. With a "deny" list the listed ones are rejected.
// The rule does nothing without one of them.
type RuleForbiddenUses struct {
	RuleBase
	allow, deny []UsesPattern
}

// NewRuleForbiddenUses creates a new RuleForbiddenUses instance from the patterns of the allow and
// deny lists. It returns nil when both lists are empty.
func NewRuleForbiddenUses(allow, deny []string) *RuleForbiddenUses {
	if len(allow) == 0 && len(deny) == 0 {
		return nil
	}
	r := &RuleForbiddenUses{
		RuleBase: RuleBase{
			name: "forbidden-uses",
			desc: "Checks `uses:` against the allow and deny lists of the configuration",
		},
	}
	for _, s := range allow {
		r.allow = append(r.allow, ParseUsesPattern(s))
	}
	for _, s := range deny {
		r.deny = append(r.deny, ParseUsesPattern(s))
	}
	return r
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleForbiddenUses) VisitJobPre(n *Job) error {
	if n.WorkflowCall != nil {
		rule.check(n.WorkflowCall.Uses)
	}
	walkSteps(n.Steps, func(s *Step) {
		if a, ok := s.Exec.(*ExecAction); ok && a != nil {
			rule.check(a.Uses)
		}
	})
	return nil
}

func (rule *RuleForbiddenUses) check(uses *String) {
	if uses == nil || uses.Pos == nil {
		return
	}
	u := ParseUses(uses.Value)
	if !u.IsRepo() || u.Dynamic {
		return
	}
	for _, p := range rule.deny {
		if p.Match(u) {
			rule.ReportIDf("forbidden-uses", uses.Pos, "%s %q is forbidden by pattern %q in the \"deny\" list of the \"forbidden-uses\" rule. remove it or change the configuration", usesNoun(u), uses.Value, p)
			return
		}
	}
	if len(rule.allow) == 0 {
		return
	}
	for _, p := range rule.allow {
		if p.Match(u) {
			return
		}
	}
	rule.ReportIDf("forbidden-uses", uses.Pos, "%s %q is not allowed because it matches no pattern in the \"allow\" list of the \"forbidden-uses\" rule (%s). use an allowed one or add a pattern to the configuration", usesNoun(u), uses.Value, quoteJoin(patternStrings(rule.allow)))
}

// usesNoun names what a reference points to in a message.
func usesNoun(u *UsesRef) string {
	if u.Kind == UsesReusableWorkflow {
		return "reusable workflow"
	}
	return "action"
}

func patternStrings(ps []UsesPattern) []string {
	ss := make([]string, len(ps))
	for i, p := range ps {
		ss[i] = p.String()
	}
	return ss
}

func init() {
	registerRules(
		RuleInfo{
			ID: "forbidden-uses", Group: RuleGroupPolicy, Summary: "An action or reusable workflow is not allowed or is denied by the configuration.", DefaultLevel: SeverityError,
			DocsAnchor: "check-forbidden-uses",
			Options: []RuleOption{
				{Name: "allow", Kind: RuleOptionStrings, Summary: "Patterns of the only actions and reusable workflows which may be used, e.g. \"actions/*\". The rule does nothing without allow or deny."},
				{Name: "deny", Kind: RuleOptionStrings, Summary: "Patterns of actions and reusable workflows which must not be used."},
			},
		},
	)
	registerRuleFactory("forbidden-uses", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("forbidden-uses") {
			return nil
		}
		r := NewRuleForbiddenUses(env.config.ruleOptionStrings("forbidden-uses", "allow"), env.config.ruleOptionStrings("forbidden-uses", "deny"))
		if r == nil {
			return nil
		}
		return []Rule{r}
	})
}
