package jactionlint

import (
	"path"
	"regexp"
	"strings"
)

type runnerOSCompat uint

const (
	compatInvalid                   = 0
	compatUbuntu2204 runnerOSCompat = 1 << iota
	compatUbuntu2404
	compatUbuntu2604
	compatMacOS140
	compatMacOS140L
	compatMacOS140XL
	compatMacOS150
	compatMacOS150Intel
	compatMacOS150L
	compatMacOS150XL
	compatMacOS260
	compatMacOS260Intel
	compatMacOS260L
	compatMacOS260XL
	compatXcode27
	compatXcode27XL
	compatWindows2022
	compatWindows2025
	compatWindows2025VS2026
	compatWindows11Arm
	compatWindows11VS2026Arm
)

// https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners
var allGitHubHostedRunnerLabels = []string{
	"windows-latest",
	"windows-latest-8-cores",
	"windows-2025",
	"windows-2025-vs2026",
	"windows-2022",
	"windows-11-arm",
	"windows-11-vs2026-arm",
	"ubuntu-slim",
	"ubuntu-latest",
	"ubuntu-latest-4-cores",
	"ubuntu-latest-8-cores",
	"ubuntu-latest-16-cores",
	"ubuntu-26.04",
	"ubuntu-26.04-arm",
	"ubuntu-24.04",
	"ubuntu-24.04-arm",
	"ubuntu-22.04",
	"ubuntu-22.04-arm",
	"macos-latest",
	"macos-latest-xlarge",
	"macos-latest-large",
	"macos-26-intel",
	"macos-26-xlarge",
	"macos-26-large",
	"macos-26",
	"macos-15-intel",
	"macos-15-xlarge",
	"macos-15-large",
	"macos-15",
	"macos-14-xlarge",
	"macos-14-large",
	"macos-14",
	"xcode-27",
	"xcode-27-xlarge",
}

// https://docs.github.com/en/actions/hosting-your-own-runners/using-self-hosted-runners-in-a-workflow#using-default-labels-to-route-jobs
var selfHostedRunnerPresetOSLabels = []string{
	"linux",
	"macos",
	"windows",
}

// https://docs.github.com/en/actions/hosting-your-own-runners/using-self-hosted-runners-in-a-workflow#using-default-labels-to-route-jobs
var selfHostedRunnerPresetOtherLabels = []string{
	"self-hosted",
	"x64",
	"arm",
	"arm64",
}

var defaultRunnerOSCompats = map[string]runnerOSCompat{
	"ubuntu-slim":            compatUbuntu2404,
	"ubuntu-latest":          compatUbuntu2404,
	"ubuntu-latest-4-cores":  compatUbuntu2404,
	"ubuntu-latest-8-cores":  compatUbuntu2404,
	"ubuntu-latest-16-cores": compatUbuntu2404,
	"ubuntu-26.04":           compatUbuntu2604,
	"ubuntu-26.04-arm":       compatUbuntu2604,
	"ubuntu-24.04":           compatUbuntu2404,
	"ubuntu-24.04-arm":       compatUbuntu2404,
	"ubuntu-22.04":           compatUbuntu2204,
	"ubuntu-22.04-arm":       compatUbuntu2204,
	"macos-latest-xlarge":    compatMacOS150XL,
	"macos-latest-large":     compatMacOS150L,
	"macos-latest":           compatMacOS150,
	"macos-26-intel":         compatMacOS260Intel,
	"macos-26-xlarge":        compatMacOS260XL,
	"macos-26-large":         compatMacOS260L,
	"macos-26":               compatMacOS260,
	"macos-15-intel":         compatMacOS150Intel,
	"macos-15-xlarge":        compatMacOS150XL,
	"macos-15-large":         compatMacOS150L,
	"macos-15":               compatMacOS150,
	"macos-14-xlarge":        compatMacOS140XL,
	"macos-14-large":         compatMacOS140L,
	"macos-14":               compatMacOS140,
	"xcode-27":               compatXcode27,
	"xcode-27-xlarge":        compatXcode27XL,
	"windows-latest":         compatWindows2022,
	"windows-latest-8-cores": compatWindows2022,
	"windows-2025":           compatWindows2025,
	"windows-2025-vs2026":    compatWindows2025VS2026,
	"windows-2022":           compatWindows2022,
	"windows-11-arm":         compatWindows11Arm,
	"windows-11-vs2026-arm":  compatWindows11VS2026Arm,
	"linux":                  compatUbuntu2604 | compatUbuntu2404 | compatUbuntu2204, // Note: "linux" does not always indicate Ubuntu. It might be Fedora or Arch or ...
	"macos":                  compatMacOS260 | compatMacOS260Intel | compatMacOS260L | compatMacOS260XL | compatXcode27 | compatXcode27XL | compatMacOS150 | compatMacOS150Intel | compatMacOS150L | compatMacOS150XL | compatMacOS140 | compatMacOS140L | compatMacOS140XL,
	"windows":                compatWindows2025VS2026 | compatWindows2025 | compatWindows2022 | compatWindows11Arm | compatWindows11VS2026Arm,
}

// RuleRunnerLabel is a rule to check runner label like "ubuntu-latest". There are two types of
// runners, GitHub-hosted runner and Self-hosted runner. GitHub-hosted runner is described at
// https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners .
// And Self-hosted runner is described at
// https://docs.github.com/en/actions/hosting-your-own-runners/using-self-hosted-runners-in-a-workflow .
type RuleRunnerLabel struct {
	RuleBase
	// Note: Using only one compatibility integer is enough to check compatibility. But we remember
	// all past compatibility values here for better error message. If accumulating all compatibility
	// values into one integer, we can no longer know what labels are conflicting.
	compats map[runnerOSCompat]*String
	// selfHosted is whether the labels of the job being checked have "self-hosted". Its other labels are chosen by
	// whoever runs the runner, so an unknown one is not a mistake.
	selfHosted bool
}

// largerRunnerSizeRe matches the size suffix that larger runners of GitHub get in their default label:
// ubuntu-latest-8-cores, windows-2022-16-core, ubuntu-24.04-xl, macos-14-xlarge, ubuntu-24.04-32cpu.
var largerRunnerSizeRe = regexp.MustCompile(`^(.+)-(?:\d+-cores?|\d+-?x?cpus?|\d+-?vcpus?|\d*xl|x{0,2}large)$`)

// largerRunnerCompat returns the compatibility of a larger runner label, which is a label of a GitHub-hosted runner
// with a size suffix. The name of a larger runner is set by whoever creates it, so a base that is a known label is the
// only part that can be checked.
func largerRunnerCompat(label string) (runnerOSCompat, bool) {
	m := largerRunnerSizeRe.FindStringSubmatch(strings.ToLower(label))
	if m == nil {
		return compatInvalid, false
	}
	c, ok := defaultRunnerOSCompats[m[1]]
	return c, ok
}

// NewRuleRunnerLabel creates new RuleRunnerLabel instance.
func NewRuleRunnerLabel() *RuleRunnerLabel {
	return &RuleRunnerLabel{
		RuleBase: RuleBase{
			name: "runner-label",
			desc: "Checks for GitHub-hosted and preset self-hosted runner labels in \"runs-on:\"",
		},
		compats: nil,
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleRunnerLabel) VisitJobPre(n *Job) error {
	if n.RunsOn == nil {
		return nil
	}

	var m *Matrix
	if n.Strategy != nil {
		m = n.Strategy.Matrix
	}

	rule.selfHosted = false
	for _, l := range n.RunsOn.Labels {
		if strings.EqualFold(l.Value, "self-hosted") {
			rule.selfHosted = true
		}
	}
	// A matrix value which is all of runs-on makes one job each: "self-hosted" in the matrix does not make the job of
	// the other values self-hosted. Only the value of a label which is next to others is a label of the same job.
	if n.RunsOn.LabelsExpr == nil && len(n.RunsOn.Labels) > 1 {
		for _, label := range n.RunsOn.Labels {
			for _, l := range rule.tryToGetLabelsInMatrix(label, m) {
				if strings.EqualFold(l.Value, "self-hosted") {
					rule.selfHosted = true
				}
			}
		}
	}
	defer func() { rule.selfHosted = false }()

	if len(n.RunsOn.Labels) == 1 {
		rule.checkLabel(n.RunsOn.Labels[0], m)
		return nil
	}

	rule.compats = map[runnerOSCompat]*String{}
	if n.RunsOn.LabelsExpr != nil {
		rule.checkLabelAndConflict(n.RunsOn.LabelsExpr, m)
	} else {
		for _, label := range n.RunsOn.Labels {
			rule.checkLabelAndConflict(label, m)
		}
	}

	rule.compats = nil // reset
	return nil
}

// https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners
func (rule *RuleRunnerLabel) checkLabelAndConflict(l *String, m *Matrix) {
	if l.ContainsExpression() {
		ss := rule.tryToGetLabelsInMatrix(l, m)
		cs := make([]runnerOSCompat, 0, len(ss))
		for _, s := range ss {
			comp := rule.verifyRunnerLabel(s)
			cs = append(cs, comp)
		}
		rule.checkCombiCompat(cs, ss)
		return
	}

	comp := rule.verifyRunnerLabel(l)
	rule.checkCompat(comp, l)
}

func (rule *RuleRunnerLabel) checkLabel(l *String, m *Matrix) {
	if l.ContainsExpression() {
		ss := rule.tryToGetLabelsInMatrix(l, m)
		for _, s := range ss {
			rule.verifyRunnerLabel(s)
		}
		return
	}

	rule.verifyRunnerLabel(l)
}

func (rule *RuleRunnerLabel) verifyRunnerLabel(label *String) runnerOSCompat {
	l := label.Value
	known := rule.getKnownLabels()

	if rule.isStrict() {
		// Only the labels listed in the config are allowed. Built-in labels are accepted only when listed.
		for _, k := range known {
			m, err := path.Match(k, l)
			if err != nil {
				rule.ReportIDf("invalid-label-pattern", label.Pos, "label pattern %q is an invalid glob. kindly check list of labels in jactionlint.yaml config file: %v", k, err)
				return compatInvalid
			}
			if m {
				return defaultRunnerOSCompats[strings.ToLower(l)] // compatInvalid when not a built-in label
			}
		}
		rule.ReportIDf(
			"unknown-runner-label",
			label.Pos,
			"label %q is not allowed. only the labels listed in \"self-hosted-runner.labels\" of jactionlint.yaml config file are allowed because \"self-hosted-runner.strict-labels\" is enabled. allowed labels are %s",
			label.Value,
			quotesAll(known),
		)
		return compatInvalid
	}

	if c, ok := defaultRunnerOSCompats[strings.ToLower(l)]; ok {
		return c
	}

	for _, p := range selfHostedRunnerPresetOtherLabels {
		if strings.EqualFold(l, p) {
			return compatInvalid
		}
	}

	if c, ok := largerRunnerCompat(l); ok {
		return c
	}

	for _, k := range known {
		m, err := path.Match(k, l)
		if err != nil {
			rule.ReportIDf("invalid-label-pattern", label.Pos, "label pattern %q is an invalid glob. kindly check list of labels in jactionlint.yaml config file: %v", k, err)
			return compatInvalid
		}
		if m {
			return compatInvalid
		}
	}

	if rule.selfHosted {
		return compatInvalid // a label of the runner of its owner
	}

	rule.ReportIDf(
		"unknown-runner-label",
		label.Pos,
		"label %q is unknown. available labels are %s. if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file",
		label.Value,
		quotesAll(
			allGitHubHostedRunnerLabels,
			selfHostedRunnerPresetOtherLabels,
			selfHostedRunnerPresetOSLabels,
			known,
		),
	)

	return compatInvalid
}

func (rule *RuleRunnerLabel) tryToGetLabelsInMatrix(label *String, m *Matrix) []*String {
	if m == nil {
		return nil
	}

	// Only when the form of "${{...}}", evaluate the expression
	if !label.IsExpressionAssigned() {
		return nil
	}

	l := strings.TrimSpace(label.Value)
	p := NewExprParser()
	expr, err := p.Parse(NewExprLexer(l[3:])) // 3 means omit first "${{"
	if err != nil {
		return nil
	}

	deref, ok := expr.(*ObjectDerefNode)
	if !ok {
		return nil
	}
	recv, ok := deref.Receiver.(*VariableNode)
	if !ok {
		return nil
	}
	if recv.Name != "matrix" {
		return nil
	}

	prop := deref.Property
	labels := []*String{}

	if m.Rows != nil {
		if row, ok := m.Rows[prop]; ok {
			for _, v := range row.Values {
				if s, ok := v.(*RawYAMLString); ok && !ContainsExpression(s.Value) {
					labels = append(labels, &String{Value: s.Value, Pos: s.Pos()})
				}
			}
		}
	}

	if m.Include != nil {
		for _, combi := range m.Include.Combinations {
			if combi.Assigns != nil {
				if assign, ok := combi.Assigns[prop]; ok {
					if s, ok := assign.Value.(*RawYAMLString); ok && !ContainsExpression(s.Value) {
						labels = append(labels, &String{Value: s.Value, Pos: s.Pos()})
					}
				}
			}
		}
	}

	return labels
}

func (rule *RuleRunnerLabel) checkConflict(comp runnerOSCompat, label *String) bool {
	for c, l := range rule.compats {
		if c&comp == 0 {
			rule.ReportIDf("conflicting-runner-labels", label.Pos, "label %q conflicts with label %q defined at %s. note: to run your job on each workers, use matrix", label.Value, l.Value, l.Pos)
			return false
		}
	}
	return true
}

func (rule *RuleRunnerLabel) checkCompat(comp runnerOSCompat, label *String) {
	if comp == compatInvalid || !rule.checkConflict(comp, label) {
		return
	}
	if _, ok := rule.compats[comp]; !ok {
		rule.compats[comp] = label
	}
}

func (rule *RuleRunnerLabel) checkCombiCompat(comps []runnerOSCompat, labels []*String) {
	for i, c := range comps {
		if c != compatInvalid && !rule.checkConflict(c, labels[i]) {
			// Overwrite the compatibility value with compatInvalid at conflicted label not to
			// register the label to `rule.compats`.
			comps[i] = compatInvalid
		}
	}
	for i, c := range comps {
		if c != compatInvalid {
			if _, ok := rule.compats[c]; !ok {
				rule.compats[c] = labels[i]
			}
		}
	}
}

func (rule *RuleRunnerLabel) getKnownLabels() []string {
	if rule.config == nil {
		return nil
	}
	return rule.config.SelfHostedRunner.Labels
}

func (rule *RuleRunnerLabel) isStrict() bool {
	return rule.config != nil && rule.config.SelfHostedRunner.StrictLabels
}

func init() {
	registerRules(
		RuleInfo{ID: "conflicting-runner-labels", Group: RuleGroupCorrectness, Summary: "The runner labels of a job conflict with each other.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-runner-labels"},
		RuleInfo{ID: "invalid-label-pattern", Group: RuleGroupCorrectness, Summary: "A runner label pattern in the configuration is not a valid glob.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-runner-labels"},
		RuleInfo{ID: "unknown-runner-label", Group: RuleGroupCorrectness, Summary: "A runner label is unknown.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-runner-labels"},
	)
	registerRuleFactory("runner-label", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleRunnerLabel()}
	})
}
