package jactionlint

import (
	"fmt"
	"strings"
)

// RuleActionInputDependencies reports inputs of well-known actions which only work together, from the table of
// action_input_dependencies.go: an input that the action ignores without another one (`run-id` of
// actions/download-artifact needs `github-token`), a group of which exactly one input is needed
// (`timeout_minutes` or `timeout_seconds` of nick-fields/retry), and inputs which exclude each other.
//
// The first two are reported as missing-action-input, which is what actionlint calls an input the action needs,
// the third as conflicting-action-inputs. The data set of popular actions does not know these rules, and the rule
// does not need it: the table has its own versions. A step whose `with:` is an expression or has an `${{ insert }}`
// key is not judged, and two inputs of a conflict are reported only when both are written out: an expression may
// be empty at run time, which is how a workflow chooses between two inputs.
type RuleActionInputDependencies struct {
	RuleBase
	wf *Workflow
}

// NewRuleActionInputDependencies creates a new RuleActionInputDependencies instance.
func NewRuleActionInputDependencies() *RuleActionInputDependencies {
	return &RuleActionInputDependencies{
		RuleBase: NewRuleBase("action-input-dependencies", "Checks inputs of well-known actions which only work together"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleActionInputDependencies) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleActionInputDependencies) VisitStep(n *Step) error {
	a, ref := stepAction(n)
	if a == nil || ref.Kind != UsesAction || a.InputsUnknown() {
		return nil
	}
	var comments *CommentIndex
	if rule.wf != nil {
		comments = rule.wf.Comments
	}
	ps, ok := resolvePopularSpec(ref, a.Uses, comments, knownInputDependencies)
	if !ok {
		return nil
	}
	entry := inputDependencyTable[ref.CanonicalName()]
	meta := PopularActions[ps.Key] // nil when the data set does not have the action
	for _, r := range entry.Rules {
		if r.Skip != nil && r.Skip(a) {
			continue
		}
		if meta != nil && !metaHasInputs(meta, r.Inputs) {
			continue // the version of the data set does not have the inputs
		}
		rule.check(a, ps.describe(a.Uses.Value), r)
	}
	return nil
}

// metaHasInputs reports whether the action defines all the inputs.
func metaHasInputs(meta *ActionMetadata, inputs []string) bool {
	for _, i := range inputs {
		if _, ok := meta.Inputs[i]; !ok {
			return false
		}
	}
	return true
}

// inputGiven reports whether the step gives the input a value; written reports that the value is written out (not an
// expression, not empty).
func inputGiven(a *ExecAction, name string) (in *Input, given, written bool) {
	in, ok := a.Inputs[name]
	if !ok || in == nil || in.Value == nil {
		return nil, false, false
	}
	v := strings.TrimSpace(in.Value.Value)
	return in, v != "", v != "" && !in.Value.ContainsExpression()
}

func (rule *RuleActionInputDependencies) check(a *ExecAction, what string, r inputDependency) {
	switch r.Kind {
	case inputRequires:
		in, g, _ := inputGiven(a, r.Inputs[0])
		if !g {
			return
		}
		for _, need := range r.Inputs[1:] {
			if _, ok := a.Inputs[need]; ok {
				return
			}
		}
		rule.errorIDAt("missing-action-input", in.Name.Pos, fmt.Sprintf(
			"input %q of action %s needs %s, without it %s. add %s",
			in.Name.Value, what, quoteOr(r.Inputs[1:]), r.Effect, quoteOr(r.Inputs[1:])))
	case inputExactlyOne, inputExclusive:
		var first *Input
		var conflict *Input
		anyGiven := false
		for _, name := range r.Inputs {
			in, g, w := inputGiven(a, name)
			if _, present := a.Inputs[name]; present {
				anyGiven = true
			}
			if !g || !w {
				continue
			}
			if first == nil {
				first = in
			} else if conflict == nil {
				conflict = in
			}
		}
		if conflict != nil {
			rule.errorIDAt("conflicting-action-inputs", conflict.Name.Pos, fmt.Sprintf(
				"inputs %q and %q of action %s exclude each other, %s. give only one of %s",
				first.Name.Value, conflict.Name.Value, what, r.Effect, quoteAnd(r.Inputs)))
			return
		}
		if r.Kind == inputExactlyOne && !anyGiven {
			rule.errorIDAt("missing-action-input", a.Uses.Pos, fmt.Sprintf(
				"missing input: action %s needs %s, %s. give one of them",
				what, quoteOr(r.Inputs), r.Effect))
		}
	}
}

func quoteOr(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, " or ")
}

func quoteAnd(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, " and ")
}

func init() {
	registerRules(
		RuleInfo{
			ID: "conflicting-action-inputs", Group: RuleGroupCorrectness,
			Summary:      "Two inputs of a well-known action exclude each other.",
			DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-action-input-dependencies",
		},
	)
	registerRuleFactory("action-input-dependencies", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleActionInputDependencies()}
	})
}
