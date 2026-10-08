package jactionlint

import (
	"strings"
	"sync"
)

// popularActionSlugs is the "owner/repo" of the popular actions, lower-cased.
var popularActionSlugs = sync.OnceValue(func() []string {
	seen := map[string]bool{}
	var ret []string
	for spec := range PopularActions {
		slug, _, _ := strings.Cut(spec, "@")
		if i := strings.IndexByte(slug, '/'); i >= 0 {
			if j := strings.IndexByte(slug[i+1:], '/'); j >= 0 {
				slug = slug[:i+1+j]
			}
		}
		slug = strings.ToLower(slug)
		if !seen[slug] {
			seen[slug] = true
			ret = append(ret, slug)
		}
	}
	return ret
})

// RuleTyposquatUses is a rule to detect actions whose "owner/repo" is one typo away from a popular
// action of another owner, like "action/checkout" instead of "actions/checkout". An attacker can
// register the misspelled account and serve malicious code to everyone who copied the typo.
type RuleTyposquatUses struct {
	RuleBase
	allow []string
}

// NewRuleTyposquatUses creates a new RuleTyposquatUses instance. The slugs ("owner/repo") in allow are
// never reported.
func NewRuleTyposquatUses(allow []string) *RuleTyposquatUses {
	low := make([]string, len(allow))
	for i, a := range allow {
		low[i] = strings.ToLower(strings.TrimSpace(a))
	}
	return &RuleTyposquatUses{
		RuleBase: RuleBase{
			name: "typosquat-uses",
			desc: "Checks for actions which look like a typo of a popular action",
		},
		allow: low,
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleTyposquatUses) VisitJobPre(n *Job) error {
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

func (rule *RuleTyposquatUses) check(uses *String) {
	if uses == nil || uses.Pos == nil {
		return
	}
	u := ParseUses(uses.Value)
	if !u.IsRepo() || u.Dynamic {
		return
	}
	slug := strings.ToLower(u.Owner + "/" + u.Repo)
	if len(u.Owner) < 3 {
		return
	}
	for _, a := range rule.allow {
		if a == slug {
			return
		}
	}
	popular := popularActionSlugs()
	for _, p := range popular {
		if p == slug {
			return // A popular action itself, however close to another one
		}
	}
	for _, p := range popular {
		owner, _, _ := strings.Cut(p, "/")
		if strings.HasPrefix(slug, owner+"/") {
			continue // The owner is right: a wrong repository name fails to resolve
		}
		if withinOneEdit(slug, p) {
			rule.ReportIDf("typosquat-uses", uses.Pos, "%q looks like a typo of the popular action %q, which belongs to another account. a typosquatted account can serve malicious code. check the name; to accept it as is, add %q to the \"allow\" option of the \"typosquat-uses\" rule", u.Owner+"/"+u.Repo, p, slug)
			return
		}
	}
}

// withinOneEdit reports whether b is one edit away from a: one character inserted, removed or
// substituted, or two neighboring characters swapped. Equal strings are not.
func withinOneEdit(a, b string) bool {
	if a == b {
		return false
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) > len(rb) {
		ra, rb = rb, ra
	}
	if len(rb)-len(ra) > 1 {
		return false
	}
	i := 0
	for i < len(ra) && ra[i] == rb[i] {
		i++
	}
	if len(ra) == len(rb) {
		if i+1 < len(ra) && ra[i] == rb[i+1] && ra[i+1] == rb[i] && string(ra[i+2:]) == string(rb[i+2:]) {
			return true // Swapped
		}
		return string(ra[i+1:]) == string(rb[i+1:]) // Substituted
	}
	return string(ra[i:]) == string(rb[i+1:]) // Inserted
}

func init() {
	registerRules(
		RuleInfo{
			ID: "typosquat-uses", Group: RuleGroupSecurity, Summary: "An action is one typo away from a popular action of another owner.", DefaultLevel: SeverityWarning, Profile: ProfileStrict,
			DocsAnchor: "check-typosquat-uses",
			Options:    []RuleOption{{Name: "allow", Kind: RuleOptionStrings, Summary: "Slugs (owner/repo) of actions which are never reported, e.g. a legitimate fork."}},
		},
	)
	registerRuleFactory("typosquat-uses", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("typosquat-uses") {
			return nil
		}
		return []Rule{NewRuleTyposquatUses(env.config.ruleOptionStrings("typosquat-uses", "allow"))}
	})
}
