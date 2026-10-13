package jactionlint

import (
	"fmt"
	"slices"
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
	rowValues, ok := ubuntuSlimRowValues(m, prop)
	if !ok {
		return nil, false
	}
	ret = append(ret, rowValues...)
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

// maxMatrixCombinations bounds the product of the rows that is built to apply "exclude".
const maxMatrixCombinations = 4096

// ubuntuSlimRowValues returns the values of the row `prop` that some combination of the rows still runs. GitHub removes the
// combinations that "exclude" lists before it adds the "include" ones. An exclusion is applied only when it is fully
// known: every row has plain strings, and the entry of "exclude" has plain strings too. Anything else removes nothing, so the
// result then has more values, never fewer (which keeps a job from being taken for a slim-only one). It returns false when
// a value of the row cannot be read.
func ubuntuSlimRowValues(m *Matrix, prop string) ([]string, bool) {
	var target *MatrixRow
	for name, row := range m.Rows {
		if strings.EqualFold(name, prop) {
			target = row
		}
	}
	if target == nil {
		return nil, true
	}
	plain := func(v RawYAMLValue) (string, bool) {
		s, ok := v.(*RawYAMLString)
		if !ok || ContainsExpression(s.Value) {
			return "", false
		}
		return s.Value, true
	}
	all := func() ([]string, bool) {
		if target.Expression != nil {
			return nil, false
		}
		var ret []string
		for _, v := range target.Values {
			s, ok := plain(v)
			if !ok {
				return nil, false
			}
			ret = append(ret, s)
		}
		return ret, true
	}

	// The exclusions which are known
	var excludes []map[string]string
	if m.Exclude != nil && m.Exclude.Expression == nil {
		for _, c := range m.Exclude.Combinations {
			if c.Expression != nil {
				continue
			}
			e := map[string]string{}
			known := len(c.Assigns) > 0
			for key, a := range c.Assigns {
				v, ok := plain(a.Value)
				if !ok {
					known = false
					break
				}
				e[strings.ToLower(key)] = v
			}
			if known {
				excludes = append(excludes, e)
			}
		}
	}
	if len(excludes) == 0 {
		return all()
	}

	// The product of the rows, which must be all known to tell which combinations are left
	names := make([]string, 0, len(m.Rows))
	values := map[string][]string{}
	size := 1
	for name, row := range m.Rows {
		if row.Expression != nil {
			return all()
		}
		var vs []string
		for _, v := range row.Values {
			s, ok := plain(v)
			if !ok {
				return all()
			}
			vs = append(vs, s)
		}
		key := strings.ToLower(name)
		names = append(names, key)
		values[key] = vs
		if size *= max(len(vs), 1); size > maxMatrixCombinations {
			return all()
		}
	}
	slices.Sort(names)
	var ret []string
	var walk func(i int, combo map[string]string)
	walk = func(i int, combo map[string]string) {
		if i == len(names) {
			for _, e := range excludes {
				match := true
				for k, v := range e {
					if cv, ok := combo[k]; !ok || cv != v {
						match = false
						break
					}
				}
				if match {
					return
				}
			}
			ret = append(ret, combo[strings.ToLower(prop)])
			return
		}
		for _, v := range values[names[i]] {
			combo[names[i]] = v
			walk(i+1, combo)
		}
		delete(combo, names[i])
	}
	if _, ok := all(); !ok {
		return nil, false
	}
	walk(0, map[string]string{})
	return ret, true
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
	for _, st := range flattenSteps(n.Steps) {
		a, ok := st.Exec.(*ExecAction)
		if !ok || a.Uses == nil {
			continue
		}
		spec := a.Uses.Value
		switch {
		case strings.HasPrefix(spec, "docker://"):
			rule.ReportIDf("ubuntu-slim-limits", a.Uses.Pos, "the Docker container action %q cannot run on ubuntu-slim: %s. use a runner with a VM such as ubuntu-latest", spec, noDaemon)
		case isLocalUsesSpec(spec):
			if why := rule.dockerActionIn(spec, map[string]bool{}); why != "" {
				rule.ReportIDf("ubuntu-slim-limits", a.Uses.Pos, "the action %q runs %s, which cannot run on ubuntu-slim: %s. use a runner with a VM such as ubuntu-latest", spec, why, noDaemon)
			}
		}
	}
	return nil
}

func isLocalUsesSpec(spec string) bool {
	return strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, selfRepositoryUsesPrefix)
}

// dockerActionIn returns what a local action runs that needs a Docker daemon, or "". The action is one itself
// (runs.using: docker), or a composite action with a step that is a Docker container action, or that calls a local action
// which is one of those, at any depth. visited holds the actions of the path being followed, so that actions which call
// each other end the search.
func (rule *RuleUbuntuSlimLimits) dockerActionIn(spec string, visited map[string]bool) string {
	if rule.localActions == nil {
		return ""
	}
	md, _ := rule.localActions.Lookup(spec)
	if md == nil {
		return ""
	}
	key := md.Dir()
	if visited[key] {
		return ""
	}
	visited[key] = true
	defer delete(visited, key)

	if strings.EqualFold(md.Runs.Using, "docker") {
		return "a Docker container action (runs.using: docker)"
	}
	for _, uses := range stepsUses(md.Runs.Steps) {
		switch {
		case strings.HasPrefix(uses, "docker://"):
			return fmt.Sprintf("the Docker container action %q in a step of the composite action", uses)
		case isLocalUsesSpec(uses):
			if why := rule.dockerActionIn(uses, visited); why != "" {
				return fmt.Sprintf("%q, which runs %s", uses, why)
			}
		}
	}
	return ""
}

// stepsUses returns the "uses" of the steps of a composite action as they were read from action.yml, including the steps
// of "parallel:" groups, in the order they appear.
func stepsUses(steps []any) []string {
	var ret []string
	for _, raw := range steps {
		var m map[string]any
		switch v := raw.(type) {
		case map[string]any:
			m = v
		case map[any]any:
			m = make(map[string]any, len(v))
			for k, x := range v {
				if ks, ok := k.(string); ok {
					m[ks] = x
				}
			}
		default:
			continue
		}
		if u, ok := m["uses"].(string); ok {
			ret = append(ret, u)
		}
		if group, ok := m["parallel"].([]any); ok {
			ret = append(ret, stepsUses(group)...)
		}
	}
	return ret
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
