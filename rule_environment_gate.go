package jactionlint

import (
	"fmt"
	"os"
	"strings"
)

// environmentGates answers, for the rules which trust the `environment:` of a job ("its reviewers decide"),
// whether the environment really holds the job back. Without the online checks it is trusted, as it always was.
// With them the environment of the repository is looked at (see onlineSession.EnvironmentGate): one with no
// required reviewers, or one that does not exist, is only a name, and the rule reports as it does for a job
// without an environment. When the lookup does not work the environment is trusted, and the lookup counts as skipped.
//
// A nil *environmentGates trusts every environment.
type environmentGates struct {
	sess  *onlineSession
	repo  string // "owner/repo" of the repository the workflow is in
	debug func(format string, args ...any)
}

// newEnvironmentGates returns the gates of the file being linted, or nil when they cannot be asked: the online
// checks are off, or the repository of the file is not known.
func newEnvironmentGates(env *RuleEnv, debug func(format string, args ...any)) *environmentGates {
	if env.online == nil {
		return nil
	}
	repo := env.repository()
	if repo == "" {
		return nil
	}
	return &environmentGates{sess: env.online, repo: repo, debug: debug}
}

// repository returns "owner/repo" of the repository of the file being linted: GITHUB_REPOSITORY, which GitHub
// Actions sets for every job, or else the url of the remote "origin" of the clone. It is empty when neither
// names a repository.
func (e *RuleEnv) repository() string {
	if v := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY")); validRepositorySlug(v) {
		return v
	}
	if e.project != nil {
		if v := githubRepositoryOf(e.project.RootDir()); validRepositorySlug(v) {
			return v
		}
	}
	return ""
}

func validRepositorySlug(s string) bool {
	owner, repo, ok := strings.Cut(s, "/")
	return ok && validGitHubOwner(owner) && validGitHubRepo(repo)
}

// noGate tells whether the environment of the job is no gate: it has no required reviewers, or it does not exist.
// The text says so, for the message of the finding. It is false for a job without an environment and when it is
// not known, so that the caller keeps trusting the environment.
func (g *environmentGates) noGate(job *Job) (note string, ok bool) {
	if g == nil || job == nil || job.Environment == nil || job.Environment.Name == nil {
		return "", false
	}
	name := job.Environment.Name.Value
	owner, repo, _ := strings.Cut(g.repo, "/")
	gate, err := g.sess.EnvironmentGate(owner, repo, name)
	if err != nil {
		if g.debug != nil {
			g.debug("environment %q of %s: not looked up (%v), so it is trusted", name, g.repo, err)
		}
		return "", false
	}
	of := "the job"
	if job.ID != nil && job.ID.Value != "" {
		of = fmt.Sprintf("job %q", job.ID.Value)
	}
	switch gate {
	case gateNone:
		return fmt.Sprintf("the environment %q of %s has no required reviewers, so no one has to approve the run", name, of), true
	case gateMissing:
		return fmt.Sprintf("the environment %q of %s does not exist in %s (GitHub creates it without protection when the job runs), so no one has to approve the run", name, of, g.repo), true
	}
	return "", false
}
