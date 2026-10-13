package jactionlint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// renovateConfigFiles are the files Renovate reads its configuration from, relative to the root of
// the repository.
var renovateConfigFiles = []string{
	"renovate.json", "renovate.json5", ".renovaterc", ".renovaterc.json", ".renovaterc.json5",
	".github/renovate.json", ".github/renovate.json5", ".gitlab/renovate.json", ".gitlab/renovate.json5",
}

// RuleDependabotMissingActionsUpdate is a rule to check that a repository which has workflows and
// uses Dependabot also keeps its actions up to date with Dependabot.
//
// It also reports the local composite actions with `uses:` which none of the github-actions updates covers.
type RuleDependabotMissingActionsUpdate struct {
	DependabotRuleBase
	project *Project
	// src is the content of the file for the fix of the second check. It can be nil.
	src *dependabotSource
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
	var updates []*DependabotUpdate
	for _, u := range d.Updates {
		if u.PackageEcosystem != nil && u.PackageEcosystem.Value == "github-actions" {
			updates = append(updates, u)
		}
	}
	if len(updates) > 0 {
		if usesRenovate(r.project.RootDir()) {
			return nil
		}
		// An update which is disabled (open-pull-requests-limit: 0, or an ignore of every dependency) opens no pull request: it
		// updates nothing, in its directory or anywhere
		var active []*DependabotUpdate
		for _, u := range updates {
			if !dependabotUpdatesDisabled(u) {
				active = append(active, u)
			}
		}
		if len(active) > 0 {
			r.checkLocalActions(active)
			return nil
		}
		if where, advice, ok := r.whereActionsAreUsed(false); ok {
			pos := updates[0].Pos
			if pos == nil {
				pos = d.Pos
			}
			r.ReportIDf("dependabot-missing-actions-update", pos, "this repository uses actions %s but every update with the \"github-actions\" package ecosystem is disabled (\"open-pull-requests-limit: 0\", or an \"ignore\" of every dependency), so Dependabot never updates them. enable one of them%s", where, advice)
		}
		return nil
	}
	if usesRenovate(r.project.RootDir()) {
		return nil
	}
	if where, advice, ok := r.whereActionsAreUsed(true); ok {
		directories := `"directory: /"`
		if advice != "" {
			directories = `"directories: [` + advice + `]"`
		}
		r.ReportIDf("dependabot-missing-actions-update", d.Pos, "this repository uses actions %s but no update has the \"github-actions\" package ecosystem, so Dependabot never updates them. add an update with \"package-ecosystem: github-actions\" and %s", where, directories)
	}
	return nil
}

func usesRenovate(root string) bool {
	for _, f := range renovateConfigFiles {
		if s, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err == nil && !s.IsDir() {
			return true
		}
	}
	// Renovate also reads the "renovate" key of package.json
	if b, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg map[string]json.RawMessage
		if json.Unmarshal(b, &pkg) == nil {
			if _, ok := pkg["renovate"]; ok {
				return true
			}
		}
	}
	return false
}

// hasExternalActions reports whether a workflow in the directory uses an action or a reusable
// workflow which Dependabot can update: not a local path and not a docker:// image. The workflows are
// parsed, so a `uses:` in the text of a script does not count and a value on the next line does. A file
// that does not parse is skipped.
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
		w, _ := Parse(b)
		if w == nil {
			continue
		}
		if workflowHasExternalActions(w) {
			return true
		}
	}
	return false
}

func init() {
	registerRules(RuleInfo{
		ID: "dependabot-missing-actions-update", Group: RuleGroupPolicy,
		Summary:      "dependabot.yml has no github-actions update although the repository has workflows using actions, or its github-actions updates do not cover the local composite actions that use actions.",
		DefaultLevel: SeverityWarning, Profile: ProfilePedantic, Fixable: true, DocsAnchor: "check-dependabot-missing-actions-update",
	})
	dependabotRuleFactories = append(dependabotRuleFactories, dependabotRuleFactory{
		kind: "dependabot-missing-actions-update",
		new: func(ctx *dependabotRuleContext) (DependabotRule, error) {
			if !ctx.config.RuleEnabled("dependabot-missing-actions-update") || ctx.project == nil {
				return nil, nil
			}
			r := NewRuleDependabotMissingActionsUpdate(ctx.project)
			if ctx.src != nil {
				r.src = newDependabotSource(ctx.src)
			}
			return r, nil
		},
	})
}

// whereActionsAreUsed says where the repository uses actions that Dependabot can update, for the message ("in its workflows",
// or with the local composite actions), and, when local actions use them, the entries of "directories" that cover the root and those
// actions (the suggestion of the other finding of the rule, see suggestDirectories); ok is false when none does. For the message of
// a disabled update the advice is the suffix ' with "directories: [...]"', for a missing update (entriesOnly) the entries alone.
func (r *RuleDependabotMissingActionsUpdate) whereActionsAreUsed(entriesOnly bool) (where, advice string, ok bool) {
	inWorkflows := hasExternalActions(r.project.WorkflowsDir())
	locals := localActionsWithExternalUses(r.project)
	switch {
	case inWorkflows && len(locals) == 0:
		where = "in its workflows"
	case inWorkflows:
		where = "in its workflows and in the local composite actions in " + showDirectories(locals)
	case len(locals) > 0:
		where = "in the local composite actions in " + showDirectories(locals)
	default:
		return "", "", false
	}
	if len(locals) > 0 {
		advice = strings.Join(suggestDirectories(nil, locals, true), ", ")
		if !entriesOnly {
			advice = " with \"directories: [" + advice + "]\""
		}
	}
	return where, advice, true
}
