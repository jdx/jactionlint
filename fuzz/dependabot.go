//go:build gofuzz

package jactionlint_fuzz

import "github.com/jdx/jactionlint/v2"

func parseDependabotPanicFree(data []byte) *jactionlint.Dependabot {
	// Avoid ParseDependabot() panicking. It panics when go-yaml panics
	defer func() { recover() }()
	d, _ := jactionlint.ParseDependabot(data)
	return d
}

func FuzzParseDependabot(data []byte) int {
	if !canParseByGoYAML(data) {
		return 0
	}

	if _, errs := jactionlint.ParseDependabot(data); len(errs) > 0 {
		return 0
	}

	return 1
}

// dependabotCounter is a rule which touches every callback of DependabotRule.
type dependabotCounter struct {
	jactionlint.DependabotRuleBase
	updates int
}

func (c *dependabotCounter) VisitDependabotUpdate(n *jactionlint.DependabotUpdate) error {
	c.updates++
	if n.PackageEcosystem != nil && n.PackageEcosystem.Value == "" {
		c.Error(n.Pos, "empty ecosystem")
	}
	return nil
}

func FuzzCheckDependabot(data []byte) int {
	d := parseDependabotPanicFree(data)
	if d == nil {
		return 0
	}

	c := &dependabotCounter{DependabotRuleBase: jactionlint.NewDependabotRuleBase("fuzz", "")}
	v := jactionlint.NewDependabotVisitor()
	v.AddPass(c)
	if err := v.Visit(d); err != nil {
		panic(err)
	}
	if c.updates != len(d.Updates) {
		panic("not all updates were visited")
	}
	return 1
}
