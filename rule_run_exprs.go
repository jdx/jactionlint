package jactionlint

import "strings"

// This file has the helpers the rules of the run-script batch share to judge the `${{ }}` expressions and
// environment variables that end up in a script.

// exprsIn returns the inner text of every `${{ }}` expression in the string, in order. String literals inside an
// expression may contain `}}`.
func exprsIn(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		j := strings.Index(s[i:], "${{")
		if j < 0 {
			break
		}
		start := i + j + 3
		end := exprClose(s, start)
		if end < 0 {
			break
		}
		out = append(out, strings.TrimSpace(s[start:end]))
		i = end + 2
	}
	return out
}

// exprClose returns the offset of the `}}` that closes an expression whose body starts at i, or -1.
func exprClose(s string, i int) int {
	for i < len(s) {
		switch {
		case s[i] == '\'':
			i++
			for i < len(s) {
				if s[i] == '\'' {
					if i+1 < len(s) && s[i+1] == '\'' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			i++
		case strings.HasPrefix(s[i:], "}}"):
			return i
		default:
			i++
		}
	}
	return -1
}

// parseExprText parses the inner text of an expression. It returns nil when the text does not parse.
func parseExprText(text string) ExprNode {
	n, err := NewExprParser().Parse(NewExprLexer(text + "}}"))
	if err != nil {
		return nil
	}
	return n
}

// exprReadsUntrustedInput reports whether the expression reads an input that an outsider controls, such as the
// title of an issue or the name of the branch of a pull request.
func exprReadsUntrustedInput(text string) bool {
	n := parseExprText(text)
	if n == nil {
		return false
	}
	u := NewUntrustedInputChecker(BuiltinUntrustedInputs)
	u.Init()
	VisitExprNode(n, func(node, _ ExprNode, entering bool) {
		if entering {
			u.OnVisitNodeEnter(node)
		} else {
			u.OnVisitNodeLeave(node)
		}
	})
	u.OnVisitEnd()
	return len(u.Errs()) > 0
}

// exprReadsContext reports whether the expression reads a property of the context with the given name (case
// insensitive), like `inputs.version` or `inputs['version']` for "inputs".
func exprReadsContext(text, context string) bool {
	n := parseExprText(text)
	if n == nil {
		return false
	}
	found := false
	VisitExprNode(n, func(node, _ ExprNode, entering bool) {
		if v, ok := node.(*VariableNode); ok && entering && strings.EqualFold(v.Name, context) {
			found = true
		}
	})
	return found
}

// trustedPaths are the properties of contexts whose value the author of the workflow or GitHub decides, or whose
// format an outsider cannot shape (a commit SHA, a number). Writing them somewhere is not a finding by itself.
var trustedPaths = map[string]bool{
	"github.sha": true, "github.repository": true, "github.repository_owner": true, "github.repository_id": true,
	"github.repository_owner_id": true, "github.run_id": true, "github.run_number": true, "github.run_attempt": true,
	"github.action_path": true, "github.event_name": true, "github.server_url": true, "github.api_url": true,
	"github.graphql_url": true, "github.job": true, "github.retention_days": true, "github.workflow_sha": true,
	"github.event.number": true, "github.event.pull_request.number": true,
	"github.event.pull_request.head.sha": true, "github.event.pull_request.base.sha": true,
	"github.event.workflow_run.id": true, "github.event.workflow_run.head_sha": true,
	// timestamps and counters that GitHub sets
	"github.event.workflow_run.run_started_at": true, "github.event.workflow_run.created_at": true,
	"github.event.workflow_run.updated_at": true, "github.event.workflow_run.run_number": true,
	"github.event.workflow_run.run_attempt": true, "github.event.pull_request.created_at": true,
	"github.event.pull_request.updated_at": true, "github.event.pull_request.id": true,
}

// trustedContexts are the contexts that are trusted as a whole.
var trustedContexts = map[string]bool{"runner": true, "matrix": true, "strategy": true, "vars": true, "secrets": true, "job": true}

// chainPath returns the dotted path of a chain of property accesses on a context ("github.event.number"), or false
// when the node is anything else.
func chainPath(n ExprNode) (string, bool) {
	switch n := n.(type) {
	case *VariableNode:
		return strings.ToLower(n.Name), true
	case *ObjectDerefNode:
		p, ok := chainPath(n.Receiver)
		if !ok {
			return "", false
		}
		return p + "." + strings.ToLower(n.Property), true
	}
	return "", false
}

// chainRoot returns the variable a chain of property accesses starts at.
func chainRoot(n ExprNode) *VariableNode {
	for {
		switch c := n.(type) {
		case *VariableNode:
			return c
		case *ObjectDerefNode:
			n = c.Receiver
		default:
			return nil
		}
	}
}

// exprIsTrusted reports whether every value the expression reads is trusted: it only reads the contexts of
// trustedContexts and the paths of trustedPaths (or nothing at all).
func exprIsTrusted(text string) bool {
	n := parseExprText(text)
	if n == nil {
		return false
	}
	covered := map[*VariableNode]bool{}
	VisitExprNode(n, func(node, _ ExprNode, entering bool) {
		if !entering {
			return
		}
		if d, ok := node.(*ObjectDerefNode); ok {
			if p, ok := chainPath(d); ok && trustedPaths[p] {
				if v := chainRoot(d); v != nil {
					covered[v] = true
				}
			}
		}
	})
	trusted := true
	VisitExprNode(n, func(node, _ ExprNode, entering bool) {
		if v, ok := node.(*VariableNode); ok && entering && !covered[v] && !trustedContexts[strings.ToLower(v.Name)] {
			trusted = false
		}
	})
	return trusted
}

// dataKind tells how much is known about a value that is written somewhere.
type dataKind int

const (
	// dataLiteral is a value fixed by the workflow author.
	dataLiteral dataKind = iota
	// dataUnknown is a value that is computed at run time from sources that are not known to be trusted.
	dataUnknown
	// dataUntrusted is a value that is known to come from an outsider.
	dataUntrusted
)

// data is the judgement of a value, with the input that made it untrusted for messages.
type data struct {
	kind dataKind
	// input is the untrusted expression, "" unless kind is dataUntrusted.
	input string
	// via is the environment variable the untrusted input went through, "" when it is used directly.
	via string
}

func (d data) worse(o data) data {
	if o.kind > d.kind {
		return o
	}
	return d
}

// judgeExprs judges the expressions of a value.
func judgeExprs(exprs []string) data {
	d := data{kind: dataLiteral}
	for _, e := range exprs {
		switch {
		case exprReadsUntrustedInput(e):
			return data{kind: dataUntrusted, input: e}
		case !exprIsTrusted(e):
			d = d.worse(data{kind: dataUnknown})
		}
	}
	return d
}

// runnerProvidedVars are the variables that the runner sets to values nobody can influence from the outside.
// GITHUB_WORKSPACE is left out on purpose: when a pull request is checked out, its files are in there.
var runnerProvidedVars = map[string]bool{
	"HOME": true, "RUNNER_TEMP": true, "RUNNER_TOOL_CACHE": true, "RUNNER_OS": true, "RUNNER_ARCH": true,
	"RUNNER_NAME": true, "GITHUB_ACTION_PATH": true, "GITHUB_RUN_ID": true, "GITHUB_RUN_NUMBER": true,
	"GITHUB_RUN_ATTEMPT": true, "GITHUB_REPOSITORY": true, "GITHUB_REPOSITORY_OWNER": true, "GITHUB_SERVER_URL": true,
	"GITHUB_API_URL": true, "GITHUB_SHA": true, "GITHUB_JOB": true, "GITHUB_EVENT_NAME": true,
}

// usesOfStep returns the action which the step runs and its parsed `uses:` value. It returns nil for a step that
// does not run an action, has no usable `uses:` or whose `uses:` is an expression.
func usesOfStep(s *Step) (*ExecAction, *UsesRef) {
	a, ok := s.Exec.(*ExecAction)
	if !ok || a.Uses == nil || a.Uses.Value == "" || a.Uses.ContainsExpression() {
		return nil, nil
	}
	return a, ParseUses(a.Uses.Value)
}

// inputValue returns the trimmed value of an input of the action. Names are case-insensitive. An input that is not
// set returns false.
func inputValue(e *ExecAction, name string) (string, bool) {
	i, ok := e.Inputs[strings.ToLower(name)]
	if !ok || i == nil || i.Value == nil {
		return "", false
	}
	return strings.TrimSpace(i.Value.Value), true
}
