package jactionlint

import "strings"

// A job or a step of a workflow with several events often says which of them it is for: `if: github.event_name ==
// 'workflow_dispatch'`. A condition that cannot be true for an event never runs for it, so the privileged events that
// are left are the ones the rule has to judge.

// eventsAfterCondition returns the events of the list that the condition may be true for. A condition is evaluated
// with the name of the event only (evalGate, as for the release triggers of cache-poisoning): what it does not know
// keeps the event, and so does an expression that cannot be parsed.
func eventsAfterCondition(events []string, cond *String) []string {
	if cond == nil || len(events) == 0 {
		return events
	}
	exprs, ok := conditionExprs(cond)
	if !ok || len(exprs) != 1 {
		return events
	}
	var ret []string
	for _, e := range events {
		if evalGate(exprs[0], triggerScenario{"event_name": e}).truth() != triFalse {
			ret = append(ret, e)
		}
	}
	return ret
}

// stepRunsAfterFailure reports whether the step can run after an earlier step failed, so that a permission check that failed
// does not hold it back. A condition without a status function has an implicit success(); with one it is evaluated for a run in
// which a step failed and nothing was cancelled: always() and failure() are true, success() and cancelled() are false (a failed
// step does not cancel the run), so `cancelled()` alone does not run and `!cancelled()` does. What is not known counts as
// possibly true.
func stepRunsAfterFailure(s *Step) bool {
	if s.If == nil {
		return false
	}
	exprs, ok := conditionExprs(s.If)
	if !ok {
		return true
	}
	if !callsStatusFunction(exprs, true) {
		return false
	}
	for _, e := range exprs {
		if evalAfterFailure(e) == triFalse {
			return false
		}
	}
	return true
}

// evalAfterFailure evaluates the condition for a run in which a step failed and that was not cancelled.
func evalAfterFailure(e ExprNode) tri {
	switch n := e.(type) {
	case *FuncCallNode:
		switch strings.ToLower(n.Callee) {
		case "always", "failure":
			return triTrue
		case "success", "cancelled":
			return triFalse
		}
	case *NotOpNode:
		switch evalAfterFailure(n.Operand) {
		case triTrue:
			return triFalse
		case triFalse:
			return triTrue
		}
	case *LogicalOpNode:
		l, r := evalAfterFailure(n.Left), evalAfterFailure(n.Right)
		if n.Kind == LogicalOpNodeKindAnd {
			if l == triFalse || r == triFalse {
				return triFalse
			}
			if l == triTrue && r == triTrue {
				return triTrue
			}
		} else {
			if l == triTrue || r == triTrue {
				return triTrue
			}
			if l == triFalse && r == triFalse {
				return triFalse
			}
		}
	}
	return triUnknown
}

// runsAfterSkippedNeeds reports whether the condition lets the job run although a job that it needs was skipped: it
// calls `always()`, `cancelled()` or `failure()`.
func runsAfterSkippedNeeds(cond *String) bool {
	exprs, ok := conditionExprs(cond)
	return !ok || callsStatusFunction(exprs, false)
}
