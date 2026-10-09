package jactionlint

import (
	"slices"
	"strings"
)

// RuleUndocumentedPermissions is a rule checker which requires a comment explaining why a permission
// scope of the GITHUB_TOKEN is granted above `read`. A scope is explained by a comment at the end of
// its line or by a block of comment lines directly above it. Comments which only configure a tool
// (such as an ignore comment) do not explain anything.
//
// With the "include-read" option every scope is checked except `contents: read`, which everybody
// needs to check out the repository.
type RuleUndocumentedPermissions struct {
	RuleBase
	comments *CommentIndex
}

// NewRuleUndocumentedPermissions creates a new RuleUndocumentedPermissions instance.
func NewRuleUndocumentedPermissions() *RuleUndocumentedPermissions {
	return &RuleUndocumentedPermissions{
		RuleBase: RuleBase{
			name: "undocumented-permissions",
			desc: "Checks that a permission scope above \"read\" has a comment explaining why it is needed",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUndocumentedPermissions) VisitWorkflowPre(n *Workflow) error {
	rule.comments = n.Comments
	if rule.Config().RuleEnabled("undocumented-permissions") {
		rule.check(n.Permissions)
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUndocumentedPermissions) VisitJobPre(n *Job) error {
	if rule.Config().RuleEnabled("undocumented-permissions") {
		rule.check(n.Permissions)
	}
	return nil
}

func (rule *RuleUndocumentedPermissions) check(p *Permissions) {
	if p == nil || p.All != nil {
		return
	}
	includeRead, _ := rule.Config().ruleOptionBool("undocumented-permissions", "include-read")

	names := make([]string, 0, len(p.Scopes))
	for name := range p.Scopes {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		s := p.Scopes[name]
		if s == nil || s.Name == nil || s.Value == nil {
			continue
		}
		switch s.Value.Value {
		case "write":
		case "read":
			if !includeRead || name == "contents" {
				continue
			}
		default:
			continue // "none" grants nothing and anything else is reported by the permissions rule
		}
		if rule.documented(s.Name.Pos.Line) {
			continue
		}
		rule.ReportIDf(
			"undocumented-permissions",
			s.Name.Pos,
			"permission %q has no comment explaining why it is needed. add a comment at the end of the line or above it",
			name+": "+s.Value.Value,
		)
	}
}

// documented reports whether a comment explains what is written at the line.
func (rule *RuleUndocumentedPermissions) documented(line int) bool {
	if c := rule.comments.Inline(line); c != nil && isExplanatoryComment(c.Text) {
		return true
	}
	return slices.ContainsFunc(rule.comments.Before(line), func(c Comment) bool { return isExplanatoryComment(c.Text) })
}

// isExplanatoryComment reports whether the text of a comment can explain something. Comments which
// only drive a tool, such as "jactionlint ignore=..." and "zizmor: ignore[...]", are not.
func isExplanatoryComment(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	for _, tool := range []string{"jactionlint", "zizmor", "actionlint", "yamllint", "prettier-ignore"} {
		if strings.HasPrefix(t, tool) {
			return false
		}
	}
	return true
}

func init() {
	registerRules(
		RuleInfo{
			ID: "undocumented-permissions", Group: RuleGroupPolicy, Summary: "A permission scope above read has no comment explaining it.",
			DefaultLevel: SeverityInfo, Profile: ProfileAll, DocsAnchor: "check-undocumented-permissions",
			Options: []RuleOption{{Name: "include-read", Kind: RuleOptionBool, Default: false, Summary: "Also require a comment for scopes granted with read, except contents: read."}},
		},
	)
	registerRuleFactory("undocumented-permissions", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleUndocumentedPermissions()}
	})
}
