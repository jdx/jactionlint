//go:build !js

package jactionlint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultGitHubAPIURL = "https://api.github.com"
	// gitHubRequestTimeout bounds one request.
	gitHubRequestTimeout = 20 * time.Second
	// maxGitHubInFlight bounds the requests running at once. GitHub throttles bursts ("secondary rate
	// limit") before the primary limit is reached.
	maxGitHubInFlight = 6
	// maxGitHubPages bounds how many pages of a list are read.
	maxGitHubPages = 10
	// maxGitHubBodyBytes bounds how much of an answer is read.
	maxGitHubBodyBytes = 16 << 20
	// maxTagDerefs bounds how many tag objects are followed to reach a commit.
	maxTagDerefs = 5
)

// httpGitHubClient is the GitHubClient which talks to the GitHub REST API. It authenticates with
// $GITHUB_TOKEN or $GH_TOKEN when one is set (unauthenticated requests work with a much lower rate
// limit), reads $GITHUB_API_URL for GitHub Enterprise Server, and keeps answers in a diskCache:
// an answer younger than the TTL is used as is, an older one is revalidated with its ETag (a 304
// answer does not count against the rate limit of an authenticated client).
//
// It never retries. After the API reports that the rate limit is reached, later requests fail
// without a network call (answers in the cache, even expired ones, are still used).
type httpGitHubClient struct {
	base   *url.URL
	hc     *http.Client
	cache  *diskCache
	ttl    time.Duration
	sem    chan struct{}
	notify func(msg string) // for notices which are not errors; can be nil
	debug  func(format string, args ...any)
	// graphql overrides the URL of the GraphQL API.
	graphql string

	mu          sync.Mutex
	token       string
	limit       *GitHubRateLimitError // set once the limit is reached
	noticedAnon bool
}

// httpGitHubOptions configures newHTTPGitHubClient.
type httpGitHubOptions struct {
	// Token is the access token. Empty means unauthenticated.
	Token string
	// BaseURL is the URL of the API.
	BaseURL string
	// CacheDir is the directory of the cache. Empty disables caching.
	CacheDir string
	// TTL is how long cached answers are used without revalidation.
	TTL time.Duration
	// HTTPClient is used instead of the default one when set.
	HTTPClient *http.Client
	// Notify receives notices such as "no token set".
	Notify func(msg string)
	// GraphQLURL is the URL of the GraphQL API. Empty derives it from BaseURL.
	GraphQLURL string
	// Debug receives a line for every request and cache hit. It can be nil.
	Debug func(format string, args ...any)
}

// newHTTPGitHubClient creates the client.
func newHTTPGitHubClient(o httpGitHubOptions) (*httpGitHubClient, error) {
	if o.BaseURL == "" {
		o.BaseURL = defaultGitHubAPIURL
	}
	base, err := url.Parse(strings.TrimRight(o.BaseURL, "/"))
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" {
		return nil, fmt.Errorf("invalid GitHub API URL %q", o.BaseURL)
	}
	hc := o.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: gitHubRequestTimeout}
	}
	c := &httpGitHubClient{base: base, hc: hc, ttl: o.TTL, token: o.Token, notify: o.Notify, debug: o.Debug, graphql: o.GraphQLURL, sem: make(chan struct{}, maxGitHubInFlight)}
	if o.CacheDir != "" {
		c.cache = newDiskCache(o.CacheDir, 0)
	}
	return c, nil
}

// githubTokenFromEnv returns the token for the GitHub API from the environment.
func githubTokenFromEnv() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// newDefaultGitHubClient creates the client used by the jactionlint command: the token and API URL
// come from the environment and the cache is in the user cache directory.
func newDefaultGitHubClient(ttl time.Duration, notify func(string), debug func(string, ...any)) (GitHubClient, error) {
	if ttl < 0 {

	}
	return newHTTPGitHubClient(httpGitHubOptions{
		Token:      githubTokenFromEnv(),
		BaseURL:    os.Getenv("GITHUB_API_URL"),
		GraphQLURL: os.Getenv("GITHUB_GRAPHQL_URL"),
		CacheDir:   defaultGitHubCacheDir(),
		TTL:        ttl,
		Notify:     notify,
		Debug:      debug,
	})
}

// httpAnswer is the answer of one request.
type httpAnswer struct {
	status int
	body   []byte
	link   string
}

func (c *httpGitHubClient) currentToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

func (c *httpGitHubClient) rateLimited() *GitHubRateLimitError {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limit != nil && !c.limit.Reset.IsZero() && time.Now().After(c.limit.Reset) {
		c.limit = nil // The window has passed
	}
	return c.limit
}

func (c *httpGitHubClient) setRateLimited(reset time.Time, authenticated bool) *GitHubRateLimitError {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limit = &GitHubRateLimitError{Reset: reset, Authenticated: authenticated}
	return c.limit
}

// answerFromEntry converts a cache entry to an answer.
func answerFromEntry(e *cacheEntry) *httpAnswer {
	return &httpAnswer{status: e.Status, body: e.Body, link: e.Link}
}

// get fetches an API URL (a path relative to the API, or an absolute URL of the same host).
func (c *httpGitHubClient) get(ctx context.Context, target string) (*httpAnswer, error) {
	u := target
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		u = c.base.String() + target
	}
	pu, err := url.Parse(u)
	if err != nil || pu.Host != c.base.Host || pu.Scheme != c.base.Scheme {
		return nil, fmt.Errorf("refusing to request %q: not an URL of the GitHub API", target)
	}

	token := c.currentToken()
	scope := "anonymous"
	if token != "" {
		h := cacheKey("token", token)
		scope = "token:" + h
	}
	key := cacheKey(scope, u)
	entry := c.cache.get(key, u)
	if entry != nil && c.ttl > 0 && time.Since(entry.Fetched) < c.ttl {
		c.logf("GET %s: from the cache", target)
		return answerFromEntry(entry), nil
	}
	if rl := c.rateLimited(); rl != nil {
		if entry != nil {
			return answerFromEntry(entry), nil // Stale data is better than none
		}
		return nil, rl
	}

	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	ans, err := c.do(ctx, u, token, entry, key, false)
	if err != nil && entry != nil && errors.Is(err, ErrGitHubRateLimited) {
		return answerFromEntry(entry), nil
	}
	return ans, err
}

// do sends one request. retryAnon is true on the single repeat without the token after the token was
// rejected.
func (c *httpGitHubClient) do(ctx context.Context, u, token string, entry *cacheEntry, key string, retryAnon bool) (*httpAnswer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "jactionlint/"+getCommandVersion())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		c.noticeAnonymous()
	}
	if entry != nil && entry.ETag != "" && entry.Status == http.StatusOK {
		req.Header.Set("If-None-Match", entry.ETag)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach the GitHub API: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("could not read the answer of the GitHub API: %w", err)
	}
	if len(body) > maxGitHubBodyBytes {
		return nil, fmt.Errorf("the answer of the GitHub API for %s is too big", resp.Request.URL.Path)
	}

	c.noteRateLimit(resp, token != "")
	c.logf("GET %s: %d (rate limit remaining: %s)", u, resp.StatusCode, resp.Header.Get("X-RateLimit-Remaining"))

	switch st := resp.StatusCode; {
	case st == http.StatusNotModified && entry != nil:
		entry.Fetched = time.Now()
		c.cache.put(key, entry)
		return answerFromEntry(entry), nil
	case st == http.StatusOK:
		e := &cacheEntry{URL: u, ETag: resp.Header.Get("ETag"), Fetched: time.Now(), Status: st, Link: resp.Header.Get("Link"), Body: body}
		c.cache.put(key, e)
		return answerFromEntry(e), nil
	case st == http.StatusNotFound || st == http.StatusGone || st == http.StatusUnavailableForLegalReasons || st == http.StatusUnprocessableEntity:
		// Remember that it does not exist for the TTL so that the same lookup is not repeated
		c.cache.put(key, &cacheEntry{URL: u, Fetched: time.Now(), Status: st})
		return &httpAnswer{status: st}, nil
	case st == http.StatusUnauthorized && token != "" && !retryAnon:
		c.dropToken()
		return c.do(ctx, u, "", nil, cacheKey("anonymous", u), true)
	case st == http.StatusForbidden || st == http.StatusTooManyRequests:
		if rl := c.limitFromAnswer(resp, body, token != ""); rl != nil {
			return nil, rl
		}
	}
	return nil, &GitHubStatusError{Status: resp.StatusCode, Message: apiMessage(body)}
}

func (c *httpGitHubClient) logf(format string, args ...any) {
	if c.debug != nil {
		c.debug(format, args...)
	}
}

func (c *httpGitHubClient) noticeAnonymous() {
	c.mu.Lock()
	first := !c.noticedAnon
	c.noticedAnon = true
	c.mu.Unlock()
	if first && c.notify != nil {
		c.notify("no GITHUB_TOKEN or GH_TOKEN is set so the online checks use unauthenticated requests, which GitHub limits to 60 per hour. set a token to raise the limit")
	}
}

func (c *httpGitHubClient) dropToken() {
	c.mu.Lock()
	had := c.token != ""
	c.token = ""
	c.noticedAnon = true // Not "no token is set": it was, and was refused
	c.mu.Unlock()
	if had && c.notify != nil {
		c.notify("the token in GITHUB_TOKEN or GH_TOKEN was rejected by the GitHub API (401). continuing without it")
	}
}

// noteRateLimit remembers that the limit is used up when the answer says so, so that no request is
// wasted afterwards.
func (c *httpGitHubClient) noteRateLimit(resp *http.Response, authenticated bool) {
	if resp.Header.Get("X-RateLimit-Remaining") != "0" {
		return
	}
	if reset := parseResetHeader(resp.Header.Get("X-RateLimit-Reset")); !reset.IsZero() && time.Now().Before(reset) {
		c.setRateLimited(reset, authenticated)
	}
}

// limitFromAnswer returns the rate limit error when a 403 or 429 answer is about the rate limit. Other
// 403 answers (a blocked repository, a missing scope) are ordinary failures.
func (c *httpGitHubClient) limitFromAnswer(resp *http.Response, body []byte, authenticated bool) *GitHubRateLimitError {
	retryAfter := resp.Header.Get("Retry-After")
	msg := strings.ToLower(apiMessage(body))
	if resp.Header.Get("X-RateLimit-Remaining") != "0" && retryAfter == "" && resp.StatusCode != http.StatusTooManyRequests && !strings.Contains(msg, "rate limit") {
		return nil
	}
	reset := parseResetHeader(resp.Header.Get("X-RateLimit-Reset"))
	if n, err := strconv.Atoi(retryAfter); err == nil && n >= 0 {
		reset = time.Now().Add(time.Duration(n) * time.Second)
	}
	if reset.IsZero() || time.Now().After(reset) {
		reset = time.Now().Add(time.Minute) // Unknown: do not ask again for a minute
	}
	return c.setRateLimited(reset, authenticated)
}

func parseResetHeader(v string) time.Time {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

func apiMessage(body []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil {
		return m.Message
	}
	return ""
}

// getJSON fetches the URL and decodes the answer. Missing things are ErrGitHubNotFound.
func (c *httpGitHubClient) getJSON(ctx context.Context, target string, out any) (*httpAnswer, error) {
	ans, err := c.get(ctx, target)
	if err != nil {
		return nil, err
	}
	if ans.status != http.StatusOK {
		return ans, fmt.Errorf("%s: %w", strings.SplitN(target, "?", 2)[0], ErrGitHubNotFound)
	}
	if out != nil {
		if err := json.Unmarshal(ans.body, out); err != nil {
			return ans, fmt.Errorf("could not decode the answer of the GitHub API for %s: %w", target, err)
		}
	}
	return ans, nil
}

// nextLink returns the URL of the next page from a Link header, or "".
func nextLink(link string) string {
	for _, part := range strings.Split(link, ",") {
		segs := strings.Split(part, ";")
		if len(segs) < 2 {
			continue
		}
		for _, s := range segs[1:] {
			if strings.TrimSpace(s) == `rel="next"` {
				return strings.Trim(strings.TrimSpace(segs[0]), "<>")
			}
		}
	}
	return ""
}

// getPages fetches a list and passes every page to the function. It reads at most maxPages pages and
// reports whether more were left.
func (c *httpGitHubClient) getPages(ctx context.Context, target string, maxPages int, page func(body []byte) error) (truncated bool, err error) {
	for i := 0; target != ""; i++ {
		if i >= maxPages {
			return true, nil
		}
		ans, err := c.get(ctx, target)
		if err != nil {
			return false, err
		}
		if ans.status != http.StatusOK {
			return false, fmt.Errorf("%s: %w", strings.SplitN(target, "?", 2)[0], ErrGitHubNotFound)
		}
		if err := page(ans.body); err != nil {
			return false, err
		}
		target = nextLink(ans.link)
	}
	return false, nil
}

func repoPath(owner, repo string) string {
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

// escapeRefName escapes a ref name for a URL path. Slashes are kept: "feature/x" is two segments.
func escapeRefName(name string) string {
	segs := strings.Split(name, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// Repository implements GitHubClient.
func (c *httpGitHubClient) Repository(ctx context.Context, owner, repo string) (*GitHubRepo, error) {
	var r GitHubRepo
	if _, err := c.getJSON(ctx, repoPath(owner, repo), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Tags implements GitHubClient.
func (c *httpGitHubClient) Tags(ctx context.Context, owner, repo string) (*GitHubTagList, error) {
	l := &GitHubTagList{}
	truncated, err := c.getPages(ctx, repoPath(owner, repo)+"/tags?per_page=100", maxGitHubPages, func(body []byte) error {
		var tags []struct {
			Name   string `json:"name"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if err := json.Unmarshal(body, &tags); err != nil {
			return err
		}
		for _, t := range tags {
			l.Tags = append(l.Tags, GitHubTag{Name: t.Name, SHA: t.Commit.SHA})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	l.Truncated = truncated
	return l, nil
}

type gitObject struct {
	Object struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"object"`
}

// ResolveRef implements GitHubClient.
func (c *httpGitHubClient) ResolveRef(ctx context.Context, owner, repo string, ns GitHubRefNamespace, name string) (string, bool, error) {
	var ref gitObject
	_, err := c.getJSON(ctx, repoPath(owner, repo)+"/git/ref/"+string(ns)+"/"+escapeRefName(name), &ref)
	if errors.Is(err, ErrGitHubNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	// An annotated tag points to a tag object which points to the commit (or to another tag)
	for i := 0; ref.Object.Type == "tag" && i < maxTagDerefs; i++ {
		sha := ref.Object.SHA
		ref = gitObject{}
		if _, err := c.getJSON(ctx, repoPath(owner, repo)+"/git/tags/"+url.PathEscape(sha), &ref); err != nil {
			return "", false, err
		}
	}
	if ref.Object.Type != "commit" {
		return "", false, nil // Points to a tree or a blob: no action can be there
	}
	return ref.Object.SHA, true, nil
}

// Branches implements GitHubClient.
func (c *httpGitHubClient) Branches(ctx context.Context, owner, repo string, limit int) (*GitHubBranchList, error) {
	l := &GitHubBranchList{}
	limit = max(limit, 1)
	perPage := min(limit, 100)
	pages := (limit + perPage - 1) / perPage
	truncated, err := c.getPages(ctx, repoPath(owner, repo)+"/branches?per_page="+strconv.Itoa(perPage), min(pages, maxGitHubPages), func(body []byte) error {
		var bs []struct {
			Name   string `json:"name"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if err := json.Unmarshal(body, &bs); err != nil {
			return err
		}
		for _, b := range bs {
			l.Branches = append(l.Branches, GitHubBranch{Name: b.Name, SHA: b.Commit.SHA})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(l.Branches) > limit {
		l.Branches, truncated = l.Branches[:limit], true
	}
	l.Truncated = truncated
	return l, nil
}

// Compare implements GitHubClient.
func (c *httpGitHubClient) Compare(ctx context.Context, owner, repo, base, head string) (GitHubCompareStatus, error) {
	var r struct {
		Status string `json:"status"`
	}
	if _, err := c.getJSON(ctx, repoPath(owner, repo)+"/compare/"+url.PathEscape(base)+"..."+url.PathEscape(head)+"?per_page=1", &r); err != nil {
		return "", err
	}
	s := GitHubCompareStatus(r.Status)
	switch s {
	case GitHubCompareIdentical, GitHubCompareAhead, GitHubCompareBehind, GitHubCompareDiverged:
		return s, nil
	}
	return "", fmt.Errorf("unexpected comparison status %q", r.Status)
}

// Advisories implements GitHubClient.
func (c *httpGitHubClient) Advisories(ctx context.Context, owner, repo string) ([]GitHubAdvisory, error) {
	return c.AdvisoriesForPackages(ctx, []string{owner + "/" + repo})
}

// AdvisoriesForPackages implements GitHubPackageAdvisoryClient. The API takes a comma separated list of
// packages in "affects", so a repository and one of its subdirectories cost one request.
func (c *httpGitHubClient) AdvisoriesForPackages(ctx context.Context, packages []string) ([]GitHubAdvisory, error) {
	var ret []GitHubAdvisory
	seen := map[string]bool{}
	q := url.Values{"ecosystem": {"actions"}, "affects": {strings.Join(packages, ",")}, "per_page": {"100"}}
	_, err := c.getPages(ctx, "/advisories?"+q.Encode(), maxGitHubPages, func(body []byte) error {
		var as []struct {
			ID              string `json:"ghsa_id"`
			CVE             string `json:"cve_id"`
			Summary         string `json:"summary"`
			Severity        string `json:"severity"`
			URL             string `json:"html_url"`
			WithdrawnAt     string `json:"withdrawn_at"`
			Vulnerabilities []struct {
				Package struct {
					Ecosystem string `json:"ecosystem"`
					Name      string `json:"name"`
				} `json:"package"`
				Range        string `json:"vulnerable_version_range"`
				FirstPatched any    `json:"first_patched_version"`
			} `json:"vulnerabilities"`
		}
		if err := json.Unmarshal(body, &as); err != nil {
			return err
		}
		for _, a := range as {
			if a.WithdrawnAt != "" || seen[a.ID] {
				continue
			}
			seen[a.ID] = true
			adv := GitHubAdvisory{ID: a.ID, CVE: a.CVE, Summary: a.Summary, Severity: a.Severity, URL: a.URL}
			for _, v := range a.Vulnerabilities {
				if !strings.EqualFold(v.Package.Ecosystem, "actions") {
					continue
				}
				patched, _ := v.FirstPatched.(string)
				adv.Vulnerabilities = append(adv.Vulnerabilities, GitHubVulnerability{Package: v.Package.Name, VulnerableRange: v.Range, FirstPatched: patched})
			}
			ret = append(ret, adv)
		}
		return nil
	})
	return ret, err
}

// onlineSupported is false where the GitHub API cannot be used (the WebAssembly build).
const onlineSupported = true

// graphqlURL returns the URL of the GraphQL API: $GITHUB_GRAPHQL_URL when set, else the sibling of the REST
// API (api.github.com/graphql, or host/api/graphql for the host/api/v3 of GitHub Enterprise Server).
func (c *httpGitHubClient) graphqlURL() string {
	if c.graphql != "" {
		return c.graphql
	}
	u := *c.base
	if p, ok := strings.CutSuffix(u.Path, "/v3"); ok {
		u.Path = p + "/graphql"
	} else {
		u.Path = strings.TrimRight(u.Path, "/") + "/graphql"
	}
	return u.String()
}

const branchScanQuery = `query($owner:String!,$name:String!,$sha:String!,$after:String){
  repository(owner:$owner,name:$name){
    refs(refPrefix:"refs/heads/",first:100,after:$after){
      pageInfo{hasNextPage endCursor}
      nodes{name compare(headRef:$sha){status}}
    }
  }
}`

// CommitOnAnyBranch implements GitHubBranchScanner with the GraphQL API, which can compare a commit with
// 100 branches in one request. GraphQL needs a token, so without one it is unavailable. The result
// is kept in the cache for the TTL like the answers of the REST API (a POST has no ETag).
func (c *httpGitHubClient) CommitOnAnyBranch(ctx context.Context, owner, repo, sha string, limit int) (GitHubBranchScan, error) {
	token := c.currentToken()
	if token == "" {
		return GitHubBranchScan{}, ErrGitHubBranchScanUnavailable
	}
	if !reSafeRepoPart.MatchString(owner) || !reSafeRepoPart.MatchString(repo) || len(sha) != 40 {
		return GitHubBranchScan{}, ErrGitHubBranchScanUnavailable
	}
	limit = max(limit, 1)
	// The answer is cached like a GET under a name that cannot be a URL of the REST API
	name := fmt.Sprintf("graphql:branch-scan:%s/%s@%s:%d", strings.ToLower(owner), strings.ToLower(repo), strings.ToLower(sha), limit)
	key := cacheKey("token:"+cacheKey("token", token), name)
	if e := c.cache.get(key, name); e != nil && c.ttl > 0 && time.Since(e.Fetched) < c.ttl {
		var scan GitHubBranchScan
		if json.Unmarshal(e.Body, &scan) == nil {
			c.logf("GraphQL %s: from the cache", name)
			return scan, nil
		}
	}
	if rl := c.rateLimited(); rl != nil {
		return GitHubBranchScan{}, rl
	}

	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return GitHubBranchScan{}, ctx.Err()
	}

	scan := GitHubBranchScan{Complete: true}
	seen := 0
	after := (*string)(nil)
	for {
		var res struct {
			Data struct {
				Repository *struct {
					Refs struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							Compare *struct {
								Status string `json:"status"`
							} `json:"compare"`
						} `json:"nodes"`
					} `json:"refs"`
				} `json:"repository"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		vars := map[string]any{"owner": owner, "name": repo, "sha": sha, "after": after}
		if err := c.postGraphQL(ctx, token, branchScanQuery, vars, &res); err != nil {
			return GitHubBranchScan{}, err
		}
		if len(res.Errors) > 0 || res.Data.Repository == nil {
			msg := "no data"
			if len(res.Errors) > 0 {
				msg = res.Errors[0].Message
			}
			// The scan is an optimization. An error of the query (a field that cannot be resolved, for
			// example) must not count as a failure of the session: the caller compares the branches over REST.
			return GitHubBranchScan{}, fmt.Errorf("%w: GraphQL: %s", ErrGitHubBranchScanUnavailable, msg)
		}
		refs := res.Data.Repository.Refs
		for _, n := range refs.Nodes {
			if seen >= limit {
				scan.Complete = false
				break
			}
			seen++
			if n.Compare != nil && (n.Compare.Status == "BEHIND" || n.Compare.Status == "IDENTICAL") {
				scan.Found = true
			}
		}
		if scan.Found {
			scan.Complete = false // Does not matter
			break
		}
		if !refs.PageInfo.HasNextPage || !scan.Complete {
			break
		}
		cursor := refs.PageInfo.EndCursor
		after = &cursor
	}
	if b, err := json.Marshal(scan); err == nil {
		c.cache.put(key, &cacheEntry{URL: name, Fetched: time.Now(), Status: http.StatusOK, Body: b})
	}
	return scan, nil
}

// postGraphQL sends a GraphQL query.
func (c *httpGitHubClient) postGraphQL(ctx context.Context, token, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlURL(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "jactionlint/"+getCommandVersion())
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach the GitHub API: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubBodyBytes+1))
	if err != nil {
		return fmt.Errorf("could not read the answer of the GitHub API: %w", err)
	}
	if len(data) > maxGitHubBodyBytes {
		return errors.New("the answer of the GitHub GraphQL API is too big")
	}
	c.noteRateLimit(resp, true)
	c.logf("POST %s: %d (rate limit remaining: %s)", c.graphqlURL(), resp.StatusCode, resp.Header.Get("X-RateLimit-Remaining"))
	switch resp.StatusCode {
	case http.StatusOK:
		return json.Unmarshal(data, out)
	case http.StatusUnauthorized:
		c.dropToken()
		return ErrGitHubBranchScanUnavailable // GraphQL needs a token. The REST API does not
	case http.StatusForbidden, http.StatusTooManyRequests:
		if rl := c.limitFromAnswer(resp, data, true); rl != nil {
			return rl
		}
	}
	return &GitHubStatusError{Status: resp.StatusCode, Message: apiMessage(data)}
}
