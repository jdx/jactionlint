package jactionlint

import "strings"

// RuleParallelSteps is a rule to check parallel steps: a 'wait' or 'cancel' step must refer to the ID
// of a preceding background step, and a 'parallel' group may only contain 'run' and 'uses' steps
// ('wait', 'wait-all', 'cancel', and nested 'parallel' steps are not allowed in it; 'background: true'
// is redundant there but accepted by GitHub, so it is not reported).
// https://github.blog/changelog/2026-06-25-actions-steps-can-now-be-run-in-parallel/
type RuleParallelSteps struct {
	RuleBase
	// background holds the IDs (lower-cased; step IDs are case insensitive) of the 'background: true'
	// steps seen so far in the current job. Steps are visited in order, including steps nested in
	// 'parallel:' groups, so a 'wait'/'cancel' reference found in this set both exists and precedes
	// the reference.
	background map[string]struct{}
	// inParallel holds the steps that are direct children of a 'parallel' group. Such steps are not
	// allowed there at all (only 'run'/'uses' are), so their 'wait'/'cancel' references are not
	// separately validated: the "not allowed inside a parallel group" error stands on its own
	// instead of cascading into a redundant "unknown reference" error from the same step.
	inParallel map[*Step]struct{}
	// pending holds the IDs (lower-cased) of the literal 'background: true' steps which no 'wait' or
	// 'wait-all' step has covered yet. Their outputs and results are not available until then.
	pending map[string]struct{}
}

// NewRuleParallelSteps creates a new RuleParallelSteps instance.
func NewRuleParallelSteps() *RuleParallelSteps {
	return &RuleParallelSteps{
		RuleBase: RuleBase{
			name: "parallel-steps",
			desc: "Checks \"wait\"/\"cancel\" references to background steps and steps forbidden inside a \"parallel\" group",
		},
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleParallelSteps) VisitJobPre(n *Job) error {
	rule.background = map[string]struct{}{}
	rule.inParallel = map[*Step]struct{}{}
	rule.pending = map[string]struct{}{}
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleParallelSteps) VisitJobPost(n *Job) error {
	rule.background = nil
	rule.inParallel = nil
	rule.pending = nil
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleParallelSteps) VisitStep(n *Step) error {
	// A step that is itself inside a 'parallel' group is already reported by checkParallelChildren as
	// not allowed there, so skip its reference check to avoid a second, redundant error.
	_, insideParallel := rule.inParallel[n]
	rule.checkPendingReads(n)
	switch e := n.Exec.(type) {
	case *ExecWait:
		if !insideParallel {
			for _, name := range e.Names {
				rule.checkRef(name)
			}
		}
		if e.All {
			clear(rule.pending)
		}
		for _, name := range e.Names {
			if name != nil {
				delete(rule.pending, strings.ToLower(name.Value))
			}
		}
	case *ExecCancel:
		if !insideParallel {
			rule.checkRef(e.Name)
		}
	case *ExecParallel:
		rule.checkParallelChildren(e.Steps)
	}

	if n.ID != nil && !n.ID.ContainsExpression() && isBackgroundStep(n) {
		rule.background[strings.ToLower(n.ID.Value)] = struct{}{}
		if n.Background.Expression == nil {
			rule.pending[strings.ToLower(n.ID.Value)] = struct{}{}
		}
	}

	return nil
}

// checkPendingReads reports the expressions of the step which read the outputs, the outcome or the
// conclusion of a background step before a 'wait' or 'wait-all' step covers it. They evaluate to an
// empty string at runtime.
func (rule *RuleParallelSteps) checkPendingReads(n *Step) {
	if len(rule.pending) == 0 {
		return
	}
	check := func(s *String, cond bool) {
		scanExpressions(s, cond, func(o *exprOccurrence) {
			VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
				if entering {
					return
				}
				var recv ExprNode
				var id string
				switch d := node.(type) {
				case *ObjectDerefNode:
					recv, id = d.Receiver, d.Property
				case *IndexAccessNode:
					// steps['my-step'] is the only way to read a step ID with a hyphen
					lit, ok := d.Index.(*StringNode)
					if !ok {
						return
					}
					recv, id = d.Operand, strings.ToLower(lit.Value)
				default:
					return
				}
				v, ok := recv.(*VariableNode)
				if !ok || v.Name != "steps" {
					return
				}
				if _, ok := rule.pending[id]; ok {
					rule.ReportIDf("background-step-not-waited", o.PosOf(node), "outputs and results of the background step %q are not available until a \"wait\" or \"wait-all\" step covers it, so this evaluates to an empty string", id)
				}
			})
		})
	}
	check(n.If, true)
	check(n.Name, false)
	if n.Env != nil {
		check(n.Env.Expression, false)
		for _, v := range n.Env.Vars {
			check(v.Value, false)
		}
	}
	if n.ContinueOnError != nil {
		check(n.ContinueOnError.Expression, false)
	}
	if n.TimeoutMinutes != nil {
		check(n.TimeoutMinutes.Expression, false)
	}
	if n.Background != nil {
		check(n.Background.Expression, false)
	}
	switch e := n.Exec.(type) {
	case *ExecRun:
		check(e.Run, false)
		check(e.Shell, false)
		check(e.WorkingDirectory, false)
	case *ExecAction:
		check(e.Uses, false)
		check(e.Entrypoint, false)
		check(e.Args, false)
		for _, in := range e.Inputs {
			if in != nil {
				check(in.Value, false)
			}
		}
	}
}

// checkParallelChildren checks the steps inside a 'parallel' group. Only 'run' and 'uses' steps are
// allowed there: 'wait', 'wait-all', 'cancel', and nested 'parallel' steps are not, because steps in a
// 'parallel' group already run in the background with an implicit wait at the end. 'background: true'
// is redundant there but accepted by GitHub, so it is not reported.
func (rule *RuleParallelSteps) checkParallelChildren(steps []*Step) {
	for _, s := range steps {
		rule.inParallel[s] = struct{}{}
		switch e := s.Exec.(type) {
		case *ExecWait:
			kind := "wait"
			if e.All {
				kind = "wait-all"
			}
			rule.ReportIDf("invalid-parallel-step", s.Pos, "%q step is not allowed inside a \"parallel\" group", kind)
		case *ExecCancel:
			rule.ReportIDf("invalid-parallel-step", s.Pos, "\"cancel\" step is not allowed inside a \"parallel\" group")
		case *ExecParallel:
			rule.ReportIDf("invalid-parallel-step", s.Pos, "\"parallel\" step cannot be nested in another \"parallel\" step")
		}
	}
}

func (rule *RuleParallelSteps) checkRef(ref *String) {
	if ref == nil || ref.Value == "" || ref.ContainsExpression() {
		// Empty values come from an already-reported parse error; expression step IDs can't be
		// resolved statically. Skip both.
		return
	}
	if _, ok := rule.background[strings.ToLower(ref.Value)]; !ok {
		rule.ReportIDf(
			"invalid-parallel-step",
			ref.Pos,
			"%q is not the ID of a preceding background step. \"wait\" and \"cancel\" steps can only refer to an earlier step that has \"background: true\"",
			ref.Value,
		)
	}
}

// isBackgroundStep reports whether the step runs (or may run) in the background. When 'background' is
// an expression, whether the step runs in the background can't be known statically, so it's treated
// as a possible background step to avoid false positives.
func isBackgroundStep(s *Step) bool {
	return s.Background != nil && (s.Background.Expression != nil || s.Background.Value)
}

func init() {
	registerRules(
		RuleInfo{ID: "invalid-parallel-step", Group: RuleGroupCorrectness, Summary: "A step is not allowed inside a parallel group or refers to a wrong step.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-parallel-step-refs"},
		RuleInfo{ID: "background-step-not-waited", Group: RuleGroupCorrectness, Summary: "Outputs or results of a background step are read before a wait step covers it.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-background-step-not-waited"},
	)
	registerRuleFactory("parallel-steps", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleParallelSteps()}
	})
}
