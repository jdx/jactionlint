package jactionlint

import (
	"bytes"
	"strings"
)

const insecureCommandsEnvVar = "ACTIONS_ALLOW_UNSECURE_COMMANDS"

// RuleInsecureCommands is a rule to detect workflows which opt in to the deprecated workflow commands
// "set-env" and "add-path" by setting ACTIONS_ALLOW_UNSECURE_COMMANDS. Any process which can write
// to the output of a step can then inject environment variables, which leads to code execution.
// https://github.blog/changelog/2020-10-01-github-actions-deprecating-set-env-and-add-path-commands/
type RuleInsecureCommands struct {
	RuleBase
	src   []byte
	lines *lineIndex // the lines of src, built when a fix needs them
}

// NewRuleInsecureCommands creates a new RuleInsecureCommands instance. The source is used to put a
// fix on the errors. It can be empty.
func NewRuleInsecureCommands(src []byte) *RuleInsecureCommands {
	return &RuleInsecureCommands{
		RuleBase: RuleBase{
			name: "insecure-commands",
			desc: "Checks that ACTIONS_ALLOW_UNSECURE_COMMANDS is not enabled",
		},
		src: src,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleInsecureCommands) VisitWorkflowPre(n *Workflow) error {
	rule.checkEnv(n.Env, "workflow")
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleInsecureCommands) VisitJobPre(n *Job) error {
	rule.checkEnv(n.Env, "job")
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleInsecureCommands) VisitStep(n *Step) error {
	rule.checkEnv(n.Env, "step")
	return nil
}

func (rule *RuleInsecureCommands) checkEnv(env *Env, where string) {
	if env == nil {
		return
	}
	v, ok := env.Vars[strings.ToLower(insecureCommandsEnvVar)]
	if !ok || v.Value == nil || !isTruthyEnvValue(v.Value.Value) {
		return
	}
	msg := "the " + where + " enables the deprecated insecure workflow commands \"set-env\" and \"add-path\" with " + insecureCommandsEnvVar +
		". any output of a step can inject environment variables with them. remove the variable and write to the files $GITHUB_ENV and $GITHUB_PATH instead"
	pos := v.Name.Pos
	if fix := rule.fix(env, v); fix != nil {
		reportWithFix(&rule.RuleBase, "insecure-commands", pos, msg, fix)
		return
	}
	rule.ReportID("insecure-commands", pos, msg)
}

// isTruthyEnvValue reports whether the runner reads the value as true. It parses the variable as a
// boolean, which accepts "true" in any case and nothing else ("1" does not enable the commands).
func isTruthyEnvValue(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "true")
}

// fix removes the variable from the source. The mapping "env:" is removed too when the variable is its
// only entry. The fix is unsafe because a step which relies on the commands stops working. No fix is
// made unless the entry is a single line of a block mapping.
func (rule *RuleInsecureCommands) fix(env *Env, v *EnvVar) *Fix {
	src := rule.src
	if len(src) == 0 || v.Name == nil || v.Name.Pos == nil {
		return nil
	}
	start := rule.lineIndex().startOf(v.Name.Pos.Line)
	if start < 0 {
		return nil
	}
	lineEnd := srcLineEnd(src, start)
	line := src[start:lineEnd]
	indent := len(line) - len(bytes.TrimLeft(line, " "))
	rest := line[indent:]
	if indent == 0 || !isKeyLine(rest, v.Name.Value) {
		return nil
	}
	// The entry must end on this line: the next non-blank line is not more indented
	next := nextLineStart(src, lineEnd)
	for next >= 0 {
		ne := srcLineEnd(src, next)
		nl := bytes.TrimRight(src[next:ne], " \t")
		if len(nl) == 0 {
			next = nextLineStart(src, ne)
			continue
		}
		ni := len(nl) - len(bytes.TrimLeft(nl, " "))
		if ni > indent {
			return nil
		}
		break
	}
	end := nextLineStart(src, lineEnd)
	if end < 0 {
		end = len(src)
	}

	if len(env.Vars) == 1 {
		// Remove the "env:" line as well, which must be the line just above
		if v.Name.Pos.Line < 2 {
			return nil
		}
		ps := rule.lineIndex().startOf(v.Name.Pos.Line - 1)
		pe := srcLineEnd(src, ps)
		pl := src[ps:pe]
		pi := len(pl) - len(bytes.TrimLeft(pl, " "))
		trimmed := strings.TrimSpace(string(pl))
		if c := strings.Index(trimmed, " #"); c >= 0 {
			trimmed = strings.TrimSpace(trimmed[:c])
		}
		if pi >= indent || trimmed != "env:" {
			return nil
		}
		start = ps
	}
	return &Fix{
		Description: "Remove " + insecureCommandsEnvVar,
		Unsafe:      true,
		Edits:       []TextEdit{{Start: start, End: end, NewText: ""}},
	}
}

// isKeyLine reports whether the text is a block mapping entry "key: value" of the key.
func isKeyLine(text []byte, key string) bool {
	s := string(text)
	for _, q := range []string{"", "\"", "'"} {
		if strings.HasPrefix(s, q+key+q) {
			after := strings.TrimLeft(s[len(q+key+q):], " ")
			return strings.HasPrefix(after, ":")
		}
	}
	return false
}

// nextLineStart returns the offset of the line after the one which ends at lineEnd, or -1.
func nextLineStart(src []byte, lineEnd int) int {
	i := bytes.IndexByte(src[lineEnd:], '\n')
	if i < 0 {
		return -1
	}
	n := lineEnd + i + 1
	if n >= len(src) {
		return -1
	}
	return n
}

func init() {
	registerRules(
		RuleInfo{ID: "insecure-commands", Group: RuleGroupSecurity, Summary: "ACTIONS_ALLOW_UNSECURE_COMMANDS enables the deprecated set-env and add-path commands.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-insecure-commands"},
	)
	registerRuleFactory("insecure-commands", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("insecure-commands") {
			return nil
		}
		return []Rule{NewRuleInsecureCommands(env.src)}
	})
}

// lineIndex returns the lines of the source of the file, which are found once for all the fixes.
func (rule *RuleInsecureCommands) lineIndex() *lineIndex {
	if rule.lines == nil {
		rule.lines = newLineIndex(rule.src)
	}
	return rule.lines
}
