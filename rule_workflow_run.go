package jactionlint

import (
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

// workflowNames is a set of workflow names defined in a project.
type workflowNames struct {
	// names are lower-cased names so that the check is lenient about letter case.
	names map[string]struct{}
	// ok is false when the set of names could not be determined reliably (a workflow file could
	// not be read or parsed, or a name is dynamic).
	ok bool
}

func readWorkflowNames(dir string, root string) workflowNames {
	var none workflowNames
	entries, err := os.ReadDir(dir)
	if err != nil {
		return none
	}
	names := map[string]struct{}{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			return none
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(b, &doc); err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
			return none
		}
		m := doc.Content[0]
		if m.Kind != yaml.MappingNode {
			return none
		}
		name := ""
		found := false
		for i := 0; i+1 < len(m.Content); i += 2 {
			if strings.EqualFold(m.Content[i].Value, "name") {
				v := m.Content[i+1]
				if v.Kind != yaml.ScalarNode {
					return none
				}
				name, found = v.Value, true
				break
			}
		}
		if !found || name == "" {
			// When no name is given, GitHub uses the file path relative to the repository root
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return none
			}
			name = filepath.ToSlash(rel)
		} else if strings.Contains(name, "${{") {
			return none
		}
		names[strings.ToLower(name)] = struct{}{}
	}
	return workflowNames{names, true}
}

// RuleWorkflowRun is a rule to check workflow names specified at 'workflows' of 'workflow_run' event.
// The names must be the names of workflows existing in the same repository.
type RuleWorkflowRun struct {
	RuleBase
	project  *Project
	siblings *siblingWorkflows // nil reads the workflows of the project for each file
	names    *workflowNames    // Lazily read only when needed
}

// NewRuleWorkflowRun creates a new RuleWorkflowRun instance. This rule is opt-in; it does nothing
// unless "check-workflow-run-names" is enabled in the config. The project parameter can be nil. In
// the case, this rule does nothing.
func NewRuleWorkflowRun(project *Project) *RuleWorkflowRun {
	return &RuleWorkflowRun{
		RuleBase: RuleBase{
			name: "workflow-run",
			desc: "Checks workflow names at \"workflows:\" of \"workflow_run\" event exist in the repository",
		},
		project: project,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleWorkflowRun) VisitWorkflowPre(n *Workflow) error {
	if rule.project == nil || !rule.Config().RuleEnabled("workflow-run-names") {
		return nil
	}
	for _, e := range n.On {
		w, ok := e.(*WebhookEvent)
		if !ok || w.Hook == nil || w.Hook.Value != "workflow_run" {
			continue
		}
		for _, name := range w.Workflows {
			if name == nil || name.Value == "" || name.ContainsExpression() || strings.ContainsAny(name.Value, "*?[]!+") {
				continue // Dynamic or glob pattern
			}
			if rule.names == nil {
				v := rule.siblings.names(rule.project)
				rule.names = &v
			}
			if !rule.names.ok {
				return nil
			}
			if _, found := rule.names.names[strings.ToLower(name.Value)]; !found {
				rule.ReportIDf(
					"workflow-run-names",
					name.Pos,
					"workflow %q specified at \"workflows\" of \"workflow_run\" event is not found in the repository. a workflow is specified by its \"name:\" or its file path when it has no name",
					name.Value,
				)
				if end, ok := name.endPos(); ok {
					rule.errs[len(rule.errs)-1].endAt(end) // the whole name, quotes included
				}
			}
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "workflow-run-names", Group: RuleGroupCorrectness, Summary: "A workflow_run event refers to a workflow which does not exist in the repository.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-workflow-run-names"},
	)
	registerRuleFactory("workflow-run", func(env *RuleEnv) []Rule {
		r := NewRuleWorkflowRun(env.project)
		r.siblings = env.localActions.siblings()
		return []Rule{r}
	})
}
