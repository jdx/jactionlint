package jactionlint

import (
	"strings"
)

// RuleUnsoundPrefixMatch is a rule to find expressions which identify an account, an organization
// or a repository by a part of its name. startsWith(github.actor, 'jdx') is also true for
// "jdx-evil", endsWith(github.repository, '/mise') for a fork of any owner, and contains() for any
// name which has the text somewhere. Names are chosen by whoever registers them, so a partial match
// does not identify the owner of the name.
type RuleUnsoundPrefixMatch struct {
	RuleBase
	refs           bool // also check the names of branches and tags
	botConditionOn bool // bot-conditions reports the comparisons with bot names in conditions
}

// NewRuleUnsoundPrefixMatch creates a new RuleUnsoundPrefixMatch instance. refs makes the rule check
// the names of branches and tags too.
func NewRuleUnsoundPrefixMatch(refs, botConditionOn bool) *RuleUnsoundPrefixMatch {
	return &RuleUnsoundPrefixMatch{
		RuleBase: RuleBase{
			name: "unsound-prefix-match",
			desc: "Checks expressions which identify an account or a repository by a prefix, a suffix or a part of its name",
		},
		refs:           refs,
		botConditionOn: botConditionOn,
	}
}

// nameKind tells what a context property holds.
type nameKind int

const (
	nameOwner nameKind = iota + 1 // the login of an account or an organization
	nameRepo                      // the full name of a repository, "owner/name"
	nameRef                       // the name of a branch or a tag
)

// identityContexts are the properties which hold a name which decides who somebody is.
var identityContexts = map[string]nameKind{
	"github.actor":                                          nameOwner,
	"github.triggering_actor":                               nameOwner,
	"github.repository_owner":                               nameOwner,
	"github.event.sender.login":                             nameOwner,
	"github.event.repository.owner.login":                   nameOwner,
	"github.event.organization.login":                       nameOwner,
	"github.event.pull_request.user.login":                  nameOwner,
	"github.event.pull_request.head.user.login":             nameOwner,
	"github.event.pull_request.head.repo.owner.login":       nameOwner,
	"github.event.pull_request.base.repo.owner.login":       nameOwner,
	"github.event.issue.user.login":                         nameOwner,
	"github.event.comment.user.login":                       nameOwner,
	"github.event.review.user.login":                        nameOwner,
	"github.event.workflow_run.actor.login":                 nameOwner,
	"github.event.workflow_run.triggering_actor.login":      nameOwner,
	"github.event.workflow_run.head_repository.owner.login": nameOwner,
	"github.repository":                                     nameRepo,
	"github.event.repository.full_name":                     nameRepo,
	"github.event.pull_request.head.repo.full_name":         nameRepo,
	"github.event.pull_request.base.repo.full_name":         nameRepo,
	"github.event.workflow_run.head_repository.full_name":   nameRepo,
	"github.ref":                            nameRef,
	"github.ref_name":                       nameRef,
	"github.head_ref":                       nameRef,
	"github.base_ref":                       nameRef,
	"github.event.pull_request.head.ref":    nameRef,
	"github.event.pull_request.base.ref":    nameRef,
	"github.event.workflow_run.head_branch": nameRef,
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnsoundPrefixMatch) VisitWorkflowPre(n *Workflow) error {
	sensitive := sensitiveContexts(n)
	workflowExprSites(n, func(site exprSite) {
		scanExpressions(site.Str, site.Cond, func(o *exprOccurrence) {
			// Outside a condition, the result of a name test is a trust decision only when it selects a
			// credential or a runner: GOOS: ${{ contains(github.repository, 'windows') && 'windows' || '' }}
			// is not
			if site.Cond || sensitiveContext(sensitive, site, o.Root) {
				rule.checkExpr(o, site.Cond)
			}
		})
	})
	return nil
}

// sensitiveContexts returns the strings of the workflow which hold what a trust decision selects: the
// runner, the environment (and with it its secrets), the image and credentials of a container or a
// service, the permissions and the secrets passed to a reusable workflow.
func sensitiveContexts(w *Workflow) map[*String]bool {
	set := map[*String]bool{}
	add := func(ss ...*String) {
		for _, s := range ss {
			if s != nil {
				set[s] = true
			}
		}
	}
	permissions := func(p *Permissions) {
		if p == nil {
			return
		}
		add(p.All)
		for _, sc := range p.Scopes {
			if sc != nil {
				add(sc.Name, sc.Value)
			}
		}
	}
	container := func(c *Container) {
		if c == nil {
			return
		}
		add(c.Image)
		if c.Credentials != nil {
			add(c.Credentials.Username, c.Credentials.Password, c.Credentials.Expression)
		}
	}
	permissions(w.Permissions)
	for _, j := range w.Jobs {
		if j.RunsOn != nil {
			add(j.RunsOn.LabelsExpr, j.RunsOn.Group)
			add(j.RunsOn.Labels...)
		}
		if j.Environment != nil {
			add(j.Environment.Name, j.Environment.URL)
		}
		permissions(j.Permissions)
		container(j.Container)
		if j.Services != nil {
			add(j.Services.Expression)
			for _, sv := range j.Services.Value {
				if sv != nil {
					container(sv.Container)
				}
			}
		}
		if j.WorkflowCall != nil {
			for _, sec := range j.WorkflowCall.Secrets {
				if sec != nil {
					add(sec.Value)
				}
			}
		}
	}
	return set
}

// sensitiveContext reports whether a name test in the site decides something which matters: the site
// is one of the sensitiveContexts, or the expression reads a secret or the token of the workflow.
func sensitiveContext(contexts map[*String]bool, site exprSite, root ExprNode) bool {
	if site.Str != nil && contexts[site.Str] {
		return true
	}
	found := false
	VisitExprNode(root, func(n, _ ExprNode, entering bool) {
		if !entering || found {
			return
		}
		switch n := n.(type) {
		case *VariableNode:
			// secrets.X, secrets['X'] and a bare secrets (toJSON(secrets)) all reach the context
			found = strings.EqualFold(n.Name, "secrets")
		case *ObjectDerefNode, *IndexAccessNode:
			if path, ok := derefPath(n); ok {
				found = strings.Join(path, ".") == "github.token"
			}
		}
	})
	return found
}

// identity returns the property name and the kind of the operand if it holds a name.
func identity(n ExprNode) (string, nameKind, bool) {
	path, ok := derefPath(n)
	if !ok {
		return "", 0, false
	}
	name := strings.Join(path, ".")
	k, ok := identityContexts[name]
	return name, k, ok
}

// exactTargets returns the properties which are compared with a whole name in a condition which must
// be true as a whole: the operands of && chains. A partial match next to such a comparison cannot
// widen who passes.
func exactTargets(root ExprNode) map[string]bool {
	exact := map[string]bool{}
	var walk func(n ExprNode)
	walk = func(n ExprNode) {
		switch n := n.(type) {
		case *LogicalOpNode:
			if n.Kind == LogicalOpNodeKindAnd {
				walk(n.Left)
				walk(n.Right)
			}
		case *CompareOpNode:
			if n.Kind != CompareOpNodeKindEq {
				return
			}
			for _, pair := range [][2]ExprNode{{n.Left, n.Right}, {n.Right, n.Left}} {
				if name, _, ok := identity(pair[0]); ok {
					if _, isLit := pair[1].(*StringNode); isLit {
						exact[name] = true
					}
				}
			}
		case *FuncCallNode:
			// contains(fromJSON('["a", "b"]'), github.actor) tests a list of whole names
			if strings.EqualFold(n.Callee, "contains") && len(n.Args) == 2 {
				if name, _, ok := identity(n.Args[1]); ok {
					if fj, ok := n.Args[0].(*FuncCallNode); ok && strings.EqualFold(fj.Callee, "fromJSON") && len(fj.Args) == 1 {
						if _, isLit := fj.Args[0].(*StringNode); isLit {
							exact[name] = true
						}
					}
				}
			}
		}
	}
	walk(root)
	return exact
}

func (rule *RuleUnsoundPrefixMatch) checkExpr(o *exprOccurrence, cond bool) {
	exact := exactTargets(o.Root)
	negations := 0
	VisitExprNode(o.Root, func(node, _ ExprNode, entering bool) {
		if _, ok := node.(*NotOpNode); ok {
			if entering {
				negations++
			} else {
				negations--
			}
			return
		}
		call, ok := node.(*FuncCallNode)
		if !ok || !entering || len(call.Args) != 2 || negations%2 == 1 {
			// A negated match only excludes some names: !startsWith(github.actor, 'bot') does not let
			// the wrong accounts in
			return
		}
		fn := strings.ToLower(call.Callee)
		if fn != "startswith" && fn != "endswith" && fn != "contains" {
			return
		}
		name, kind, ok := identity(call.Args[0])
		if !ok {
			return
		}
		lit, ok := call.Args[1].(*StringNode)
		if !ok || lit.Value == "" || exact[name] {
			return
		}
		if kind == nameRef && !rule.refs {
			return
		}
		if cond && rule.botConditionOn && kind == nameOwner && botLiteral(lit) != "" {
			if _, spoofable := spoofableActors[name]; spoofable {
				return // bot-conditions reports the comparison with a bot
			}
		}
		why, advice, unsound := partialMatch(fn, kind, lit.Value)
		if !unsound {
			return
		}
		fnName := map[string]string{"startswith": "startsWith", "endswith": "endsWith", "contains": "contains"}[fn]
		rule.ReportIDf("unsound-prefix-match", o.PosOf(call), "%s(%s, %q) is also true for %s, which anybody can create. %s", fnName, name, lit.Value, why, advice)
	})
}

// partialMatch tells whether the test fn of a literal against a name of the kind accepts more than
// one name that the author means, with the description of the extra names and advice.
func partialMatch(fn string, kind nameKind, lit string) (why, advice string, unsound bool) {
	compare := "compare the whole name with == or, for several names, test a list with contains(fromJSON('[\"a\", \"b\"]'), value)"
	switch kind {
	case nameOwner:
		if strings.HasSuffix(lit, "[bot]") && fn != "contains" {
			// Only GitHub appends "[bot]", so a name ending with it is a whole name, or all apps
			return "", "", false
		}
		switch fn {
		case "startswith":
			return "an account whose name only begins with " + quote(lit) + ", such as " + quote(lit+"-evil"), compare, true
		case "endswith":
			return "an account whose name only ends with " + quote(lit) + ", such as " + quote("evil-"+lit), compare, true
		}
		return "an account whose name only contains " + quote(lit), compare, true
	case nameRepo:
		switch fn {
		case "startswith":
			if strings.Contains(lit, "/") {
				return "", "", false // the owner is complete, so only the owner can create a repository
			}
			return "a repository of an owner whose name only begins with " + quote(lit) + ", such as " + quote(lit+"-evil/x"),
				"end the prefix with a slash to match the owner, as in " + quote(lit+"/") + ", or " + compare, true
		case "endswith":
			return "a fork of the repository by any owner, such as " + quote("evil/"+strings.TrimPrefix(lit, "/")),
				compare, true
		}
		return "a repository whose name only contains " + quote(lit), compare, true
	case nameRef:
		if fn == "startswith" && strings.HasSuffix(lit, "/") {
			return "", "", false // a namespace like release/ is meant to match every name in it
		}
		fix := "compare the whole name"
		if fn == "startswith" {
			fix += ", or end the prefix with a slash if it is a namespace"
		}
		return "a branch or a tag whose name only " + map[string]string{"startswith": "begins with", "endswith": "ends with", "contains": "contains"}[fn] + " " + quote(lit), fix, true
	}
	return "", "", false
}

func init() {
	registerRules(
		RuleInfo{
			ID: "unsound-prefix-match", Group: RuleGroupSecurity, Summary: "An account, an owner or a repository is identified by a prefix, a suffix or a part of its name.",
			DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-unsound-prefix-match",
			Options: []RuleOption{{Name: "refs", Kind: RuleOptionBool, Default: false, Summary: "Also check the names of branches and tags (github.ref, github.head_ref, ...). A prefix test of a ref is often meant, so this is noisy."}},
		},
	)
	registerRuleFactory("unsound-prefix-match", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("unsound-prefix-match") {
			return nil
		}
		refs, _ := env.config.ruleOptionBool("unsound-prefix-match", "refs")
		return []Rule{NewRuleUnsoundPrefixMatch(refs, env.config.RuleEnabled("bot-conditions"))}
	})
}
