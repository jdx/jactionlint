package jactionlint

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

// runsAfterSkippedNeeds reports whether the condition lets the job run although a job that it needs was skipped: it
// calls `always()`, `cancelled()` or `failure()`.
func runsAfterSkippedNeeds(cond *String) bool {
	exprs, ok := conditionExprs(cond)
	return !ok || callsStatusFunction(exprs, false)
}
