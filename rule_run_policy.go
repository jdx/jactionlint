package jactionlint

import (
	"strings"
)

// RuleRunPolicy is an opt-in rule to enforce style policies on 'run:' steps. It does nothing unless
// "require-shell" or "max-run-lines" is set in the configuration file.
//
//   - "require-shell": every 'run:' step must have an explicit shell, either by 'shell:' of the step or by
//     'defaults.run.shell' of the job or the workflow. The default shell differs between 'bash -e {0}' (when
//     the shell is omitted) and 'bash --noprofile --norc -eo pipefail {0}' (when 'shell: bash' is set).
//   - "max-run-lines": a 'run:' script must not have more non-blank lines than the configured value.
type RuleRunPolicy struct {
	RuleBase
	workflowShell *String
	jobShell      *String
}

// NewRuleRunPolicy creates a new RuleRunPolicy instance.
func NewRuleRunPolicy() *RuleRunPolicy {
	return &RuleRunPolicy{
		RuleBase: RuleBase{
			name: "run-policy",
			desc: "Checks opt-in policies for scripts in \"run:\" (\"require-shell\" and \"max-run-lines\" in the config)",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleRunPolicy) VisitWorkflowPre(n *Workflow) error {
	rule.workflowShell = nil
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.workflowShell = n.Defaults.Run.Shell
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleRunPolicy) VisitJobPre(n *Job) error {
	rule.jobShell = nil
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.jobShell = n.Defaults.Run.Shell
	}
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleRunPolicy) VisitJobPost(n *Job) error {
	rule.jobShell = nil
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleRunPolicy) VisitStep(n *Step) error {
	cfg := rule.Config()
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil {
		return nil
	}

	if cfg.RuleEnabled("require-shell") && run.Shell == nil && rule.jobShell == nil && rule.workflowShell == nil {
		rule.ReportID(
			"require-shell",
			run.RunPos,
			"shell is not set explicitly. set \"shell:\" at the step or \"defaults.run.shell\" because \"require-shell\" is enabled",
		)
	}

	if m, ok := cfg.ruleOptionNumber("max-run-lines", "max"); ok && cfg.RuleEnabled("max-run-lines") && m > 0 {
		max := int(m)
		if lines := countScriptLines(run.Run.Value); lines > max {
			rule.ReportIDf(
				"max-run-lines",
				run.Run.Pos,
				"script in \"run:\" has %d lines but at most %d lines are allowed because \"max-run-lines\" is set. consider moving it to a script file or an action",
				lines,
				max,
			)
		}
	}
	return nil
}

// countScriptLines counts non-blank lines in the script.
func countScriptLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

func init() {
	registerRules(
		RuleInfo{ID: "max-run-lines", Group: RuleGroupStyle, Summary: "A run: script has more lines than allowed.", DefaultLevel: SeverityError, Profile: ProfilePedantic, DocsAnchor: "check-run-policy", Options: []RuleOption{{Name: "max", Kind: RuleOptionInt, Default: DefaultMaxRunLines, Summary: "The maximum number of non-blank lines of a run: script."}}},
		RuleInfo{ID: "require-shell", Group: RuleGroupStyle, Summary: "A run: step does not set the shell explicitly.", DefaultLevel: SeverityError, Profile: ProfilePedantic, DocsAnchor: "check-run-policy"},
	)
	registerRuleFactory("run-policy", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleRunPolicy()}
	})
}
