//go:build gofuzz

package jactionlint_fuzz

import "github.com/jdx/jactionlint/v2"

func parseWorkflowPanicFree(data []byte) *jactionlint.Workflow {
	// Avoid Parse() panicking. It panics when go-yaml panics
	defer func() { recover() }()
	w, _ := jactionlint.Parse(data)
	return w
}

func FuzzCheck(data []byte) int {
	w := parseWorkflowPanicFree(data)
	if w == nil {
		return 0
	}

	ac := jactionlint.NewLocalActionsCache(nil, nil)
	wc := jactionlint.NewLocalReusableWorkflowCache(nil, "", nil)

	rules := []jactionlint.Rule{
		jactionlint.NewRuleMatrix(),
		jactionlint.NewRuleCredentials(),
		jactionlint.NewRuleShellName(),
		jactionlint.NewRuleRunnerLabel(),
		jactionlint.NewRuleEvents(),
		jactionlint.NewRuleGlob(),
		jactionlint.NewRuleJobNeeds(),
		jactionlint.NewRuleAction(ac),
		jactionlint.NewRuleEnvVar(),
		jactionlint.NewRuleID(),
		jactionlint.NewRuleExpression(ac, wc),
		jactionlint.NewRuleWorkflowCall("test.yaml", wc),
		jactionlint.NewRulePermissions(),
		jactionlint.NewRuleDeprecatedCommands(),
		jactionlint.NewRuleIfCond(),
	}

	v := jactionlint.NewVisitor()
	for _, rule := range rules {
		v.AddPass(rule)
	}

	if err := v.Visit(w); err != nil {
		return 0
	}

	return 1
}
