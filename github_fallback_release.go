package jactionlint

import "context"

// Release implements GitHubReleaseClient for the host which has the repository, like the questions about its tags: the Server is
// asked first and github.com only for a repository which the Server does not have, so that the policy "immutable-release" of the
// unpinned-uses rule works for the actions of the Server and for the ones that GitHub Connect syncs from github.com. A tag
// without a release is "not found" on the host which has the repository, and is not looked up on the other one.
func (c *fallbackGitHubClient) Release(ctx context.Context, owner, repo, tag string) (*GitHubRelease, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return nil, r.err
	}
	rc, ok := r.client.(GitHubReleaseClient)
	if !ok {
		return nil, errNoReleaseLookup
	}
	v, err := rc.Release(ctx, owner, repo, tag)
	return v, c.fromHost(r, err)
}
