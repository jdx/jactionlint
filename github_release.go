package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// This file has the lookup of the release of a tag, which tells whether the release is immutable. GitHub locks
// the tag of an immutable release (it cannot be moved or deleted) and its assets, so
// that `uses: owner/repo@v1.2.3` runs the same code for as long as the release exists. The release object of the
// REST API says so with "immutable": true (the endpoint "Get a release by tag name").

// GitHubRelease is the release of a tag.
type GitHubRelease struct {
	// Tag is the name of the tag of the release.
	Tag string `json:"tag"`
	// Immutable is true when the release is immutable: its tag and its assets cannot be changed any more.
	Immutable bool `json:"immutable"`
}

// GitHubReleaseClient is an optional interface of a GitHubClient. A client which implements it can tell whether
// the release of a tag is immutable, which the "immutable-release" policy of the unpinned-uses rule needs.
type GitHubReleaseClient interface {
	// Release returns the published release of the tag. It returns ErrGitHubNotFound when the tag has no release (the
	// tags which are not releases, such as the v4 which moves with every v4.x.y, have none).
	Release(ctx context.Context, owner, repo, tag string) (*GitHubRelease, error)
}

var (
	errNoReleaseLookup    = errors.New("the GitHub client cannot look up releases")
	errInvalidReleaseName = errors.New("not a tag name that can be looked up")
)

type releaseKey struct {
	repoKey
	tag string
}

// Release returns the published release of the tag, or ErrGitHubNotFound (wrapped) when the tag has none. The error
// is errNoReleaseLookup when the client cannot look releases up, which is no failure.
func (s *onlineSession) Release(owner, repo, tag string) (*GitHubRelease, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	rc, ok := s.client.(GitHubReleaseClient)
	if !ok {
		return nil, errNoReleaseLookup
	}
	// A tag with a slash is more than one segment of the path, which this does not guess
	if !validGitRefName(tag) || strings.Contains(tag, "/") {
		return nil, fmt.Errorf("%q: %w", tag, errInvalidReleaseName)
	}
	return s.releases.get(releaseKey{newRepoKey(owner, repo), tag}, func() (*GitHubRelease, error) {
		return callFor(s, owner+"/"+repo, func(ctx context.Context) (*GitHubRelease, error) { return rc.Release(ctx, owner, repo, tag) })
	})
}

// fixtureRelease is the recorded answer about the release of one tag.
type fixtureRelease struct {
	// Found is false when GitHub answered "not found": the tag has no release.
	Found     bool `json:"found"`
	Immutable bool `json:"immutable,omitempty"`
}

// Release implements GitHubReleaseClient.
func (c *FixtureGitHubClient) Release(_ context.Context, owner, repo, tag string) (*GitHubRelease, error) {
	r, err := c.repo(owner, repo, "release")
	if err != nil {
		return nil, err
	}
	e, ok := r.Releases[tag]
	if !ok {
		return nil, fmt.Errorf("the GitHub fixtures have no release of the tag %q for %s/%s", tag, owner, repo)
	}
	if !e.Found {
		return nil, fmt.Errorf("release of %q of %s/%s: %w", tag, owner, repo, ErrGitHubNotFound)
	}
	return &GitHubRelease{Tag: tag, Immutable: e.Immutable}, nil
}

// Release implements GitHubReleaseClient when the wrapped client does.
func (c *RecordingGitHubClient) Release(ctx context.Context, owner, repo, tag string) (*GitHubRelease, error) {
	rc, ok := c.inner.(GitHubReleaseClient)
	if !ok {
		return nil, errNoReleaseLookup
	}
	rel, err := rc.Release(ctx, owner, repo, tag)
	record := func(e *fixtureRelease) {
		c.rec(owner, repo, func(r *fixtureRepo) {
			if r.Releases == nil {
				r.Releases = map[string]*fixtureRelease{}
			}
			r.Releases[tag] = e
		})
	}
	switch {
	case err == nil:
		record(&fixtureRelease{Found: true, Immutable: rel.Immutable})
	case isNotFound(err):
		record(&fixtureRelease{})
	}
	return rel, err
}
