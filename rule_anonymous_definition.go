package jactionlint

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
)

// RuleAnonymousDefinition is a rule to detect workflows and jobs without a "name:". GitHub renders
// such a workflow with its file path in the UI, which makes it hard to tell which definition runs.
// https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#name
type RuleAnonymousDefinition struct {
	RuleBase
	path  string
	src   []byte
	lines *lineIndex // the lines of src, built when a fix needs them
}

// NewRuleAnonymousDefinition creates a new RuleAnonymousDefinition instance. The path and the source
// of the file are used to put a fix on the error. They can be empty.
func NewRuleAnonymousDefinition(path string, src []byte) *RuleAnonymousDefinition {
	return &RuleAnonymousDefinition{
		RuleBase: RuleBase{
			name: "anonymous-definition",
			desc: "Checks that workflows and jobs have a \"name:\"",
		},
		path: path,
		src:  src,
	}
}

// isCopilotSetupSteps reports whether the path is the workflow that prepares the environment of the Copilot coding agent.
// GitHub runs it when the agent starts, whatever its triggers are, from a job with a fixed ID, and accepts few keys in
// it. A name tells nothing there and a concurrency group has no use (zizmor#1481).
func isCopilotSetupSteps(path string) bool {
	switch strings.ToLower(filepath.Base(filepath.ToSlash(path))) {
	case "copilot-setup-steps.yml", "copilot-setup-steps.yaml":
		return true
	}
	return false
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleAnonymousDefinition) VisitWorkflowPre(n *Workflow) error {
	if n.Name != nil || isCopilotSetupSteps(rule.path) {
		return nil
	}

	const msg = "workflow has no \"name:\", so GitHub shows its file path in the Actions UI. add a top-level \"name:\" to make the workflow recognizable"
	line, ok := rule.firstKeyLine()
	if !ok {
		rule.ReportID("anonymous-definition", &Pos{Line: 1, Col: 1}, msg)
		return nil
	}
	pos := &Pos{Line: line, Col: 1}

	name := rule.deriveName(n)
	if name == "" {
		rule.ReportID("anonymous-definition", pos, msg)
		return nil
	}
	at := rule.lineIndex().startOf(line)
	text := "name: " + RenderYAMLValue(name) + srcLineBreak(rule.src)
	reportWithFix(&rule.RuleBase, "anonymous-definition", pos, msg, &Fix{
		Description: "Add name: " + name,
		Edits:       []TextEdit{{Start: at, End: at, NewText: text}},
	})
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleAnonymousDefinition) VisitJobPre(n *Job) error {
	if n.Name != nil || n.WorkflowCall != nil || n.ID == nil || n.ID.Pos == nil || isCopilotSetupSteps(rule.path) {
		// A job which calls a reusable workflow is shown as "caller / called job" and is not reported
		return nil
	}
	msg := "job " + strconv.Quote(n.ID.Value) + " has no \"name:\", so the Actions UI shows only its ID. add a \"name:\" to describe what the job does"
	pos := n.ID.Pos
	if edit, ok := rule.jobNameEdit(n); ok {
		reportWithFix(&rule.RuleBase, "anonymous-definition", pos, msg, &Fix{
			Description: "Add name: " + n.ID.Value + " to job " + n.ID.Value,
			Edits:       []TextEdit{edit},
		})
		return nil
	}
	rule.ReportID("anonymous-definition", pos, msg)
	return nil
}

// jobNameEdit makes the edit which puts "name: {id}" as the first key of the job. The job must be a
// block mapping which starts on the line after its key. GitHub shows the ID of a job without a name,
// so the fix does not change how the job is displayed.
func (rule *RuleAnonymousDefinition) jobNameEdit(n *Job) (TextEdit, bool) {
	src := rule.src
	start := rule.lineIndex().startOf(n.ID.Pos.Line)
	if len(src) == 0 || start < 0 {
		return TextEdit{}, false
	}
	end := srcLineEnd(src, start)
	line := string(src[start:end])
	trimmed := strings.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	if indent == 0 || n.ID.Pos.Col != indent+1 {
		return TextEdit{}, false
	}
	// "id:" with an optional comment and nothing else
	key := strings.TrimRight(trimmed, " \t")
	if c := strings.Index(key, " #"); c >= 0 {
		key = strings.TrimRight(key[:c], " \t")
	}
	if key != n.ID.Value+":" && key != strconv.Quote(n.ID.Value)+":" && key != "'"+n.ID.Value+"':" {
		return TextEdit{}, false
	}
	// The indentation of the first child
	for off := nextLineStart(src, end); off >= 0; {
		e := srcLineEnd(src, off)
		l := string(src[off:e])
		t := strings.TrimLeft(l, " ")
		if t == "" || strings.HasPrefix(t, "#") {
			off = nextLineStart(src, e)
			continue
		}
		childIndent := len(l) - len(t)
		if childIndent <= indent {
			return TextEdit{}, false
		}
		at := nextLineStart(src, end)
		return TextEdit{Start: at, End: at, NewText: strings.Repeat(" ", childIndent) + "name: " + RenderYAMLValue(n.ID.Value) + srcLineBreak(src)}, true
	}
	return TextEdit{}, false
}

// firstKeyLine returns the line of the first key of the top-level mapping. The fix puts the name
// there. It returns false when the mapping is not written in the block style at the first column.
func (rule *RuleAnonymousDefinition) firstKeyLine() (int, bool) {
	return firstKeyLine(rule.src)
}

// firstKeyLine returns the 1-based line of the first key of the top-level mapping of a workflow
// file. It returns false when the source is empty or the mapping is not written in the block style
// at the first column.
func firstKeyLine(src []byte) (int, bool) {
	if len(src) == 0 {
		return 0, false
	}
	lines := bytes.Split(src, []byte("\n"))
	for i, l := range lines {
		t := bytes.TrimRight(l, "\r \t")
		switch {
		case len(t) == 0, t[0] == '#', bytes.Equal(t, []byte("---")), t[0] == '%':
			continue
		case t[0] == ' ' || t[0] == '\t' || t[0] == '{' || t[0] == '[' || t[0] == '-' || t[0] == '&' || t[0] == '*' || t[0] == '!':
			return 0, false
		}
		return i + 1, true
	}
	return 0, false
}

// deriveName makes a name for the workflow from its file name. When the file name is not known (for
// example the source comes from stdin) the ID of the only job is used instead.
func (rule *RuleAnonymousDefinition) deriveName(n *Workflow) string {
	if base := filepath.Base(filepath.ToSlash(rule.path)); rule.path != "" && !strings.HasPrefix(base, "<") {
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		if stem != "" && stem != "." {
			return stem
		}
	}
	if len(n.Jobs) == 1 {
		for _, j := range n.Jobs {
			if j != nil && j.ID != nil {
				return j.ID.Value
			}
		}
	}
	return ""
}

func init() {
	registerRules(
		RuleInfo{ID: "anonymous-definition", Group: RuleGroupPolicy, Summary: "A workflow has no top-level name:.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, Fixable: true, DocsAnchor: "check-anonymous-definition"},
	)
	registerRuleFactory("anonymous-definition", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("anonymous-definition") {
			return nil
		}
		return []Rule{NewRuleAnonymousDefinition(env.path, env.src)}
	})
}

// lineIndex returns the lines of the source of the file, which are found once for all the fixes.
func (rule *RuleAnonymousDefinition) lineIndex() *lineIndex {
	if rule.lines == nil {
		rule.lines = newLineIndex(rule.src)
	}
	return rule.lines
}
