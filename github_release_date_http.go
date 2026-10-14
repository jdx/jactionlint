//go:build !js

package jactionlint

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ReleaseDate implements GitHubReleaseDateClient. It asks for the release of the tag, and for the
// commit only when the tag has none (the answer of the release has no commit date). A tag with a "/" is
// not asked for: the API client refuses a path segment with one, and such tags of a monorepo
// ("save/v1.0.0") are looked up by their commit.
func (c *httpGitHubClient) ReleaseDate(ctx context.Context, owner, repo, tag, sha string) (*GitHubReleaseDate, error) {
	var d GitHubReleaseDate
	if tag != "" && validGitRefName(tag) && !strings.Contains(tag, "/") {
		path, err := repoURL(owner, repo, "releases", "tags", tag)
		if err != nil {
			return nil, err
		}
		var rel struct {
			PublishedAt time.Time `json:"published_at"`
			Draft       bool      `json:"draft"`
		}
		_, err = c.getJSON(ctx, path, &rel)
		switch {
		case err == nil:
			if !rel.Draft {
				d.PublishedAt = rel.PublishedAt
			}
		case errors.Is(err, ErrGitHubNotFound):
			// A tag without a release
		default:
			return nil, err
		}
	}
	if !d.PublishedAt.IsZero() || !validGitSHA(sha) {
		return &d, nil
	}
	// The commit object, which is small. The commit of the REST API lists the files of the commit as well.
	path, err := repoURL(owner, repo, "git", "commits", strings.ToLower(sha))
	if err != nil {
		return nil, err
	}
	var cm struct {
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	}
	if _, err := c.getJSON(ctx, path, &cm); err != nil && !errors.Is(err, ErrGitHubNotFound) {
		return nil, err
	}
	d.CommittedAt = cm.Committer.Date
	return &d, nil
}
