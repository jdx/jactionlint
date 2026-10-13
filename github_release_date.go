package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GitHubReleaseDate says when a version of an action was released. The data is what the check of
// the release cooldown (the rule action-release-cooldown) needs.
type GitHubReleaseDate struct {
	// PublishedAt is when the release of GitHub named by the tag was published. It is the zero time
	// when the tag has no release.
	PublishedAt time.Time `json:"published_at,omitzero"`
	// CommittedAt is the committer date of the commit. A client sets it when the tag has no release (the
	// date is then the best that is known). A tag which was moved to an older commit carries
	// the date of that commit, so the date can be older than the release it stands for.
	CommittedAt time.Time `json:"committed_at,omitzero"`
}

// Time returns when the version was released: the publication of the release, else the date of the commit.
// The boolean is false when neither is known.
func (d GitHubReleaseDate) Time() (t time.Time, release, ok bool) {
	switch {
	case !d.PublishedAt.IsZero():
		return d.PublishedAt, true, true
	case !d.CommittedAt.IsZero():
		return d.CommittedAt, false, true
	}
	return time.Time{}, false, false
}

// GitHubReleaseDateClient is an optional interface of a GitHubClient. A client which implements it answers
// the rule action-release-cooldown. Without it the rule has nothing to check and says nothing.
type GitHubReleaseDateClient interface {
	// ReleaseDate returns when the tag was released. tag can be empty when the commit is not tagged. sha is the
	// commit the tag points to, or the commit itself. When the tag has no release, or tag is empty,
	// the answer has the date of the commit. A tag or a commit which does not exist gives an answer
	// without dates, not an error.
	ReleaseDate(ctx context.Context, owner, repo, tag, sha string) (*GitHubReleaseDate, error)
}

// errReleaseDatesUnsupported is why the date of a release is not known: the client cannot tell.
var errReleaseDatesUnsupported = errors.New("the GitHub client cannot look up the date of a release")

type releaseDateKey struct {
	repoKey
	tag, sha string
}

// ReleaseDate returns when the tag (which can be empty) and the commit were released.
func (s *onlineSession) ReleaseDate(owner, repo, tag, sha string) (*GitHubReleaseDate, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	rc, ok := s.client.(GitHubReleaseDateClient)
	what := owner + "/" + repo
	return s.releaseDates.get(releaseDateKey{newRepoKey(owner, repo), tag, strings.ToLower(sha)}, func() (*GitHubReleaseDate, error) {
		if !ok {
			// Not silence: the rule which asked checked nothing for this version, which is a skipped lookup (and fails --online=strict)
			s.mu.Lock()
			defer s.mu.Unlock()
			s.registryFailedLocked(failOther, "release-dates-unsupported", what, errReleaseDatesUnsupported,
				"online: the client of GitHub cannot look up the date of a release, so action-release-cooldown checks nothing. --verbose lists every skipped lookup.")
			return nil, errReleaseDatesUnsupported
		}
		return callFor(s, what, func(ctx context.Context) (*GitHubReleaseDate, error) {
			return rc.ReleaseDate(ctx, owner, repo, tag, sha)
		})
	})
}

// fixtureReleaseKey is the key of the dates in the fixtures: the tag, or the commit when there is none.
func fixtureReleaseKey(tag, sha string) string {
	if tag != "" {
		return "tags/" + tag
	}
	return "commits/" + strings.ToLower(sha)
}

// ReleaseDate implements GitHubReleaseDateClient.
func (c *FixtureGitHubClient) ReleaseDate(_ context.Context, owner, repo, tag, sha string) (*GitHubReleaseDate, error) {
	r, err := c.repo(owner, repo, "release date")
	if err != nil {
		return nil, err
	}
	d, ok := r.ReleaseDates[fixtureReleaseKey(tag, sha)]
	if !ok {
		return nil, fmt.Errorf("the GitHub fixtures have no release date for %s/%s %s", owner, repo, fixtureReleaseKey(tag, sha))
	}
	cp := *d
	return &cp, nil
}

// ReleaseDate implements GitHubReleaseDateClient.
func (c *RecordingGitHubClient) ReleaseDate(ctx context.Context, owner, repo, tag, sha string) (*GitHubReleaseDate, error) {
	rc, ok := c.inner.(GitHubReleaseDateClient)
	if !ok {
		return nil, errReleaseDatesUnsupported
	}
	v, err := rc.ReleaseDate(ctx, owner, repo, tag, sha)
	if err == nil {
		c.rec(owner, repo, func(r *fixtureRepo) {
			if r.ReleaseDates == nil {
				r.ReleaseDates = map[string]*GitHubReleaseDate{}
			}
			cp := *v
			r.ReleaseDates[fixtureReleaseKey(tag, sha)] = &cp
		})
	}
	return v, err
}
