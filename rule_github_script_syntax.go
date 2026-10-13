package jactionlint

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"
)

// RuleGithubScriptSyntax checks the JavaScript of the `script` input of actions/github-script for syntax errors with
// `node --check`.
//
// The action runs the script as the body of an async function (`new AsyncFunction(...names, script)`), so `await` and
// `return` are allowed at the top level and the names of the arguments (`github`, `context`, `core`, ...) are taken:
// `const exec = require(...)` fails with "Identifier 'exec' has already been declared". The rule checks the script
// wrapped in the same way, with the arguments of the version of the action when the reference names one
// (see githubScriptArguments), and reports the first syntax error that node finds, at its place in the workflow.
//
// It is careful not to report what the action would run:
//
//   - An expression `${{ }}` is replaced by underscores of the same length, an identifier, which is valid where a
//     value or a name can be. A script with an expression that spans lines, that follows a number, or that is not
//     closed is not checked.
//   - Only the syntax is checked: node does not run anything, so a missing module or an undefined variable is not an
//     error.
//   - It needs `node` (version 20 or later, so that newer syntax is not taken for an error) in the PATH. Without it
//     the rule does nothing.
type RuleGithubScriptSyntax struct {
	RuleBase
	cmd  *externalCommand
	mu   sync.Mutex
	tool *externalTool // counts the scripts which were checked
}

// NewRuleGithubScriptSyntax creates a new RuleGithubScriptSyntax instance. The executable is node. When it is not found,
// an error is returned.
func NewRuleGithubScriptSyntax(executable string, proc *concurrentProcess) (*RuleGithubScriptSyntax, error) {
	// Node writes the syntax error to stderr
	cmd, err := proc.newCommandRunner(executable, true)
	if err != nil {
		return nil, err
	}
	return &RuleGithubScriptSyntax{
		RuleBase: RuleBase{
			name: "github-script-syntax",
			desc: "Checks the JavaScript of the script of actions/github-script for syntax errors with node --check",
		},
		cmd: cmd,
	}, nil
}

// VisitWorkflowPost is callback when visiting Workflow node after visiting its children.
func (rule *RuleGithubScriptSyntax) VisitWorkflowPost(n *Workflow) error {
	return rule.cmd.wait() // Wait until all processes running for this rule
}

// VisitStep is callback when visiting Step node.
func (rule *RuleGithubScriptSyntax) VisitStep(n *Step) error {
	script, args := githubScriptOf(n)
	if script == nil {
		return nil
	}
	rule.check(script, args)
	return nil
}

// githubScriptOf returns the script of a step of actions/github-script and the names that the action passes to it,
// or nil when the step is not one or has no script.
func githubScriptOf(n *Step) (*String, []string) {
	e, ok := n.Exec.(*ExecAction)
	if !ok || e.Uses == nil || e.Uses.ContainsExpression() {
		return nil, nil
	}
	u := ParseUses(e.Uses.Value)
	if u.Kind != UsesAction || !strings.EqualFold(u.CanonicalName(), "actions/github-script") {
		return nil, nil
	}
	in := e.Inputs["script"]
	if in == nil || in.Value == nil || strings.TrimSpace(in.Value.Value) == "" {
		return nil, nil
	}
	return in.Value, githubScriptArguments(u.Ref)
}

// githubScriptObserver counts the scripts that would have been checked when node is not available, for the note
// that says so (see externalTool). It reports nothing.
type githubScriptObserver struct {
	RuleBase
	tool *externalTool
}

// VisitStep is callback when visiting Step node.
func (o *githubScriptObserver) VisitStep(n *Step) error {
	if script, _ := githubScriptOf(n); script != nil && githubScriptSource(script.Value) != "" {
		o.tool.skip()
	}
	return nil
}

// githubScriptSource returns the script as node gets it, with the expressions replaced, or "" when it is not
// checked: it has an expression over several lines, directly after a number, or not closed.
func githubScriptSource(src string) string {
	if strings.Contains(src, "${{") {
		if hasMultilineExpression(src) || expressionAfterNumber.MatchString(src) {
			return ""
		}
		src = sanitizeExpressionsInScript(src)
		if strings.Contains(src, "${{") {
			return ""
		}
	}
	return src
}

// githubScriptArguments returns the names that the action passes to the script, which the script cannot declare
// again. The arguments that all versions have are returned for a reference that does not tell the version (a commit,
// a branch). They are context, core, github, io and require since v1.
func githubScriptArguments(ref string) []string {
	args := []string{"context", "core", "github", "io", "require"}
	m := githubScriptMajorRe.FindStringSubmatch(ref)
	if m == nil {
		return args
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return args
	}
	if major >= 3 {
		args = append(args, "glob") // v3
	}
	if major >= 4 {
		args = append(args, "exec", "__original_require__") // v4
	}
	if major >= 8 {
		args = append(args, "octokit") // v8
	}
	if major >= 9 {
		args = append(args, "getOctokit") // v9
	}
	return args
}

var githubScriptMajorRe = regexp.MustCompile(`^v?(\d{1,3})(?:\.\d+){0,2}$`)

// expressionAfterNumber matches a `${{` directly after a digit: `1${{ x }}` is not an identifier when the
// expression is replaced by one.
var expressionAfterNumber = regexp.MustCompile(`[0-9]\$\{\{`)

func (rule *RuleGithubScriptSyntax) check(script *String, args []string) {
	src := githubScriptSource(script.Value)
	if src == "" {
		return
	}
	rule.tool.checked()
	// The wrapper is one line, so the line of node is the line of the script plus one
	wrapped := "(async function(" + strings.Join(args, ", ") + ") {\n" + src + "\n})\n"
	pos := script.Pos
	rule.Debug("%s: Running node --check for the script of actions/github-script:\n%s", pos, wrapped)

	rule.cmd.run([]string{"--check", "-"}, wrapped, func(out []byte, err error) error {
		if err != nil {
			// node could not check it (crashed, not found): nothing is known about the script
			rule.Debug("node --check failed: %v", err)
			return nil
		}
		synErr, ok := parseNodeSyntaxError(string(out))
		if !ok {
			return nil
		}
		rule.mu.Lock()
		defer rule.mu.Unlock()
		if synErr.line > strings.Count(src, "\n")+1 {
			// Node found the error in the closing lines of the wrapper: the script is not complete
			synErr.msg = "Unexpected end of input"
			synErr.line, synErr.col, synErr.n = 0, 0, 0
		}
		start, end := githubScriptRegion(script, src, synErr)
		if start == nil {
			start = pos
		}
		rule.ReportIDf("github-script-syntax", start, "the script of actions/github-script has a syntax error: %s", synErr.msg)
		if end != nil {
			rule.errs[len(rule.errs)-1].endAt(end)
		}
		return nil
	})
}

// nodeSyntaxError is what `node --check` prints for a syntax error.
type nodeSyntaxError struct {
	msg  string
	line int // the line of the script, 1-based, 0 when unknown
	col  int // the column in UTF-16 units, 0-based
	n    int // the number of carets
}

var nodeLocationRe = regexp.MustCompile(`^\[stdin\]:(\d+)$`)

// parseNodeSyntaxError reads the output of `node --check`:
//
//	[stdin]:2
//	const x = ;
//	          ^
//
//	SyntaxError: Unexpected token ';'
//	    at wrapSafe (node:internal/modules/cjs/loader:1861:18)
func parseNodeSyntaxError(out string) (nodeSyntaxError, bool) {
	var e nodeSyntaxError
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if m := nodeLocationRe.FindStringSubmatch(l); m != nil && e.line == 0 && i+2 < len(lines) {
			n, _ := strconv.Atoi(m[1])
			if caret := lines[i+2]; strings.Contains(caret, "^") {
				e.line = n - 1 // the wrapper has the first line
				e.col = utf16Len(caret[:strings.Index(caret, "^")])
				e.n = strings.Count(caret, "^")
			}
			continue
		}
		if strings.HasPrefix(l, "SyntaxError: ") {
			e.msg = strings.TrimPrefix(l, "SyntaxError: ")
			return e, e.msg != ""
		}
	}
	return e, false
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += len(utf16.Encode([]rune{r}))
	}
	return n
}

// githubScriptRegion returns where node found the error in the workflow file. src is the script that node got.
func githubScriptRegion(str *String, src string, e nodeSyntaxError) (start, end *Pos) {
	if e.line < 1 || str.src == nil {
		return nil, nil
	}
	off := 0
	for l := 1; l < e.line; l++ {
		i := strings.IndexByte(src[off:], '\n')
		if i < 0 {
			return nil, nil
		}
		off += i + 1
	}
	// Columns are in UTF-16 units
	so, units := off, 0
	for so < len(src) && src[so] != '\n' && units < e.col {
		r, w := utf8.DecodeRuneInString(src[so:])
		units += len(utf16.Encode([]rune{r}))
		so += w
	}
	l, c, ok := str.valueAt(so)
	if !ok {
		return nil, nil
	}
	start = &Pos{Line: l, Col: c}
	eo := so
	// The carets of node are UTF-16 units too: a character outside the BMP is two of them
	for units := 0; units < max(e.n, 1) && eo < len(src) && src[eo] != '\n'; {
		r, w := utf8.DecodeRuneInString(src[eo:])
		units += len(utf16.Encode([]rune{r}))
		eo += w
	}
	if l, c, ok := str.valueAt(eo); ok {
		end = &Pos{Line: l, Col: c}
	}
	return start, end
}

// nodeUsable reports whether the node executable is version 20 or later. The answer is kept for the process: the
// rule is created for every workflow.
var nodeUsable sync.Map // resolved path -> bool

func nodeIsRecent(path string) bool {
	if v, ok := nodeUsable.Load(path); ok {
		return v.(bool)
	}
	ok := false
	if out, err := exec.Command(path, "--version").Output(); err == nil {
		var major int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "v%d", &major); err == nil {
			ok = major >= 20
		}
	}
	nodeUsable.Store(path, ok)
	return ok
}

func init() {
	registerRules(
		RuleInfo{ID: "github-script-syntax", Group: RuleGroupCorrectness, Summary: "The script of actions/github-script has a JavaScript syntax error.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-github-script-syntax"},
	)
	registerRuleFactory("github-script-syntax", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("github-script-syntax") {
			return nil // turned off: node is not needed, and a missing node is not worth a note
		}
		t := env.tools.tool("node")
		r, err := NewRuleGithubScriptSyntax("node", env.proc)
		if err != nil {
			env.Skip("node was not found, so the scripts of actions/github-script are not checked")
			t.missing()
			env.observe(&githubScriptObserver{RuleBase: RuleBase{name: "github-script-syntax"}, tool: t})
			return nil
		}
		if !nodeIsRecent(r.cmd.exe) {
			env.Skip("node is older than version 20, so it may not know the syntax that the scripts use")
			t.missing()
			env.observe(&githubScriptObserver{RuleBase: RuleBase{name: "github-script-syntax"}, tool: t})
			return nil
		}
		t.found()
		r.tool = t
		return []Rule{r}
	})
}
