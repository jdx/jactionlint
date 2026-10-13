package jactionlint

import (
	"context"
	"errors"
	"strings"
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
		info2, err2 := c.secondary.Repository(ctx, owner, repo)
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
	return r.client.Tags(ctx, owner, repo)
}

// ResolveRef implements GitHubClient.
func (c *fallbackGitHubClient) ResolveRef(ctx context.Context, owner, repo string, ns GitHubRefNamespace, name string) (string, bool, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return "", false, r.err
	}
	return r.client.ResolveRef(ctx, owner, repo, ns, name)
}

// Branches implements GitHubClient.
func (c *fallbackGitHubClient) Branches(ctx context.Context, owner, repo string, limit int) (*GitHubBranchList, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return nil, r.err
	}
	return r.client.Branches(ctx, owner, repo, limit)
}

// Compare implements GitHubClient.
func (c *fallbackGitHubClient) Compare(ctx context.Context, owner, repo, base, head string) (GitHubCompareStatus, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return "", r.err
	}
	return r.client.Compare(ctx, owner, repo, base, head)
}

// Advisories implements GitHubClient. The advisories of the actions are the ones of github.com, whichever host
// has the repository: an Enterprise Server has no database of them for actions.
func (c *fallbackGitHubClient) Advisories(ctx context.Context, owner, repo string) ([]GitHubAdvisory, error) {
	return c.secondary.Advisories(ctx, owner, repo)
}

// AdvisoriesForPackages implements GitHubPackageAdvisoryClient.
func (c *fallbackGitHubClient) AdvisoriesForPackages(ctx context.Context, packages []string) ([]GitHubAdvisory, error) {
	if pc, ok := c.secondary.(GitHubPackageAdvisoryClient); ok {
		return pc.AdvisoriesForPackages(ctx, packages)
	}
	if len(packages) == 0 {
		return nil, nil
	}
	owner, rest, _ := strings.Cut(packages[0], "/")
	repo, _, _ := strings.Cut(rest, "/")
	return c.secondary.Advisories(ctx, owner, repo)
}

// CommitOnAnyBranch implements GitHubBranchScanner for the host which has the repository.
func (c *fallbackGitHubClient) CommitOnAnyBranch(ctx context.Context, owner, repo, sha string, limit int) (GitHubBranchScan, error) {
	r := c.route(ctx, owner, repo)
	if r.err != nil {
		return GitHubBranchScan{}, ErrGitHubBranchScanUnavailable
	}
	if s, ok := r.client.(GitHubBranchScanner); ok {
		return s.CommitOnAnyBranch(ctx, owner, repo, sha, limit)
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
