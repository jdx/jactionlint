package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
)

// maxConsecutiveGitHubFailures is how many failed requests in a row stop the online rules for the
// rest of the run. A refused or unreachable API stops them at once (see onlineSession.record).
const maxConsecutiveGitHubFailures = 3

// GitHubStatusError is the error of a GitHubClient when the API answered with a failure status which
// is neither "not found" nor "rate limited".
type GitHubStatusError struct {
	// Status is the HTTP status code.
	Status int
	// Message is what the API said, if anything.
	Message string
}

func (e *GitHubStatusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitHub API returned status %d", e.Status)
	}
	return fmt.Sprintf("GitHub API returned status %d: %s", e.Status, e.Message)
}

// memo caches the result of a function per key and runs it at most once for each key, also when many
// goroutines ask for the same key at the same time (files are linted in parallel). Errors are
// cached too so that a failing lookup is never repeated.
type memo[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]*memoEntry[V]
}

type memoEntry[V any] struct {
	once sync.Once
	v    V
	err  error
}

func (m *memo[K, V]) get(k K, fn func() (V, error)) (V, error) {
	m.mu.Lock()
	if m.m == nil {
		m.m = map[K]*memoEntry[V]{}
	}
	e, ok := m.m[k]
	if !ok {
		e = &memoEntry[V]{}
		m.m[k] = e
	}
	m.mu.Unlock()
	e.once.Do(func() { e.v, e.err = fn() })
	return e.v, e.err
}

type repoKey struct{ owner, repo string }

func newRepoKey(owner, repo string) repoKey {
	return repoKey{strings.ToLower(owner), strings.ToLower(repo)}
}

type refKey struct {
	repoKey
	ns   GitHubRefNamespace
	name string
}

type compareKey struct {
	repoKey
	base, head string
}

type refResult struct {
	sha   string
	found bool
}

// tagIndex is the tags of a repository indexed both ways.
type tagIndex struct {
	byName    map[string]string
	bySHA     map[string][]string // sorted by name
	truncated bool
}

func newTagIndex(l *GitHubTagList) *tagIndex {
	idx := &tagIndex{byName: make(map[string]string, len(l.Tags)), bySHA: map[string][]string{}, truncated: l.Truncated}
	for _, t := range l.Tags {
		sha := strings.ToLower(t.SHA)
		idx.byName[t.Name] = sha
		idx.bySHA[sha] = append(idx.bySHA[sha], t.Name)
	}
	for _, names := range idx.bySHA {
		slices.Sort(names)
	}
	return idx
}

// onlineSession is what the online rules use to reach GitHub during one run of the linter. It wraps a
// GitHubClient and
//
//   - remembers every answer, so the same repository, tag or commit is asked for once however many
//     steps and files mention it,
//   - stops the online rules for the rest of the run, with one warning, when the API cannot be used
//     (rate limited, unreachable, token rejected, repeated server errors, or interrupted),
//   - never retries a failed request.
//
// It is safe for concurrent use.
type onlineSession struct {
	client GitHubClient
	ctx    context.Context
	warn   func(msg string)

	mu       sync.Mutex
	stopErr  error
	failures int

	repos      memo[repoKey, *GitHubRepo]
	tags       memo[repoKey, *tagIndex]
	refs       memo[refKey, refResult]
	branches   memo[repoKey, *GitHubBranchList]
	compares   memo[compareKey, GitHubCompareStatus]
	advisories memo[repoKey, []GitHubAdvisory]
	origins    memo[originKey, originResult]
}

type originKey struct {
	repoKey
	sha   string
	limit int
}

type originResult struct {
	origin commitOrigin
}

func newOnlineSession(ctx context.Context, client GitHubClient, warn func(string)) *onlineSession {
	if ctx == nil {
		ctx = context.Background()
	}
	return &onlineSession{client: client, ctx: ctx, warn: warn}
}

// stopped returns the reason why the online rules stopped, or nil.
func (s *onlineSession) stopped() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopErr == nil {
		if err := s.ctx.Err(); err != nil {
			s.stopLocked(fmt.Errorf("interrupted: %w", err))
		}
	}
	return s.stopErr
}

func (s *onlineSession) stopLocked(err error) {
	if s.stopErr != nil {
		return
	}
	s.stopErr = err
	if s.warn != nil {
		s.warn(fmt.Sprintf("the online checks were stopped and the findings which need GitHub are missing: %s", err))
	}
}

// record looks at the result of a request and decides whether the online rules can go on.
func (s *onlineSession) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var netErr net.Error
	var statusErr *GitHubStatusError
	switch {
	case err == nil, errors.Is(err, ErrGitHubNotFound):
		s.failures = 0
	case errors.Is(err, ErrGitHubBranchScanUnavailable):
		// Not a failure: the caller does it another way
	case errors.Is(err, ErrGitHubRateLimited), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded) && s.ctx.Err() != nil:
		s.stopLocked(err)
	case errors.As(err, &statusErr):
		s.failures++
		if s.failures >= maxConsecutiveGitHubFailures || statusErr.Status == 401 {
			s.stopLocked(err)
		}
	case errors.As(err, &netErr):
		s.stopLocked(err) // Probably offline. Do not wait for the timeout of every lookup.
	default:
		s.failures++
		if s.failures >= maxConsecutiveGitHubFailures {
			s.stopLocked(err)
		}
	}
}

// call runs one request unless the session is stopped, and records its outcome.
func call[T any](s *onlineSession, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	if err := s.stopped(); err != nil {
		return zero, err
	}
	v, err := fn(s.ctx)
	s.record(err)
	return v, err
}

// Repository returns the metadata of the repository.
func (s *onlineSession) Repository(owner, repo string) (*GitHubRepo, error) {
	return s.repos.get(newRepoKey(owner, repo), func() (*GitHubRepo, error) {
		return call(s, func(ctx context.Context) (*GitHubRepo, error) { return s.client.Repository(ctx, owner, repo) })
	})
}

// Tags returns the tags of the repository.
func (s *onlineSession) Tags(owner, repo string) (*tagIndex, error) {
	return s.tags.get(newRepoKey(owner, repo), func() (*tagIndex, error) {
		l, err := call(s, func(ctx context.Context) (*GitHubTagList, error) { return s.client.Tags(ctx, owner, repo) })
		if err != nil {
			return nil, err
		}
		return newTagIndex(l), nil
	})
}

func (s *onlineSession) resolveRef(owner, repo string, ns GitHubRefNamespace, name string) (refResult, error) {
	return s.refs.get(refKey{newRepoKey(owner, repo), ns, name}, func() (refResult, error) {
		return call(s, func(ctx context.Context) (refResult, error) {
			sha, found, err := s.client.ResolveRef(ctx, owner, repo, ns, name)
			return refResult{strings.ToLower(sha), found}, err
		})
	})
}

// TagCommit returns the commit the tag of the name points to. The boolean is false when the
// repository has no such tag.
func (s *onlineSession) TagCommit(owner, repo, name string) (string, bool, error) {
	idx, err := s.Tags(owner, repo)
	if err == nil {
		if sha, ok := idx.byName[name]; ok {
			return sha, true, nil
		}
		if !idx.truncated {
			return "", false, nil
		}
	} else if s.stopped() != nil {
		return "", false, err
	}
	// The list is incomplete (or could not be read): ask for the tag itself
	r, err := s.resolveRef(owner, repo, GitHubRefTags, name)
	return r.sha, r.found, err
}

// BranchCommit returns the commit the branch of the name points to. The boolean is false when the
// repository has no such branch.
func (s *onlineSession) BranchCommit(owner, repo, name string) (string, bool, error) {
	r, err := s.resolveRef(owner, repo, GitHubRefHeads, name)
	return r.sha, r.found, err
}

// Branches returns at most limit branches of the repository. The first call for a repository decides
// how many are read.
func (s *onlineSession) Branches(owner, repo string, limit int) (*GitHubBranchList, error) {
	return s.branches.get(newRepoKey(owner, repo), func() (*GitHubBranchList, error) {
		return call(s, func(ctx context.Context) (*GitHubBranchList, error) {
			return s.client.Branches(ctx, owner, repo, limit)
		})
	})
}

// Compare compares two commits of the repository.
func (s *onlineSession) Compare(owner, repo, base, head string) (GitHubCompareStatus, error) {
	return s.compares.get(compareKey{newRepoKey(owner, repo), base, head}, func() (GitHubCompareStatus, error) {
		return call(s, func(ctx context.Context) (GitHubCompareStatus, error) {
			return s.client.Compare(ctx, owner, repo, base, head)
		})
	})
}

// Advisories returns the security advisories of the action repository.
func (s *onlineSession) Advisories(owner, repo string) ([]GitHubAdvisory, error) {
	return s.advisories.get(newRepoKey(owner, repo), func() ([]GitHubAdvisory, error) {
		return call(s, func(ctx context.Context) ([]GitHubAdvisory, error) { return s.client.Advisories(ctx, owner, repo) })
	})
}
