package jactionlint

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// This file has what the template injection checks share: which parts of a step are code, which
// contexts are attacker controlled, how a value flows through env:, and the fixes which move an
// expansion into an environment variable. The checks themselves are in rule_expression.go (the
// contexts known to be attacker controlled) and rule_template_injection.go (everything else).

// codeExecInputs lists the inputs of well-known actions whose value is run as code, like the run:
// of a step. An expression in them is expanded into the source code just as in a run: script. The
// keys are lower case owner/repo and the values are lower case input names.
var codeExecInputs = map[string][]string{
	"actions/github-script":      {"script"},
	"amadevus/pwsh-script":       {"script"},
	"appleboy/ssh-action":        {"script"},
	"addnab/docker-run-action":   {"options", "run"},
	"azure/cli":                  {"inlinescript"},
	"azure/powershell":           {"inlinescript"},
	"borales/actions-yarn":       {"cmd"},
	"cardinalby/js-eval-action":  {"expression"},
	"cypress-io/github-action":   {"build", "command", "install-command", "start"},
	"devcontainers/ci":           {"runcmd"},
	"jannekem/run-python-action": {"code"},
	"mathiasvr/command-output":   {"run"},
	"matootie/dokku":             {"command"},
	"nick-fields/retry":          {"command", "on_retry_command"},
}

// isCodeExecInput reports whether the input of the action given by the uses: value is run as code.
// The input name must be lower case.
func isCodeExecInput(uses, input string) bool {
	name, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(uses)), "@")
	return slices.Contains(codeExecInputs[name], input)
}

// tiCode is a string of a step which is run as code.
type tiCode struct {
	Str *String
	// Run is the step when the string is its run: script. It is nil for inputs of actions.
	Run *ExecRun
}

// codeStringsOf returns the strings of the step which are run as code in a deterministic order.
func codeStringsOf(step *Step) []tiCode {
	switch e := step.Exec.(type) {
	case *ExecRun:
		if e.Run != nil {
			return []tiCode{{e.Run, e}}
		}
	case *ExecAction:
		if e.Uses == nil {
			return nil
		}
		var names []string
		for n, in := range e.Inputs {
			if in.Value != nil && isCodeExecInput(e.Uses.Value, n) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		ret := make([]tiCode, 0, len(names))
		for _, n := range names {
			ret = append(ret, tiCode{Str: e.Inputs[n].Value})
		}
		return ret
	}
	return nil
}

// ctxRef is a reference to a context property in an expression such as github.event.issue.title.
type ctxRef struct {
	// Path holds the names normalized by normalizeContextName. A '*' is an object filter (github.event.*.body,
	// labels.*.name). A "[]" is an index which is a number or an expression (labels[0], labels[matrix.i]); it
	// reaches the elements of an array like a filter does, but it is a different way to write the access.
	Path []string
	// Node is the root of the reference.
	Node ExprNode
}

// String joins the path. An index shows as '*' like a filter does, which is how the untrusted inputs are listed.
func (r *ctxRef) String() string {
	return strings.ReplaceAll(strings.Join(r.Path, "."), "[]", "*")
}

// Display returns the reference as it is written in the expression when it is a plain property
// access, so that messages keep the case of names. Otherwise it returns the lower case path.
func (r *ctxRef) Display(src string) string {
	off := r.Node.Token().Offset
	if off >= 0 && off < len(src) {
		end := off
		for end < len(src) && isExprIdentChar(src[end]) {
			end++
		}
		if t := src[off:end]; strings.EqualFold(t, r.String()) {
			return t
		}
	}
	return r.String()
}

// exprContextRefs lists the context properties whose value can flow into the result of the
// expression. Properties which are only compared or tested (contains, ==, !) do not count since the
// result is then a boolean.
func exprContextRefs(n ExprNode) []ctxRef {
	switch n := n.(type) {
	case *VariableNode:
		return []ctxRef{{Path: []string{normalizeContextName(n.Name)}, Node: n}}
	case *ObjectDerefNode:
		return appendSeg(exprContextRefs(n.Receiver), normalizeContextName(n.Property))
	case *ArrayDerefNode:
		return appendSeg(exprContextRefs(n.Receiver), "*")
	case *IndexAccessNode:
		seg := "[]"
		if s, ok := n.Index.(*StringNode); ok {
			seg = normalizeContextName(s.Value)
		}
		return appendSeg(exprContextRefs(n.Operand), seg)
	case *LogicalOpNode:
		if n.Kind == LogicalOpNodeKindAnd {
			// The left operand is the result only when it is falsy, which is an empty string, "false" or "0".
			return exprContextRefs(n.Right)
		}
		return append(exprContextRefs(n.Left), exprContextRefs(n.Right)...)
	case *FuncCallNode:
		switch strings.ToLower(n.Callee) {
		case "contains", "startswith", "endswith", "hashfiles", "success", "failure", "always", "cancelled":
			return nil
		}
		var ret []ctxRef
		for _, a := range n.Args {
			ret = append(ret, exprContextRefs(a)...)
		}
		return ret
	}
	return nil
}

// chainRefs lists every property access of the expression wherever it is, including the ones whose
// value never reaches the result.
func chainRefs(n ExprNode) []ctxRef {
	switch n := n.(type) {
	case *VariableNode, *ObjectDerefNode, *ArrayDerefNode, *IndexAccessNode:
		return exprContextRefs(n)
	case *NotOpNode:
		return chainRefs(n.Operand)
	case *CompareOpNode:
		return append(chainRefs(n.Left), chainRefs(n.Right)...)
	case *LogicalOpNode:
		return append(chainRefs(n.Left), chainRefs(n.Right)...)
	case *FuncCallNode:
		var ret []ctxRef
		for _, a := range n.Args {
			ret = append(ret, chainRefs(a)...)
		}
		return ret
	}
	return nil
}

func appendSeg(refs []ctxRef, seg string) []ctxRef {
	for i := range refs {
		refs[i].Path = append(slices.Clone(refs[i].Path), seg)
	}
	return refs
}

// untrustedMatch is how a property relates to the known attacker controlled properties.
type untrustedMatch int

const (
	untrustedNone untrustedMatch = iota
	// untrustedLeaf is an attacker controlled property itself, e.g. github.event.issue.title.
	untrustedLeaf
	// untrustedSubtree is an object which has attacker controlled properties, e.g. github.event.issue.
	untrustedSubtree
)

// matchUntrusted looks the path up in BuiltinUntrustedInputs. For a subtree it also returns one of
// the attacker controlled properties below it.
func matchUntrusted(path []string) (untrustedMatch, string) {
	if len(path) == 0 {
		return untrustedNone, ""
	}
	root, ok := BuiltinUntrustedInputs[path[0]]
	if !ok {
		return untrustedNone, ""
	}
	cur := []*UntrustedInputMap{root}
	for _, seg := range path[1:] {
		var next []*UntrustedInputMap
		for _, m := range cur {
			if c, ok := m.findObjectProp(seg); ok {
				next = append(next, c)
			} else if seg == "[]" {
				// An index reaches the elements of an array, which the table has as "*"
				if c, ok := m.findArrayElem(); ok {
					next = append(next, c)
				}
			} else if seg == "*" {
				for _, c := range m.Children {
					next = append(next, c)
				}
			}
		}
		cur = next
		if len(cur) == 0 {
			return untrustedNone, ""
		}
	}
	for _, m := range cur {
		if m.Children == nil {
			return untrustedLeaf, m.String()
		}
	}
	var leaves []string
	for _, m := range cur {
		leaves = append(leaves, firstUntrustedLeaf(m))
	}
	sort.Strings(leaves)
	return untrustedSubtree, leaves[0]
}

func firstUntrustedLeaf(m *UntrustedInputMap) string {
	for m.Children != nil {
		names := make([]string, 0, len(m.Children))
		for n := range m.Children {
			names = append(names, n)
		}
		sort.Strings(names)
		m = m.Children[names[0]]
	}
	return m.String()
}

// fixedGithubProps are the properties of the github context which an attacker cannot make
// dangerous: numbers, SHAs, names GitHub validates and values which the workflow author sets.
var fixedGithubProps = map[string]bool{
	"action": true, "action_path": true, "action_ref": true, "action_repository": true, "action_status": true,
	"actor": true, "actor_id": true, "api_url": true, "event_name": true, "graphql_url": true, "job": true,
	"path": true, "ref_protected": true, "ref_type": true, "repository": true, "repository_id": true,
	"repository_owner": true, "repository_owner_id": true, "retention_days": true, "run_attempt": true,
	"run_id": true, "run_number": true, "secret_source": true, "server_url": true, "sha": true, "token": true,
	"triggering_actor": true, "workflow": true, "workflow_ref": true, "workflow_sha": true, "workspace": true,
	"env": true, "output": true, "state": true, "step_summary": true,
}

// fixedEventLeaves are the last names of github.event properties which hold numbers, IDs, SHAs, URLs
// built by GitHub and account or repository names, which GitHub restricts to safe characters.
var fixedEventLeaves = map[string]bool{
	"number": true, "id": true, "node_id": true, "sha": true, "html_url": true, "login": true, "full_name": true,
	"created_at": true, "updated_at": true, "merged": true, "draft": true, "run_number": true, "run_attempt": true,
}

// isFixedRef reports whether the expansion of the property is never attacker controlled text.
func isFixedRef(path []string) bool {
	switch path[0] {
	case "runner", "strategy", "job", "secrets", "vars":
		return true // vars are set by the people who can change the workflow
	case "github":
		if len(path) < 2 {
			return false
		}
		if path[1] == "event" {
			if len(path) < 3 {
				return false
			}
			if path[2] == "inputs" || path[2] == "client_payload" {
				return false // typed by whoever starts the run or sends the dispatch, whatever the name is
			}
			return fixedEventLeaves[path[len(path)-1]] || (path[2] == "repository" && len(path) == 4 && path[3] == "name")
		}
		return fixedGithubProps[path[1]]
	case "needs":
		return len(path) == 3 && path[2] == "result"
	case "steps":
		return len(path) == 3 && (path[2] == "outcome" || path[2] == "conclusion")
	}
	return false
}

// isTrustedRef reports whether the property holds a value that an attacker cannot choose: the
// properties isFixedRef knows, the values of a matrix which are all literals, inputs which are
// not free text, and environment variables which are set to a literal.
func (c *tiContext) isTrustedRef(path []string) bool {
	if isFixedRef(path) {
		return true
	}
	switch path[0] {
	case "matrix":
		return len(path) == 2 && c.matrixIsLiteral(path[1])
	case "inputs":
		return len(path) == 2 && c.inputIsNotText(path[1])
	case "github":
		return len(path) == 4 && path[1] == "event" && path[2] == "inputs" && c.inputIsNotText(path[3])
	case "env":
		if len(path) != 2 {
			return false
		}
		v := c.lookupEnv(path[1])
		return v != nil && v.Value != nil && !v.Value.ContainsExpression()
	}
	return false
}

// matrixIsLiteral reports whether every value of the matrix variable is written in the workflow.
func (c *tiContext) matrixIsLiteral(name string) bool {
	if c.job == nil || c.job.Strategy == nil || c.job.Strategy.Matrix == nil {
		return false
	}
	m := c.job.Strategy.Matrix
	if m.Expression != nil {
		return false
	}
	literal := func(v RawYAMLValue) bool {
		s, ok := v.(*RawYAMLString)
		return ok && !strings.Contains(s.Value, "${{")
	}
	found := false
	if row, ok := m.Rows[name]; ok {
		if row.Expression != nil {
			return false
		}
		for _, v := range row.Values {
			if !literal(v) {
				return false
			}
		}
		found = true
	}
	if m.Include != nil {
		if m.Include.Expression != nil {
			return false
		}
		for _, comb := range m.Include.Combinations {
			if comb.Expression != nil {
				return false
			}
			if a, ok := comb.Assigns[name]; ok {
				if !literal(a.Value) {
					return false
				}
				found = true
			}
		}
	}
	return found
}

// inputIsNotText reports whether the input of the workflow is a boolean, a number, a choice or an
// environment, whose values an attacker cannot make into code. An undeclared input is text.
func (c *tiContext) inputIsNotText(name string) bool {
	if c.wf == nil {
		return false
	}
	found := false
	for _, e := range c.wf.On {
		switch e := e.(type) {
		case *WorkflowDispatchEvent:
			if in, ok := e.Inputs[name]; ok {
				switch in.Type {
				case WorkflowDispatchEventInputTypeNumber, WorkflowDispatchEventInputTypeBoolean,
					WorkflowDispatchEventInputTypeChoice, WorkflowDispatchEventInputTypeEnvironment:
					found = true
				default:
					return false
				}
			}
		case *WorkflowCallEvent:
			for _, in := range e.Inputs {
				if in.ID == name {
					if in.Type != WorkflowCallEventInputTypeBoolean && in.Type != WorkflowCallEventInputTypeNumber {
						return false
					}
					found = true
				}
			}
		}
	}
	return found
}

var knownContexts = map[string]bool{
	"github": true, "env": true, "vars": true, "job": true, "jobs": true, "steps": true, "runner": true,
	"secrets": true, "strategy": true, "matrix": true, "needs": true, "inputs": true,
}

// tiContext is where a step is, which decides how env: values are looked up.
type tiContext struct {
	wf   *Workflow
	job  *Job
	step *Step
}

// lookupEnv finds the definition of an environment variable for the step: the step, the job, then
// the workflow. Names are case-insensitive.
func (c *tiContext) lookupEnv(name string) *EnvVar {
	name = strings.ToLower(name)
	for _, e := range []*Env{c.step.Env, c.jobEnv(), c.wfEnv()} {
		if e == nil {
			continue
		}
		if v, ok := e.Vars[name]; ok {
			return v
		}
	}
	return nil
}

func (c *tiContext) jobEnv() *Env {
	if c.job == nil {
		return nil
	}
	return c.job.Env
}

func (c *tiContext) wfEnv() *Env {
	if c.wf == nil {
		return nil
	}
	return c.wf.Env
}

// envTaint returns an attacker controlled property which the environment variable is set from, or
// an empty string.
func (c *tiContext) envTaint(name string, depth int) string {
	if depth > 4 {
		return ""
	}
	v := c.lookupEnv(name)
	if v == nil || v.Value == nil {
		return ""
	}
	for _, sp := range scanExprs(v.Value) {
		for _, r := range exprContextRefs(sp.Node) {
			if m, leaf := matchUntrusted(r.Path); m != untrustedNone {
				return leaf
			}
			if r.Path[0] == "env" && len(r.Path) == 2 {
				if t := c.envTaint(r.Path[1], depth+1); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

// untrustedErrs returns the findings of the check for contexts which are known to be attacker
// controlled. This is the check which the expression rule runs on scripts.
func untrustedErrs(n ExprNode) []*ExprError {
	c := NewUntrustedInputChecker(BuiltinUntrustedInputs)
	c.Init()
	VisitExprNode(n, func(n, _ ExprNode, entering bool) {
		if entering {
			c.OnVisitNodeEnter(n)
		} else {
			c.OnVisitNodeLeave(n)
		}
	})
	c.OnVisitEnd()
	return c.Errs()
}

// tiTier tells which of the template injection findings an expression is.
type tiTier int

const (
	// tiNone is an expression which is not worth a finding.
	tiNone tiTier = iota
	// tiDirect is an attacker controlled property. The expression rule reports it, unless the class
	// has a Ref: the property was found by matchUntrusted only, which ignores the case.
	tiDirect
	// tiSubtree is an object which holds attacker controlled properties, e.g. toJSON(github.event).
	tiSubtree
	// tiEnv is an environment variable which holds an attacker controlled property.
	tiEnv
	// tiExpansion is any other expansion into a script whose value is free text.
	tiExpansion
	// tiTrusted is an expansion into a script whose value an attacker cannot control, like
	// github.repository. It is not a vulnerability but easy to get wrong when the script changes.
	tiTrusted
)

type tiClass struct {
	Tier tiTier
	// Ref is the reference which decided the tier.
	Ref *ctxRef
	// Source is the attacker controlled property for tiSubtree and tiEnv.
	Source string
}

// ID returns the rule ID of the finding. tiNone has none.
func (t tiTier) ID() string {
	switch t {
	case tiDirect, tiSubtree, tiEnv:
		return "template-injection"
	case tiExpansion:
		return "template-injection-expansion"
	case tiTrusted:
		return "template-injection-trusted"
	}
	return ""
}

// classify decides which finding an expression in a script is.
func (c *tiContext) classify(sp *exprSpan) tiClass {
	if len(untrustedErrs(sp.Node)) > 0 {
		return tiClass{Tier: tiDirect}
	}
	refs := exprContextRefs(sp.Node)
	known := refs[:0:0]
	for _, r := range refs {
		if knownContexts[r.Path[0]] {
			known = append(known, r) // an unknown context is an error of the expression rule
		}
	}
	refs = known
	for i := range refs {
		r := &refs[i]
		switch m, leaf := matchUntrusted(r.Path); m {
		case untrustedLeaf:
			// A filter or an index on the way (labels.*.name, labels[0].name) still ends in an attacker controlled
			// leaf. The Ref is kept: a reference is dropped only when it is known to be trusted.
			return tiClass{Tier: tiDirect, Ref: r}
		case untrustedSubtree:
			return tiClass{Tier: tiSubtree, Ref: r, Source: leaf}
		}
	}
	for i := range refs {
		r := &refs[i]
		if r.Path[0] == "env" && len(r.Path) == 2 {
			if t := c.envTaint(r.Path[1], 0); t != "" {
				return tiClass{Tier: tiEnv, Ref: r, Source: t}
			}
		}
	}
	for i := range refs {
		r := &refs[i]
		if !c.isTrustedRef(r.Path) {
			return tiClass{Tier: tiExpansion, Ref: r}
		}
	}
	if len(refs) > 0 {
		return tiClass{Tier: tiTrusted, Ref: &refs[0]}
	}
	// A context which is only compared or tested decides the script but cannot become code.
	for _, r := range chainRefs(sp.Node) {
		if knownContexts[r.Path[0]] {
			r := r
			return tiClass{Tier: tiTrusted, Ref: &r}
		}
	}
	// A function with no context at all (hashFiles('**/go.sum'), success()) is not constant either
	if !NewExprSemanticsChecker(false, nil, nil).IsConstant(sp.Node) {
		var call *FuncCallNode
		VisitExprNode(sp.Node, func(n, _ ExprNode, entering bool) {
			if f, ok := n.(*FuncCallNode); ok && entering && call == nil {
				call = f
			}
		})
		if call != nil {
			return tiClass{Tier: tiTrusted, Ref: &ctxRef{Path: []string{call.Callee + "()"}, Node: call}}
		}
	}
	return tiClass{Tier: tiNone}
}

// tiEnabled reports whether the configuration reports the findings of the tier.
func tiEnabled(cfg *Config, t tiTier) bool {
	id := t.ID()
	return id != "" && cfg.RuleEnabled(id)
}

// --- fixes ---------------------------------------------------------------------------------------

var (
	simpleRefRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)*$`)
	envNameRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	envNameClean = regexp.MustCompile(`[^A-Z0-9_]+`)
)

// simpleRefText returns the dotted path of an expression which is nothing but a property access
// such as github.event.issue.title, and its text. The boolean is false for anything else.
func simpleRefText(sp *exprSpan) ([]string, string, bool) {
	text := strings.TrimSpace(sp.Src)
	if !simpleRefRe.MatchString(text) {
		return nil, "", false
	}
	n := sp.Node
	var path []string
	for {
		switch v := n.(type) {
		case *ObjectDerefNode:
			path = append(path, strings.ToLower(v.Property))
			n = v.Receiver
			continue
		case *VariableNode:
			path = append(path, strings.ToLower(v.Name))
		default:
			return nil, "", false
		}
		break
	}
	slices.Reverse(path)
	if strings.ToLower(text) != strings.Join(path, ".") {
		return nil, "", false
	}
	switch path[0] {
	case "github", "inputs", "matrix", "needs", "steps", "env", "vars", "runner", "secrets", "job", "strategy":
		return path, text, true
	}
	return nil, "", false
}

var reservedEnvNames = map[string]bool{
	"PATH": true, "HOME": true, "PWD": true, "OLDPWD": true, "SHELL": true, "USER": true, "CI": true, "IFS": true,
	"LD_PRELOAD": true, "LD_LIBRARY_PATH": true, "NODE_OPTIONS": true, "BASH_ENV": true, "ENV": true,
	"TMPDIR": true, "LANG": true, "TERM": true, "SHLVL": true, "UID": true, "EUID": true, "PS1": true,
	"HOSTNAME": true, "RANDOM": true, "SECONDS": true, "LINENO": true, "OPTIND": true, "PPID": true,
}

// envNameFor makes the name of the environment variable which holds the expression. The name comes
// from the last names of the path, e.g. github.event.issue.title is ISSUE_TITLE.
func envNameFor(path []string) string {
	segs := path
	if len(segs) > 0 && segs[0] == "github" {
		segs = segs[1:]
	}
	if len(segs) > 0 && segs[0] == "event" {
		segs = segs[1:]
	}
	var keep []string
	for _, s := range segs {
		if s != "outputs" {
			keep = append(keep, s)
		}
	}
	if len(keep) > 4 {
		keep = keep[len(keep)-4:]
	}
	name := envNameClean.ReplaceAllString(strings.ToUpper(strings.Join(keep, "_")), "_")
	name = strings.Trim(name, "_")
	if name == "" {
		name = "VALUE"
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "_" + name
	}
	for _, p := range []string{"GITHUB_", "RUNNER_", "ACTIONS_", "INPUT_"} {
		if strings.HasPrefix(name, p) {
			return "EXPR_" + name
		}
	}
	if reservedEnvNames[name] {
		return "EXPR_" + name
	}
	return name
}

// posixShell reports whether the run: script of the step is run by bash or sh, which is what the
// fixes know how to write. It uses the shell of the step, then the defaults of the job and the
// workflow, then the shell of the runner, which it knows for the usual runner labels only.
func posixShell(wf *Workflow, job *Job, run *ExecRun) bool {
	var shell *String
	switch {
	case run.Shell != nil:
		shell = run.Shell
	case job != nil && job.Defaults != nil && job.Defaults.Run != nil && job.Defaults.Run.Shell != nil:
		shell = job.Defaults.Run.Shell
	case wf != nil && wf.Defaults != nil && wf.Defaults.Run != nil && wf.Defaults.Run.Shell != nil:
		shell = wf.Defaults.Run.Shell
	}
	if shell != nil {
		return posixShellTemplate(shell.Value)
	}
	if job == nil || job.RunsOn == nil || job.RunsOn.LabelsExpr != nil || len(job.RunsOn.Labels) == 0 {
		return false
	}
	unix := false
	for _, l := range job.RunsOn.Labels {
		if l.ContainsExpression() {
			return false
		}
		v := strings.ToLower(l.Value)
		if strings.Contains(v, "windows") {
			return false
		}
		if strings.Contains(v, "ubuntu") || strings.Contains(v, "linux") || strings.Contains(v, "macos") {
			unix = true
		}
	}
	return unix
}

// posixShellTemplate reports whether the value of "shell:" runs the script with a POSIX shell the fixes
// can write for: bash, sh, dash or zsh, alone or with plain options and the "{0}" placeholder for the
// script file, like "bash {0}" or "bash --noprofile --norc -eo pipefail {0}". A word with quotes,
// variables or operators, or a placeholder anywhere else than the end, is not understood.
func posixShellTemplate(value string) bool {
	words := strings.Fields(value)
	if len(words) == 0 {
		return false
	}
	switch strings.ToLower(path.Base(words[0])) {
	case "bash", "sh", "dash", "zsh":
	default:
		return false
	}
	for i, w := range words[1:] {
		if w == "{0}" {
			if i != len(words)-2 {
				return false
			}
			continue
		}
		if !shellTemplateWordRe.MatchString(w) {
			return false
		}
	}
	return true
}

var shellTemplateWordRe = regexp.MustCompile(`^[A-Za-z0-9_.+-]+$`)

// tiFixInput is what is needed to fix the expansions in one script.
type tiFixInput struct {
	idx *sourceIndex
	ctx tiContext
	cfg *Config
	str *String
	run *ExecRun
}

// planTemplateInjectionFixes returns the fix for each expansion of the script by the byte offset of
// its "${{" in the value of the string. All the expansions which can be fixed together get the same
// fix: it replaces them with shell variables and defines the variables in the env: of the step, so
// that applying one of the fixes does all of them. Expansions which cannot be fixed are missing.
//
// Fixes which are not provably equivalent (the value is not in quotes, so quoting it changes word
// splitting) are separate and marked unsafe. Only run: scripts for bash and sh are fixed.
func planTemplateInjectionFixes(in tiFixInput) map[int]*Fix {
	if in.idx == nil || in.run == nil || in.str == nil || !posixShell(in.ctx.wf, in.ctx.job, in.run) {
		return nil
	}
	spans := in.idx.scanExprs(in.str)
	if len(spans) == 0 {
		return nil
	}
	shSpans := make([]shSpan, len(spans))
	for i, sp := range spans {
		shSpans[i] = shSpan{sp.Start, sp.End}
	}
	places := analyzeShellPlaceholders(in.str.Value, shSpans)

	type candidate struct {
		span   *exprSpan
		place  shPlace
		path   []string
		text   string
		srcOff int
	}
	var cands []candidate
	for i := range spans {
		sp := &spans[i]
		if places[i].Cannot {
			continue
		}
		if !tiEnabled(in.cfg, in.ctx.classify(sp).Tier) {
			continue
		}
		path, text, ok := simpleRefText(sp)
		if !ok {
			continue
		}
		off, ok := in.idx.valueOffset(in.str, sp.Start)
		if !ok || !in.idx.matches(off, sp.Text(in.str)) {
			continue
		}
		cands = append(cands, candidate{sp, places[i], path, text, off})
	}
	if len(cands) == 0 {
		return nil
	}

	taken := map[string]bool{}
	addTaken := func(e *Env) {
		if e == nil {
			return
		}
		for _, v := range e.Vars {
			taken[strings.ToLower(v.Name.Value)] = true
		}
	}
	addTaken(in.ctx.step.Env)
	addTaken(in.ctx.jobEnv())
	addTaken(in.ctx.wfEnv())

	fixes := map[int]*Fix{}
	for _, unsafe := range []bool{false, true} {
		var edits []TextEdit
		var starts []int
		defined := map[string]string{} // lower case expression text -> variable name
		var newVars []string           // lines "NAME: ${{ expr }}"
		for _, c := range cands {
			if (c.place.Unsafe != "") != unsafe {
				continue
			}
			var name string
			if v, ok := defaultEnvVar(in.ctx.job, strings.Join(c.path, ".")); ok {
				// The runner sets the variable for every step.
				name = v
			} else if c.path[0] == "env" && len(c.path) == 2 {
				// The variable exists already: the shell can read it directly.
				name = strings.Split(c.text, ".")[1]
				if !envNameRe.MatchString(name) || in.ctx.lookupEnv(name) == nil {
					continue
				}
			} else {
				key := strings.ToLower(c.text)
				var ok bool
				if name, ok = defined[key]; !ok {
					name = reuseEnvName(in.ctx.step.Env, key)
					if name == "" {
						name = uniqueEnvName(envNameFor(c.path), taken)
						newVars = append(newVars, fmt.Sprintf("%s: ${{ %s }}", name, c.text))
					}
					defined[key] = name
				}
			}
			repl, ok := shellReplacement(c.place.Quote, name, in.str.Quoted)
			if !ok {
				continue
			}
			start, end := c.srcOff, c.srcOff+(c.span.End-c.span.Start)
			if c.place.Quote == shSingle && !in.str.Quoted && c.span.Start > 0 && c.span.End < len(in.str.Value) &&
				in.str.Value[c.span.Start-1] == '\'' && in.str.Value[c.span.End] == '\'' &&
				in.idx.matches(start-1, "'") && in.idx.matches(end, "'") {
				// '${{ x }}' is the whole single quoted word: replace the quotes too
				repl, start, end = `"${`+name+`}"`, start-1, end+1
			}
			edits = append(edits, TextEdit{start, end, repl})
			starts = append(starts, c.span.Start)
		}
		if len(edits) == 0 {
			continue
		}
		if len(newVars) > 0 {
			ins, ok := envInsertion(in, newVars)
			if !ok {
				continue
			}
			edits = append(edits, ins)
		}
		desc := "Pass the expression through an environment variable"
		if len(starts) > 1 {
			desc = fmt.Sprintf("Pass %d expressions through environment variables", len(starts))
		}
		f := &Fix{Description: desc, Unsafe: unsafe, Edits: edits}
		for _, s := range starts {
			fixes[s] = f
		}
	}
	return fixes
}

// defaultEnvVars maps the properties of the github and runner contexts to the environment variables
// which the runner sets to the same values for every step.
// https://docs.github.com/en/actions/reference/workflows-and-actions/variables#default-environment-variables
var defaultEnvVars = map[string]string{
	"github.actor": "GITHUB_ACTOR", "github.actor_id": "GITHUB_ACTOR_ID",
	"github.api_url": "GITHUB_API_URL", "github.base_ref": "GITHUB_BASE_REF", "github.event_name": "GITHUB_EVENT_NAME",
	"github.graphql_url": "GITHUB_GRAPHQL_URL", "github.head_ref": "GITHUB_HEAD_REF", "github.job": "GITHUB_JOB",
	"github.ref": "GITHUB_REF", "github.ref_name": "GITHUB_REF_NAME", "github.ref_protected": "GITHUB_REF_PROTECTED",
	"github.ref_type": "GITHUB_REF_TYPE", "github.repository": "GITHUB_REPOSITORY", "github.repository_id": "GITHUB_REPOSITORY_ID",
	"github.repository_owner": "GITHUB_REPOSITORY_OWNER", "github.repository_owner_id": "GITHUB_REPOSITORY_OWNER_ID",
	"github.run_attempt": "GITHUB_RUN_ATTEMPT", "github.run_id": "GITHUB_RUN_ID", "github.run_number": "GITHUB_RUN_NUMBER",
	"github.server_url": "GITHUB_SERVER_URL", "github.sha": "GITHUB_SHA", "github.triggering_actor": "GITHUB_TRIGGERING_ACTOR",
	"github.workflow": "GITHUB_WORKFLOW", "github.workflow_ref": "GITHUB_WORKFLOW_REF", "github.workflow_sha": "GITHUB_WORKFLOW_SHA",
	"runner.arch": "RUNNER_ARCH", "runner.name": "RUNNER_NAME", "runner.os": "RUNNER_OS", "runner.debug": "RUNNER_DEBUG",
	"runner.environment": "RUNNER_ENVIRONMENT",
}

// containerEnvVars are the same, but in a job that runs in a container the context has the path on the host
// and the variable has the path in the container.
var containerEnvVars = map[string]string{
	"github.workspace": "GITHUB_WORKSPACE", "runner.temp": "RUNNER_TEMP", "runner.tool_cache": "RUNNER_TOOL_CACHE",
}

func defaultEnvVar(job *Job, path string) (string, bool) {
	if v, ok := defaultEnvVars[path]; ok {
		return v, true
	}
	if v, ok := containerEnvVars[path]; ok && job != nil && job.Container == nil {
		return v, true
	}
	return "", false
}

// reuseEnvName finds a variable of the step env which is already set from the expression.
func reuseEnvName(env *Env, lowerText string) string {
	if env == nil {
		return ""
	}
	for _, v := range env.Vars {
		if v.Value == nil {
			continue
		}
		t := strings.ToLower(strings.TrimSpace(v.Value.Value))
		if strings.HasPrefix(t, "${{") && strings.HasSuffix(t, "}}") && strings.TrimSpace(t[3:len(t)-2]) == lowerText && envNameRe.MatchString(v.Name.Value) {
			return v.Name.Value
		}
	}
	return ""
}

func uniqueEnvName(base string, taken map[string]bool) string {
	name := base
	for i := 2; taken[strings.ToLower(name)]; i++ {
		name = fmt.Sprintf("%s_%d", base, i)
	}
	taken[strings.ToLower(name)] = true
	return name
}

// shellReplacement is the text which replaces a placeholder so that the shell expands the variable.
func shellReplacement(q shQuote, name string, yamlQuoted bool) (string, bool) {
	var r string
	switch q {
	case shDouble:
		r = "${" + name + "}"
	case shUnquoted:
		r = `"${` + name + `}"`
	case shSingle:
		r = `'"${` + name + `}"'`
	}
	if yamlQuoted && strings.ContainsAny(r, `"'\`) {
		return "", false // would need YAML escapes
	}
	return r, true
}

// envInsertion returns the edit which adds the variables to the env: of the step, or creates the
// env: after the run: script. The lines are "NAME: value".
func envInsertion(in tiFixInput, vars []string) (TextEdit, bool) {
	idx := in.idx
	nl := "\n"
	if bytesContainsCRLF(idx.src) {
		nl = "\r\n"
	}
	step := in.ctx.step
	if env := step.Env; env != nil {
		if env.Expression != nil || len(env.Vars) == 0 {
			return TextEdit{}, false
		}
		var first *EnvVar
		for _, v := range env.Vars {
			if first == nil || v.Name.Pos.IsBefore(first.Name.Pos) {
				first = v
			}
		}
		key, ok := idx.offset(first.Name.Pos.Line, first.Name.Pos.Col)
		if !ok {
			return TextEdit{}, false
		}
		ls := idx.lineStart(first.Name.Pos.Line)
		prefix := string(idx.src[ls:key])
		if strings.TrimLeft(prefix, " ") != "" {
			return TextEdit{}, false
		}
		var b strings.Builder
		for _, v := range vars {
			b.WriteString(prefix + v + nl)
		}
		return TextEdit{ls, ls, b.String()}, true
	}

	// Insert a new env: after the lines of the run: key.
	kp := in.run.RunPos
	if kp == nil {
		return TextEdit{}, false
	}
	key, ok := idx.offset(kp.Line, kp.Col)
	if !ok || !idx.matches(key, "run:") {
		return TextEdit{}, false
	}
	ls := idx.lineStart(kp.Line)
	prefix := string(idx.src[ls:key])
	if t := strings.TrimLeft(prefix, " "); t != "" && t != "- " {
		return TextEdit{}, false // a flow mapping or something else we do not edit
	}
	if len(prefix) != kp.Col-1 {
		return TextEdit{}, false
	}
	indent := strings.Repeat(" ", kp.Col-1)
	last := kp.Line
	for l := kp.Line + 1; l <= len(idx.lineStarts); l++ {
		line := idx.src[idx.lineStart(l):idx.lineEnd(l)]
		trimmed := strings.TrimLeft(string(line), " ")
		if trimmed == "" {
			continue
		}
		if len(line)-len(trimmed) <= kp.Col-1 {
			break
		}
		last = l
	}
	text := indent + "env:" + nl
	for _, v := range vars {
		text += indent + "  " + v + nl
	}
	if last < len(idx.lineStarts) {
		// Insert at the start of the next line
		at := idx.lineStart(last + 1)
		return TextEdit{at, at, text}, true
	}
	// The last line has no terminator
	at := len(idx.src)
	return TextEdit{at, at, nl + strings.TrimSuffix(text, nl)}, true
}

func bytesContainsCRLF(src []byte) bool {
	return strings.Contains(string(src), "\r\n")
}
