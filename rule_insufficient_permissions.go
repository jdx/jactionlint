package jactionlint

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleInsufficientPermissions reports a step whose action (or command) needs a permission of the GITHUB_TOKEN
// that the explicit `permissions:` of its job do not grant. The step then fails with "Resource not accessible by
// integration" (an OIDC login: with no token to request), which is the failure of the first run in a repository
// that has just been locked down with `permissions:`.
//
// It judges only what is certain. The permissions are those of the job, or of the workflow when the job sets none;
// a workflow without `permissions:` at both levels has the default of the repository, which is unknown here, so the
// rule says nothing (workflow-call-permissions makes the same choice for callers). A value that is an expression is
// not judged either. The needs come from the table of action_permissions.go, which lists only what the
// documentation of an action says. A step with another token than the GITHUB_TOKEN, `continue-on-error` or an
// `if: false` is skipped. The commands `gh release create` and `git push` are judged under the same conditions:
// the token of the command is the GITHUB_TOKEN of the job.
//
// The need of actions/checkout for `contents: read` holds in private repositories only, so it is reported with the
// option pedantic.
type RuleInsufficientPermissions struct {
	RuleBase
	runContext
	// emit receives what a step needs: the rule reports it against the permissions of the job, the fix of
	// missing-permissions only collects the commands which need something (see jobCommandNeeds).
	emit func(pos *Pos, what string, needs permNeeds, end *Pos)
}

// NewRuleInsufficientPermissions creates a new RuleInsufficientPermissions instance.
func NewRuleInsufficientPermissions() *RuleInsufficientPermissions {
	return &RuleInsufficientPermissions{
		RuleBase: NewRuleBase("insufficient-permissions", "Checks that the explicit permissions of a job grant what its actions and commands need from the GITHUB_TOKEN"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleInsufficientPermissions) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleInsufficientPermissions) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// grantedPermissions are the explicit permissions of a job.
type grantedPermissions struct {
	p *Permissions
	// inherited is true when the job sets none and the workflow does.
	inherited bool
}

// effectivePermissions returns the explicit permissions of the job: its own, else the ones of the workflow. It
// returns nil when none are set.
func effectivePermissions(w *Workflow, j *Job) *grantedPermissions {
	switch {
	case j != nil && j.Permissions != nil:
		return &grantedPermissions{p: j.Permissions}
	case w != nil && w.Permissions != nil:
		return &grantedPermissions{p: w.Permissions, inherited: true}
	}
	return nil
}

// level returns the level the token has for the scope. It returns false when it cannot be told: the value is an
// expression or not a permission.
func (g *grantedPermissions) level(scope string) (int, bool) {
	p := g.p
	if p.All != nil {
		if p.All.ContainsExpression() {
			return 0, false
		}
		switch strings.ToLower(p.All.Value) {
		case "read-all":
			return clampLevelForScope(scope, permLevelRead), true
		case "write-all":
			return clampLevelForScope(scope, permLevelWrite), true
		}
		return 0, false
	}
	if p.Scopes == nil {
		return 0, false
	}
	s, ok := p.Scopes[scope]
	if !ok {
		return permLevelNone, true // a scope that is not listed is none once any scope is set
	}
	if s == nil || s.Value == nil || s.Value.ContainsExpression() {
		return 0, false
	}
	switch strings.ToLower(s.Value.Value) {
	case "read", "write", "none":
		return permissionLevel(s.Value.Value), true
	}
	return 0, false
}

// missing returns the needs which the permissions do not grant. ok is false when one of the scopes cannot be told.
func (g *grantedPermissions) missing(n permNeeds) (all, any []permNeed, ok bool) {
	for _, need := range n.All {
		l, known := g.level(need.Scope)
		if !known {
			return nil, nil, false
		}
		if l < need.Level {
			all = append(all, need)
		}
	}
	if len(n.Any) > 0 {
		granted := false
		for _, need := range n.Any {
			l, known := g.level(need.Scope)
			if !known {
				return nil, nil, false
			}
			if l >= need.Level {
				granted = true
			}
		}
		if !granted {
			any = n.Any
		}
	}
	return all, any, true
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleInsufficientPermissions) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	if !rule.Config().RuleEnabled("insufficient-permissions") || rule.wf == nil || rule.wf.Action != nil || n.WorkflowCall != nil || isStaticallyFalse(n.If) {
		return nil
	}
	g := effectivePermissions(rule.wf, n)
	if g == nil {
		return nil
	}
	pedantic := rule.pedantic("insufficient-permissions")
	rule.emit = func(pos *Pos, what string, needs permNeeds, end *Pos) { rule.report(g, pos, what, needs, end) }
	steps := flattenSteps(n.Steps)
	trusted := newCheckoutTrail()
	for _, s := range steps {
		if stepRuns(s) {
			rule.checkStep(s, trusted, pedantic)
		}
		if !isStaticallyFalse(s.If) {
			trusted.observe(s)
		}
	}
	return nil
}

// checkStep judges one step.
func (rule *RuleInsufficientPermissions) checkStep(s *Step, trusted *checkoutTrail, pedantic bool) {
	switch e := s.Exec.(type) {
	case *ExecAction:
		a, ref := stepAction(s)
		if a == nil || ref.Kind != UsesAction || a.InputsUnknown() {
			return
		}
		name := ref.CanonicalName()
		if entry, ok := actionPermissionTable[name]; ok {
			needs := entry.Needs(actionStep{step: s, act: a, ctx: &rule.runContext})
			if !needs.Pedantic || pedantic {
				rule.emit(a.Uses.Pos, name, needs, nil)
			}
		} else if gitPushingFromCheckout[name] && trusted.persists(rule) {
			rule.emit(a.Uses.Pos, name, permNeeds{All: writes("contents")}, nil)
		}
	case *ExecRun:
		rule.checkScript(s, e, trusted)
	}
}

// stepRuns reports whether the step can be judged: it is not skipped by a condition that is false and it does not
// ignore its own failure, which is what a step does that is allowed to fail without the author caring.
func stepRuns(s *Step) bool {
	if isStaticallyFalse(s.If) {
		return false
	}
	if s.ContinueOnError != nil && (s.ContinueOnError.Expression != nil || s.ContinueOnError.Value) {
		return false
	}
	return true
}

// report reports the needs of one step that the permissions do not grant.
func (rule *RuleInsufficientPermissions) report(g *grantedPermissions, pos *Pos, what string, needs permNeeds, end *Pos) {
	all, any, ok := g.missing(needs)
	if !ok || (len(all) == 0 && len(any) == 0) {
		return
	}
	var want, has []string
	for _, n := range all {
		want = append(want, fmt.Sprintf("%q", n.String()))
		l, _ := g.level(n.Scope)
		has = append(has, fmt.Sprintf("%q", n.Scope+": "+permissionLevelName(l)))
	}
	if len(any) > 0 {
		var alt []string
		for _, n := range any {
			alt = append(alt, fmt.Sprintf("%q", n.String()))
			l, _ := g.level(n.Scope)
			has = append(has, fmt.Sprintf("%q", n.Scope+": "+permissionLevelName(l)))
		}
		want = append(want, strings.Join(alt, " or "))
	}
	where := "this job"
	fix := fmt.Sprintf("grant %s in the \"permissions:\" of the job", strings.Join(want, " and "))
	if g.inherited {
		where = "the workflow (this job sets none)"
		fix += ", which replace the ones of the workflow, so list every scope the job needs"
	}
	msg := fmt.Sprintf("%s needs %s, but the permissions of %s give %s, so the step fails. %s", what, strings.Join(want, " and "), where, strings.Join(has, " and "), fix)
	e := rule.errorIDAt("insufficient-permissions", pos, msg)
	if end != nil {
		e.endAt(end)
	}
}

// gitPushingFromCheckout are the actions which push with the credentials that actions/checkout leaves in the git
// configuration, and have no credential input of their own.
//
// stefanzweifel/git-auto-commit-action: "uses: actions/checkout ... permissions: contents: write # Give the default
// GITHUB_TOKEN write permission to commit and push the added or changed files to the repository."
// https://github.com/stefanzweifel/git-auto-commit-action#usage
var gitPushingFromCheckout = map[string]bool{
	"stefanzweifel/git-auto-commit-action": true,
}

// checkoutTrail follows the checkouts of a job to tell whether a push of a later step uses the GITHUB_TOKEN of the
// job: the checkouts before it persist the token (the default) in the git configuration of the one repository.
type checkoutTrail struct {
	seen   int
	custom bool // a checkout that does not persist the GITHUB_TOKEN of the job
}

func newCheckoutTrail() *checkoutTrail { return &checkoutTrail{} }

// observe records the step, which comes before the next one.
func (t *checkoutTrail) observe(s *Step) {
	a, ref := stepAction(s)
	if a == nil || !ref.isRepoAction("actions/checkout") {
		return
	}
	t.seen++
	if a.InputsUnknown() {
		t.custom = true // the inputs may give a token or an SSH key
		return
	}
	st := actionStep{step: s, act: a}
	if !st.tokenIsGitHubToken("token") || st.set("ssh-key") || st.set("repository") || st.set("path") {
		t.custom = true
		return
	}
	if v, ok := st.literal("persist-credentials"); st.set("persist-credentials") && (!ok || !isTrueLiteral(v)) {
		t.custom = true
	}
}

// persists reports whether the checkouts before the step leave the GITHUB_TOKEN of the job in the git configuration
// and nothing in the job changes the credentials or the remote of git.
func (t *checkoutTrail) persists(rule *RuleInsufficientPermissions) bool {
	if t.seen == 0 || t.custom {
		return false
	}
	return !rule.jobMentions(gitCredentialWords...)
}

// gitCredentialWords are words (lower case) which show that the job sets up the credentials or the remote of git by
// itself, so that a push does not use the token the checkout persisted.
var gitCredentialWords = []string{
	"insteadof", "credential", "sshcommand", "extraheader", "remote set-url", "remote add", "remote rename", "remote remove",
	"ssh-agent", "ssh-key", "git_ssh", "x-access-token", "gh auth", "git_askpass", "deploy_key", "deploy-key",
}

// ghTokenAssignRe matches a script which sets the token of the gh CLI itself.
var ghTokenAssignRe = regexp.MustCompile(`(?i)\b(?:GH|GITHUB)_TOKEN\s*=`)

// ghNeeds says what the gh commands need: `gh <command> <subcommand>`. Each comes from the endpoint the
// subcommand calls (https://docs.github.com/en/rest/authentication/permissions-required-for-github-apps) and
// from the manual of the CLI (https://cli.github.com/manual/).
var ghNeeds = map[string]map[string]permNeeds{
	// "Create a release", "Update a release", "Delete a release", "Upload a release asset": contents: write
	"release": {
		"create": {All: writes("contents")}, "upload": {All: writes("contents")}, "edit": {All: writes("contents")},
		"delete": {All: writes("contents")}, "delete-asset": {All: writes("contents")},
	},
	// "Create a pull request": pull-requests: write
	"pr": {"create": {All: writes("pull-requests")}},
	// "Create an issue": issues: write
	"issue": {"create": {All: writes("issues")}},
	// "Create a workflow dispatch event", "Cancel a workflow run", "Re-run a workflow": actions: write
	"workflow": {"run": {All: writes("actions")}},
	"run":      {"cancel": {All: writes("actions")}, "rerun": {All: writes("actions")}},
}

// checkScript judges the commands of a script.
func (rule *RuleInsufficientPermissions) checkScript(step *Step, run *ExecRun, trail *checkoutTrail) {
	sc, origin := rule.script(run)
	if sc == nil || run.Run == nil {
		return
	}
	push := run.WorkingDirectory == nil && !scriptChangesDirectory(sc)
	gh := rule.ghTokenIsGitHubToken(step) && !ghTokenAssignRe.MatchString(run.Run.Value) && !strings.Contains(strings.ToLower(run.Run.Value), "gh auth")
	for _, c := range sc.Commands {
		if c.Tested || assignsToken(c) {
			continue
		}
		start, end := commandRange(sc, origin, c)
		if c.Tool == "gh" && gh {
			if needs, ok := ghCommandNeeds(c); ok {
				rule.emit(start, "gh "+strings.Join(positionalWords(c, 2), " "), needs, end)
			}
			continue
		}
		if sub, args := gitCommand(c); sub == "push" && push && trail.persists(rule) && pushesOrigin(c, args) {
			rule.emit(start, "git push", permNeeds{All: writes("contents")}, end)
		}
	}
}

// assignsToken reports whether the command is given a token on its own line: `GH_TOKEN=$PAT gh ...`.
func assignsToken(c *runscript.Command) bool {
	for _, a := range c.Assigns {
		switch strings.ToUpper(a.Name) {
		case "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GH_HOST", "GH_REPO":
			return true
		}
	}
	return false
}

// ghTokenIsGitHubToken reports whether the gh CLI of the step authenticates with the GITHUB_TOKEN of the job: the
// variable GH_TOKEN (which comes first) or GITHUB_TOKEN is set to it. Without one the CLI has no credential.
func (rule *RuleInsufficientPermissions) ghTokenIsGitHubToken(step *Step) bool {
	if rule.envIsDynamic(step) {
		// an env: which is an expression or has an insert key may set the variable, so the token is not told
		return false
	}
	if v, ok := rule.envValue(step, "GH_TOKEN"); ok {
		return v != nil && isGitHubTokenValue(v.Value)
	}
	if v, ok := rule.envValue(step, "GITHUB_TOKEN"); ok {
		return v != nil && isGitHubTokenValue(v.Value)
	}
	return false
}

// ghCommandNeeds returns the needs of a gh command. A command for another repository or host (--repo) is not
// judged: the GITHUB_TOKEN cannot write to it anyway, and what it needs there is not told by this file.
func ghCommandNeeds(c *runscript.Command) (permNeeds, bool) {
	if len(c.Positional) < 2 || c.HasFlag("-R", "--repo", "--hostname") || c.HasFlag("-h", "--help") {
		return permNeeds{}, false
	}
	group, ok := ghNeeds[c.Positional[0].Value]
	if !ok {
		return permNeeds{}, false
	}
	n, ok := group[c.Positional[1].Value]
	return n, ok
}

// positionalWords returns the first n positional words of a command.
func positionalWords(c *runscript.Command, n int) []string {
	var out []string
	for i, w := range c.Positional {
		if i >= n {
			break
		}
		out = append(out, w.Value)
	}
	return out
}

// pushesOrigin reports whether the `git push` pushes to the remote of the checkout, by its name or by the default
// remote, and really pushes. A URL, a variable as the remote and a dry run are not judged.
func pushesOrigin(c *runscript.Command, args []string) bool {
	var positional []string
	for _, a := range args {
		switch {
		case a == "--dry-run" || a == "-n" || a == "--help" || a == "-h":
			return false
		case strings.HasPrefix(a, "-"):
			continue
		default:
			positional = append(positional, a)
		}
	}
	for _, w := range c.Words {
		if w.Value == "-C" || w.Subst {
			return false
		}
	}
	if len(positional) == 0 {
		return true
	}
	return positional[0] == "origin"
}

// scriptChangesDirectory reports whether the script moves to another directory, where another repository may be.
func scriptChangesDirectory(s *runscript.Script) bool {
	for _, c := range s.Commands {
		switch c.Name {
		case "cd", "pushd", "popd":
			return true
		}
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{
			ID: "insufficient-permissions", Group: RuleGroupCorrectness,
			Summary:      "The explicit permissions of a job do not grant what an action or command of the job needs from the GITHUB_TOKEN.",
			DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-insufficient-permissions",
			Options: []RuleOption{{Name: "pedantic", Kind: RuleOptionBool, Summary: "Also report actions/checkout without \"contents: read\", which only a private repository needs. Unset, it runs under the pedantic profile."}},
		},
	)
	registerRuleFactory("insufficient-permissions", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleInsufficientPermissions()}
	})
}

// jobCommandNeeds returns the commands and actions of the job which need a scope of the GITHUB_TOKEN that the table of
// the actions does not know: the gh commands, git push and the actions which push with the credentials of the checkout.
// The fix of missing-permissions uses it to tell that it cannot say what such a job needs.
func jobCommandNeeds(w *Workflow, j *Job) []string {
	rule := &RuleInsufficientPermissions{}
	rule.enterWorkflow(w)
	rule.enterJob(j)
	var found []string
	rule.emit = func(_ *Pos, what string, _ permNeeds, _ *Pos) { found = append(found, what) }
	trusted := newCheckoutTrail()
	for _, s := range flattenSteps(j.Steps) {
		if stepRuns(s) {
			switch e := s.Exec.(type) {
			case *ExecAction:
				if a, ref := stepAction(s); a != nil && ref.Kind == UsesAction && !a.InputsUnknown() && gitPushingFromCheckout[ref.CanonicalName()] && trusted.persists(rule) {
					found = append(found, ref.CanonicalName())
				}
			case *ExecRun:
				rule.checkScript(s, e, trusted)
			}
		}
		if !isStaticallyFalse(s.If) {
			trusted.observe(s)
		}
	}
	return found
}
