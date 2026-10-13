package jactionlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type shellIsPythonKind int

const (
	shellIsPythonKindUnspecified shellIsPythonKind = iota
	shellIsPythonKindPython
	shellIsPythonKindNotPython
)

func getShellIsPythonKind(shell *String) shellIsPythonKind {
	if shell == nil {
		return shellIsPythonKindUnspecified
	}
	if shell.Value == "python" || strings.HasPrefix(shell.Value, "python ") {
		return shellIsPythonKindPython
	}
	return shellIsPythonKindNotPython
}

// RulePyflakes is a rule to check Python scripts at 'run:' using pyflakes.
// https://github.com/PyCQA/pyflakes
type RulePyflakes struct {
	RuleBase
	cmd                   *externalCommand
	workflowShellIsPython shellIsPythonKind
	jobShellIsPython      shellIsPythonKind
	mu                    sync.Mutex
	// ruff is true when the command is ruff instead of pyflakes (the option --ruff)
	ruff bool
}

func newRulePyflakes(cmd *externalCommand) *RulePyflakes {
	return &RulePyflakes{
		RuleBase: RuleBase{
			name: "pyflakes",
			desc: "Checks for Python script when \"shell: python\" is configured using Pyflakes",
		},
		cmd:                   cmd,
		workflowShellIsPython: shellIsPythonKindUnspecified,
		jobShellIsPython:      shellIsPythonKindUnspecified,
	}
}

// NewRuleRuff creates the rule for the Python scripts of "run:" which runs ruff (the pyflakes rules F of ruff)
// instead of pyflakes. It reports the same rule ID "pyflakes". The executable is the command name or the path
// of ruff.
func NewRuleRuff(executable string, proc *concurrentProcess) (*RulePyflakes, error) {
	cmd, err := proc.newCommandRunner(executable, false)
	if err != nil {
		return nil, err
	}
	r := newRulePyflakes(cmd)
	r.ruff = true
	r.desc = "Checks for Python script when \"shell: python\" is configured using ruff"
	return r, nil
}

// ruffArgs is how ruff checks the script on stdin: only the rules F of pyflakes, whatever the configuration of the
// directory ruff runs in says, and JSON.
var ruffArgs = []string{"check", "--isolated", "--no-cache", "--select", "F", "--output-format", "json", "-"}

// ruffDiagnostic is one element of the JSON array of ruff.
type ruffDiagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Location struct {
		Row    int `json:"row"`
		Column int `json:"column"`
	} `json:"location"`
}

// NewRulePyflakes creates new RulePyflakes instance. Parameter executable can be command name
// or relative/absolute file path. When the given executable is not found in system, it returns
// an error.
func NewRulePyflakes(executable string, proc *concurrentProcess) (*RulePyflakes, error) {
	// Combine output because pyflakes outputs lint errors to stdout and outputs syntax errors to stderr. (#411)
	cmd, err := proc.newCommandRunner(executable, true)
	if err != nil {
		return nil, err
	}
	return newRulePyflakes(cmd), nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RulePyflakes) VisitJobPre(n *Job) error {
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.jobShellIsPython = getShellIsPythonKind(n.Defaults.Run.Shell)
	}
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RulePyflakes) VisitJobPost(n *Job) error {
	rule.jobShellIsPython = shellIsPythonKindUnspecified // reset
	return nil
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RulePyflakes) VisitWorkflowPre(n *Workflow) error {
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.workflowShellIsPython = getShellIsPythonKind(n.Defaults.Run.Shell)
	}
	return nil
}

// VisitWorkflowPost is callback when visiting Workflow node after visiting its children.
func (rule *RulePyflakes) VisitWorkflowPost(n *Workflow) error {
	rule.workflowShellIsPython = shellIsPythonKindUnspecified // reset
	return rule.cmd.wait()                                    // Wait until all processes running for this rule
}

// VisitStep is callback when visiting Step node.
func (rule *RulePyflakes) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil {
		return nil
	}

	if !rule.isPythonShell(run) {
		return nil
	}

	rule.runPyflakes(run.Run.Value, run.RunPos)
	return nil
}

func (rule *RulePyflakes) isPythonShell(r *ExecRun) bool {
	if k := getShellIsPythonKind(r.Shell); k != shellIsPythonKindUnspecified {
		return k == shellIsPythonKindPython
	}

	if rule.jobShellIsPython != shellIsPythonKindUnspecified {
		return rule.jobShellIsPython == shellIsPythonKindPython
	}

	return rule.workflowShellIsPython == shellIsPythonKindPython
}

func (rule *RulePyflakes) runPyflakes(src string, pos *Pos) {
	src = sanitizeExpressionsInScript(src) // Defined at rule_shellcheck.go
	rule.Debug("%s: Running %s for Python script:\n%s", pos, rule.cmd.exe, src)

	if rule.ruff {
		rule.runRuff(src, pos)
		return
	}
	rule.cmd.run([]string{}, src, func(stdout []byte, err error) error {
		if err != nil {
			rule.Debug("Command %s failed: %v", rule.cmd.exe, err)
			return fmt.Errorf("`%s` did not run successfully while checking script at %s: %w", rule.cmd.exe, pos, err)
		}
		if len(stdout) == 0 {
			return nil
		}

		for len(stdout) > 0 {
			if stdout, err = rule.parseNextError(stdout, pos); err != nil {
				return err
			}
		}
		return nil
	})
}

func (rule *RulePyflakes) runRuff(src string, pos *Pos) {
	rule.cmd.run(ruffArgs, src, func(stdout []byte, err error) error {
		if err != nil {
			rule.Debug("Command %s failed: %v", rule.cmd.exe, err)
			return fmt.Errorf("`%s` did not run successfully while checking script at %s: %w", rule.cmd.exe, pos, err)
		}
		var ds []ruffDiagnostic
		if err := json.Unmarshal(stdout, &ds); err != nil {
			return fmt.Errorf("could not parse JSON output from ruff while checking script at %s: %w: stdout=%q", pos, err, stdout)
		}
		rule.mu.Lock()
		defer rule.mu.Unlock()
		for _, d := range ds {
			msg := d.Message
			if strings.HasPrefix(d.Code, "F") { // not "invalid-syntax", which has no number
				msg = d.Code + " " + msg
			}
			rule.ReportIDf("pyflakes", pos, "ruff reported issue in this script: %d:%d: %s", d.Location.Row, d.Location.Column, msg)
		}
		return nil
	})
}

func (rule *RulePyflakes) parseNextError(stdout []byte, pos *Pos) ([]byte, error) {
	b := stdout

	// Search the start of error message.
	idx := bytes.Index(b, []byte("<stdin>:"))
	if idx == -1 {
		// Syntax errors from pyflake consist of multiple lines. Skip subsequent lines. (#411)
		// ```
		// <stdin>:1:7: unexpected EOF while parsing
		// print(
		//       ^
		// ```
		return nil, nil
	}
	b = b[idx+len("<stdin>:"):]

	idx = bytes.IndexByte(b, '\n')
	if idx == -1 {
		return nil, fmt.Errorf(`error message from pyflakes does not end with \n nor \r\n while checking script at %s. output: %q`, pos, stdout)
	}

	msg := b[:idx]
	if i := len(msg) - 1; i >= 0 && msg[i] == '\r' {
		msg = msg[:i]
	}
	b = b[idx+1:]

	// This method needs to be thread-safe since concurrentProcess.run calls its callback in a different goroutine.
	rule.mu.Lock()
	rule.ReportIDf("pyflakes", pos, "pyflakes reported issue in this script: %s", msg)
	rule.mu.Unlock()

	return b, nil
}

func init() {
	registerRules(
		RuleInfo{ID: "pyflakes", Group: RuleGroupCorrectness, Summary: "pyflakes reported an issue in a Python script.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-pyflakes-integ"},
	)
	registerRuleFactory("pyflakes", func(env *RuleEnv) []Rule {
		if env.ruff != "" {
			r, err := NewRuleRuff(env.ruff, env.proc)
			if err != nil {
				env.Skip(err.Error())
				return nil
			}
			return []Rule{r}
		}
		if env.pyflakes == "" {
			env.Skip("pyflakes command name was empty")
			return nil
		}
		r, err := NewRulePyflakes(env.pyflakes, env.proc)
		if err != nil {
			env.Skip(err.Error())
			return nil
		}
		return []Rule{r}
	})
}
