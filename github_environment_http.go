//go:build !js

package jactionlint

import (
	"context"
	"fmt"
)

// Environment implements GitHubEnvironmentClient. The endpoint needs the permission "Actions: read" of a
// token (the scope "repo" of a classic one for a private repository), and shows the protection rules of a
// public repository to everybody.
func (c *httpGitHubClient) Environment(ctx context.Context, owner, repo, name string) (*GitHubEnvironment, error) {
	if !validEnvironmentName(name) {
		return nil, fmt.Errorf("%w: environment %q", errInvalidGitHubName, name)
	}
	path, err := repoURL(owner, repo, "environments", name)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Name            string `json:"name"`
		ProtectionRules []struct {
			Type string `json:"type"`
		} `json:"protection_rules"`
	}
	if _, err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	env := &GitHubEnvironment{Name: raw.Name}
	for _, r := range raw.ProtectionRules {
		if r.Type == "required_reviewers" {
			env.RequiredReviewers = true
		}
	}
	return env, nil
}
