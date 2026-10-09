package jactionlint

import (
	"strings"
)

// RuleMisfeature is a rule to find usages of features which GitHub Actions has but nobody should
// rely on: the pip-install input of actions/setup-python, the Windows cmd shell and shells which
// are not documented by GitHub.
type RuleMisfeature struct {
	RuleBase
}

// NewRuleMisfeature creates a new RuleMisfeature instance.
func NewRuleMisfeature() *RuleMisfeature {
	return &RuleMisfeature{
		RuleBase: RuleBase{
			name: "misfeature",
			desc: "Checks for usages of GitHub Actions features which are considered misfeatures",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleMisfeature) VisitWorkflowPre(n *Workflow) error {
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.checkShell(n.Defaults.Run.Shell)
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleMisfeature) VisitJobPre(n *Job) error {
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.checkShell(n.Defaults.Run.Shell)
	}
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleMisfeature) VisitStep(n *Step) error {
	switch e := n.Exec.(type) {
	case *ExecRun:
		rule.checkShell(e.Shell)
	case *ExecAction:
		if e.Uses == nil {
			return nil
		}
		name, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(e.Uses.Value)), "@")
		if name != "actions/setup-python" {
			return nil
		}
		if in, ok := e.Inputs["pip-install"]; ok && in.Name != nil {
			rule.ReportIDf("misfeature", in.Name.Pos, "\"pip-install\" of actions/setup-python installs packages into the global Python environment, which is hard to audit and can break the resolution of dependencies. create a virtual environment and install the packages in a \"run:\" step instead")
		}
	}
	return nil
}

// wellKnownShells are the shells which GitHub documents for "shell:".
var wellKnownShells = map[string]bool{"bash": true, "pwsh": true, "powershell": true, "python": true, "sh": true, "cmd": true}

// shellBaseName returns the name of the shell program of the first word of "shell:", lower case, without the
// directory (both / and \\ separate directories, whatever the runner) and without ".exe".
func shellBaseName(program string) string {
	program = strings.ToLower(program)
	if i := strings.LastIndexAny(program, `/\\`); i >= 0 {
		program = program[i+1:]
	}
	return strings.TrimSuffix(program, ".exe")
}

func (rule *RuleMisfeature) checkShell(s *String) {
	if s == nil || s.ContainsExpression() {
		return
	}
	fields := strings.Fields(strings.ToLower(s.Value))
	if len(fields) == 0 {
		return
	}
	name := shellBaseName(fields[0])
	switch {
	case name == "cmd":
		rule.ReportIDf("misfeature", s.Pos, "shell %q is the Windows cmd shell, which has no formal grammar so scripts cannot be analyzed reliably, and it has not been the default shell of Windows runners since 2019. use \"pwsh\", \"bash\" or another shell instead", s.Value)
	case !wellKnownShells[name] && rule.pedantic("misfeature"):
		rule.ReportIDf("misfeature", s.Pos, "shell %q is not one of the shells documented by GitHub (bash, pwsh, powershell, python, sh and cmd). it may not exist on every runner and scripts for it cannot be analyzed", s.Value)
		rule.errs[len(rule.errs)-1].RetiredID = "misfeature-custom-shell"
	}
}

func init() {
	registerRules(
		RuleInfo{ID: "misfeature", Group: RuleGroupSecurity, Summary: "A misfeature of GitHub Actions is used: the pip-install input of setup-python or the cmd shell. With the option pedantic, a shell that GitHub does not document too.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-misfeature", Options: []RuleOption{pedanticOption}},
	)
	registerRuleFactory("misfeature", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleMisfeature()}
	})
}
