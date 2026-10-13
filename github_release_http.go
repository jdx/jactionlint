//go:build !js

package jactionlint

import (
	"context"
	"fmt"
	"strings"
)

// Release implements GitHubReleaseClient: GET /repos/{owner}/{repo}/releases/tags/{tag}, which answers 404 for a
// tag without a published release (a draft is not found either).
func (c *httpGitHubClient) Release(ctx context.Context, owner, repo, tag string) (*GitHubRelease, error) {
	if !validGitRefName(tag) || strings.Contains(tag, "/") {
		return nil, fmt.Errorf("%w: release of %q", errInvalidGitHubName, tag)
	}
	path, err := repoURL(owner, repo, "releases", "tags", tag)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Tag       string `json:"tag_name"`
		Immutable bool   `json:"immutable"`
	}
	if _, err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	return &GitHubRelease{Tag: raw.Tag, Immutable: raw.Immutable}, nil
}
