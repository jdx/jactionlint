package jactionlint

import "context"

// FileContent implements GitHubContentClient for the host which has the repository, like the questions about its tags: the Server
// is asked first and github.com only for a repository which the Server does not have. The metadata (action.yml) of an action
// that GitHub Connect syncs from github.com is on github.com, so without this the checks of the inputs of remote actions did
// nothing on a Server with the fallback on.
func (c *fallbackGitHubClient) FileContent(ctx context.Context, owner, repo, ref, path string) ([]byte, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return nil, r.err
	}
	cc, ok := r.client.(GitHubContentClient)
	if !ok {
		return nil, errOnlineNoFiles
	}
	v, err := cc.FileContent(ctx, owner, repo, ref, path)
	return v, c.fromHost(r, err)
}
