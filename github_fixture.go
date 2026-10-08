package jactionlint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// This file has the record/replay GitHubClient. A recording client wraps a real one and keeps every
// answer; its JSON (RecordingGitHubClient.Fixtures) can be saved and later served by a fixture
// client, so tests of the online rules need no network and see the same data every time. The
// tests of jactionlint itself and the examples of the documentation use it
// (testdata/online/github.json).

// gitHubFixtures is the file format. It is plain JSON organized by repository, so a fixture can be
// edited by hand.
type gitHubFixtures struct {
	// Repos maps "owner/repo" in lower case to the recorded data.
	Repos map[string]*fixtureRepo `json:"repos"`
}

type fixtureRepo struct {
	// Missing means the repository does not exist: every call fails with ErrGitHubNotFound.
	Missing    bool                           `json:"missing,omitempty"`
	Repo       *GitHubRepo                    `json:"repo,omitempty"`
	Tags       *GitHubTagList                 `json:"tags,omitempty"`
	Branches   *GitHubBranchList              `json:"branches,omitempty"`
	Refs       map[string]*fixtureRef         `json:"refs,omitempty"`    // "heads/main", "tags/v1"
	Compare    map[string]GitHubCompareStatus `json:"compare,omitempty"` // "base...head"; "not-found" for unknown commits
	Advisories *fixtureAdvisories             `json:"advisories,omitempty"`
}

type fixtureRef struct {
	SHA   string `json:"sha,omitempty"`
	Found bool   `json:"found"`
}

type fixtureAdvisories struct {
	List []GitHubAdvisory `json:"list"`
}

const fixtureCompareNotFound GitHubCompareStatus = "not-found"

func fixtureKey(owner, repo string) string { return strings.ToLower(owner + "/" + repo) }

// FixtureGitHubClient is a GitHubClient which serves recorded data. A call for data which was not
// recorded fails, so a test notices when a rule starts asking for something new. Create it with
// NewFixtureGitHubClient.
type FixtureGitHubClient struct {
	data gitHubFixtures

	mu    sync.Mutex
	calls int
}

// NewFixtureGitHubClient parses fixtures made by a RecordingGitHubClient (or written by hand) and
// returns a client which serves them.
func NewFixtureGitHubClient(data []byte) (*FixtureGitHubClient, error) {
	var f gitHubFixtures
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("invalid GitHub fixtures: %w", err)
	}
	if f.Repos == nil {
		f.Repos = map[string]*fixtureRepo{}
	}
	return &FixtureGitHubClient{data: f}, nil
}

// Calls returns how many calls the client served. Tests use it to check that lookups are deduplicated.
func (c *FixtureGitHubClient) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *FixtureGitHubClient) repo(owner, repo, what string) (*fixtureRepo, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	r, ok := c.data.Repos[fixtureKey(owner, repo)]
	if !ok {
		return nil, fmt.Errorf("the GitHub fixtures have no data for %s/%s (%s)", owner, repo, what)
	}
	if r.Missing {
		return nil, fmt.Errorf("%s/%s: %w", owner, repo, ErrGitHubNotFound)
	}
	return r, nil
}

// Repository implements GitHubClient.
func (c *FixtureGitHubClient) Repository(_ context.Context, owner, repo string) (*GitHubRepo, error) {
	r, err := c.repo(owner, repo, "repository")
	if err != nil {
		return nil, err
	}
	if r.Repo == nil {
		return nil, fmt.Errorf("the GitHub fixtures have no repository data for %s/%s", owner, repo)
	}
	cp := *r.Repo
	return &cp, nil
}

// Tags implements GitHubClient.
func (c *FixtureGitHubClient) Tags(_ context.Context, owner, repo string) (*GitHubTagList, error) {
	r, err := c.repo(owner, repo, "tags")
	if err != nil {
		return nil, err
	}
	if r.Tags == nil {
		return nil, fmt.Errorf("the GitHub fixtures have no tags for %s/%s", owner, repo)
	}
	cp := *r.Tags
	return &cp, nil
}

// ResolveRef implements GitHubClient.
func (c *FixtureGitHubClient) ResolveRef(_ context.Context, owner, repo string, ns GitHubRefNamespace, name string) (string, bool, error) {
	r, err := c.repo(owner, repo, "ref")
	if err != nil {
		return "", false, err
	}
	ref, ok := r.Refs[string(ns)+"/"+name]
	if !ok {
		return "", false, fmt.Errorf("the GitHub fixtures have no ref %s/%s for %s/%s", ns, name, owner, repo)
	}
	return ref.SHA, ref.Found, nil
}

// Branches implements GitHubClient.
func (c *FixtureGitHubClient) Branches(_ context.Context, owner, repo string, limit int) (*GitHubBranchList, error) {
	r, err := c.repo(owner, repo, "branches")
	if err != nil {
		return nil, err
	}
	if r.Branches == nil {
		return nil, fmt.Errorf("the GitHub fixtures have no branches for %s/%s", owner, repo)
	}
	cp := *r.Branches
	if limit > 0 && len(cp.Branches) > limit {
		cp.Branches, cp.Truncated = cp.Branches[:limit], true
	}
	return &cp, nil
}

// Compare implements GitHubClient.
func (c *FixtureGitHubClient) Compare(_ context.Context, owner, repo, base, head string) (GitHubCompareStatus, error) {
	r, err := c.repo(owner, repo, "compare")
	if err != nil {
		return "", err
	}
	s, ok := r.Compare[base+"..."+head]
	if !ok {
		return "", fmt.Errorf("the GitHub fixtures have no comparison %s...%s for %s/%s", base, head, owner, repo)
	}
	if s == fixtureCompareNotFound {
		return "", fmt.Errorf("%s...%s: %w", base, head, ErrGitHubNotFound)
	}
	return s, nil
}

// Advisories implements GitHubClient.
func (c *FixtureGitHubClient) Advisories(_ context.Context, owner, repo string) ([]GitHubAdvisory, error) {
	r, err := c.repo(owner, repo, "advisories")
	if err != nil {
		return nil, err
	}
	if r.Advisories == nil {
		return nil, fmt.Errorf("the GitHub fixtures have no advisories for %s/%s", owner, repo)
	}
	return append([]GitHubAdvisory(nil), r.Advisories.List...), nil
}

// RecordingGitHubClient wraps a GitHubClient and records the answers it gives. Fixtures returns them
// in the format NewFixtureGitHubClient reads.
type RecordingGitHubClient struct {
	inner GitHubClient

	mu   sync.Mutex
	data gitHubFixtures
}

// NewRecordingGitHubClient wraps the client.
func NewRecordingGitHubClient(inner GitHubClient) *RecordingGitHubClient {
	return &RecordingGitHubClient{inner: inner, data: gitHubFixtures{Repos: map[string]*fixtureRepo{}}}
}

func (c *RecordingGitHubClient) rec(owner, repo string, fn func(r *fixtureRepo)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := fixtureKey(owner, repo)
	r := c.data.Repos[k]
	if r == nil {
		r = &fixtureRepo{}
		c.data.Repos[k] = r
	}
	fn(r)
}

// Fixtures returns the recorded answers as JSON, sorted so that recording twice gives the same file.
func (c *RecordingGitHubClient) Fixtures() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.data.Repos))
	for k := range c.data.Repos {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, r := range c.data.Repos {
		if r.Tags != nil {
			sort.Slice(r.Tags.Tags, func(i, j int) bool { return r.Tags.Tags[i].Name < r.Tags.Tags[j].Name })
		}
	}
	b, err := json.MarshalIndent(c.data, "", "  ") // Map keys are sorted by encoding/json
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Repository implements GitHubClient.
func (c *RecordingGitHubClient) Repository(ctx context.Context, owner, repo string) (*GitHubRepo, error) {
	v, err := c.inner.Repository(ctx, owner, repo)
	switch {
	case err == nil:
		c.rec(owner, repo, func(r *fixtureRepo) { cp := *v; r.Repo = &cp })
	case isNotFound(err):
		c.rec(owner, repo, func(r *fixtureRepo) { r.Missing = true })
	}
	return v, err
}

// Tags implements GitHubClient.
func (c *RecordingGitHubClient) Tags(ctx context.Context, owner, repo string) (*GitHubTagList, error) {
	v, err := c.inner.Tags(ctx, owner, repo)
	switch {
	case err == nil:
		c.rec(owner, repo, func(r *fixtureRepo) { cp := *v; r.Tags = &cp })
	case isNotFound(err):
		c.rec(owner, repo, func(r *fixtureRepo) { r.Missing = true })
	}
	return v, err
}

// ResolveRef implements GitHubClient.
func (c *RecordingGitHubClient) ResolveRef(ctx context.Context, owner, repo string, ns GitHubRefNamespace, name string) (string, bool, error) {
	sha, found, err := c.inner.ResolveRef(ctx, owner, repo, ns, name)
	switch {
	case err == nil:
		c.rec(owner, repo, func(r *fixtureRepo) {
			if r.Refs == nil {
				r.Refs = map[string]*fixtureRef{}
			}
			r.Refs[string(ns)+"/"+name] = &fixtureRef{SHA: sha, Found: found}
		})
	case isNotFound(err):
		c.rec(owner, repo, func(r *fixtureRepo) { r.Missing = true })
	}
	return sha, found, err
}

// Branches implements GitHubClient.
func (c *RecordingGitHubClient) Branches(ctx context.Context, owner, repo string, limit int) (*GitHubBranchList, error) {
	v, err := c.inner.Branches(ctx, owner, repo, limit)
	switch {
	case err == nil:
		c.rec(owner, repo, func(r *fixtureRepo) { cp := *v; r.Branches = &cp })
	case isNotFound(err):
		c.rec(owner, repo, func(r *fixtureRepo) { r.Missing = true })
	}
	return v, err
}

// Compare implements GitHubClient.
func (c *RecordingGitHubClient) Compare(ctx context.Context, owner, repo, base, head string) (GitHubCompareStatus, error) {
	s, err := c.inner.Compare(ctx, owner, repo, base, head)
	record := func(s GitHubCompareStatus) {
		c.rec(owner, repo, func(r *fixtureRepo) {
			if r.Compare == nil {
				r.Compare = map[string]GitHubCompareStatus{}
			}
			r.Compare[base+"..."+head] = s
		})
	}
	switch {
	case err == nil:
		record(s)
	case isNotFound(err):
		record(fixtureCompareNotFound)
	}
	return s, err
}

// Advisories implements GitHubClient.
func (c *RecordingGitHubClient) Advisories(ctx context.Context, owner, repo string) ([]GitHubAdvisory, error) {
	v, err := c.inner.Advisories(ctx, owner, repo)
	if err == nil {
		c.rec(owner, repo, func(r *fixtureRepo) { r.Advisories = &fixtureAdvisories{List: append([]GitHubAdvisory{}, v...)} })
	}
	return v, err
}

func isNotFound(err error) bool {
	return errors.Is(err, ErrGitHubNotFound)
}
