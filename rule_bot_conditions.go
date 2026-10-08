package jactionlint

import (
	"fmt"
	"regexp"
	"strings"
)

// RuleBotConditions is a rule to find conditions which trust an account by checking who "the actor"
// is. github.actor is the account which caused the last event, not the one who created the pull
// request or the branch, so a condition like github.actor == 'dependabot[bot]' can be satisfied by
// a change that dependabot never authored.
type RuleBotConditions struct {
	RuleBase
	src    *sourceIndex
	prOnly bool // the only events are pull_request and pull_request_target, which have the PR author
}

// NewRuleBotConditions creates a new RuleBotConditions instance. The source of the file lets the
// rule attach fixes to its findings. It can be nil.
func NewRuleBotConditions(src []byte) *RuleBotConditions {
	r := &RuleBotConditions{
		RuleBase: RuleBase{
			name: "bot-conditions",
			desc: "Checks for conditions which trust a bot account by github.actor",
		},
	}
	if src != nil {
		r.src = newSourceIndex(src)
	}
	return r
}

// spoofableActors are the properties which hold the last actor. The value is the property of the
// pull request author to use instead, or an empty string when there is none.
var spoofableActors = map[string]string{
	"github.actor":                "github.event.pull_request.user.login",
	"github.triggering_actor":     "github.event.pull_request.user.login",
	"github.event.sender.login":   "github.event.pull_request.user.login",
	"github.actor_id":             "github.event.pull_request.user.id",
	"github.event.sender.id":      "github.event.pull_request.user.id",
	"github.event.sender.node_id": "",
}

// botIDs are the account IDs of GitHub's well-known bots.
var botIDs = map[string]string{"49699333": "dependabot[bot]", "41898282": "github-actions[bot]", "29139614": "renovate[bot]"}

var botAccountRe = regexp.MustCompile(`[A-Za-z0-9_.-]+\[bot\]`)

var botNamePrefixes = []string{"dependabot", "renovate", "github-actions", "copilot"}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleBotConditions) VisitWorkflowPre(n *Workflow) error {
	// For the metadata of an action the events are those of the workflows which call it; none are known
	// when no local workflow calls it, which makes the suggestions that need a pull request unavailable.
	events := n.TriggerEvents()
	rule.prOnly = len(events) > 0
	for _, e := range events {
		name := strings.ToLower(e.EventName())
		if name != "pull_request" && name != "pull_request_target" {
			rule.prOnly = false
		}
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleBotConditions) VisitJobPre(n *Job) error {
	rule.checkCond(n.If)
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleBotConditions) VisitStep(n *Step) error {
	rule.checkCond(n.If)
	return nil
}

func (rule *RuleBotConditions) checkCond(s *String) {
	if s == nil {
		return
	}
	node, text, base, ok := parseWholeExpr(s.Value)
	if !ok {
		return
	}
	reported := false
	// negations is the number of "!" operators around the node. An odd number inverts the meaning:
	// the actor test then only decides what the guarded job or step skips, a spoofed bot gains nothing.
	negations := 0
	VisitExprNode(node, func(n, _ ExprNode, entering bool) {
		if _, ok := n.(*NotOpNode); ok {
			if entering {
				negations++
			} else {
				negations--
			}
		}
		if !entering || reported {
			return // one finding for a condition is enough: the fix is the same for all of them
		}
		inverted := negations%2 == 1
		switch n := n.(type) {
		case *CompareOpNode:
			// "x != bot" is an equality when it is negated an odd number of times, and "x == bot" is not
			// one then
			switch {
			case n.Kind == CompareOpNodeKindEq && !inverted, n.Kind == CompareOpNodeKindNotEq && inverted:
			default:
				return
			}
			if ref, other := spoofableOperand(n.Left, n.Right); ref != nil {
				reported = rule.checkBot(s, text, base, ref, other)
			} else if ref, other := spoofableOperand(n.Right, n.Left); ref != nil {
				reported = rule.checkBot(s, text, base, ref, other)
			}
		case *FuncCallNode:
			if inverted || len(n.Args) != 2 {
				return // !contains(github.actor, '[bot]') only skips what the condition guards
			}
			switch strings.ToLower(n.Callee) {
			case "startswith", "endswith":
				if ref, other := spoofableOperand(n.Args[0], n.Args[1]); ref != nil {
					reported = rule.checkBot(s, text, base, ref, other)
				}
			case "contains":
				if ref, other := spoofableOperand(n.Args[0], n.Args[1]); ref != nil {
					reported = rule.checkBot(s, text, base, ref, other)
				} else if ref, other := spoofableOperand(n.Args[1], n.Args[0]); ref != nil {
					reported = rule.checkBot(s, text, base, ref, other)
				}
			}
		}
	})
}

// spoofableOperand returns the operand a if it is a property that holds the last actor, with the
// other operand.
func spoofableOperand(a, other ExprNode) (ExprNode, ExprNode) {
	if path, ok := derefPath(a); ok {
		if _, ok := spoofableActors[strings.Join(path, ".")]; ok {
			return a, other
		}
	}
	return nil, nil
}

// derefPath returns the names of a property access such as github.event.sender.login.
func derefPath(n ExprNode) ([]string, bool) {
	var path []string
	for {
		switch v := n.(type) {
		case *ObjectDerefNode:
			path = append(path, strings.ToLower(v.Property))
			n = v.Receiver
			continue
		case *VariableNode:
			path = append(path, strings.ToLower(v.Name))
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path, true
		}
		return nil, false
	}
}

// botLiteral returns a bot account name or ID the operand holds, or an empty string.
func botLiteral(n ExprNode) string {
	switch n := n.(type) {
	case *StringNode:
		v := strings.ToLower(n.Value)
		if strings.Contains(v, "[bot]") {
			if m := botAccountRe.FindString(n.Value); m != "" {
				return m
			}
			return n.Value
		}
		if b, ok := botIDs[v]; ok {
			return b
		}
		for _, p := range botNamePrefixes {
			if v == p || strings.HasPrefix(v, p+"[") {
				return n.Value
			}
		}
	case *IntNode:
		if b, ok := botIDs[fmt.Sprint(n.Value)]; ok {
			return b
		}
	case *FuncCallNode:
		if strings.EqualFold(n.Callee, "fromJSON") && len(n.Args) == 1 {
			return botLiteral(n.Args[0])
		}
	}
	return ""
}

func (rule *RuleBotConditions) checkBot(s *String, text string, base int, ref, other ExprNode) bool {
	bot := botLiteral(other)
	if bot == "" {
		return false
	}
	path, _ := derefPath(ref)
	name := strings.Join(path, ".")
	tok := ref.Token()
	pos := tokenPos(rule.src, s, base, tok)
	rule.ReportIDf("bot-conditions", pos, "%q holds the account of the last event and not the author of the change, so it can be spoofed and comparing it with the bot %q does not prove that the bot made the change. check the author of the pull request instead, e.g. %q", name, bot, preferredActor(name))
	// The author of the pull request is a different account from the actor when somebody else pushed
	// to the branch. The condition then holds where it did not, so the fix is unsafe.
	repl := spoofableActors[name]
	if repl == "" || !rule.prOnly || rule.src == nil {
		return true
	}
	off, ok := rule.src.valueOffset(s, base+tok.Offset)
	if !ok {
		return true
	}
	// Only a plain access written as it is in the table
	end := tok.Offset + len(name)
	if end > len(text) || !strings.EqualFold(text[tok.Offset:end], name) || !rule.src.matches(off, text[tok.Offset:end]) {
		return true
	}
	if end < len(text) && isExprIdentChar(text[end]) {
		return true
	}
	rule.errs[len(rule.errs)-1].Fix = &Fix{
		Description: fmt.Sprintf("Check %s instead of %s", repl, name),
		Unsafe:      true,
		Edits:       []TextEdit{{off, off + len(name), repl}},
	}
	return true
}

func preferredActor(name string) string {
	if strings.HasSuffix(name, "id") {
		return "github.event.pull_request.user.id"
	}
	return "github.event.pull_request.user.login"
}

func init() {
	registerRules(
		RuleInfo{ID: "bot-conditions", Group: RuleGroupSecurity, Summary: "A condition trusts a bot by github.actor, which can be spoofed.", DefaultLevel: SeverityWarning, Profile: ProfileStrict, Fixable: true, DocsAnchor: "check-bot-conditions"},
	)
	registerRuleFactory("bot-conditions", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleBotConditions(env.src)}
	})
}
