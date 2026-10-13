package jactionlint

import (
	"context"
	"fmt"
	"strings"
)

// The implementations of GitHubEnvironmentClient of the fixtures and the recorder.

// fixtureEnvironment is the recorded answer about one environment.
type fixtureEnvironment struct {
	// Found is false when GitHub answered "not found".
	Found             bool `json:"found"`
	RequiredReviewers bool `json:"required_reviewers,omitempty"`
}

// Environment implements GitHubEnvironmentClient.
func (c *FixtureGitHubClient) Environment(_ context.Context, owner, repo, name string) (*GitHubEnvironment, error) {
	r, err := c.repo(owner, repo, "environment")
	if err != nil {
		return nil, err
	}
	e, ok := r.Environments[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("the GitHub fixtures have no environment %q for %s/%s", name, owner, repo)
	}
	if !e.Found {
		return nil, fmt.Errorf("environment %q of %s/%s: %w", name, owner, repo, ErrGitHubNotFound)
	}
	return &GitHubEnvironment{Name: name, RequiredReviewers: e.RequiredReviewers}, nil
}

// Environment implements GitHubEnvironmentClient when the wrapped client does.
func (c *RecordingGitHubClient) Environment(ctx context.Context, owner, repo, name string) (*GitHubEnvironment, error) {
	ec, ok := c.inner.(GitHubEnvironmentClient)
	if !ok {
		return nil, errNoEnvironmentLookup
	}
	env, err := ec.Environment(ctx, owner, repo, name)
	record := func(e *fixtureEnvironment) {
		c.rec(owner, repo, func(r *fixtureRepo) {
			if r.Environments == nil {
				r.Environments = map[string]*fixtureEnvironment{}
			}
			r.Environments[strings.ToLower(name)] = e
		})
	}
	switch {
	case err == nil:
		record(&fixtureEnvironment{Found: true, RequiredReviewers: env.RequiredReviewers})
	case isNotFound(err):
		// The repository exists (or Repository records it as missing): it is the environment that is not there
		record(&fixtureEnvironment{})
	}
	return env, err
}
