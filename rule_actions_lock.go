package jactionlint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The rule "actions-lock" checks the repositories which lock their actions with GitHub's Actions lock file
// (.github/workflows/actions.lock, see actions_lock.go). It never reports anything when the repository has no
// such file. It reports from two sides:
//
//   - in the workflows (and local actions): an action which a workflow listed in the lock file uses and the lock file
//     does not pin, which makes the workflow fail the lock check at runtime;
//   - in the lock file itself: entries which do not follow the format, and entries which are stale, which no workflow
//     uses any more.
//
// The rule "actions-lock-drift" is the online counterpart, for the pins of the lock file which no longer match GitHub.

const (
	actionsLockID      = "actions-lock"
	actionsLockDriftID = "actions-lock-drift"
)

// RuleActionsLock reports the actions of a workflow (or of a local action) which the Actions lock file does not
// list for the workflows which run it.
type RuleActionsLock struct {
	RuleBase
	scope    *lockScope
	wf       *Workflow
	unlisted map[string]bool
}

// NewRuleActionsLock creates a new RuleActionsLock instance. The scope is the lock file with the workflows it is
// read for, see RuleEnv.lockScope.
func NewRuleActionsLock(scope *lockScope) *RuleActionsLock {
	return &RuleActionsLock{
		RuleBase: RuleBase{
			name: "actions-lock",
			desc: "Checks that the actions of a workflow are in the Actions lock file when the repository has one",
		},
		scope:    scope,
		unlisted: map[string]bool{},
	}
}

// VisitWorkflowPre implements Pass.
func (r *RuleActionsLock) VisitWorkflowPre(n *Workflow) error {
	r.wf = n
	return nil
}

// VisitStep implements Pass.
func (r *RuleActionsLock) VisitStep(n *Step) error {
	e, ok := n.Exec.(*ExecAction)
	if !ok || e.Uses == nil || e.Uses.ContainsExpression() {
		return nil
	}
	u := ParseUses(e.Uses.Value)
	if u.Kind != UsesAction || u.Dynamic {
		return nil // Local actions, Docker images and reusable workflows are not part of the lock file
	}
	lock := r.scope.lock
	commentChecked := false
	for _, key := range r.scope.keys {
		if !lock.listsWorkflow(key) {
			// Workflows are onboarded to the lock file one by one, so a workflow which is not listed is a choice,
			// unless the repository wants all of them in it
			if r.pedantic(actionsLockID) && !r.unlisted[key] {
				r.unlisted[key] = true
				r.ReportIDf("actions-lock", e.Uses.Pos, "%s is not listed in %s, which the repository has, so it is not held to the lock file: its actions can change without notice. run `gh actions-lock` to add it", r.subject(key), actionsLockPath)
			}
			continue
		}
		state, dep, others := lock.lookup(key, u)
		switch state {
		case lockMissing:
			// One finding for each workflow which runs a local action: the message names it
			r.ReportIDf("actions-lock", e.Uses.Pos, "action %q is not in %s for %s. a workflow which the lock file lists must list every action it uses. run `gh actions-lock` to add it", u.Raw, actionsLockPath, r.subject(key))
		case lockOtherRef:
			r.ReportIDf("actions-lock", e.Uses.Pos, "action %q is not in %s for %s, which pins %s at %s. the ref was changed after the lock file was written. run `gh actions-lock` to lock the new ref", u.Raw, actionsLockPath, r.subject(key), strings.ToLower(u.Owner+"/"+u.Repo), quotes(others))
		case lockListed:
			// The comment is about the line, not about a workflow: one finding even when several workflows list the pin
			if !commentChecked {
				commentChecked = true
				r.checkVersionComment(e, u, dep)
			}
		}
	}
	return nil
}

// subject names what the lock file is read for: the workflow itself, or a workflow which runs the local action.
func (r *RuleActionsLock) subject(key string) string {
	if r.wf != nil && r.wf.Action != nil {
		return fmt.Sprintf("workflow %q, which runs this action", key)
	}
	return "this workflow"
}

// checkVersionComment reports a commit SHA in `uses:` whose version comment names another version than the
// lock file resolved the commit from. Only the pedantic checks do: the version comment is a convention, and the lock
// file of a workflow which was pinned by hand names the tag it found, which is not necessarily the one of the comment.
func (r *RuleActionsLock) checkVersionComment(e *ExecAction, u *UsesRef, dep *lockDependency) {
	if !r.pedantic(actionsLockID) || r.wf == nil || r.wf.Comments == nil || dep == nil || u.RefKind != RefFullSHA || !dep.HasRef {
		return
	}
	c := r.wf.Comments.Inline(e.Uses.Pos.Line)
	if c == nil {
		return
	}
	commentV, ok := commentVersion(c.Text)
	if !ok {
		return
	}
	lockV, ok := commentVersion(dep.Ref)
	if !ok || versionCovers(commentV, lockV) {
		return // The lock file has a branch, or the comment names the version, or its major or minor release
	}
	r.ReportIDf("actions-lock", e.Uses.Pos, "the version comment %q of %q names another version than the lock file, which resolved this commit from %q. update the comment, or run `gh actions-lock` if the commit changed", "# "+c.Text, u.Raw, dep.Ref)
}

// versionCovers reports whether the version of a comment names the version of the lock file or the series it belongs to:
// "v4" and "v4.3" are true for "v4.3.1", as Dependabot and Renovate write them. The "v" prefix does not matter.
func versionCovers(comment, locked string) bool {
	c := strings.ToLower(strings.TrimLeft(comment, "vV"))
	l := strings.ToLower(strings.TrimLeft(locked, "vV"))
	return l == c || strings.HasPrefix(l, c+".")
}

// lockFileChecks are the checks of the lock file itself. They run for the file .github/workflows/actions.lock,
// which the repository mode lints with the workflows.
type lockFileChecks struct {
	RuleBase
	lock    *actionsLock
	project *Project
	linter  *Linter
	cfg     *Config
}

func (c *lockFileChecks) report(line, col int, format string, args ...any) {
	if line <= 0 {
		line = 1
	}
	if col <= 0 {
		col = 1
	}
	c.ReportIDf("actions-lock", &Pos{Line: line, Col: col}, format, args...)
}

var reLockVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// reLockHostname is what the format allows in "hostname": github.com or the tenant of a GHE.com data residency instance.
var reLockHostname = regexp.MustCompile(`^(github\.com|[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.ghe\.com)$`)

// run reports what is wrong with the lock file.
func (c *lockFileChecks) run() {
	l := c.lock
	for _, p := range l.Problems {
		c.report(p.Line, p.Col, "%s", p.Msg)
	}
	if l.fatal {
		return
	}
	if l.Version != "" && !actionsLockVersions[l.Version] {
		if !reLockVersion.MatchString(l.Version) {
			c.report(l.VersionLine, l.VersionCol, "version %q is not a version of the lock file format (the current one is v0.0.3)", l.Version)
		}
		return // A version whose shape is not known: nothing else is read from it
	}

	c.checkDependencies()
	c.checkWorkflows()
	if c.project != nil {
		c.checkUsage()
	}
}

func (c *lockFileChecks) checkDependencies() {
	l := c.lock
	for _, d := range l.Dependencies {
		name := d.Pin.Raw
		switch {
		case !d.HasRef:
			c.report(d.Line, d.Col, "dependency %q has no \"ref\", the ref which its commit was resolved from", name)
		case d.Ref != d.Pin.Ref && !isFullSHARef(d.Pin.Ref):
			c.report(d.RefLine, d.RefCol, "\"ref\" %q of dependency %q differs from the ref of its key, so the file was edited by hand", d.Ref, name)
		}
		switch {
		case !d.HasCommit:
			c.report(d.Line, d.Col, "dependency %q has no \"commit\"", name)
		case !reLockCommit.MatchString(d.Commit):
			c.report(d.CommitLine, d.CommitCol, "\"commit\" %q of dependency %q is not a digest: it must be \"sha1-\" and 40 hex digits (or \"sha256-\" and 64)", d.Commit, name)
		case isFullSHARef(d.Pin.Ref) && !strings.EqualFold(lockCommitHex(d.Commit), d.Pin.Ref):
			c.report(d.CommitLine, d.CommitCol, "the key of dependency %q is a commit SHA but \"commit\" %q is another commit", name, d.Commit)
		}
		if d.OwnerIDLine == 0 || d.OwnerIDBad {
			c.report(max(d.OwnerIDLine, d.Line), max(d.OwnerIDCol, d.Col), "\"owner_id\" of dependency %q must be a positive integer, the ID of the owner of the repository", name)
		}
		if d.RepoIDLine == 0 || d.RepoIDBad {
			c.report(max(d.RepoIDLine, d.Line), max(d.RepoIDCol, d.Col), "\"repo_id\" of dependency %q must be a positive integer, the ID of the repository", name)
		}
		if d.Hostname != "" && !reLockHostname.MatchString(d.Hostname) {
			c.report(d.Line, d.Col, "\"hostname\" %q of dependency %q must be \"github.com\" or a lower case tenant such as \"octocorp.ghe.com\"", d.Hostname, name)
		}
		for _, u := range d.Uses {
			if _, ok := l.deps[u.Key]; !ok {
				c.report(u.Line, u.Col, "%q is used by dependency %q but has no entry under \"dependencies\"", u.Raw, name)
			}
		}
	}
	c.checkCycles()
}

// checkCycles reports a dependency which uses itself, directly or not. The format is a graph without cycles.
func (c *lockFileChecks) checkCycles() {
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	reported := false
	var visit func(d *lockDependency)
	visit = func(d *lockDependency) {
		if reported || state[d.Pin.Key] == done {
			return
		}
		state[d.Pin.Key] = visiting
		for _, u := range d.Uses {
			next := c.lock.deps[u.Key]
			if next == nil {
				continue
			}
			if state[next.Pin.Key] == visiting {
				c.report(u.Line, u.Col, "dependency %q uses %q, which uses it back: the dependencies must not form a cycle", d.Pin.Raw, u.Raw)
				reported = true
				return
			}
			visit(next)
		}
		state[d.Pin.Key] = done
	}
	for _, d := range c.lock.Dependencies {
		visit(d)
	}
}

func (c *lockFileChecks) checkWorkflows() {
	for _, w := range c.lock.Workflows {
		if err := checkLockWorkflowPath(w.Key); err != "" {
			c.report(w.Line, w.Col, "the path of the workflow %q %s", w.Key, err)
		}
		for _, p := range w.Pins {
			if _, ok := c.lock.deps[p.Key]; !ok {
				c.report(p.Line, p.Col, "%q is listed for the workflow %q but has no entry under \"dependencies\"", p.Raw, w.Key)
			}
		}
	}
}

// checkLockWorkflowPath returns why the path is not a path of a file of the repository, which the keys of
// "workflows" must be, or "".
func checkLockWorkflowPath(p string) string {
	switch {
	case strings.HasPrefix(p, "/"):
		return "must be relative to the root of the repository"
	case strings.ContainsAny(p, "\\:"):
		return "must not contain a backslash or a colon"
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "must not go up to a parent directory"
		}
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return "must not contain control characters"
		}
	}
	return ""
}

// isFullSHARef reports whether the ref is a full commit hash: 40 hex digits (SHA-1) or 64 (SHA-256), as the format
// takes it.
func isFullSHARef(ref string) bool {
	return (len(ref) == 40 || len(ref) == 64) && isHex(ref)
}

// lockUsage is what a workflow uses, as the lock file records it.
type lockUsage struct {
	// keys are the pins "owner/repo@ref" of the repository actions which the workflow uses, with the owner and the
	// repo in lower case, and for a commit SHA also "owner/repo@sha" in lower case.
	keys map[string]bool
	// complete is false when something the workflow runs could not be read (an expression in uses:, a local action
	// which is missing or invalid), so that an entry which nothing uses may be used by what was not read.
	complete bool
}

// workflowUsage reads the workflow of the lock file and what its local actions use. It returns nil when the
// workflow does not exist, and an incomplete usage when it cannot be parsed.
func (c *lockFileChecks) workflowUsage(key string) *lockUsage {
	file := filepath.Join(c.project.root, filepath.FromSlash(key))
	src, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	u := &lockUsage{keys: map[string]bool{}, complete: true}
	w, errs := Parse(src)
	if w == nil {
		u.complete = false
		return u
	}
	if len(errs) > 0 {
		// A step which did not parse (a syntax error, an unknown key, a value of the wrong type) may be the one that
		// uses the action, so the pins are not judged
		u.complete = false
	}
	g := c.linter.callGraphOf(c.project)
	seen := map[string]bool{}
	var scan func(w *Workflow)
	add := func(ref *UsesRef) {
		nwo := strings.ToLower(ref.Owner + "/" + ref.Repo)
		u.keys[nwo+"@"+ref.Ref] = true
		if ref.RefKind == RefFullSHA {
			u.keys[nwo+"@"+strings.ToLower(ref.Ref)] = true
		}
	}
	scan = func(w *Workflow) {
		for _, j := range w.Jobs {
			if j == nil {
				continue
			}
			if j.WorkflowCall != nil && j.WorkflowCall.Uses != nil {
				if j.WorkflowCall.Uses.ContainsExpression() {
					u.complete = false
				}
				// The current gh actions-lock does not record reusable workflows, but lock files of other versions
				// list them: what is listed is used, so that it is not reported as stale.
				if ref := ParseUses(j.WorkflowCall.Uses.Value); ref.Kind == UsesReusableWorkflow && !ref.Dynamic {
					add(ref)
				}
			}
			walkSteps(j.Steps, func(s *Step) {
				e, ok := s.Exec.(*ExecAction)
				if !ok || e.Uses == nil {
					return
				}
				if e.Uses.ContainsExpression() {
					u.complete = false
					return
				}
				if t, ok := localTarget(e.Uses.Value); ok {
					if t.kind != 'a' || seen[t.path] {
						return
					}
					seen[t.path] = true
					if act := g.actions[t.path]; act != nil {
						if !c.actionReadsClean(g, t.path) {
							u.complete = false
						}
						scan(act)
					} else {
						u.complete = false
					}
					return
				}
				ref := ParseUses(e.Uses.Value)
				if ref.Kind != UsesAction {
					return
				}
				add(ref)
			})
		}
	}
	scan(w)
	return u
}

// actionReadsClean reports whether the metadata of the local action in the directory parses without an error. The call
// graph keeps the tree and drops the errors, so the file is read once more.
func (c *lockFileChecks) actionReadsClean(g *callGraph, dir string) bool {
	rel := g.actionFiles[dir]
	if rel == "" {
		return false
	}
	src, err := os.ReadFile(filepath.Join(g.root, filepath.FromSlash(rel)))
	if err != nil {
		return false
	}
	_, errs := ParseAction(src)
	return len(errs) == 0
}

// checkUsage reports the entries of the lock file which nothing uses: the workflows which do not exist, the
// actions which a workflow does not use any more, and the dependencies which no workflow reaches.
func (c *lockFileChecks) checkUsage() {
	l := c.lock
	allKnown := true
	reached := map[string]bool{}
	var reach func(key string)
	reach = func(key string) {
		if reached[key] {
			return
		}
		reached[key] = true
		if d := l.deps[key]; d != nil {
			for _, u := range d.Uses {
				reach(u.Key)
			}
		}
	}
	for _, w := range l.Workflows {
		if checkLockWorkflowPath(w.Key) != "" {
			allKnown = false
			continue
		}
		usage := c.workflowUsage(w.Key)
		if usage == nil {
			c.report(w.Line, w.Col, "the workflow %q does not exist, so its entry is stale. run `gh actions-lock` to prune it", w.Key)
			allKnown = false // The dependencies it left are pruned with it: they are not reported one by one
			continue
		}
		for _, p := range w.Pins {
			reach(p.Key)
		}
		if !usage.complete {
			allKnown = false
			continue
		}
		for _, p := range w.Pins {
			if usage.keys[p.Key] {
				continue
			}
			if d := l.deps[p.Key]; d != nil && usage.keys[strings.ToLower(p.Owner+"/"+p.Repo)+"@"+strings.ToLower(lockCommitHex(d.Commit))] {
				continue // The workflow was pinned by hand to the locked commit
			}
			c.report(p.Line, p.Col, "%q is listed for the workflow %q but the workflow does not use it. run `gh actions-lock` to prune the entry", p.Raw, w.Key)
		}
	}
	if !allKnown {
		return // Something which a workflow runs was not read, and may use the dependencies
	}
	for _, d := range l.Dependencies {
		if !reached[d.Pin.Key] {
			c.report(d.Line, d.Col, "dependency %q is not used by any workflow. run `gh actions-lock` to prune it", d.Pin.Raw)
		}
	}
}

// checkOnline compares the pins of the lock file with GitHub: the repository is the one which was locked (it was
// not deleted and created again under the same name), it was not renamed or transferred, the commit is part of the
// history of the repository, and a version tag still points at the locked commit.
func (c *lockFileChecks) checkOnline(sess *onlineSession) {
	limit := DefaultImpostorMaxBranches
	if v, ok := c.cfg.ruleOptionNumber("impostor-commit", "max-branches"); ok {
		limit = int(v)
	}
	deps := append([]*lockDependency(nil), c.lock.Dependencies...)
	sort.SliceStable(deps, func(i, j int) bool { return deps[i].Line < deps[j].Line })
	for _, d := range deps {
		if (d.Hostname != "" && d.Hostname != "github.com") || sess.stopped() != nil {
			continue // Another instance than the one the client talks to
		}
		hex := lockCommitHex(d.Commit)
		if hex == "" || !validGitHubOwner(d.Pin.Owner) || !validGitHubRepo(d.Pin.Repo) {
			continue
		}
		sha := strings.ToLower(hex)
		owner, repo := d.Pin.Owner, d.Pin.Repo
		slug := owner + "/" + repo

		info, err := sess.Repository(owner, repo)
		if err != nil {
			continue // Not found can be a private repository: no verdict
		}
		switch {
		case info.ID != 0 && d.RepoID > 0 && info.ID != d.RepoID:
			// The ID stays when a repository is renamed or transferred, so another one means another repository
			c.reportDrift(d, "%s is not the repository which was locked any more: the lock file has repo_id %d, GitHub has repo_id %d for the name. it was deleted and created again, or the name was taken over. do not run `gh actions-lock --relock` before you know why", slug, d.RepoID, info.ID)
			continue
		case info.Owner != nil && info.Owner.ID != 0 && d.OwnerID > 0 && info.Owner.ID != d.OwnerID,
			info.FullName != "" && !strings.EqualFold(info.FullName, slug):
			now := info.FullName
			if now == "" {
				now = slug + " (owner_id " + strconv.FormatInt(ownerIDOf(info), 10) + " now, the lock file has " + strconv.FormatInt(d.OwnerID, 10) + ")"
			}
			c.reportDrift(d, "%s was renamed or transferred to %s. the lock file still has the old name, which the runner refuses. update `uses:` and run `gh actions-lock`", slug, now)
			continue
		}

		if len(sha) != 40 {
			continue // The client looks commits up by SHA-1: the identity of the repository was checked above
		}
		if d.HasRef && isFullVersion(d.Ref) {
			if tagSHA, isTag, err := sess.TagCommit(owner, repo, d.Ref); err == nil && isTag && !strings.EqualFold(tagSHA, sha) {
				c.reportDrift(d, "tag %q of %s points to commit %s now, but the lock file has %s. a version tag was moved, which is how a compromised release can look. check the tag before you run `gh actions-lock --relock`", d.Ref, slug, shortSHA(tagSHA), shortSHA(sha))
				continue
			}
		}
		origin, err := sess.commitOrigin(owner, repo, sha, limit)
		if err == nil && origin == originImpostor {
			c.reportDrift(d, "the locked commit %s is on no branch or tag of %s. it can come from a fork of the repository, where anyone can create a commit that looks like part of it (an impostor commit), or the branch which had it was rewritten", shortSHA(sha), slug)
		}
	}
}

func ownerIDOf(r *GitHubRepo) int64 {
	if r.Owner == nil {
		return 0
	}
	return r.Owner.ID
}

var reFullVersion = regexp.MustCompile(`^[vV]?\d+\.\d+\.\d+([-+][0-9A-Za-z.+-]+)?$`)

// isFullVersion reports whether the ref is a version with all three numbers, v4.2.1, which names one release for good.
func isFullVersion(ref string) bool {
	return reFullVersion.MatchString(ref)
}

func (c *lockFileChecks) reportDrift(d *lockDependency, format string, args ...any) {
	c.ReportIDf("actions-lock-drift", &Pos{Line: max(d.Line, 1), Col: max(d.Col, 1)}, format, args...)
}

// isActionsLockPath reports whether the path is the lock file of a repository.
func isActionsLockPath(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	return p == actionsLockPath || strings.HasSuffix(p, "/"+actionsLockPath)
}

// isActionsLockFile is isActionsLockPath for a path given to the linter, which is resolved against the working directory.
func (l *Linter) isActionsLockFile(p string) bool {
	if p != l.stdin && !filepath.IsAbs(p) {
		p = filepath.Join(l.cwd, p)
	}
	return isActionsLockPath(p)
}

// actionsLockFiles returns the path of the lock file of the project when it has one.
func (p *Project) actionsLockFiles() []string {
	f := filepath.Join(p.root, filepath.FromSlash(actionsLockPath))
	if s, err := os.Stat(f); err == nil && !s.IsDir() {
		return []string{f}
	}
	return nil
}

// checkActionsLock checks the content of an Actions lock file.
func (l *Linter) checkActionsLock(file string, content []byte, project *Project, cfg *Config, bl *baselineState, start time.Time) ([]*Error, error) {
	c := &lockFileChecks{
		RuleBase: RuleBase{name: "actions-lock", desc: "Checks the Actions lock file"},
		lock:     parseActionsLock(file, content),
		project:  project,
		linter:   l,
		cfg:      cfg,
	}
	c.SetConfig(cfg)
	if cfg.RuleEnabled(actionsLockID) {
		c.run()
	}
	if cfg.RuleEnabled(actionsLockDriftID) && c.lock.usable() {
		sess, err := l.onlineSession(cfg)
		if err != nil {
			return nil, err
		}
		if sess != nil {
			c.checkOnline(sess)
		}
	}
	if l.logLevel >= LogLevelVerbose {
		l.log("Found", len(c.errs), "errors in", time.Since(start).Milliseconds(), "ms for", file)
	}
	return l.finishCheck(file, content, c.Errs(), cfg, bl, start, false, &ignoreContext{project: project}), nil
}

func init() {
	registerRules(
		RuleInfo{
			ID: actionsLockID, Group: RuleGroupSecurity, Summary: "The Actions lock file does not match the workflows: an action is missing from it, an entry is stale or malformed.",
			DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-actions-lock",
			Options: []RuleOption{pedanticOption},
		},
		RuleInfo{
			ID: actionsLockDriftID, Group: RuleGroupSecurity, Summary: "A pin of the Actions lock file no longer matches GitHub: the repository changed owner or was replaced, a version tag moved, or the commit is not in the history of the repository.",
			DefaultLevel: SeverityError, Online: true, DocsAnchor: "check-actions-lock",
		},
	)
	registerRuleFactory("actions-lock", func(env *RuleEnv) []Rule {
		scope := env.lockScope()
		if scope == nil || !env.config.RuleEnabled(actionsLockID) {
			return nil
		}
		return []Rule{NewRuleActionsLock(scope)}
	})
}
