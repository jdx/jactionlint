// Package actionlint provides linting functionality for GitHub Actions workflows.
package actionlint

import (
	"slices"
	"strconv"
	"strings"
)

// RequiredActionRule represents a requirement that a specific GitHub Action (or reusable workflow)
// is used in workflows, with an optional version constraint.
type RequiredActionRule struct {
	// Action is the name of the required action without version (e.g. "actions/checkout"). Sub-paths
	// are part of the name (e.g. "github/codeql-action/init"). The comparison is case-insensitive.
	Action string `yaml:"action"`
	// Version is the optional required ref (e.g. "v4" or a full commit SHA). It is compared exactly.
	// When empty, any version is accepted.
	Version string `yaml:"version"`
}

// RuleRequiredActions implements a linting rule that checks for the presence and version
// of required GitHub Actions within workflows. It is enabled only when "required-actions" is
// configured in the config file.
type RuleRequiredActions struct {
	RuleBase
	required []RequiredActionRule
}

// NewRuleRequiredActions creates a new instance of RuleRequiredActions with the specified
// required actions. Returns nil if no required actions are provided.
func NewRuleRequiredActions(required []RequiredActionRule) *RuleRequiredActions {
	if len(required) == 0 {
		return nil
	}
	return &RuleRequiredActions{
		RuleBase: RuleBase{
			name: "required-actions",
			desc: "Checks that required GitHub Actions are used in workflows",
		},
		required: required,
	}
}

// VisitWorkflowPre analyzes the workflow to ensure all required actions are present
// with correct versions. It reports errors for missing or mismatched versions.
func (rule *RuleRequiredActions) VisitWorkflowPre(workflow *Workflow) error {
	if workflow == nil {
		return nil
	}

	// Collect all versions used for each (lower-cased) action name. Map iteration order of jobs is
	// random so the error position is the earliest job to be deterministic.
	found := map[string][]string{}
	var pos *Pos
	add := func(uses *String) {
		if uses == nil {
			return
		}
		if name, ver := parseActionRef(uses.Value); name != "" {
			k := strings.ToLower(name)
			found[k] = append(found[k], ver)
		}
	}
	for _, job := range workflow.Jobs {
		if job == nil {
			continue
		}
		if job.Pos != nil && (pos == nil || job.Pos.IsBefore(pos)) {
			pos = job.Pos
		}
		if job.WorkflowCall != nil {
			add(job.WorkflowCall.Uses)
		}
		for _, step := range job.Steps {
			if step == nil {
				continue
			}
			if exec, ok := step.Exec.(*ExecAction); ok && exec != nil {
				add(exec.Uses)
			}
		}
	}
	if pos == nil {
		pos = &Pos{Line: 1, Col: 1}
	}

	for _, req := range rule.required {
		vers, ok := found[strings.ToLower(req.Action)]
		if !ok {
			if req.Version == "" {
				rule.Errorf(pos, "required action %q is not used in this workflow", req.Action)
			} else {
				rule.Errorf(pos, "required action %q (version %q) is not used in this workflow", req.Action, req.Version)
			}
			continue
		}
		if req.Version == "" || slices.Contains(vers, req.Version) {
			continue
		}
		rule.Errorf(pos, "action %q must use version %q but found %s", req.Action, req.Version, quoteJoin(vers))
	}

	return nil
}

func quoteJoin(vs []string) string {
	seen := map[string]struct{}{}
	var qs []string
	for _, v := range vs {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		qs = append(qs, strconv.Quote(v))
	}
	slices.Sort(qs)
	return strings.Join(qs, ", ")
}

// parseActionRef extracts the action name and version from a GitHub Action reference.
// Returns empty strings for local actions, Docker images or malformed strings.
// Example: "actions/checkout@v3" returns ("actions/checkout", "v3")
func parseActionRef(uses string) (name string, version string) {
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "docker://") {
		return "", ""
	}
	name, version, ok := strings.Cut(uses, "@")
	if !ok || name == "" || version == "" || !strings.Contains(name, "/") || strings.HasPrefix(name, "/") {
		return "", ""
	}
	return name, version
}
