package jactionlint

import "context"

// Environment implements GitHubEnvironmentClient. The environments are the ones of the repository of the workflow, which is
// on the Server whatever the actions are: only the Server is asked, and github.com never learns the name of the repository.
//
// The wrapper does not implement GitHubOwnerClient on purpose. The owner of a repository which neither host has is not known
// to github.com either when it is an organization of the Server, so the answer "no such owner" of github.com would say
// nothing, and the repojacking rule skips instead.
func (c *fallbackGitHubClient) Environment(ctx context.Context, owner, repo, name string) (*GitHubEnvironment, error) {
	ec, ok := c.primary.(GitHubEnvironmentClient)
	if !ok {
		return nil, errNoEnvironmentLookup
	}
	return ec.Environment(ctx, owner, repo, name)
}
