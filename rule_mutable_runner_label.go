package jactionlint

import (
	"fmt"
	"regexp"
	"strings"
)

// RuleMutableRunnerLabel reports GitHub-hosted runner labels that GitHub moves to newer images
// ("ubuntu-latest"). A job on such a label can break without any change in the repository.
type RuleMutableRunnerLabel struct {
	RuleBase
	src *sourceIndex
}

// NewRuleMutableRunnerLabel creates a new RuleMutableRunnerLabel instance. The source is used to
// attach a fix; it can be nil.
func NewRuleMutableRunnerLabel(src []byte) *RuleMutableRunnerLabel {
	r := &RuleMutableRunnerLabel{
		RuleBase: RuleBase{
			name: "mutable-runner-label",
			desc: "Checks for runner labels that GitHub moves to newer images",
		},
	}
	if len(src) > 0 {
		r.src = newSourceIndex(src)
	}
	return r
}

var mutableRunnerLabelRegex = regexp.MustCompile(`(?i)^(ubuntu|windows|macos)-latest(-.+)?$`)
var fixedRunnerLabelRegex = regexp.MustCompile(`^(ubuntu|windows|macos)-[0-9]+(\.[0-9]+)?$`)

// currentRunnerLabel returns the fixed label that a "-latest" label is the same image as at the
// moment, according to the table of runner labels, or "".
func currentRunnerLabel(label string) string {
	l := strings.ToLower(label)
	m := mutableRunnerLabelRegex.FindStringSubmatch(l)
	if m == nil {
		return ""
	}
	os := m[1]
	compat, ok := defaultRunnerOSCompats[os+"-latest"]
	if !ok {
		return ""
	}
	for _, c := range allGitHubHostedRunnerLabels {
		if fixedRunnerLabelRegex.MatchString(c) && strings.HasPrefix(c, os+"-") && defaultRunnerOSCompats[c] == compat {
			cand := c + strings.TrimPrefix(l, os+"-latest")
			for _, known := range allGitHubHostedRunnerLabels {
				if known == cand {
					return cand
				}
			}
		}
	}
	return ""
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleMutableRunnerLabel) VisitJobPre(n *Job) error {
	if n.RunsOn == nil || !rule.Config().RuleEnabled("mutable-runner-label") {
		return nil
	}
	for _, l := range n.RunsOn.Labels {
		if strings.EqualFold(l.Value, "self-hosted") {
			return nil // the other labels are labels of the runner pool
		}
	}
	var m *Matrix
	if n.Strategy != nil {
		m = n.Strategy.Matrix
	}
	if n.RunsOn.LabelsExpr != nil {
		for _, l := range matrixLabels(n.RunsOn.LabelsExpr, m) {
			rule.check(l, false)
		}
		return nil
	}
	for _, l := range n.RunsOn.Labels {
		if l.ContainsExpression() {
			for _, v := range matrixLabels(l, m) {
				rule.check(v, false)
			}
			continue
		}
		rule.check(l, true)
	}
	return nil
}

func (rule *RuleMutableRunnerLabel) check(l *String, direct bool) {
	if !mutableRunnerLabelRegex.MatchString(l.Value) {
		return
	}
	msg := fmt.Sprintf("runner label %q is an alias that GitHub moves to newer images, so the job can break without a change in this repository.", l.Value)
	if cur := currentRunnerLabel(l.Value); cur != "" {
		msg += fmt.Sprintf(" use a fixed label such as %q, which is the same image today", cur)
	} else {
		msg += " use a label with a version"
	}
	rule.ReportID("mutable-runner-label", l.Pos, msg)
	if !direct {
		// The value is also used by expressions on the matrix, so it is not replaced by a fix
		return
	}
	pins, _ := rule.Config().ruleOptionStringMap("mutable-runner-label", "pin")
	to, ok := lookupFold(pins, l.Value)
	if !ok {
		return
	}
	if edit, ok := rule.labelEdit(l, to); ok {
		errs := rule.Errs()
		errs[len(errs)-1].Fix = &Fix{Description: fmt.Sprintf("Use %s as configured in the pin option", to), Edits: []TextEdit{edit}}
	}
}

func lookupFold(m map[string]string, key string) (string, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

// labelEdit returns the edit that replaces the label at the position when the source has it as a
// plain or quoted scalar.
func (rule *RuleMutableRunnerLabel) labelEdit(l *String, to string) (TextEdit, bool) {
	off, ok := rule.src.offsetOf(l.Pos)
	if !ok {
		return TextEdit{}, false
	}
	src := rule.src.src
	if off < len(src) && (src[off] == '"' || src[off] == '\'') {
		off++
	}
	end := off + len(l.Value)
	if end > len(src) || string(src[off:end]) != l.Value {
		return TextEdit{}, false // an escape, an anchor or a folded scalar
	}
	return TextEdit{Start: off, End: end, NewText: to}, true
}

// matrixLabels returns the values of the matrix that `${{ matrix.key }}` selects, with their positions.
func matrixLabels(label *String, m *Matrix) []*String {
	if m == nil || !label.IsExpressionAssigned() {
		return nil
	}
	exprs, ok := label.templateExprs()
	if !ok || len(exprs) != 1 {
		return nil
	}
	chain, _, ok := chainOf(exprs[0])
	if !ok || len(chain) != 2 || chain[0] != "matrix" {
		return nil
	}
	prop := chain[1]
	var ret []*String
	add := func(v RawYAMLValue) {
		if s, ok := v.(*RawYAMLString); ok && !ContainsExpression(s.Value) {
			ret = append(ret, &String{Value: s.Value, Pos: s.Pos()})
		}
	}
	for name, row := range m.Rows {
		if strings.EqualFold(name, prop) {
			for _, v := range row.Values {
				add(v)
			}
		}
	}
	if m.Include != nil {
		for _, c := range m.Include.Combinations {
			for key, a := range c.Assigns {
				if strings.EqualFold(key, prop) {
					add(a.Value)
				}
			}
		}
	}
	return ret
}

func init() {
	registerRules(
		RuleInfo{
			ID: "mutable-runner-label", Group: RuleGroupPolicy, Summary: "A runner label is an alias that GitHub moves to newer images, such as ubuntu-latest.",
			DefaultLevel: SeverityWarning, Profile: ProfilePedantic, Fixable: true, DocsAnchor: "check-mutable-runner-label",
			Options: []RuleOption{{
				Name: "pin", Kind: RuleOptionStringMap, Validate: validateRunnerPins,
				Summary: "Maps a moving label to the fixed label that --fix writes in its place, e.g. ubuntu-latest: ubuntu-24.04. There is no default: without an entry the finding has no fix.",
			}},
		},
	)
	registerRuleFactory("mutable-runner-label", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleMutableRunnerLabel(env.Source())}
	})
}

func validateRunnerPins(v any) error {
	m, _ := v.(map[string]string)
	for from, to := range m {
		if strings.TrimSpace(to) == "" || mutableRunnerLabelRegex.MatchString(to) {
			return fmt.Errorf("the label for %q must be a fixed label, not %q", from, to)
		}
	}
	return nil
}
