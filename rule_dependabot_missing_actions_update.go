package jactionlint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// renovateConfigFiles are the files Renovate reads its configuration from, relative to the root of
// the repository.
var renovateConfigFiles = []string{
	"renovate.json", "renovate.json5", ".renovaterc", ".renovaterc.json", ".renovaterc.json5",
	".github/renovate.json", ".github/renovate.json5", ".gitlab/renovate.json", ".gitlab/renovate.json5",
}

// externalUsesPattern matches a "uses:" key (of a step or a job) and captures its value.
var externalUsesPattern = regexp.MustCompile(`(?m)^[ \t]*(?:-[ \t]+)?uses[ \t]*:[ \t]*["']?([^\s"'#]+)`)

// RuleDependabotMissingActionsUpdate is a rule to check that a repository which has workflows and
// uses Dependabot also keeps its actions up to date with Dependabot.
type RuleDependabotMissingActionsUpdate struct {
	DependabotRuleBase
	project *Project
}

// NewRuleDependabotMissingActionsUpdate creates a new RuleDependabotMissingActionsUpdate instance.
// project is the repository which has the Dependabot configuration. It can be nil, in which case the
// rule does nothing.
func NewRuleDependabotMissingActionsUpdate(project *Project) *RuleDependabotMissingActionsUpdate {
	return &RuleDependabotMissingActionsUpdate{
		DependabotRuleBase: NewDependabotRuleBase("dependabot-missing-actions-update", "Checks that dependabot.yml updates github-actions when the repository has workflows"),
		project:            project,
	}
}

// VisitDependabotPost is callback when visiting the root node after visiting its children.
func (r *RuleDependabotMissingActionsUpdate) VisitDependabotPost(d *Dependabot) error {
	if r.project == nil || d.Pos == nil {
		return nil
	}
	for _, u := range d.Updates {
		if u.PackageEcosystem != nil && u.PackageEcosystem.Value == "github-actions" {
			return nil
		}
	}
	if usesRenovate(r.project.RootDir()) || !hasExternalActions(r.project.WorkflowsDir()) {
		return nil
	}
	r.ReportID("dependabot-missing-actions-update", d.Pos, "this repository uses actions in its workflows but no update has the \"github-actions\" package ecosystem, so Dependabot never updates them. add an update with \"package-ecosystem: github-actions\" and \"directory: /\"")
	return nil
}

func usesRenovate(root string) bool {
	for _, f := range renovateConfigFiles {
		if s, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err == nil && !s.IsDir() {
			return true
		}
	}
	return false
}

// hasExternalActions reports whether a workflow in the directory uses an action or a reusable
// workflow which Dependabot can update: not a local path and not a docker:// image.
func hasExternalActions(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !(strings.HasSuffix(n, ".yml") || strings.HasSuffix(n, ".yaml")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		for _, m := range externalUsesPattern.FindAllSubmatch(b, -1) {
			v := string(m[1])
			if !strings.HasPrefix(v, "./") && !strings.HasPrefix(v, "docker://") && !strings.HasPrefix(v, "${{") {
				return true
			}
		}
	}
	return false
}

func init() {
	registerRules(RuleInfo{
		ID: "dependabot-missing-actions-update", Group: RuleGroupPolicy,
		Summary:      "dependabot.yml has no github-actions update although the repository has workflows using actions.",
		DefaultLevel: SeverityWarning, Profile: ProfileStrict, DocsAnchor: "check-dependabot-missing-actions-update",
	})
	dependabotRuleFactories = append(dependabotRuleFactories, dependabotRuleFactory{
		kind: "dependabot-missing-actions-update",
		new: func(ctx *dependabotRuleContext) (DependabotRule, error) {
			if !ctx.config.RuleEnabled("dependabot-missing-actions-update") || ctx.project == nil {
				return nil, nil
			}
			return NewRuleDependabotMissingActionsUpdate(ctx.project), nil
		},
	})
}
