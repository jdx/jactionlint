package jactionlint

import "strings"

// A triggerScenario describes a run of the workflow which publishes: the values of the github context that
// are known for it. A key which is missing is unknown (for example the ref of a run that is not
// a release).
type triggerScenario map[string]string

// releaseScenarios lists the runs on which a release workflow publishes: the release event and a push of a tag.
// Both have a tag as the ref.
var (
	scenarioRelease = triggerScenario{"event_name": "release", "ref": "refs/tags/v1.0.0", "ref_type": "tag", "ref_name": "v1.0.0"}
	scenarioTagPush = triggerScenario{"event_name": "push", "event.ref": "refs/tags/v1.0.0", "ref": "refs/tags/v1.0.0", "ref_type": "tag", "ref_name": "v1.0.0"}
)

// refPrefixKey names the scenario value which says what the ref of the run starts with when the ref itself is not
// known: a pushed branch has a ref of "refs/heads/" and a name which is not known.
const refPrefixKey = "ref_prefix"

// refPrefix returns the known start of the ref which n reads (github.ref or github.event.ref) in the scenario, when
// the ref itself is not known.
func refPrefix(n ExprNode, sc triggerScenario) (string, bool) {
	p, ok := sc[refPrefixKey]
	d, isDeref := n.(*ObjectDerefNode)
	if !ok || !isDeref || d.Property != "ref" {
		return "", false
	}
	switch r := d.Receiver.(type) {
	case *VariableNode:
		return p, r.Name == "github"
	case *ObjectDerefNode:
		v, isVar := r.Receiver.(*VariableNode)
		return p, isVar && v.Name == "github" && r.Property == "event"
	}
	return "", false
}

// prefixGate evaluates a comparison of a ref which is known by its start only with a literal. exact is true
// for ==, false for startsWith. It is unknown when the literal is longer than the known start and agrees with it.
func prefixGate(prefix, lit string, exact bool) gateValue {
	prefix, lit = strings.ToLower(prefix), strings.ToLower(lit)
	switch {
	case strings.HasPrefix(lit, prefix):
		if exact || lit != prefix {
			return gateValue{}
		}
		return gateBool(true)
	case strings.HasPrefix(prefix, lit):
		if exact {
			return gateBool(false)
		}
		return gateBool(true)
	}
	return gateBool(false)
}

// tri is a truth value of three states, because the condition of a job often depends on things which are not known
// while linting (the result of another job, an input, the matrix).
type tri int

const (
	triUnknown tri = iota
	triFalse
	triTrue
)

// gateValue is the result of evaluating a part of a condition: nothing is known, a boolean or a string.
type gateValue struct {
	kind int // 0 unknown, 1 bool, 2 string
	b    bool
	s    string
}

func (v gateValue) truth() tri {
	switch v.kind {
	case 1:
		if v.b {
			return triTrue
		}
		return triFalse
	case 2:
		if v.s != "" {
			return triTrue
		}
		return triFalse
	}
	return triUnknown
}

func gateBool(b bool) gateValue { return gateValue{kind: 1, b: b} }

// parseGateExpression parses the value of an if: or of an input, with or without ${{ }} around it. It returns
// nil when the text is not one expression or does not parse.
func parseGateExpression(s string) ExprNode {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "${{") && strings.HasSuffix(s, "}}") && strings.Count(s, "${{") == 1 {
		s = s[len("${{") : len(s)-len("}}")]
	} else if strings.Contains(s, "${{") {
		return nil // text around an expression: not a condition
	}
	e, err := NewExprParser().Parse(NewExprLexer(strings.TrimSpace(s) + "}}"))
	if err != nil {
		return nil
	}
	return e
}

// evalGate evaluates the expression for a scenario as far as the values are known.
func evalGate(n ExprNode, sc triggerScenario) gateValue {
	switch n := n.(type) {
	case *BoolNode:
		return gateBool(n.Value)
	case *StringNode:
		return gateValue{kind: 2, s: n.Value}
	case *ObjectDerefNode:
		if v, ok := n.Receiver.(*VariableNode); ok && v.Name == "github" {
			if s, ok := sc[n.Property]; ok {
				return gateValue{kind: 2, s: s}
			}
		}
		// github.event.ref: the payload of a push has the ref, other payloads do not
		if d, ok := n.Receiver.(*ObjectDerefNode); ok && n.Property == "ref" && d.Property == "event" {
			if v, ok := d.Receiver.(*VariableNode); ok && v.Name == "github" {
				if s, ok := sc["event.ref"]; ok {
					return gateValue{kind: 2, s: s}
				}
			}
		}
	case *NotOpNode:
		switch evalGate(n.Operand, sc).truth() {
		case triTrue:
			return gateBool(false)
		case triFalse:
			return gateBool(true)
		}
	case *LogicalOpNode:
		l := evalGate(n.Left, sc)
		r := evalGate(n.Right, sc)
		lt, rt := l.truth(), r.truth()
		if n.Kind == LogicalOpNodeKindAnd {
			switch {
			case lt == triFalse:
				return l
			case lt == triTrue:
				return r
			case rt == triFalse: // either the left one is returned (false) or the right one
				return gateBool(false)
			}
		} else {
			switch {
			case lt == triTrue:
				return l
			case lt == triFalse:
				return r
			case rt == triTrue:
				return gateBool(true)
			}
		}
	case *CompareOpNode:
		if !n.Kind.IsEqualityOp() {
			break
		}
		for _, side := range [][2]ExprNode{{n.Left, n.Right}, {n.Right, n.Left}} {
			if p, ok := refPrefix(side[0], sc); ok {
				if lit := evalGate(side[1], sc); lit.kind == 2 {
					g := prefixGate(p, lit.s, true)
					if g.kind == 1 && n.Kind != CompareOpNodeKindEq {
						g = gateBool(!g.b)
					}
					return g
				}
			}
		}
		l, r := evalGate(n.Left, sc), evalGate(n.Right, sc)
		if l.kind == 2 && r.kind == 2 {
			eq := strings.EqualFold(l.s, r.s)
			return gateBool(eq == (n.Kind == CompareOpNodeKindEq))
		}
		if l.kind == 1 && r.kind == 1 {
			return gateBool((l.b == r.b) == (n.Kind == CompareOpNodeKindEq))
		}
	case *FuncCallNode:
		switch strings.ToLower(n.Callee) {
		case "always", "success":
			return gateBool(true)
		case "startswith", "endswith", "contains":
			if len(n.Args) != 2 {
				break
			}
			if p, ok := refPrefix(n.Args[0], sc); ok && strings.ToLower(n.Callee) == "startswith" {
				if lit := evalGate(n.Args[1], sc); lit.kind == 2 {
					return prefixGate(p, lit.s, false)
				}
			}
			a, b := evalGate(n.Args[0], sc), evalGate(n.Args[1], sc)
			if a.kind != 2 || b.kind != 2 {
				break
			}
			x, y := strings.ToLower(a.s), strings.ToLower(b.s)
			switch strings.ToLower(n.Callee) {
			case "startswith":
				return gateBool(strings.HasPrefix(x, y))
			case "endswith":
				return gateBool(strings.HasSuffix(x, y))
			}
			return gateBool(strings.Contains(x, y))
		}
	}
	return gateValue{}
}

// gateAllowsRun reports whether a condition (an if:, or the value of an input that switches a feature on)
// lets the run through in at least one of the scenarios. A condition which cannot be evaluated lets it through,
// unless it mentions the trigger: someone wrote it to keep the run away from some triggers, and
// guessing wrong would report the workflow for something it does not do.
func gateAllowsRun(cond string, scenarios []triggerScenario) bool {
	return gateAllows(cond, scenarios, false)
}

// gateAllows is gateAllowsRun; offWhenTrue is for an input which switches the feature off when it is true.
func gateAllows(cond string, scenarios []triggerScenario, offWhenTrue bool) bool {
	e := parseGateExpression(cond)
	for _, sc := range scenarios {
		if e == nil {
			if looksAtTrigger(cond) {
				continue
			}
			return true
		}
		on, off := triTrue, triFalse
		if offWhenTrue {
			on, off = triFalse, triTrue
		}
		switch evalGate(e, sc).truth() {
		case on:
			return true
		case off:
		default:
			if !looksAtTrigger(cond) {
				return true
			}
		}
	}
	return len(scenarios) == 0
}

// cacheCanRunOnReleaseTrigger tells whether the cache of the step is used in a run that publishes. The run
// is one of the scenarios (a release event, a push of a tag, or the events of a workflow that has
// a publishing job). The cache is used when none of these keeps it away:
//
//   - the if: of the job, which decides whether the job runs at all. A publish job with
//     "if: startsWith(github.ref, 'refs/tags/')" runs, a test job with "if: github.event_name == 'pull_request'"
//     does not, and a job which has no condition, only needs other jobs or has "if: always()" does.
//   - the if: of the step, evaluated the same way;
//   - the inputs of the action that switch its caching on (cacheGateInputs), evaluated the same way,
//     where a value that is false or empty switches it off.
//
// Everything else a step or an input contains (a key, a version, a path) is not looked at.
func cacheCanRunOnReleaseTrigger(job *Job, s *Step, a *ExecAction, name string, scenarios []triggerScenario) bool {
	if job != nil && job.If != nil && !gateAllowsRun(job.If.Value, scenarios) {
		return false
	}
	if s.If != nil && !gateAllowsRun(s.If.Value, scenarios) {
		return false
	}
	for _, g := range cacheGateInputs[name] {
		if v, ok := a.input(g.input); ok && strings.Contains(v, "${{") && !gateAllows(v, scenarios, g.offWhenTrue) {
			return false
		}
	}
	// The automatic caching of setup-node v5 is switched off by "package-manager-cache"; an explicit "cache" is
	// not affected by it, so it decides only when there is no explicit "cache" or the cache is off ("cache: false"
	// or empty), where the automatic caching is what remains
	if name == "actions/setup-node" {
		if c, explicit := a.input("cache"); !explicit || isFalseLiteral(strings.TrimSpace(c)) || strings.TrimSpace(c) == "" {
			if v, ok := a.input("package-manager-cache"); ok && strings.Contains(v, "${{") && !gateAllows(v, scenarios, false) {
				return false
			}
		}
	}
	return true
}
