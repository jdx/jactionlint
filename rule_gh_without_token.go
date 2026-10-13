package jactionlint

import (
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleGhWithoutToken detects a `run:` script that runs the GitHub CLI when no token reaches the step.
//
// In a workflow, gh refuses to run commands that talk to GitHub unless one of GH_TOKEN, GITHUB_TOKEN,
// GH_ENTERPRISE_TOKEN or GITHUB_ENTERPRISE_TOKEN is in its environment, and prints "set the GH_TOKEN environment
// variable". The runner image has gh, but does not set the token: `env: GH_TOKEN: ${{ github.token }}` does.
//
// The rule is silent whenever it cannot be sure: a token variable named anywhere in the script, in the `env:` of the
// step, the job (and its container) or the workflow, or written to $GITHUB_ENV by an earlier step; an `env:` that is
// an expression; `gh auth ...`; a runner that is not shown to be GitHub-hosted (a self-hosted runner can be logged in
// already); a script that is not bash or sh.
type RuleGhWithoutToken struct {
	RuleBase
	runContext
	// exported are the steps of the current job which come after a step that may have put a token in the environment
	// of the job (a script that writes it to $GITHUB_ENV, a github-script that exports it).
	exported map[*Step]bool
	// hosted is whether the runner of the current job is shown to be GitHub-hosted.
	hosted bool
	// tokenInJob is whether the env of the workflow, of the job or of its container holds a token, or may do (an
	// env that is an expression).
	tokenInJob bool
	// windowsMaybe is whether the job may run on Windows, whose default shell is not bash.
	windowsMaybe bool
}

// NewRuleGhWithoutToken creates a new RuleGhWithoutToken instance.
func NewRuleGhWithoutToken() *RuleGhWithoutToken {
	return &RuleGhWithoutToken{
		RuleBase: NewRuleBase("gh-without-token", "Checks for the gh command at \"run:\" without GH_TOKEN or GITHUB_TOKEN"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleGhWithoutToken) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// ghTokenNames are the variables gh reads its token from.
var ghTokenNames = []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"}

// ghTokenMention matches a token variable, or a way to log gh in, anywhere in a script or a script of an action.
var ghTokenMention = regexp.MustCompile(`GH_TOKEN|GITHUB_TOKEN|GH_ENTERPRISE_TOKEN|GITHUB_ENTERPRISE_TOKEN|github\.token|hosts\.yml|gh auth login`)

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleGhWithoutToken) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	rule.exported = nil
	rule.hosted, rule.windowsMaybe = jobRunsOnGitHubHosted(n)
	rule.tokenInJob = false

	// An env that is an expression may contain anything
	unknown := false
	for _, env := range []*Env{rule.workflowEnv(), n.Env, containerEnv(n)} {
		set, u := envHoldsGhToken(env)
		rule.tokenInJob = rule.tokenInJob || set
		unknown = unknown || u
	}
	if unknown {
		rule.tokenInJob = true
	}
	if !rule.hosted || rule.tokenInJob {
		return nil
	}
	seen := false
	for _, s := range n.Steps {
		if seen {
			if rule.exported == nil {
				rule.exported = map[*Step]bool{}
			}
			rule.exported[s] = true
		}
		if stepMayExportGhToken(s) {
			seen = true
		}
	}
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleGhWithoutToken) VisitJobPost(n *Job) error {
	rule.leaveJob()
	rule.exported = nil
	return nil
}

func containerEnv(j *Job) *Env {
	if j == nil || j.Container == nil {
		return nil
	}
	return j.Container.Env
}

// envHoldsGhToken reports whether the env sets one of the variables of the token, and whether the env is an
// expression, which can set anything.
func envHoldsGhToken(env *Env) (set, unknown bool) {
	if env == nil {
		return false, false
	}
	if env.Expression != nil {
		return false, true
	}
	for _, v := range env.Vars {
		if v == nil || v.Name == nil {
			continue
		}
		if ContainsExpression(v.Name.Value) {
			unknown = true
			continue
		}
		for _, n := range ghTokenNames {
			if strings.EqualFold(v.Name.Value, n) {
				set = true
			}
		}
	}
	return set, unknown
}

// stepMayExportGhToken reports whether the step may put a token in the environment of the following steps.
func stepMayExportGhToken(s *Step) bool {
	switch e := s.Exec.(type) {
	case *ExecRun:
		if e.Run == nil {
			return false
		}
		// the script mentions the token and the file of the environment, or logs gh in
		return strings.Contains(e.Run.Value, "gh auth login") ||
			(strings.Contains(e.Run.Value, "GITHUB_ENV") && ghTokenMention.MatchString(e.Run.Value))
	case *ExecAction:
		if e.Uses == nil {
			return false
		}
		if strings.HasPrefix(e.Uses.Value, "./") {
			return true // the metadata of a local action is not read here: it can export anything
		}
		if in, ok := e.Inputs["script"]; ok && in != nil && in.Value != nil {
			return strings.Contains(in.Value.Value, "exportVariable") || strings.Contains(in.Value.Value, "GITHUB_ENV")
		}
	}
	return false
}

// ghTopLevelCommands are the commands of gh which need an authenticated user, so that gh stops when there is no
// token. The rest is either help, configuration and extensions, which gh runs without one, or a command of an
// extension, which gh passes on.
var ghTopLevelCommands = map[string]bool{
	"api": true, "attestation": true, "cache": true, "codespace": true, "gist": true, "gpg-key": true, "issue": true,
	"label": true, "org": true, "pr": true, "project": true, "release": true, "repo": true, "ruleset": true,
	"run": true, "search": true, "secret": true, "ssh-key": true, "status": true, "variable": true, "workflow": true,
}

// VisitStep is callback when visiting Step node.
func (rule *RuleGhWithoutToken) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil || !rule.hosted || rule.tokenInJob || rule.exported[n] {
		return nil
	}
	if rule.windowsMaybe && !rule.shellIsSet(run) {
		return nil // the default shell of that runner is not bash
	}
	if !strings.Contains(run.Run.Value, "gh") || ghTokenMention.MatchString(run.Run.Value) {
		return nil
	}
	if set, unknown := envHoldsGhToken(n.Env); set || unknown {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	for _, c := range s.Commands {
		if !ghNeedsToken(c) {
			continue
		}
		if envAssignsGhToken(c) {
			continue
		}
		start, end := commandRange(s, origin, c)
		rule.errorIDAt("gh-without-token", start, "command "+quote(ghCommandText(c))+" stops with \"set the GH_TOKEN environment variable\" because neither GH_TOKEN nor GITHUB_TOKEN is set for this step, its job or the workflow. set `GH_TOKEN: ${{ github.token }}` in the `env:` of the step").endAt(end)
		return nil // one finding for each step
	}
	return nil
}

// ghCommandText returns `gh pr list` for the command: gh, its command and, if static, the subcommand.
func ghCommandText(c *runscript.Command) string {
	text := "gh " + c.Verb()
	if sub := c.Sub(1); sub != "" && !strings.HasPrefix(sub, "-") && c.Verb() != "api" {
		text += " " + sub
	}
	return text
}

// shellIsSet reports whether the shell of the step is given by the step, the job or the workflow.
func (c *runContext) shellIsSet(run *ExecRun) bool {
	switch {
	case run.Shell != nil:
		return true
	case c.job != nil && c.job.Defaults != nil && c.job.Defaults.Run != nil && c.job.Defaults.Run.Shell != nil:
		return true
	case c.wf != nil && c.wf.Defaults != nil && c.wf.Defaults.Run != nil && c.wf.Defaults.Run.Shell != nil:
		return true
	}
	return false
}

// ghNeedsToken reports whether the command is a gh command which stops when gh has no token.
func ghNeedsToken(c *runscript.Command) bool {
	if c.Tool != "gh" || c.Name != "gh" {
		return false
	}
	if c.HasFlag("--help", "-h", "--version") {
		return false
	}
	return ghTopLevelCommands[c.Verb()]
}

// envAssignsGhToken reports whether the command is run with a token in its environment: `GH_TOKEN=x gh ...` or
// `env GH_TOKEN=x gh ...`.
func envAssignsGhToken(c *runscript.Command) bool {
	for _, a := range c.Assigns {
		for _, n := range ghTokenNames {
			if a.Name == n {
				return true
			}
		}
	}
	for _, w := range c.Words {
		for _, n := range ghTokenNames {
			if strings.HasPrefix(w.Value, n+"=") {
				return true
			}
		}
	}
	return false
}

// jobRunsOnGitHubHosted reports whether every runner the job can run on is shown to be GitHub-hosted, and whether
// one of them is a Windows runner. A self-hosted runner can be authenticated already, a runner group or a label of a
// larger runner is chosen by its owner, and a value that cannot be resolved is not known.
func jobRunsOnGitHubHosted(j *Job) (hosted, windows bool) {
	if j == nil || j.RunsOn == nil || j.RunsOn.Group != nil {
		return false, false
	}
	var m *Matrix
	if j.Strategy != nil {
		m = j.Strategy.Matrix
	}
	var labels []string
	add := func(l *String) bool {
		if l == nil {
			return false
		}
		if !l.ContainsExpression() {
			labels = append(labels, l.Value)
			return true
		}
		vs, ok := matrixLabelValues(l, m)
		labels = append(labels, vs...)
		return ok
	}
	if j.RunsOn.LabelsExpr != nil {
		if !add(j.RunsOn.LabelsExpr) {
			return false, false
		}
	}
	for _, l := range j.RunsOn.Labels {
		if !add(l) {
			return false, false
		}
	}
	if len(labels) == 0 {
		return false, false
	}
	for _, l := range labels {
		lower := strings.ToLower(l)
		if !isGitHubHostedLabel(lower) {
			return false, false
		}
		if strings.HasPrefix(lower, "windows") {
			windows = true
		}
	}
	return true, windows
}

// isGitHubHostedLabel reports whether the lower case label is a label of a GitHub-hosted runner, or the default
// label of a larger runner (a hosted label with a size suffix).
func isGitHubHostedLabel(lower string) bool {
	if lower == "linux" || lower == "macos" || lower == "windows" {
		return false // the labels of a self-hosted runner
	}
	if _, ok := defaultRunnerOSCompats[lower]; ok {
		return true
	}
	_, ok := largerRunnerCompat(lower)
	return ok
}

// matrixLabelValues returns every value which `${{ matrix.name }}` can have as a runner label. It reports false when
// one of them is not known (an expression, a value that is not a string, a matrix computed by an expression).
func matrixLabelValues(label *String, m *Matrix) ([]string, bool) {
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
	var out []string
	one := func(v RawYAMLValue) bool {
		s, ok := v.(*RawYAMLString)
		if !ok || ContainsExpression(s.Value) {
			return false
		}
		out = append(out, s.Value)
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
			if !one(v) {
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
				if strings.EqualFold(key, prop) && !one(a.Value) {
					return nil, false
				}
			}
		}
	}
	return out, len(out) > 0
}

func init() {
	registerRules(
		RuleInfo{ID: "gh-without-token", Group: RuleGroupCorrectness, Summary: "A script runs the gh command although no GH_TOKEN or GITHUB_TOKEN reaches the step, so gh stops.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-gh-without-token"},
	)
	registerRuleFactory("gh-without-token", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleGhWithoutToken()}
	})
}
