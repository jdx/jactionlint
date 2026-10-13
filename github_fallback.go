package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// fallbackGitHubClient is the GitHubClient of a GitHub Enterprise Server (or GHE.com) whose actions come from
// github.com: the Server resolves `uses: actions/checkout@v4` from github.com when GitHub Connect syncs
// it, so the repository is on the Server's API only when someone copied it there. It asks the primary client (the
// Enterprise Server) about a repository first and, only when the answer is "not found", the secondary client
// (api.github.com, with its own token). The Server wins when both have the repository, like it does when
// it resolves the action. The host which has a repository answers every later question about it.
//
// Repositories that the Server does not have are named to github.com. That is why the fallback is opt-in, and the
// deny list of the online options keeps the internal ones out.
type fallbackGitHubClient struct {
	primary, secondary GitHubClient
	routes             memo[repoKey, fallbackRoute]

	mu        sync.Mutex
	down      error // why github.com cannot be asked any more in this run, see fromSecondary
	transient int   // failures in a row which suggest an outage (timeouts, server errors), see fromSecondary
}

// hostScopedError is the failure of one of the two hosts. The online session counts and warns about it like any
// other, but does not stop the lookups because of it: the rate limit of github.com (60 an hour without a token) says
// nothing about the Server, which can go on answering for the repositories it has.
type hostScopedError struct {
	err  error
	host string // for messages
}

// gitHubComHost is the host of the secondary client.
const gitHubComHost = "api.github.com"

func (e *hostScopedError) Error() string { return e.err.Error() }
func (e *hostScopedError) Unwrap() error { return e.err }

// fromSecondary makes the error of a call to github.com one of the host. A failure which asking again cannot fix (the rate
// limit, a rejected token, no network) is remembered, so that the next repositories do not ask in vain, and so are
// timeouts and server errors when there are several in a row, like the session does for a host: a github.com which a firewall
// blackholes would otherwise make every action which the Server does not have wait for the whole timeout.
//
// Every result of a call to github.com goes through here, and this is the one place which counts: an answer of any kind (a
// repository, "not found", a refusal, a rejected request) proves that the host is there and ends the streak, and only the failures
// which say that it is not (a timeout, a server error, a connection which broke) extend it.
func (c *fallbackGitHubClient) fromSecondary(err error) error {
	if err == nil || errors.Is(err, ErrGitHubNotFound) {
		c.answered()
		return err
	}
	if errors.Is(err, ErrGitHubBranchScanUnavailable) {
		return err // nothing was asked
	}
	var scoped *hostScopedError
	if errors.As(err, &scoped) {
		return err
	}
	var status *GitHubStatusError
	switch kind := classifyFailure(err); {
	case kind == failRateLimit || kind == failUnauthorized || kind == failNetwork:
		c.mu.Lock()
		if c.down == nil {
			c.down = err
		}
		c.mu.Unlock()
	case kind == failTimeout || kind == failServer || (kind == failOther && !errors.As(err, &status)):
		c.mu.Lock()
		c.transient++
		if c.transient >= maxConsecutiveGitHubFailures && c.down == nil {
			c.down = fmt.Errorf("skipped after %d failed lookups in a row: %w", c.transient, err)
		}
		c.mu.Unlock()
	default:
		c.answered() // a refusal or a rejected request: the host answered
	}
	return &hostScopedError{err, gitHubComHost}
}

// answered ends the streak of failures of github.com.
func (c *fallbackGitHubClient) answered() {
	c.mu.Lock()
	c.transient = 0
	c.mu.Unlock()
}

// secondaryDown returns the failure which stops the lookups on github.com, or nil.
func (c *fallbackGitHubClient) secondaryDown() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.down == nil {
		return nil
	}
	return &hostScopedError{c.down, gitHubComHost}
}

// fallbackRoute is which host has a repository.
type fallbackRoute struct {
	// client answers the questions about the repository. It is the primary one when no host has it.
	client GitHubClient
	// repo is the answer about the repository, which both hosts gave once and is not asked for again.
	repo *GitHubRepo
	// err is why the repository cannot be asked about: not found on both hosts, or a failure.
	err error
	// secondary is whether github.com is the host.
	secondary bool
}

func newFallbackGitHubClient(primary, secondary GitHubClient) *fallbackGitHubClient {
	return &fallbackGitHubClient{primary: primary, secondary: secondary}
}

// route finds the host of the repository. It asks the primary host about the repository, and github.com only when
// that is not found there.
func (c *fallbackGitHubClient) route(ctx context.Context, owner, repo string) fallbackRoute {
	r, _ := c.routes.get(newRepoKey(owner, repo), func() (fallbackRoute, error) {
		info, err := c.primary.Repository(ctx, owner, repo)
		if err == nil {
			return fallbackRoute{client: c.primary, repo: info}, nil
		}
		if !errors.Is(err, ErrGitHubNotFound) {
			return fallbackRoute{client: c.primary, err: err}, nil
		}
		if down := c.secondaryDown(); down != nil {
			return fallbackRoute{client: c.secondary, err: down, secondary: true}, nil
		}
		info2, err2 := c.secondary.Repository(ctx, owner, repo)
		err2 = c.fromSecondary(err2) // also the answers which are no failure end the streak of failures
		switch {
		case err2 == nil:
			return fallbackRoute{client: c.secondary, repo: info2, secondary: true}, nil
		case errors.Is(err2, ErrGitHubNotFound):
			return fallbackRoute{client: c.primary, err: err}, nil // on neither: the answer of the Server
		}
		return fallbackRoute{client: c.secondary, err: err2, secondary: true}, nil
	})
	return r
}

// Repository implements GitHubClient.
func (c *fallbackGitHubClient) Repository(ctx context.Context, owner, repo string) (*GitHubRepo, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return nil, r.err
	}
	cp := *r.repo
	return &cp, nil
}

// Tags implements GitHubClient.
func (c *fallbackGitHubClient) Tags(ctx context.Context, owner, repo string) (*GitHubTagList, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return nil, r.err
	}
	v, err := r.client.Tags(ctx, owner, repo)
	return v, c.fromHost(r, err)
}

// ResolveRef implements GitHubClient.
func (c *fallbackGitHubClient) ResolveRef(ctx context.Context, owner, repo string, ns GitHubRefNamespace, name string) (string, bool, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return "", false, r.err
	}
	sha, found, err := r.client.ResolveRef(ctx, owner, repo, ns, name)
	return sha, found, c.fromHost(r, err)
}

// Branches implements GitHubClient.
func (c *fallbackGitHubClient) Branches(ctx context.Context, owner, repo string, limit int) (*GitHubBranchList, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return nil, r.err
	}
	v, err := r.client.Branches(ctx, owner, repo, limit)
	return v, c.fromHost(r, err)
}

// Compare implements GitHubClient.
func (c *fallbackGitHubClient) Compare(ctx context.Context, owner, repo, base, head string) (GitHubCompareStatus, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return "", r.err
	}
	v, err := r.client.Compare(ctx, owner, repo, base, head)
	return v, c.fromHost(r, err)
}

// Advisories implements GitHubClient. The advisories of the actions are the ones of github.com, whichever host
// has the repository: an Enterprise Server has no database of them for actions.
func (c *fallbackGitHubClient) Advisories(ctx context.Context, owner, repo string) ([]GitHubAdvisory, error) {
	if down := c.secondaryDown(); down != nil {
		return nil, down
	}
	v, err := c.secondary.Advisories(ctx, owner, repo)
	return v, c.fromSecondary(err)
}

// AdvisoriesForPackages implements GitHubPackageAdvisoryClient.
func (c *fallbackGitHubClient) AdvisoriesForPackages(ctx context.Context, packages []string) ([]GitHubAdvisory, error) {
	if down := c.secondaryDown(); down != nil {
		return nil, down
	}
	if pc, ok := c.secondary.(GitHubPackageAdvisoryClient); ok {
		v, err := pc.AdvisoriesForPackages(ctx, packages)
		return v, c.fromSecondary(err)
	}
	if len(packages) == 0 {
		return nil, nil
	}
	owner, rest, _ := strings.Cut(packages[0], "/")
	repo, _, _ := strings.Cut(rest, "/")
	v, err := c.secondary.Advisories(ctx, owner, repo)
	return v, c.fromSecondary(err)
}

// fromHost marks the error of a call to the host of the route as the one of that host when it is github.com.
func (c *fallbackGitHubClient) fromHost(r fallbackRoute, err error) error {
	if r.secondary {
		return c.fromSecondary(err)
	}
	return err
}

// CommitOnAnyBranch implements GitHubBranchScanner for the host which has the repository.
func (c *fallbackGitHubClient) CommitOnAnyBranch(ctx context.Context, owner, repo, sha string, limit int) (GitHubBranchScan, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return GitHubBranchScan{}, ErrGitHubBranchScanUnavailable
	}
	if s, ok := r.client.(GitHubBranchScanner); ok {
		v, err := s.CommitOnAnyBranch(ctx, owner, repo, sha, limit)
		return v, c.fromHost(r, err)
	}
	return GitHubBranchScan{}, ErrGitHubBranchScanUnavailable
}

// answerStats implements answerStatsSource: the answers of both hosts.
func (c *fallbackGitHubClient) answerStats() OnlineAnswers {
	var total OnlineAnswers
	for _, cl := range []GitHubClient{c.primary, c.secondary} {
		src, ok := cl.(answerStatsSource)
		if !ok {
			continue
		}
		a := src.answerStats()
		total.Network += a.Network
		total.Cache += a.Cache
		total.Stale += a.Stale
		if a.OldestFetched != "" && (total.OldestFetched == "" || a.OldestFetched < total.OldestFetched) {
			total.OldestFetched = a.OldestFetched
		}
		total.CacheTTLSeconds = max(total.CacheTTLSeconds, a.CacheTTLSeconds)
	}
	return total
}
