package jactionlint

// A job or a step whose "if:" is the literal false never runs. The rules about security and policy have nothing to
// say about what never runs, like zizmor (zizmor#2059): `pip install foo` under `if: false` is not an unpinned
// install. The rules of the correctness group still look at it, because a syntax error in it breaks the workflow
// file whether the step runs or not.

// isStaticallyFalse reports whether the condition is the literal false: `if: false`, `if: ${{ false }}` and
// `if: ${{ !true }}`. An empty condition is true.
func isStaticallyFalse(s *String) bool {
	if s == nil {
		return false
	}
	node, _, _, ok := parseWholeExpr(s.Value)
	if !ok {
		return false
	}
	return evalLiteralBool(node) == condFalse
}

// evalLiteralBool evaluates an expression which is made of boolean literals, "!" and "&&"/"||" only.
func evalLiteralBool(n ExprNode) condTruth {
	switch n := n.(type) {
	case *BoolNode:
		if n.Value {
			return condTrue
		}
		return condFalse
	case *NotOpNode:
		switch evalLiteralBool(n.Operand) {
		case condTrue:
			return condFalse
		case condFalse:
			return condTrue
		}
	case *LogicalOpNode:
		l, r := evalLiteralBool(n.Left), evalLiteralBool(n.Right)
		switch n.Kind {
		case LogicalOpNodeKindAnd:
			if l == condFalse || r == condFalse {
				return condFalse
			}
			if l == condTrue && r == condTrue {
				return condTrue
			}
		case LogicalOpNodeKindOr:
			if l == condTrue || r == condTrue {
				return condTrue
			}
			if l == condFalse && r == condFalse {
				return condFalse
			}
		}
	}
	return condUnknown
}

// unreachableAt reports whether the line belongs to a job or a step which never runs.
func (idx *scopeIndex) unreachableAt(line int) bool {
	if idx == nil {
		return false
	}
	var job *jobRange
	for i := range idx.jobs {
		if j := &idx.jobs[i]; j.start <= line && line <= j.end {
			if job != nil {
				return false // ambiguous: flow style
			}
			job = j
		}
	}
	if job == nil {
		return false
	}
	if job.dead {
		return true
	}
	var step *stepRange
	for i := range job.steps {
		if s := &job.steps[i]; s.start <= line && line <= s.end {
			if step != nil {
				return false
			}
			step = s
		}
	}
	return step != nil && step.dead
}

// dropUnreachable removes the findings of the rules which are not about correctness from the jobs and the steps that
// never run.
func dropUnreachable(errs []*Error, idx *scopeIndex) []*Error {
	if idx == nil {
		return errs
	}
	kept := errs[:0]
	for _, e := range errs {
		if info, ok := ruleIndex[e.ID]; ok && info.Group != RuleGroupCorrectness && e.Line > 0 && idx.unreachableAt(e.Line) {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}
