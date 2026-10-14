package jactionlint

import (
	"regexp"
	"sort"
	"strings"
)

// RuleExpressionDelimiters detects text which is meant to be an expression but is not one, because its
// delimiters are wrong. GitHub expands `${{ }}` only. Everything else reaches the action, the environment
// variable or the condition as written, so the workflow does not do what its author meant and nothing says so.
//
// It reports, with the one ID "malformed-expression-delimiters":
//
//   - `${ github.ref }`: one brace instead of two. The fix writes `${{ github.ref }}`.
//   - `$ {{ github.sha }}` and `${ { github.sha }}`: a space between the `$` and the braces. The fix removes it.
//   - `{{ github.sha }}`: no `$`. The fix adds it.
//   - with the option pedantic, `$HOME` and `${HOME}` in a `with:` value: no shell expands them there.
//
// The first three are reported only when the text inside starts with the name of a context or a function of
// expressions, so that the templates of Go, Jinja and Helm (`{{ .Values.x }}`) and the shell syntax (`${VAR}`) are never
// reported. A `run:` script is never looked at: it is shell. The inputs which hold code (`script`, `command`, ...)
// are not looked at either, because the code may use `${...}` for itself.
type RuleExpressionDelimiters struct {
	RuleBase
	src *sourceIndex
	// inAction is whether the file is the metadata of a composite action, where github.action_path exists.
	inAction bool
}

// NewRuleExpressionDelimiters creates a new RuleExpressionDelimiters instance. The source of the file lets the rule
// attach fixes to its findings. It can be nil.
func NewRuleExpressionDelimiters(src []byte) *RuleExpressionDelimiters {
	r := &RuleExpressionDelimiters{
		RuleBase: NewRuleBase("malformed-expression-delimiters", "Checks for expressions written with the wrong delimiters, like \"${ github.ref }\" or \"{{ github.sha }}\""),
	}
	if src != nil {
		r.src = newSourceIndex(src)
	}
	return r
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleExpressionDelimiters) VisitWorkflowPre(n *Workflow) error {
	rule.inAction = n.Action != nil
	rule.walk(n.Name, false)
	rule.walk(n.RunName, false)
	rule.walk(n.Env, false)
	rule.walk(n.Concurrency, false)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleExpressionDelimiters) VisitJobPre(n *Job) error {
	rule.walk(n.Name, false)
	rule.condition(n.If)
	rule.walk(n.RunsOn, false)
	rule.walk(n.Environment, false)
	rule.walk(n.Concurrency, false)
	rule.walk(n.Outputs, false)
	rule.walk(n.Env, false)
	rule.walk(n.Container, false)
	rule.walk(n.Services, false)
	rule.walk(n.Strategy, false)
	if c := n.WorkflowCall; c != nil {
		for _, in := range c.Inputs {
			if in != nil {
				rule.walk(in.Value, false) // the workflow it calls may pass the text to a shell
			}
		}
		for _, in := range c.Secrets {
			if in != nil {
				rule.walk(in.Value, false)
			}
		}
	}
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleExpressionDelimiters) VisitStep(n *Step) error {
	rule.walk(n.Name, false)
	rule.condition(n.If)
	rule.walk(n.Env, false)
	switch e := n.Exec.(type) {
	case *ExecRun:
		rule.walk(e.WorkingDirectory, false)
	case *ExecAction:
		names := make([]string, 0, len(e.Inputs))
		for name := range e.Inputs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			in := e.Inputs[name]
			if in == nil || in.Value == nil {
				continue
			}
			// The keys of the map are in lower case. The name as written tells the words of a camelCase name (afterBuild).
			written := name
			if in.Name != nil {
				written = in.Name.Value
			}
			if !inputHoldsCode(written) {
				rule.check(in.Value, true)
			}
		}
	}
	return nil
}

func (rule *RuleExpressionDelimiters) condition(s *String) {
	if s != nil {
		rule.check(s, false)
	}
}

// walk checks every string below the node. The values of `with:` get the checks of shell variables when withInputs is set.
func (rule *RuleExpressionDelimiters) walk(node any, withInputs bool) {
	walkStrings(node, func(s *String) { rule.check(s, withInputs) })
}

// codeInputWords are the words in the name of an input which hold code of a shell or a language.
var codeInputWords = map[string]bool{
	"script": true, "scripts": true, "command": true, "commands": true, "cmd": true, "cmds": true, "args": true,
	"arguments": true, "exec": true, "eval": true, "shell": true, "hook": true, "hooks": true, "before": true, "after": true,
	"instructions": true, "options": true, "flags": true, "entrypoint": true, "expression": true, "query": true,
	"code": true, "pre": true, "post": true,
}

// inputHoldsCode reports whether the input of an action is a script, a command line or a similar text which the action
// passes to a shell or an interpreter, in which `${...}` has a meaning of its own. The name is split into words at
// `-`, `_`, `.` and the capitals of camelCase (afterBuild). `run` is code at the end of a name (pre-run) or alone, not
// in run-id or run-number.
func inputHoldsCode(name string) bool {
	var words []string
	cur := []rune{}
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	for _, r := range name {
		switch {
		case r == '-' || r == '_' || r == '.':
			flush()
		case r >= 'A' && r <= 'Z':
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	for i, w := range words {
		if codeInputWords[w] || (w == "run" && i == len(words)-1) {
			return true
		}
	}
	return false
}

// delimiterStart matches what an expression starts with: a context and a property, or a function.
var delimiterStart = regexp.MustCompile(`^\s*(?:(?:github|env|vars|secrets|matrix|needs|steps|job|jobs|runner|inputs|strategy)(?:\.[A-Za-z_*]|\[)|(?:contains|startsWith|endsWith|format|join|toJSON|fromJSON|hashFiles)\()`)

// spacedDollar matches the characters between the `$` and the `{{` of `$ {{` and `${ {`.
var spacedDollar = regexp.MustCompile(`^(?:[ \t]+\{\{|\{[ \t]+\{)`)

var shellVar = regexp.MustCompile(`^\$(?:([A-Za-z_][A-Za-z0-9_]*)|\{([A-Za-z_][A-Za-z0-9_]*)\})`)

// check looks at one string.
func (rule *RuleExpressionDelimiters) check(s *String, withInput bool) {
	if s == nil || !strings.ContainsAny(s.Value, "{$") {
		return
	}
	v := s.Value
	spans := scanExprs(s)
	next := 0 // the first span that ends after the offset
	// A value of several lines is a script, and the name of an environment variable is in capitals: `${pull_title}` is
	// a placeholder of the action.
	pedantic := withInput && !strings.Contains(v, "\n") && rule.pedantic("malformed-expression-delimiters")
	for i := 0; i < len(v); i++ {
		for next < len(spans) && spans[next].End <= i {
			next++
		}
		if next < len(spans) && i >= spans[next].Start {
			i = spans[next].End - 1
			continue
		}
		switch v[i] {
		case '$':
			rest := v[i+1:]
			switch {
			case strings.HasPrefix(rest, "{{"):
				// an expression which does not end is reported by the rule "expression"
				i += 2
			case spacedDollar.MatchString(rest):
				rule.spaced(s, i)
				i += len(spacedDollar.FindString(rest)) // the braces are not a `{{` without a `$`
			case strings.HasPrefix(rest, "{") && delimiterStart.MatchString(rest[1:]):
				rule.singleBrace(s, i)
			case pedantic && (i == 0 || v[i-1] != '$' && v[i-1] != '\\'):
				if m := shellVar.FindStringSubmatch(v[i:]); m != nil && (m[1]+m[2]) == strings.ToUpper(m[1]+m[2]) {
					rule.shellVariable(s, i, m)
				}
			}
		case '{':
			if strings.HasPrefix(v[i:], "{{") && (i == 0 || v[i-1] != '$' && v[i-1] != '{') && !strings.HasPrefix(v[i:], "{{{") &&
				delimiterStart.MatchString(v[i+2:]) && strings.Contains(v[i+2:], "}}") {
				rule.noDollar(s, i)
			}
		}
	}
}

// singleBrace reports `${ github.ref }`, which starts at the offset.
func (rule *RuleExpressionDelimiters) singleBrace(s *String, off int) {
	v := s.Value
	end := closingBrace(v, off+2)
	if end < 0 {
		return // not an expression in braces: some text which looks like one
	}
	inner := v[off+2 : end]
	pos := rule.pos(s, off)
	text := v[off : end+1]
	e := rule.errorIDAt("malformed-expression-delimiters", pos, quote(text)+" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{"+inner+"}}`").endAt(rule.pos(s, end+1))
	if strings.Contains(inner, "\n") {
		e.NoFix = delimiterNoFix("the expression spans several lines")
		return
	}
	if _, err := NewExprParser().Parse(NewExprLexer(inner + "}}")); err != nil {
		e.NoFix = delimiterNoFix("the text in the braces is not a valid expression, so jactionlint cannot tell what was meant")
		return
	}
	rule.fix(e, s, "Write the expression with two braces", off, end+1, []delimiterEdit{{off + 2, off + 2, "{"}, {end, end, "}"}})
}

// spaced reports `$ {{` and `${ {`, which start at the offset of the `$`.
func (rule *RuleExpressionDelimiters) spaced(s *String, off int) {
	v := s.Value
	m := spacedDollar.FindString(v[off+1:])
	// the part between the `$` and the text of the expression
	body := v[off+1+len(m):]
	if !delimiterStart.MatchString(body) {
		return
	}
	end := strings.Index(body, "}}")
	if end < 0 {
		return
	}
	pos := rule.pos(s, off)
	text := "$" + m + body[:end+2]
	fixed := "${{" + body[:end+2]
	e := rule.errorIDAt("malformed-expression-delimiters", pos, quote(text)+" is not an expression: the opening delimiter must be written `${{` without a space. write "+quote(fixed)).endAt(rule.pos(s, off+1+len(m)+end+2))
	// remove the spaces: they follow the `$` in `$ {{` and the first brace in `${ {`
	from, spaces := off+1, m[:len(m)-2]
	if m[0] == '{' {
		from, spaces = off+2, m[1:len(m)-1]
	}
	rule.fix(e, s, "Remove the space in the opening delimiter", off, off+1+len(m)+end+2, []delimiterEdit{{from, from + len(spaces), ""}})
}

// noDollar reports `{{ github.sha }}`, which starts at the offset.
func (rule *RuleExpressionDelimiters) noDollar(s *String, off int) {
	v := s.Value
	end := strings.Index(v[off+2:], "}}")
	text := v[off : off+2+end+2]
	e := rule.errorIDAt("malformed-expression-delimiters", rule.pos(s, off), quote(text)+" is not an expression: it has no `$`, and GitHub expands only `${{ }}`, so this text reaches the workflow as it is. write "+quote("$"+text)).endAt(rule.pos(s, off+2+end+2))
	rule.fix(e, s, "Add the missing $", off, off+2+end+2, []delimiterEdit{{off, off, "$"}})
}

// shellVariable reports (with the option pedantic) `$HOME` or `${HOME}` in an input of an action.
func (rule *RuleExpressionDelimiters) shellVariable(s *String, off int, m []string) {
	name := m[1]
	if name == "" {
		name = m[2]
	}
	use := "pass it with `${{ env." + name + " }}`, which reads a variable of the workflow, the job or the step"
	if name == "GITHUB_ACTION_PATH" && rule.inAction {
		// the context exists only in a composite action
		use = "use `${{ github.action_path }}`"
	} else if ctx, ok := contextOfRunnerVariable(name); ok {
		use = "use `${{ " + ctx + " }}`"
	} else if strings.HasPrefix(name, "GITHUB_") || strings.HasPrefix(name, "RUNNER_") || name == "HOME" || name == "PATH" || name == "USER" {
		use = "no context has it (`env` has only the variables the workflow sets), so compute it in an earlier step and pass it as an output"
	}
	// Advice only: the text may be meant as it is, and where the value comes from is a choice, so the rule has no fix for it
	rule.errorIDAt("malformed-expression-delimiters", rule.pos(s, off), quote(m[0])+" is not expanded here: GitHub passes the value of this input as it is, and no shell reads it. "+use).endAt(rule.pos(s, off+len(m[0])))
}

// contextOfRunnerVariable returns the expression with the value of a default environment variable of the runner.
func contextOfRunnerVariable(name string) (string, bool) {
	m := map[string]string{
		"GITHUB_WORKSPACE": "github.workspace", "GITHUB_SHA": "github.sha", "GITHUB_REF": "github.ref",
		"GITHUB_REF_NAME": "github.ref_name", "GITHUB_REF_TYPE": "github.ref_type", "GITHUB_REPOSITORY": "github.repository",
		"GITHUB_REPOSITORY_OWNER": "github.repository_owner", "GITHUB_RUN_ID": "github.run_id",
		"GITHUB_RUN_NUMBER": "github.run_number", "GITHUB_RUN_ATTEMPT": "github.run_attempt", "GITHUB_ACTOR": "github.actor",
		"GITHUB_EVENT_NAME": "github.event_name", "GITHUB_HEAD_REF": "github.head_ref", "GITHUB_BASE_REF": "github.base_ref",
		"GITHUB_SERVER_URL": "github.server_url", "GITHUB_API_URL": "github.api_url", "GITHUB_JOB": "github.job",
		"GITHUB_WORKFLOW": "github.workflow", "GITHUB_TRIGGERING_ACTOR": "github.triggering_actor",
		"RUNNER_TEMP": "runner.temp", "RUNNER_OS": "runner.os", "RUNNER_ARCH": "runner.arch", "RUNNER_NAME": "runner.name",
		"RUNNER_TOOL_CACHE": "runner.tool_cache", "RUNNER_DEBUG": "runner.debug", "RUNNER_ENVIRONMENT": "runner.environment",
		"GITHUB_TOKEN": "github.token", "GITHUB_EVENT_PATH": "github.event_path", "GITHUB_ACTION_REPOSITORY": "github.action_repository",
		"GITHUB_RETENTION_DAYS": "github.retention_days",
	}
	c, ok := m[name]
	return c, ok
}

// pos returns the position of the offset of the value of the string in the source.
func (rule *RuleExpressionDelimiters) pos(s *String, off int) *Pos {
	line, col := rule.src.valuePosition(s, off)
	return &Pos{Line: line, Col: col}
}

func delimiterNoFix(why string) *NoFix {
	return &NoFix{Code: NoFixUnsupportedShape, Reason: why}
}

// delimiterEdit is an edit at offsets of the value of a string.
type delimiterEdit struct {
	start, end int
	text       string
}

// fix attaches the fix made of the edits to the error. The edits are at offsets of the value of the string, inside the
// region [from, to) of it. The region must be written in the file exactly as it is in the value: when the scalar has
// escapes or folded lines in the region, or cannot hold the inserted text, no fix is offered and the finding says why.
func (rule *RuleExpressionDelimiters) fix(e *Error, s *String, description string, from, to int, edits []delimiterEdit) {
	if rule.src == nil || !rule.src.valid {
		return
	}
	base, ok := rule.src.valueOffset(s, from)
	if !ok || base+(to-from) > len(rule.src.src) || string(rule.src.src[base:base+(to-from)]) != s.Value[from:to] {
		e.NoFix = delimiterNoFix("the text is written in a way the fix cannot follow exactly (escapes or folded lines)")
		return
	}
	fix := &Fix{Description: description}
	for _, ed := range edits {
		start, end := base+ed.start-from, base+ed.end-from
		text := ed.text
		if text != "" {
			site, ok := rule.src.sites().at(start)
			if !ok {
				e.NoFix = delimiterNoFix("the fix cannot tell how the scalar is written")
				return
			}
			if text, ok = site.Insert(ed.text); !ok {
				e.NoFix = delimiterNoFix("the scalar is written in a way that cannot hold the text of the fix (a flow collection)")
				return
			}
		}
		fix.Edits = append(fix.Edits, TextEdit{Start: start, End: end, NewText: text})
	}
	e.Fix = fix
}

// closingBrace returns the offset of the `}` which closes the text that starts at i, or -1. Parentheses, brackets and
// strings in single quotes are skipped, as in an expression.
func closingBrace(v string, i int) int {
	depth := 0
	for ; i < len(v); i++ {
		switch v[i] {
		case '\'':
			for i++; i < len(v); i++ {
				if v[i] == '\'' {
					if i+1 < len(v) && v[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case '{':
			return -1
		case '}':
			if depth <= 0 {
				return i
			}
		case '\n':
			return -1
		}
	}
	return -1
}

func init() {
	registerRules(
		RuleInfo{ID: "malformed-expression-delimiters", Group: RuleGroupCorrectness, Summary: "Text such as ${ github.ref }, $ {{ x }} or {{ github.sha }} is meant to be an expression but is not one. With the option pedantic, so is a shell variable in the input of an action.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, Fixable: true, DocsAnchor: "check-malformed-expression-delimiters", Options: []RuleOption{pedanticOption}},
	)
	registerRuleFactory("malformed-expression-delimiters", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleExpressionDelimiters(env.src)}
	})
}
