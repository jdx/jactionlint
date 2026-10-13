package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// This file has the lookup of the environments of a repository, which tells whether the `environment:` of a
// job is a gate that someone has to open (required reviewers) or only a name. The rules which trust an
// environment (untrusted-checkout, untrusted-artifact, agentic-actions) use it with the online checks.

// GitHubEnvironment is an environment of a repository.
type GitHubEnvironment struct {
	// Name is the name of the environment.
	Name string `json:"name"`
	// RequiredReviewers is true when the environment has a "required_reviewers" protection rule: a job which uses it
	// waits until one of the reviewers approves. Other rules (a wait timer, the branches which may deploy) do not
	// make anyone look at what the job runs.
	RequiredReviewers bool `json:"required_reviewers"`
}

// GitHubEnvironmentClient is an optional interface of a GitHubClient. A client which implements it can tell what
// protects an environment, which jactionlint asks for when a workflow relies on the reviewers of an
// environment. Without it the environment is trusted as it is without the online checks.
type GitHubEnvironmentClient interface {
	// Environment returns the environment of the repository. It returns ErrGitHubNotFound when GitHub has no such
	// environment, or does not show it to the client: a repository which is private to the client looks the same.
	Environment(ctx context.Context, owner, repo, name string) (*GitHubEnvironment, error)
}

// environmentGate is what is known about the protection of an environment.
type environmentGate int

const (
	// gateUnknown means the lookup did not work: the environment is trusted as before.
	gateUnknown environmentGate = iota
	// gateReviewers means the environment has required reviewers.
	gateReviewers
	// gateNone means the environment exists and has no required reviewers.
	gateNone
	// gateMissing means the environment does not exist in a public repository. A job which names it creates it,
	// empty, when it runs.
	gateMissing
)

type environmentKey struct {
	repoKey
	name string
}

type environmentResult struct {
	gate environmentGate
}

const maxGitHubEnvironmentLen = 255

// validEnvironmentName reports whether the name of an environment can be put in the path of a request as it is:
// no control character, no slash, backslash or percent sign (which a request would have to escape, and which
// proxies may decode twice) and not a dot segment.
func validEnvironmentName(s string) bool {
	if s == "" || len(s) > maxGitHubEnvironmentLen || s == "." || s == ".." || ContainsExpression(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune("/\\%?#", r) {
			return false
		}
	}
	return true
}

var (
	errNoEnvironmentLookup    = errors.New("the GitHub client cannot look up environments")
	errInvalidEnvironmentName = errors.New("not a valid environment name")
)

// EnvironmentGate tells whether the environment of the repository has required reviewers. It is one request
// for each environment of the run, and one more for the repository when GitHub answers "not found" and the
// repository has not been asked for yet. The answer is unknown (and an error) when the lookup failed, which counts
// as a skipped lookup: a 403 (the token cannot read environments), a 404 for a private repository (the
// environment may exist, or the token may not see it), or any failure of the API.
func (s *onlineSession) EnvironmentGate(owner, repo, name string) (environmentGate, error) {
	if !s.allows(owner, repo) {
		return gateUnknown, errOnlineExcluded
	}
	ec, ok := s.client.(GitHubEnvironmentClient)
	if !ok {
		return gateUnknown, errNoEnvironmentLookup
	}
	if !validEnvironmentName(name) {
		return gateUnknown, fmt.Errorf("environment %q: %w", name, errInvalidEnvironmentName)
	}
	what := fmt.Sprintf("environment %q of %s/%s", name, owner, repo)
	r, err := s.environments.get(environmentKey{newRepoKey(owner, repo), strings.ToLower(name)}, func() (environmentResult, error) {
		env, err := callFor(s, what, func(ctx context.Context) (*GitHubEnvironment, error) { return ec.Environment(ctx, owner, repo, name) })
		switch {
		case err == nil && env.RequiredReviewers:
			return environmentResult{gateReviewers}, nil
		case err == nil:
			return environmentResult{gateNone}, nil
		case !errors.Is(err, ErrGitHubNotFound):
			return environmentResult{}, err
		}
		// Not found is also what a private repository answers a token that may not read its environments. A public
		// one shows them to everybody, so there the environment does not exist.
		info, rerr := s.Repository(owner, repo)
		switch {
		case rerr != nil:
			return environmentResult{}, err // the lookup of the repository counted itself
		case info.Private:
			s.mu.Lock()
			s.failedLocked(failNotFound, what, err)
			s.mu.Unlock()
			return environmentResult{}, err
		}
		return environmentResult{gateMissing}, nil
	})
	return r.gate, err
}
