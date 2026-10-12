package jactionlint

import (
	"regexp"
	"slices"
	"strings"
)

// A triggerScenario describes a run of the workflow which publishes: the values of the github context that
// are known for it. A key which is missing is unknown (for example the ref of a run that is not
// a release).
type triggerScenario map[string]string

// scenarioRelease is the run of the release event. The runs of a push of a tag are tagPushScenarios. Both have a tag as the ref.
var scenarioRelease = triggerScenario{"event_name": "release", "ref": "refs/tags/v1.0.0", "ref_type": "tag", "ref_name": "v1.0.0"}

// plainTagPatternRe is a tag pattern with "*" as its only wildcard.
var plainTagPatternRe = regexp.MustCompile(`^[A-Za-z0-9._/*-]+$`)

// tagPushScenarios are the runs of a push of a tag that the `tags` filter lets through, one for each positive
// pattern. The name of the tag is not known, and a condition on it ("startsWith(github.ref, 'refs/tags/preview-')")
// has to be judged for tags that the filter accepts: a pattern with "*" as its only wildcard gives a name ("v*" gives
// v1.0.0, "preview-*" gives preview-1.0.0). Any other pattern, and one that lets every tag through, gives v1.0.0.
// The condition lets the run through when it does for one of them.
func tagPushScenarios(f *WebhookEventFilter) []triggerScenario {
	names := []string{}
	if !f.IsEmpty() {
		for _, v := range f.Values {
			if v == nil || strings.HasPrefix(strings.TrimSpace(v.Value), "!") {
				continue
			}
			name := "v1.0.0"
			if pat := strings.TrimSpace(v.Value); plainTagPatternRe.MatchString(pat) && strings.Trim(pat, "*") != "" {
				name = strings.ReplaceAll(strings.ReplaceAll(pat, "**", "*"), "*", "1.0.0")
			}
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		names = append(names, "v1.0.0")
	}
	ret := make([]triggerScenario, 0, len(names))
	for _, name := range names {
		ref := "refs/tags/" + name
		ret = append(ret, triggerScenario{"event_name": "push", "event.ref": ref, "ref": ref, "ref_type": "tag", "ref_name": name})
	}
	return ret
}

// scenarioBranchPush is the push of a branch: the ref starts with refs/heads/, its type is branch and the name is
// not known.
var scenarioBranchPush = triggerScenario{"event_name": "push", "ref_type": "branch", refPrefixKey: "refs/heads/"}

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

// mentionsTrigger reports whether the expression reads the event or the ref which started the run
// (github.event_name, github.ref, github.ref_name, github.ref_type).
func mentionsTrigger(n ExprNode) bool {
	found := false
	VisitExprNode(n, func(node, _ ExprNode, entering bool) {
		if d, ok := node.(*ObjectDerefNode); ok && entering {
			if v, ok := d.Receiver.(*VariableNode); ok && v.Name == "github" && (d.Property == "event_name" || strings.HasPrefix(d.Property, "ref")) {
				found = true
			}
		}
	})
	return found
}

// unknownMentionsTrigger reports whether a part of the condition which is not known in the scenario reads the
// trigger. A condition that is not known as a whole is taken for one written to keep some triggers away, and the run
// is left out then (see gateAllowsRun). That is right when the part that is not known is about the trigger, and not
// when the trigger part is known and what is not known is something else: in
// "github.event_name == 'push' && !contains(needs.release.outputs.version, '-')" the trigger decides for the run
// that is being checked, and the output of another job is what stays open, which says nothing about the trigger.
func unknownMentionsTrigger(n ExprNode, sc triggerScenario) bool {
	if evalGate(n, sc).truth() != triUnknown {
		return false
	}
	switch n := n.(type) {
	case *LogicalOpNode:
		return unknownMentionsTrigger(n.Left, sc) || unknownMentionsTrigger(n.Right, sc)
	case *NotOpNode:
		return unknownMentionsTrigger(n.Operand, sc)
	}
	return mentionsTrigger(n)
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
			if !looksAtTrigger(cond) || !unknownMentionsTrigger(e, sc) {
				return true
			}
		}
	}
	return len(scenarios) == 0
}

// gateInputIsRedundant reports whether the input is one of two switches of an action which the other one keeps on: the
// action turns the cache of maturin-action on for "cache: sccache" whatever "sccache" says, so a condition in "sccache"
// alone does not keep it away. Another literal does not turn it on. An expression in "cache" does when it is not known
// to be false in the runs that publish (without scenarios it always does: the finding stays).
func gateInputIsRedundant(name string, a *ExecAction, input string, scenarios []triggerScenario) bool {
	if name != "pyo3/maturin-action" || input != "sccache" {
		return false
	}
	v, ok := a.input("cache")
	v = strings.TrimSpace(v)
	switch {
	case !ok:
		return false
	case strings.Contains(v, "${{"):
		return gateAllows(v, scenarios, false)
	}
	return strings.EqualFold(v, "sccache")
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
		if v, ok := a.input(g.input); ok && !gateInputIsRedundant(name, a, g.input, scenarios) && strings.Contains(v, "${{") && !gateAllows(v, scenarios, g.offWhenTrue) {
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
