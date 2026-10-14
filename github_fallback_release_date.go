package jactionlint

import "context"

// ReleaseDate implements GitHubReleaseDateClient for the host which has the repository, like the release and the tags: the
// Server is asked first and github.com only for a repository which the Server does not have, so that the rule
// action-release-cooldown works for the actions of the Server and for the ones that GitHub Connect syncs from github.com. The
// failures of github.com are counted like the other lookups of that host, and nothing of the Server, its token included, goes to
// github.com.
func (c *fallbackGitHubClient) ReleaseDate(ctx context.Context, owner, repo, tag, sha string) (*GitHubReleaseDate, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return nil, r.err
	}
	rc, ok := r.client.(GitHubReleaseDateClient)
	if !ok {
		return nil, errReleaseDatesUnsupported
	}
	v, err := rc.ReleaseDate(ctx, owner, repo, tag, sha)
	return v, c.fromHost(r, err)
}
