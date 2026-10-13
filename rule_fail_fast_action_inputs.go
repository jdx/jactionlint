package jactionlint

import (
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v4"
)

// RuleFailFastActionInputs reports a step of an action which does not fail by default when the thing it exists for did
// not happen: `actions/upload-artifact` only warns when its `path` matches no file, `softprops/action-gh-release`
// publishes without the `files` that matched nothing, a test report action passes when it found no test result. The
// build is then green although it is broken, and nobody notices until the artifact or the release is needed. Each
// action has an input that makes it fail (the table of action_fail_fast.go); the finding asks for it.
//
// It is in the pedantic profile: a missing file is sometimes fine, and nothing says which jobs it is for. A step which sets
// the input, with any value, has decided; a step that only runs after a failure or always (`if: failure()`,
// `if: always()`, which upload logs and reports on a best effort basis) and a step with `continue-on-error` are not
// reported.
//
// The fix adds the input to the `with:` of the step, creating it when there is none. It is unsafe: a build that was
// green without the files fails with it, which is the point, but it changes what the workflow does.
type RuleFailFastActionInputs struct {
	RuleBase
	src []byte
	wf  *Workflow
	idx *sourceIndex
	doc *yaml.Node
	bad bool
	// steps maps the position of the `uses` value of each step mapping of the document to the step.
	steps   map[[2]int]stepMapping
	pending []pendingInputFix
}

type pendingInputFix struct {
	err   *Error
	edit  TextEdit
	path  []int
	key   string
	value string
}

// NewRuleFailFastActionInputs creates a new RuleFailFastActionInputs instance. The source of the file is needed to
// attach fixes. It can be nil, in which case findings carry no fix.
func NewRuleFailFastActionInputs(src []byte) *RuleFailFastActionInputs {
	return &RuleFailFastActionInputs{
		RuleBase: NewRuleBase("fail-fast-action-inputs", "Checks that actions which pass by default when nothing was produced or found are told to fail"),
		src:      src,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleFailFastActionInputs) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	return nil
}

// statusFunctionRe matches the status functions of an `if:` condition.
var statusFunctionRe = regexp.MustCompile(`(?i)\b(always|failure|cancelled)\s*\(`)

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleFailFastActionInputs) VisitJobPre(n *Job) error {
	if !rule.Config().RuleEnabled("fail-fast-action-inputs") || isStaticallyFalse(n.If) {
		return nil
	}
	for _, s := range flattenSteps(n.Steps) {
		a, ref := stepAction(s)
		if a == nil || ref.Kind != UsesAction || a.InputsUnknown() || isStaticallyFalse(s.If) {
			continue
		}
		entry, ok := failFastTable[ref.CanonicalName()]
		if !ok || hasInput(a, entry.Name) || (entry.Relevant != nil && !entry.Relevant(a)) {
			continue
		}
		if s.ContinueOnError != nil && (s.ContinueOnError.Expression != nil || s.ContinueOnError.Value) {
			continue
		}
		if s.If != nil && statusFunctionRe.MatchString(s.If.Value) {
			continue
		}
		if entry.NoInput != nil && entry.NoInput(ref, rule.comment(a)) {
			continue
		}
		rule.ReportIDf("fail-fast-action-inputs", a.Uses.Pos,
			"%s does not fail by default: %s. set %q (the default is %q)",
			ref.CanonicalName(), entry.Problem, entry.Name+": "+entry.Value, entry.Default)
		rule.attachFix(a, entry)
	}
	return nil
}

// comment returns the comment at the end of the line of `uses:`.
func (rule *RuleFailFastActionInputs) comment(a *ExecAction) string {
	if rule.wf == nil || rule.wf.Comments == nil || a.Uses.Pos == nil {
		return ""
	}
	if c := rule.wf.Comments.Inline(a.Uses.Pos.Line); c != nil {
		return c.Text
	}
	return ""
}

var failFastNoFix = &NoFix{Code: NoFixUnsupportedShape, Reason: "the step is not written in a way the fix can edit exactly, add the input under with: by hand"}

func (rule *RuleFailFastActionInputs) attachFix(a *ExecAction, entry failFastInput) {
	if rule.src == nil {
		return
	}
	e := rule.errs[len(rule.errs)-1]
	if rule.idx == nil {
		rule.idx = newSourceIndex(rule.src)
	}
	if rule.doc == nil && !rule.bad {
		var doc yaml.Node
		if err := yaml.Unmarshal(rule.src, &doc); err != nil {
			rule.bad = true
		} else {
			rule.doc = &doc
		}
	}
	if rule.bad {
		e.NoFix = failFastNoFix
		return
	}
	if rule.steps == nil {
		rule.steps = map[[2]int]stepMapping{}
		indexStepMappings(rule.doc, nil, rule.steps)
	}
	edit, path, ok := withInputEdit(rule.steps, rule.src, rule.idx, a, entry.Name, entry.Value)
	if !ok {
		e.NoFix = failFastNoFix
		return
	}
	// Unsafe: a build which is green without the files is red with the input. That is the point of the finding, but
	// it changes what the workflow does.
	e.Fix = &Fix{
		Description: fmt.Sprintf("Set %s: %s", entry.Name, entry.Value),
		Unsafe:      true,
		Edits:       []TextEdit{edit},
	}
	rule.pending = append(rule.pending, pendingInputFix{err: e, edit: edit, path: path, key: entry.Name, value: entry.Value})
}

// VisitWorkflowPost is callback when visiting Workflow node after visiting its children. It verifies the fixes: the
// file must still be YAML with the input in the same step.
func (rule *RuleFailFastActionInputs) VisitWorkflowPost(n *Workflow) error {
	pending := rule.pending
	rule.pending = nil
	if len(pending) == 0 {
		return nil
	}
	var check func(set []pendingInputFix)
	check = func(set []pendingInputFix) {
		if inputFixesHold(rule.src, set) {
			return
		}
		if len(set) == 1 {
			set[0].err.Fix = nil
			set[0].err.NoFix = failFastNoFix
			return
		}
		check(set[:len(set)/2])
		check(set[len(set)/2:])
	}
	check(pending)
	return nil
}

// inputFixesHold applies the edits of the fixes together and reports whether the result is YAML in which every step
// has its input.
func inputFixesHold(src []byte, fixes []pendingInputFix) bool {
	edits := make([]TextEdit, len(fixes))
	for i, f := range fixes {
		edits[i] = f.edit
	}
	var ndoc yaml.Node
	if err := yaml.Unmarshal(applyEdits(src, edits), &ndoc); err != nil {
		return false
	}
	for _, f := range fixes {
		nstep := followPath(&ndoc, f.path)
		if nstep == nil {
			return false
		}
		_, nwith := mappingEntry(nstep, "with")
		if nwith == nil || nwith.Kind != yaml.MappingNode {
			return false
		}
		_, nv := mappingEntry(nwith, f.key)
		if nv == nil || nv.Kind != yaml.ScalarNode || nv.Value != f.value {
			return false
		}
	}
	return true
}

// withInputEdit computes the edit which adds the input `key: value` to the `with:` of the step of the action, creating
// the mapping when the step has none, with the path to the step in the document. It returns false when the YAML is
// written in a way that cannot be edited with certainty. The caller verifies the edited file (inputFixesHold).
func withInputEdit(steps map[[2]int]stepMapping, src []byte, idx *sourceIndex, a *ExecAction, key, value string) (TextEdit, []int, bool) {
	if a.Uses == nil || a.Uses.Pos == nil || !idx.valid {
		return TextEdit{}, nil, false
	}
	found, ok := steps[[2]int{a.Uses.Pos.Line, a.Uses.Pos.Col}]
	step, path := found.node, found.path
	if !ok || step == nil {
		return TextEdit{}, nil, false
	}
	usesKey, usesVal := mappingEntry(step, "uses")
	if usesKey == nil {
		return TextEdit{}, nil, false
	}
	entry := key + ": " + value
	if step.Style&yaml.FlowStyle != 0 {
		withKey, withVal := mappingEntry(step, "with")
		var edit TextEdit
		var ok bool
		switch {
		case withKey == nil:
			edit, ok = persistCredentialsFlowEdit(src, idx, step, "with: {"+entry+"}")
		case withVal.Kind == yaml.MappingNode && withVal.Style&yaml.FlowStyle != 0:
			edit, ok = persistCredentialsFlowEdit(src, idx, withVal, entry)
		}
		return edit, path, ok
	}
	nl := idx.newline()
	withKey, withVal := mappingEntry(step, "with")

	var edit TextEdit
	switch {
	case withKey == nil:
		if usesVal.Line != usesKey.Line || strings.Contains(usesVal.Value, "\n") {
			return TextEdit{}, nil, false
		}
		at := idx.lineEnd(usesVal.Line)
		pad := strings.Repeat(" ", usesKey.Column-1)
		edit = TextEdit{Start: at, End: at, NewText: nl + pad + "with:" + nl + pad + "  " + entry}
	case withVal.Kind == yaml.ScalarNode && withVal.Tag == "!!null" && withVal.Value == "":
		if withVal.Line != withKey.Line {
			return TextEdit{}, nil, false
		}
		at := idx.lineEnd(withKey.Line)
		pad := strings.Repeat(" ", withKey.Column-1+2)
		edit = TextEdit{Start: at, End: at, NewText: nl + pad + entry}
	case withVal.Kind == yaml.MappingNode && withVal.Style&yaml.FlowStyle == 0 && len(withVal.Content) > 0:
		first := withVal.Content[0]
		if first.Line == withKey.Line {
			return TextEdit{}, nil, false
		}
		if k, _ := mappingEntry(withVal, key); k != nil {
			return TextEdit{}, nil, false
		}
		// Insert before the first entry but after the `with:` line, so that comments above the first entry stay
		// with it.
		if withKey.Line+1 > len(idx.lineStarts) {
			return TextEdit{}, nil, false
		}
		at := idx.lineStarts[withKey.Line]
		pad := strings.Repeat(" ", first.Column-1)
		edit = TextEdit{Start: at, End: at, NewText: pad + entry + nl}
	case withVal.Kind == yaml.MappingNode && withVal.Style&yaml.FlowStyle != 0:
		var ok bool
		if edit, ok = persistCredentialsFlowEdit(src, idx, withVal, entry); !ok {
			return TextEdit{}, nil, false
		}
	default:
		return TextEdit{}, nil, false
	}
	return edit, path, true
}

func init() {
	registerRules(
		RuleInfo{
			ID: "fail-fast-action-inputs", Group: RuleGroupCorrectness,
			Summary:      "An action which passes by default when nothing was produced or found is not told to fail.",
			DefaultLevel: SeverityWarning, Profile: ProfilePedantic, Fixable: true, DocsAnchor: "check-fail-fast-action-inputs",
		},
	)
	registerRuleFactory("fail-fast-action-inputs", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleFailFastActionInputs(env.Source())}
	})
}
