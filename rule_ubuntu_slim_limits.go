package jactionlint

import (
	"strings"
)

// ubuntuSlimMaxTimeoutMinutes is the job timeout of the single-CPU runner: "The job timeout for single-CPU runners is 15
// minutes. If a job reaches this limit, the job is terminated and fails."
// https://docs.github.com/en/actions/reference/runners/github-hosted-runners#single-cpu-runners
const ubuntuSlimMaxTimeoutMinutes = 15

// RuleUbuntuSlimLimits reports what a job on ubuntu-slim cannot do. The runner runs the job inside an unprivileged
// container instead of a VM, which has no Docker daemon ("using Docker-in-Docker ... not supported"), and ends a job after
// 15 minutes. A job container, services and a Docker container action always fail; a timeout above 15 minutes is
// reported with the option pedantic only.
type RuleUbuntuSlimLimits struct {
	RuleBase
	localActions *LocalActionsCache
}

// NewRuleUbuntuSlimLimits creates a new RuleUbuntuSlimLimits instance. The cache of local actions can be nil.
func NewRuleUbuntuSlimLimits(localActions *LocalActionsCache) *RuleUbuntuSlimLimits {
	return &RuleUbuntuSlimLimits{
		RuleBase: RuleBase{
			name: "ubuntu-slim-limits",
			desc: "Checks for jobs on ubuntu-slim runners that use what the single-CPU runner does not support",
		},
		localActions: localActions,
	}
}

// ubuntuSlimMatrixValues returns every value that `${{ matrix.<key> }}` can take, or false when the matrix is not fully known
// (an expression somewhere in it, or a key that no row or include defines). Unlike matrixLabels it never drops a value
// that it cannot read, because the caller concludes something from all of them.
func ubuntuSlimMatrixValues(label *String, m *Matrix) ([]string, bool) {
	if m == nil || m.Expression != nil || !label.IsExpressionAssigned() {
		return nil, false
	}
	exprs, ok := label.templateExprs()
	if !ok || len(exprs) != 1 {
		return nil, false
	}
	chain, _, ok := chainOf(exprs[0])
	if !ok || len(chain) != 2 || chain[0] != "matrix" {
		return nil, false
	}
	prop := chain[1]
	var ret []string
	add := func(v RawYAMLValue) bool {
		s, ok := v.(*RawYAMLString)
		if !ok || ContainsExpression(s.Value) {
			return false
		}
		ret = append(ret, s.Value)
		return true
	}
	for name, row := range m.Rows {
		if !strings.EqualFold(name, prop) {
			continue
		}
		if row.Expression != nil {
			return nil, false
		}
		for _, v := range row.Values {
			if !add(v) {
				return nil, false
			}
		}
	}
	if m.Include != nil {
		if m.Include.Expression != nil {
			return nil, false
		}
		for _, c := range m.Include.Combinations {
			if c.Expression != nil {
				return nil, false
			}
			for key, a := range c.Assigns {
				if strings.EqualFold(key, prop) && !add(a.Value) {
					return nil, false
				}
			}
		}
	}
	return ret, len(ret) > 0
}

// runsOnlyOnUbuntuSlim reports whether every runner that the job can get is ubuntu-slim: its labels, and the values of the
// matrix that they select, are all known and all "ubuntu-slim". A job that may also run elsewhere, or whose runner is not
// known, is not one.
func runsOnlyOnUbuntuSlim(n *Job) bool {
	r := n.RunsOn
	if r == nil || r.Group != nil {
		return false
	}
	var m *Matrix
	if n.Strategy != nil {
		m = n.Strategy.Matrix
	}
	var labels []*String
	if r.LabelsExpr != nil {
		labels = append(labels, r.LabelsExpr)
	}
	labels = append(labels, r.Labels...)
	if len(labels) == 0 {
		return false
	}
	for _, l := range labels {
		if l.ContainsExpression() {
			vs, ok := ubuntuSlimMatrixValues(l, m)
			if !ok {
				return false
			}
			for _, v := range vs {
				if !strings.EqualFold(v, "ubuntu-slim") {
					return false
				}
			}
			continue
		}
		if !strings.EqualFold(l.Value, "ubuntu-slim") {
			return false
		}
	}
	return true
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUbuntuSlimLimits) VisitJobPre(n *Job) error {
	if n.WorkflowCall != nil || !runsOnlyOnUbuntuSlim(n) {
		return nil
	}

	// A timeout above 15 is a setting that never takes effect, but it only fails a job which runs longer than 15 minutes.
	// Generated workflows set it on jobs that finish in seconds, so it is the pedantic tier of the audit.
	if t := n.TimeoutMinutes; t != nil && t.Expression == nil && t.Value > ubuntuSlimMaxTimeoutMinutes && rule.pedantic("ubuntu-slim-limits") {
		rule.ReportIDf("ubuntu-slim-limits", t.Pos, "\"timeout-minutes\" is %v, but GitHub terminates a job on a single-CPU runner (ubuntu-slim) after %d minutes, so the job fails at that time. set %d or less, or use a runner with more CPUs for a job that takes longer", t.Value, ubuntuSlimMaxTimeoutMinutes, ubuntuSlimMaxTimeoutMinutes)
	}

	const noDaemon = "ubuntu-slim runs the job inside an unprivileged container, which has no Docker daemon"
	if n.Container != nil && n.Container.Pos != nil {
		rule.ReportIDf("ubuntu-slim-limits", n.Container.Pos, "\"container\" cannot be used on ubuntu-slim: %s, so the job container cannot start. use a runner with a VM such as ubuntu-latest", noDaemon)
	}
	if s := n.Services; s != nil && s.Expression == nil && len(s.Value) > 0 && s.Pos != nil {
		rule.ReportIDf("ubuntu-slim-limits", s.Pos, "\"services\" cannot be used on ubuntu-slim: %s, so the service containers cannot start. use a runner with a VM such as ubuntu-latest", noDaemon)
	}
	for _, st := range n.Steps {
		a, ok := st.Exec.(*ExecAction)
		if !ok || a.Uses == nil {
			continue
		}
		spec := a.Uses.Value
		switch {
		case strings.HasPrefix(spec, "docker://"):
			rule.ReportIDf("ubuntu-slim-limits", a.Uses.Pos, "the Docker container action %q cannot run on ubuntu-slim: %s. use a runner with a VM such as ubuntu-latest", spec, noDaemon)
		case rule.localActions != nil && (strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "$/")):
			if md, _ := rule.localActions.Lookup(spec); md != nil && strings.EqualFold(md.Runs.Using, "docker") {
				rule.ReportIDf("ubuntu-slim-limits", a.Uses.Pos, "the action %q is a Docker container action (runs.using: docker) and cannot run on ubuntu-slim: %s. use a runner with a VM such as ubuntu-latest", spec, noDaemon)
			}
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{
			ID: "ubuntu-slim-limits", Group: RuleGroupCorrectness, Summary: "A job on ubuntu-slim has a container, services or a Docker container action, which the single-CPU runner cannot run. With the option pedantic, a timeout-minutes above its limit of 15 minutes counts too.",
			DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-ubuntu-slim-limits", Options: []RuleOption{pedanticOption},
		},
	)
	registerRuleFactory("ubuntu-slim-limits", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleUbuntuSlimLimits(env.localActions)}
	})
}
