package jactionlint

import (
	"fmt"
	"sort"
	"strings"
)

// usesPolicy says how strongly a `uses:` value must be pinned.
type usesPolicy int

const (
	// usesPolicyHashPin requires an immutable reference: a full commit SHA for actions and reusable
	// workflows, a content digest for Docker images. This is the policy of everything that no pattern
	// matches.
	usesPolicyHashPin usesPolicy = iota
	// usesPolicyRefPin requires a ref (a tag, a branch or a commit SHA); for Docker images a tag other
	// than "latest" or a digest.
	usesPolicyRefPin
	// usesPolicyAny requires nothing.
	usesPolicyAny
)

func parseUsesPolicy(s string) (usesPolicy, bool) {
	switch s {
	case "hash-pin":
		return usesPolicyHashPin, true
	case "ref-pin":
		return usesPolicyRefPin, true
	case "any":
		return usesPolicyAny, true
	}
	return 0, false
}

// usesPattern is a parsed key of the "policies" option of the unpinned-uses rule. It matches the
// repository of a `uses:` value: "*" (everything), "owner/*", "owner/repo" or "owner/repo/path"
// (the path and everything below it). "owner/repo/*" is the same as "owner/repo".
type usesPattern struct {
	owner, repo, path string
	// specificity orders the patterns: the more specific one wins whatever the order they are written in.
	specificity int
}

func parseUsesPattern(s string) (usesPattern, error) {
	var p usesPattern
	if s == "" || strings.ContainsAny(s, "@ \t") || strings.Contains(s, "://") {
		return p, fmt.Errorf("pattern %q is invalid. it must look like \"*\", \"owner/*\", \"owner/repo\" or \"owner/repo/path\"", s)
	}
	if s == "*" {
		return p, nil
	}
	parts := strings.Split(strings.TrimSuffix(s, "/"), "/")
	if parts[len(parts)-1] == "*" {
		parts = parts[:len(parts)-1]
	}
	for _, part := range parts {
		if part == "" || strings.Contains(part, "*") {
			return p, fmt.Errorf("pattern %q is invalid. \"*\" is only allowed as the whole pattern or as the last part like \"owner/*\"", s)
		}
	}
	switch {
	case len(parts) == 0:
		return p, fmt.Errorf("pattern %q is invalid", s)
	case len(parts) == 1:
		p.owner = parts[0]
		p.specificity = 1
	default:
		p.owner, p.repo = parts[0], parts[1]
		p.path = strings.Join(parts[2:], "/")
		p.specificity = 2 + len(parts) - 2
	}
	return p, nil
}

func (p usesPattern) matches(u *UsesRef) bool {
	if p.owner == "" {
		return true
	}
	if !strings.EqualFold(p.owner, u.Owner) {
		return false
	}
	if p.repo == "" {
		return true
	}
	if !strings.EqualFold(p.repo, u.Repo) {
		return false
	}
	if p.path == "" {
		return true
	}
	sub := strings.TrimRight(u.Subpath, "/")
	return sub == p.path || strings.HasPrefix(sub, p.path+"/")
}

// validateUsesPolicies checks the "policies" option of the unpinned-uses rule.
func validateUsesPolicies(v any) error {
	m, _ := v.(map[string]string)
	for pat, pol := range m {
		if _, err := parseUsesPattern(pat); err != nil {
			return err
		}
		if _, ok := parseUsesPolicy(pol); !ok {
			return fmt.Errorf("policy %q of %q is invalid. available policies are \"hash-pin\", \"ref-pin\" and \"any\"", pol, pat)
		}
	}
	return nil
}

// usesPolicyFor returns the policy for the repository of the reference: the one of the most specific
// pattern which matches, or hash-pin when none does.
func usesPolicyFor(policies map[string]string, u *UsesRef) usesPolicy {
	best, bestSpec := usesPolicyHashPin, -1
	// Sort for a deterministic result when two patterns are equally specific
	pats := make([]string, 0, len(policies))
	for p := range policies {
		pats = append(pats, p)
	}
	sort.Strings(pats)
	for _, raw := range pats {
		p, err := parseUsesPattern(raw)
		if err != nil || !p.matches(u) || p.specificity <= bestSpec {
			continue
		}
		if pol, ok := parseUsesPolicy(policies[raw]); ok {
			best, bestSpec = pol, p.specificity
		}
	}
	return best
}

// dockerPolicy returns the policy for Docker images. Patterns describe repositories, so only the
// catch-all "*" pattern applies to an image.
func dockerPolicy(policies map[string]string) usesPolicy {
	if pol, ok := policies["*"]; ok {
		if p, ok := parseUsesPolicy(pol); ok {
			return p
		}
	}
	return usesPolicyHashPin
}

// unpinnedUsesMessage returns the message to report with the "unpinned-uses" ID when the `uses:` value
// does not meet the policy of the configuration, or "" when it does or the rule is disabled. Local
// paths, invalid values and values with an expression are never reported: they are either pinned by
// definition or checked by other rules. workflowCall is true for the `uses:` of a job, which calls a
// reusable workflow, and false for the one of a step, which runs an action.
func unpinnedUsesMessage(cfg *Config, u *UsesRef, workflowCall bool) string {
	if !cfg.RuleEnabled("unpinned-uses") || u.Dynamic {
		return ""
	}
	policies, _ := cfg.ruleOptionStringMap("unpinned-uses", "policies")

	switch u.Kind {
	case UsesAction, UsesReusableWorkflow, UsesInvalid:
		// A value with an empty part such as "owner/repo@" is reported as invalid and as unpinned
		if u.Kind == UsesInvalid && u.Problem != usesProblemEmptyPart {
			return ""
		}
		if usesPolicyFor(policies, u) != usesPolicyHashPin || u.RefKind == RefFullSHA {
			return ""
		}
		if workflowCall {
			return fmt.Sprintf("reusable workflow call %q must be pinned to a full-length commit SHA like \"owner/repo/path/to/workflow.yml@{sha}\" because the \"unpinned-uses\" rule is enabled", u.Raw)
		}
		return fmt.Sprintf("action %q must be pinned to a full-length commit SHA like \"{owner}/{repo}@{sha}\" because the \"unpinned-uses\" rule is enabled", u.Raw)
	case UsesDocker:
		switch dockerPolicy(policies) {
		case usesPolicyAny:
			return ""
		case usesPolicyRefPin:
			if u.Digest != "" || (u.HasTag && u.Tag != "" && u.Tag != "latest") {
				return ""
			}
			return fmt.Sprintf("docker image must be pinned to a tag other than \"latest\" like \"docker://{image}:{tag}\" because the \"unpinned-uses\" rule is enabled: %q", u.Raw)
		default:
			if u.RefKind == RefDigest {
				return ""
			}
			return fmt.Sprintf("docker image must be pinned to a digest like \"docker://{image}@sha256:{digest}\" because the \"unpinned-uses\" rule is enabled: %q", u.Raw)
		}
	}
	return ""
}

// unpinnedUsesOptions are the "policies" option of the unpinned-uses rule.
var unpinnedUsesOptions = []RuleOption{{
	Name: "policies", Kind: RuleOptionStringMap, Default: map[string]string{},
	Summary:  "How strongly to pin the actions matching a pattern: hash-pin (full commit SHA, the default for everything), ref-pin (any tag, branch or SHA) or any. The most specific pattern wins. Patterns are \"*\", \"owner/*\", \"owner/repo\" and \"owner/repo/path\". Docker images follow the \"*\" policy.",
	Validate: validateUsesPolicies,
}}
