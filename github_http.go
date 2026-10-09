//go:build !js

package jactionlint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultGitHubAPIURL = "https://api.github.com"
	// gitHubRequestTimeout bounds one request.
	gitHubRequestTimeout = 20 * time.Second
	// maxGitHubPages bounds how many pages of a list are read.
	maxGitHubPages = 10
	// maxGitHubBodyBytes bounds how much of an answer is read.
	maxGitHubBodyBytes = 16 << 20
	// maxTagDerefs bounds how many tag objects are followed to reach a commit.
	maxTagDerefs = 5
	// maxRateLimitWaits bounds how often one request waits for a primary rate limit to reset.
	maxRateLimitWaits = 2
	// backoffBase and backoffCap bound the exponential backoff between retries.
	backoffBase = 500 * time.Millisecond
	backoffCap  = 8 * time.Second
	// unknownLimitWait is how long a rate limit of unknown length is assumed to last: GitHub asks
	// to wait at least a minute when a secondary rate limit says nothing else.
	unknownLimitWait = time.Minute
	// maxAPIMessageBytes bounds the text of an API error message that is shown.
	maxAPIMessageBytes = 200
)

// httpGitHubClient is the GitHubClient which talks to the GitHub REST API. It
//
//   - authenticates with the token it is given (see tokenDiscovery) and sends it only to its own API
//     host, never over plain http except to the loopback address, and never prints it,
//   - keeps answers in a diskCache: an answer younger than the TTL is used as is, an older one is
//     revalidated with its ETag (a 304 answer does not count against the rate limit of an
//     authenticated client). Answers are keyed by API host and token, but a public answer fetched
//     without a token is also used by a client with one,
//   - reads X-RateLimit-* and Retry-After. A rate limit which resets within MaxWait is waited for;
//     one which resets later makes the requests fail at once (without a network call) with an
//     error naming the reset time. Answers in the cache, even expired ones, are still used,
//   - retries server errors (500, 502, 503, 504), secondary rate limits and a request which timed out once
//     with exponential backoff and jitter, a bounded number of times,
//   - in the offline mode never uses the network: only cached answers are returned.
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

	offline     bool
	retries     int
	maxWait     time.Duration
	tokenSource string
	red         redactor

	// Replaced by the tests to avoid real waiting.
	now    func() time.Time
	sleep  func(ctx context.Context, d time.Duration) error
	jitter func(d time.Duration) time.Duration

	mu          sync.Mutex
	token       string
	limit       *GitHubRateLimitError // set once the limit is reached
	noticedAnon bool
}

// httpGitHubOptions configures newHTTPGitHubClient.
type httpGitHubOptions struct {
	// Token is the access token. Empty means unauthenticated.
	Token string
	// TokenSource says where the token came from (a variable name or a file), for messages.
	TokenSource string
	// BaseURL is the URL of the API.
	BaseURL string
	// CacheDir is the directory of the cache. Empty disables caching.
	CacheDir string
	// TTL is how long cached answers are used without revalidation.
	TTL time.Duration
	// Offline makes the client answer from the cache only.
	Offline bool
	// Retries is how often a retryable failure is repeated. Nil means defaultOnlineRetries.
	Retries *int
	// MaxWait is the longest wait for a rate limit. Nil means defaultOnlineMaxWait.
	MaxWait *time.Duration
	// Concurrency is the number of requests in flight at most. Zero means defaultOnlineInFlight.
	Concurrency int
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
	base, err := parseAPIURL(o.BaseURL)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: gitHubRequestTimeout}
	if o.HTTPClient != nil {
		cp := *o.HTTPClient // Not modified: the caller may use it for something else
		hc = &cp
	}
	// A request which carries the token is never redirected to another origin (see authRedirectPolicy)
	hc.CheckRedirect = authRedirectPolicy(hc.CheckRedirect)
	conc := o.Concurrency
	if conc <= 0 {
		conc = defaultOnlineInFlight
	}
	c := &httpGitHubClient{
		base: base, hc: hc, ttl: o.TTL, token: o.Token, tokenSource: o.TokenSource, notify: o.Notify, debug: o.Debug,
		sem: make(chan struct{}, min(conc, maxOnlineInFlight)), offline: o.Offline,
		retries: defaultOnlineRetries, maxWait: defaultOnlineMaxWait,
		now: time.Now, sleep: sleepContext, jitter: jitterBackoff,
	}
	if o.Retries != nil {
		c.retries = max(*o.Retries, 0)
	}
	if o.MaxWait != nil {
		c.maxWait = max(*o.MaxWait, 0)
	}
	c.red.add(o.Token)
	if c.token != "" && !tokenIsSafeToSend(base) {
		c.token = ""
		c.noticef("not sending the token to %s because it is not https", base.Host)
	}
	if o.GraphQLURL != "" {
		if gu, err := url.Parse(o.GraphQLURL); err == nil && gu.Host == base.Host && gu.Scheme == base.Scheme && gu.User == nil {
			c.graphql = o.GraphQLURL
		} else {
			c.noticef("ignoring GITHUB_GRAPHQL_URL: it is not on the host of the REST API (%s)", base.Host)
		}
	}
	if o.CacheDir != "" {
		c.cache = newDiskCache(o.CacheDir, 0)
	}
	return c, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// jitterBackoff returns a random duration in [d/2, d].
func jitterBackoff(d time.Duration) time.Duration {
	if d <= 1 {
		return d
	}
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

// backoff is the wait before retry number attempt+1: 0.5s, 1s, 2s, ... up to 8s, with jitter.
func (c *httpGitHubClient) backoff(attempt int) time.Duration {
	d := backoffBase << min(attempt, 10)
	return c.jitter(min(d, backoffCap))
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

// newDefaultGitHubClient creates the client used by the jactionlint command: the API URL and the token
// come from the options and the environment and the cache is in the user cache directory.
func newDefaultGitHubClient(o defaultClientOptions) (GitHubClient, error) {
	apiURL := o.Options.APIURL
	trusted := o.OptionsTrusted
	if apiURL == "" {
		apiURL = onlineAPIDefaults(os.Getenv)
		trusted = true // The environment is the user's
	}
	base, err := parseAPIURL(orDefault(apiURL, defaultGitHubAPIURL))
	if err != nil {
		return nil, err
	}
	co := httpGitHubOptions{
		BaseURL:     base.String(),
		GraphQLURL:  os.Getenv("GITHUB_GRAPHQL_URL"),
		CacheDir:    defaultGitHubCacheDir(),
		TTL:         o.TTL,
		Offline:     o.Options.Mode.Offline(),
		Retries:     o.Options.Retries,
		MaxWait:     o.Options.MaxRateLimitWait,
		Concurrency: o.Options.Concurrency,
		Notify:      o.Notify,
		Debug:       o.Debug,
	}
	if !trusted {
		// A repository chose the host: the GraphQL variable is the user's but only valid for its own host
		co.GraphQLURL = ""
	}
	ctx := o.Context
	if ctx == nil {
		ctx = context.Background()
	}
	useGH := o.Options.GitHubCLI == nil || *o.Options.GitHubCLI
	d := tokenDiscovery{
		host: base.Host, dotCom: base.Host == "api.github.com", trusted: trusted,
		tokenEnv: o.Options.TokenEnv, tokenFile: o.Options.TokenFile, useGH: useGH,
		getenv: os.Getenv, readFile: readTokenFileLimited, runGH: runGHAuthToken,
	}
	d.host = ghHostname(base.Host)
	tok, notices := d.discover(ctx)
	for _, n := range notices {
		if o.Notify != nil {
			o.Notify(n)
		}
	}
	co.Token, co.TokenSource = tok.token, tok.source
	if o.Verbose != nil {
		if tok.token != "" {
			o.Verbose("online: using the token from %s for %s", tok.source, base.Host)
		} else {
			o.Verbose("online: no token for %s", base.Host)
		}
	}
	return newHTTPGitHubClient(co)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// httpResponse is one answer, read completely.
type httpResponse struct {
	status int
	header http.Header
	body   []byte
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
	if c.limit != nil && !c.limit.Reset.IsZero() && c.now().After(c.limit.Reset) {
		c.limit = nil // The window has passed
	}
	return c.limit
}

func (c *httpGitHubClient) setRateLimited(reset time.Time, authenticated bool) *GitHubRateLimitError {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limit = &GitHubRateLimitError{Reset: reset, Authenticated: authenticated, now: c.now}
	return c.limit
}

// answerFromEntry converts a cache entry to an answer.
func answerFromEntry(e *cacheEntry) *httpAnswer {
	return &httpAnswer{status: e.Status, body: e.Body, link: e.Link}
}

// scope tells apart the cached answers of different API hosts and credentials: private repositories
// answer differently to different tokens.
func (c *httpGitHubClient) scope(token string) string {
	if token == "" {
		return c.base.Host + "|anonymous"
	}
	return c.base.Host + "|token:" + cacheKey("token", token)
}

// lookup finds the cached answer of a request. A public answer fetched without a token (status 200)
// also serves a client with a token, since a token cannot make public data differ; a cached "not
// found" does not, because the token may see more.
func (c *httpGitHubClient) lookup(u, token string) (entry *cacheEntry, key string) {
	key = cacheKey(c.scope(token), u)
	if entry = c.cache.get(key, u); entry != nil || token == "" {
		return entry, key
	}
	if e := c.cache.get(cacheKey(c.scope(""), u), u); e != nil && e.Status == http.StatusOK {
		return e, key
	}
	return nil, key
}

// get fetches an API URL (a path relative to the API, or an absolute URL of the same host).
func (c *httpGitHubClient) get(ctx context.Context, target string) (*httpAnswer, error) {
	u := target
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		u = c.base.String() + target
	}
	pu, err := url.Parse(u)
	if err != nil || !sameOrigin(pu, c.base) {
		return nil, fmt.Errorf("refusing to request %q: not an URL of the GitHub API", c.red.redact(strings.SplitN(target, "?", 2)[0]))
	}
	if err := c.checkAPIPath(pu); err != nil {
		return nil, fmt.Errorf("refusing to request %q: %w", target, err)
	}

	token := c.currentToken()
	entry, key := c.lookup(u, token)
	if entry != nil && (c.offline || (c.ttl > 0 && c.now().Sub(entry.Fetched) < c.ttl)) {
		c.logf("GET %s: from the cache", target)
		return answerFromEntry(entry), nil
	}
	if c.offline {
		return nil, fmt.Errorf("%s: %w", strings.SplitN(target, "?", 2)[0], ErrGitHubNotCached)
	}
	if rl := c.rateLimited(); rl != nil {
		wait := rl.Reset.Sub(c.now())
		if rl.Reset.IsZero() || wait > c.maxWait {
			if entry != nil {
				return answerFromEntry(entry), nil // Stale data is better than none
			}
			return nil, rl
		}
		c.logf("rate limit reached: waiting %s", wait.Round(time.Millisecond))
		if err := c.sleep(ctx, wait+time.Second); err != nil {
			return nil, err
		}
	}

	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	ans, err := c.do(ctx, u, token, entry, key, false, false)
	if err != nil && entry != nil && errors.Is(err, ErrGitHubRateLimited) {
		return answerFromEntry(entry), nil
	}
	return ans, err
}

// send sends one request and reads the whole answer.
func (c *httpGitHubClient) send(ctx context.Context, u, token string, entry *cacheEntry) (*httpResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "jactionlint/"+getCommandVersion())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if c.currentToken() == "" {
		c.noticeAnonymous() // Not when only this request goes without the token
	}
	if entry != nil && entry.ETag != "" && entry.Status == http.StatusOK {
		req.Header.Set("If-None-Match", entry.ETag)
	}
	return c.roundTrip(req)
}

func (c *httpGitHubClient) roundTrip(req *http.Request) (*httpResponse, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, c.wrapTransport(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubBodyBytes+1))
	if err != nil {
		return nil, c.wrapTransport(fmt.Errorf("could not read the answer: %w", err))
	}
	if len(body) > maxGitHubBodyBytes {
		return nil, &redactedError{msg: fmt.Sprintf("the answer of the GitHub API for %s is too big", resp.Request.URL.Path)}
	}
	return &httpResponse{status: resp.StatusCode, header: resp.Header, body: body}, nil
}

// wrapTransport makes the error of a request printable: the text is redacted and says what was
// tried. The cause stays in the chain so that callers can tell a timeout from a failed DNS lookup.
func (c *httpGitHubClient) wrapTransport(err error) error {
	return &redactedError{msg: c.red.redact("could not reach the GitHub API: " + err.Error()), err: err}
}

// retryableTransport reports whether a failed request may work when repeated: a timeout or a connection
// which was cut. A name that does not resolve, a refused connection or a refused redirect will not
// change in a second.
func retryableTransport(err error) bool {
	var dns *net.DNSError
	switch {
	case errors.As(err, &dns):
		return dns.IsTimeout || dns.IsTemporary
	case errors.Is(err, context.Canceled), errors.Is(err, syscall.ECONNREFUSED):
		return false
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, syscall.ECONNRESET):
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// do sends a request and repeats it as the answer calls for. retryAnon is true on the single repeat
// without the token after the token was rejected.
//
// probe marks the anonymous request that asks again after a token was refused for a repository: its rate
// limit is not the one of the token, so it never changes what the client knows about the limit.
func (c *httpGitHubClient) do(ctx context.Context, u, token string, entry *cacheEntry, key string, retryAnon, probe bool) (*httpAnswer, error) {
	limitWaits := 0
	for attempt := 0; ; attempt++ {
		res, err := c.send(ctx, u, token, entry)
		if err != nil {
			// A timeout is repeated once: a second timeout means the server is not answering
			if attempt < min(c.retries, 1) && retryableTransport(err) && ctx.Err() == nil {
				d := c.backoff(attempt)
				c.logf("GET %s: %v. trying again in %s", u, err, d.Round(time.Millisecond))
				if serr := c.sleep(ctx, d); serr != nil {
					return nil, serr
				}
				continue
			}
			return nil, err
		}

		if !probe {
			c.noteRateLimit(res, token != "")
		}
		c.logf("GET %s: %d (rate limit remaining: %s)", u, res.status, res.header.Get("X-RateLimit-Remaining"))

		switch st := res.status; {
		case st == http.StatusNotModified && entry != nil:
			entry.Fetched = c.now()
			c.cache.put(key, entry)
			return answerFromEntry(entry), nil
		case st == http.StatusOK:
			e := &cacheEntry{URL: u, ETag: res.header.Get("ETag"), Fetched: c.now(), Status: st, Link: res.header.Get("Link"), Body: res.body}
			c.cache.put(key, e)
			return answerFromEntry(e), nil
		case st == http.StatusNotFound || st == http.StatusGone || st == http.StatusUnavailableForLegalReasons || st == http.StatusUnprocessableEntity:
			// Remember that it does not exist for the TTL so that the same lookup is not repeated
			c.cache.put(key, &cacheEntry{URL: u, Fetched: c.now(), Status: st})
			return &httpAnswer{status: st}, nil
		case st == http.StatusUnauthorized && token != "" && !retryAnon:
			c.dropToken()
			return c.do(ctx, u, "", nil, cacheKey(c.scope(""), u), true, false)
		case probe && (st == http.StatusForbidden || st == http.StatusTooManyRequests):
			// Whatever the reason, the caller keeps the first answer
		case st == http.StatusForbidden || st == http.StatusTooManyRequests:
			li := c.limitInfo(res)
			if li.kind == limitNone {
				// A token which may not read this repository (a fine-grained token or an app token
				// scoped to other repositories) is refused even for public data: ask without it
				// once. The token is kept for the other requests.
				if st == http.StatusForbidden && token != "" && !retryAnon {
					if ans, err := c.do(ctx, u, "", nil, cacheKey(c.scope(""), u), true, true); err == nil && ans.status == http.StatusOK {
						return ans, nil
					}
				}
				break
			}
			wait, ok := c.limitWait(li, attempt)
			if ok && li.kind == limitPrimary {
				ok = limitWaits < maxRateLimitWaits
				limitWaits++
			}
			if ok && li.kind == limitSecondary {
				ok = attempt < c.retries
			}
			if ok {
				c.logf("GET %s: rate limited. trying again in %s", u, wait.Round(time.Millisecond))
				if serr := c.sleep(ctx, wait); serr != nil {
					return nil, serr
				}
				if li.kind == limitPrimary {
					attempt-- // Waiting for a reset is not a retry
				}
				continue
			}
			reset := li.reset
			if reset.IsZero() {
				reset = c.now().Add(unknownLimitWait)
			}
			return nil, c.setRateLimited(reset, token != "")
		case st == http.StatusInternalServerError || st == http.StatusBadGateway || st == http.StatusServiceUnavailable || st == http.StatusGatewayTimeout:
			if attempt < c.retries {
				d := c.backoff(attempt)
				if ra, ok := parseRetryAfter(res.header.Get("Retry-After"), c.now()); ok && ra <= c.maxWait {
					d = ra
				}
				c.logf("GET %s: %d. trying again in %s", u, st, d.Round(time.Millisecond))
				if serr := c.sleep(ctx, d); serr != nil {
					return nil, serr
				}
				continue
			}
		}
		return nil, c.statusError(res)
	}
}

// statusError describes a failed answer. What the API said is redacted and cut short.
func (c *httpGitHubClient) statusError(res *httpResponse) error {
	msg := c.red.redact(apiMessage(res.body))
	if len(msg) > maxAPIMessageBytes {
		msg = msg[:maxAPIMessageBytes] + "..."
	}
	return &GitHubStatusError{Status: res.status, Message: msg}
}

func (c *httpGitHubClient) logf(format string, args ...any) {
	if c.debug != nil {
		c.debug("%s", c.red.redact(fmt.Sprintf(format, args...)))
	}
}

func (c *httpGitHubClient) noticef(format string, args ...any) {
	if c.notify != nil {
		c.notify(c.red.redact(fmt.Sprintf(format, args...)))
	}
}

func (c *httpGitHubClient) noticeAnonymous() {
	c.mu.Lock()
	first := !c.noticedAnon
	c.noticedAnon = true
	c.mu.Unlock()
	if first {
		c.noticef("no token found (GITHUB_TOKEN, GH_TOKEN, -online-token-file or gh auth login) so the online checks use unauthenticated requests, which GitHub limits to 60 per hour. set a token to raise the limit")
	}
}

func (c *httpGitHubClient) dropToken() {
	c.mu.Lock()
	had := c.token != ""
	c.token = ""
	c.noticedAnon = true // Not "no token is set": it was, and was refused
	c.mu.Unlock()
	if had {
		src := c.tokenSource
		if src == "" {
			src = "the environment"
		}
		c.noticef("the token from %s was rejected by the GitHub API (401). continuing without it", src)
	}
}

// noteRateLimit remembers that the limit is used up when the answer says so, so that no request is
// wasted afterwards.
func (c *httpGitHubClient) noteRateLimit(res *httpResponse, authenticated bool) {
	if res.header.Get("X-RateLimit-Remaining") != "0" {
		return
	}
	if reset := parseResetHeader(res.header.Get("X-RateLimit-Reset")); !reset.IsZero() && c.now().Before(reset) {
		c.setRateLimited(reset, authenticated)
	}
}

type limitKind int

const (
	limitNone limitKind = iota
	// limitPrimary is the hourly quota. The answer says when it resets.
	limitPrimary
	// limitSecondary is the throttle on bursts. Retry-After says how long to wait, if anything.
	limitSecondary
)

type limitDetails struct {
	kind limitKind
	// reset is when the limit ends. It is zero when unknown.
	reset time.Time
	// retryAfter is the wait the server asked for. It is zero when it said nothing.
	retryAfter time.Duration
}

// limitInfo tells whether a 403 or 429 answer is about a rate limit. Other 403 answers (a blocked
// repository, a missing scope) are ordinary failures.
func (c *httpGitHubClient) limitInfo(res *httpResponse) limitDetails {
	now := c.now()
	ra, hasRA := parseRetryAfter(res.header.Get("Retry-After"), now)
	msg := strings.ToLower(apiMessage(res.body))
	remaining0 := res.header.Get("X-RateLimit-Remaining") == "0"
	secondary := strings.Contains(msg, "secondary rate limit") || strings.Contains(msg, "abuse")
	switch {
	case !remaining0 && !hasRA && res.status != http.StatusTooManyRequests && !strings.Contains(msg, "rate limit") && !secondary:
		return limitDetails{}
	case hasRA || secondary || (res.status == http.StatusTooManyRequests && !remaining0):
		d := limitDetails{kind: limitSecondary, retryAfter: ra}
		if hasRA {
			d.reset = now.Add(ra)
		}
		return d
	}
	d := limitDetails{kind: limitPrimary, reset: parseResetHeader(res.header.Get("X-RateLimit-Reset"))}
	if !d.reset.IsZero() && now.After(d.reset) {
		d.reset = time.Time{}
	}
	return d
}

// limitWait returns how long to wait before trying again, and whether to do it at all: the wait must
// not exceed the maximum. A secondary limit that gave no time is retried with the exponential backoff.
func (c *httpGitHubClient) limitWait(li limitDetails, attempt int) (time.Duration, bool) {
	switch li.kind {
	case limitSecondary:
		if li.retryAfter > 0 {
			return li.retryAfter + c.jitter(time.Second), li.retryAfter <= c.maxWait
		}
		return c.backoff(attempt), true
	case limitPrimary:
		if li.reset.IsZero() {
			return 0, false
		}
		wait := li.reset.Sub(c.now())
		return max(wait, 0) + time.Second, wait <= c.maxWait
	}
	return 0, false
}

// parseRetryAfter parses the Retry-After header: seconds or an HTTP date.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
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

// errInvalidGitHubName is returned instead of a request for a name which is not a plain GitHub name.
var errInvalidGitHubName = errors.New("not a valid GitHub name")

// repoURL is the only place which builds the path of a repository request. The owner and the repository
// are validated here, every segment is escaped on its own and joined with "/", so what a workflow
// contains can never add, remove or change a segment of the path. The tail segments are passed
// as they are (not split) and need to have been validated: ref names with validGitRefName, which
// is why they go through refSegments.
func repoURL(owner, repo string, tail ...string) (string, error) {
	if !validGitHubOwner(owner) || !validGitHubRepo(repo) {
		return "", fmt.Errorf("%w: %q/%q", errInvalidGitHubName, owner, repo)
	}
	segs := append([]string{"repos", owner, repo}, tail...)
	for i, s := range segs {
		if s == "" || s == "." || s == ".." {
			return "", fmt.Errorf("%w: empty or dot path segment", errInvalidGitHubName)
		}
		segs[i] = url.PathEscape(s)
	}
	return "/" + strings.Join(segs, "/"), nil
}

// refSegments validates a ref name and splits it in the segments it is made of ("feature/x" is two).
func refSegments(name string) ([]string, error) {
	if !validGitRefName(name) {
		return nil, fmt.Errorf("%w: ref %q", errInvalidGitHubName, name)
	}
	return strings.Split(name, "/"), nil
}

// checkAPIPath refuses a URL whose path is not below the path of the API or has a dot segment, however
// it is escaped. It is the last line of defense for every request, whoever built the URL.
func (c *httpGitHubClient) checkAPIPath(u *url.URL) error {
	prefix := strings.TrimRight(c.base.EscapedPath(), "/") + "/"
	if !strings.HasPrefix(u.EscapedPath(), prefix) {
		return fmt.Errorf("path %q is not below %q", u.EscapedPath(), prefix)
	}
	for _, seg := range strings.Split(u.EscapedPath(), "/") {
		d, err := url.PathUnescape(seg)
		if err != nil || d == "." || d == ".." || strings.ContainsAny(d, "/\\") {
			return fmt.Errorf("path %q has a segment which changes the path", u.EscapedPath())
		}
	}
	return nil
}

// sameOrigin reports whether two URLs have the same scheme and host.
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

// authRedirectPolicy wraps the redirect policy of a client. A request which carries the token is never
// redirected to another origin (the token would go with it, or the request would silently run
// without it), and the header is dropped from the redirected request of any other kind as well.
func authRedirectPolicy(next func(req *http.Request, via []*http.Request) error) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !sameOrigin(req.URL, via[0].URL) {
			if via[0].Header.Get("Authorization") != "" {
				return fmt.Errorf("refusing to follow a redirect of an authenticated request to another host (%s)", req.URL.Host)
			}
			req.Header.Del("Authorization")
		}
		if next != nil {
			return next(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
}

// Repository implements GitHubClient.
func (c *httpGitHubClient) Repository(ctx context.Context, owner, repo string) (*GitHubRepo, error) {
	var r GitHubRepo
	path, err := repoURL(owner, repo)
	if err != nil {
		return nil, err
	}
	if _, err := c.getJSON(ctx, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Tags implements GitHubClient.
func (c *httpGitHubClient) Tags(ctx context.Context, owner, repo string) (*GitHubTagList, error) {
	l := &GitHubTagList{}
	path, err := repoURL(owner, repo, "tags")
	if err != nil {
		return nil, err
	}
	truncated, err := c.getPages(ctx, path+"?per_page=100", maxGitHubPages, func(body []byte) error {
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
	segs, err := refSegments(name)
	if err != nil {
		return "", false, err
	}
	path, err := repoURL(owner, repo, append([]string{"git", "ref", string(ns)}, segs...)...)
	if err != nil {
		return "", false, err
	}
	_, err = c.getJSON(ctx, path, &ref)
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
		tagPath, err := repoURL(owner, repo, "git", "tags", sha)
		if err != nil { // The SHA comes from the API; it is escaped and checked for dot segments like the rest
			return "", false, err
		}
		if _, err := c.getJSON(ctx, tagPath, &ref); err != nil {
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
	path, err := repoURL(owner, repo, "branches")
	if err != nil {
		return nil, err
	}
	truncated, err := c.getPages(ctx, path+"?per_page="+strconv.Itoa(perPage), min(pages, maxGitHubPages), func(body []byte) error {
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
	if !validGitRefName(base) || !validGitRefName(head) {
		return "", fmt.Errorf("%w: compare %q...%q", errInvalidGitHubName, base, head)
	}
	// "base...head" is one path segment, in which '/' of a branch name is escaped
	path, err := repoURL(owner, repo, "compare", base+"..."+head)
	if err != nil {
		return "", err
	}
	if _, err := c.getJSON(ctx, path+"?per_page=1", &r); err != nil {
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
	if token == "" && !c.offline {
		return GitHubBranchScan{}, ErrGitHubBranchScanUnavailable
	}
	if !validGitHubOwner(owner) || !validGitHubRepo(repo) || len(sha) != 40 || !validGitSHA(sha) {
		return GitHubBranchScan{}, ErrGitHubBranchScanUnavailable
	}
	limit = max(limit, 1)
	// The answer is cached like a GET under a name that cannot be a URL of the REST API
	name := fmt.Sprintf("graphql:branch-scan:%s/%s@%s:%d", strings.ToLower(owner), strings.ToLower(repo), strings.ToLower(sha), limit)
	key := cacheKey(c.scope(token), name)
	if e := c.cache.get(key, name); e != nil && (c.offline || (c.ttl > 0 && c.now().Sub(e.Fetched) < c.ttl)) {
		var scan GitHubBranchScan
		if json.Unmarshal(e.Body, &scan) == nil {
			c.logf("GraphQL %s: from the cache", name)
			return scan, nil
		}
	}
	if c.offline {
		return GitHubBranchScan{}, ErrGitHubBranchScanUnavailable // Compared one by one from the cache
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

// postGraphQL sends a GraphQL query. The token goes only to the GraphQL URL, which is on the host of
// the REST API.
func (c *httpGitHubClient) postGraphQL(ctx context.Context, token, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	gu := c.graphqlURL()
	limitWaits := 0
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, gu, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "jactionlint/"+getCommandVersion())
		res, err := c.roundTrip(req)
		if err != nil {
			// Like the REST requests: a timeout is repeated once
			if attempt < min(c.retries, 1) && retryableTransport(err) && ctx.Err() == nil {
				d := c.backoff(attempt)
				c.logf("POST %s: %v. trying again in %s", gu, err, d.Round(time.Millisecond))
				if serr := c.sleep(ctx, d); serr != nil {
					return serr
				}
				continue
			}
			return err
		}
		c.noteRateLimit(res, true)
		c.logf("POST %s: %d (rate limit remaining: %s)", gu, res.status, res.header.Get("X-RateLimit-Remaining"))
		switch res.status {
		case http.StatusOK:
			return json.Unmarshal(res.body, out)
		case http.StatusUnauthorized:
			c.dropToken()
			return ErrGitHubBranchScanUnavailable // GraphQL needs a token. The REST API does not
		case http.StatusForbidden, http.StatusTooManyRequests:
			li := c.limitInfo(res)
			if li.kind == limitNone {
				break
			}
			wait, ok := c.limitWait(li, attempt)
			if ok && li.kind == limitPrimary {
				ok = limitWaits < maxRateLimitWaits
				limitWaits++
			}
			if ok && li.kind == limitSecondary {
				ok = attempt < c.retries
			}
			if ok {
				c.logf("POST %s: rate limited. trying again in %s", gu, wait.Round(time.Millisecond))
				if serr := c.sleep(ctx, wait); serr != nil {
					return serr
				}
				if li.kind == limitPrimary {
					attempt-- // Waiting for a reset is not a retry
				}
				continue
			}
			reset := li.reset
			if reset.IsZero() {
				reset = c.now().Add(unknownLimitWait)
			}
			return c.setRateLimited(reset, true)
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			if attempt < c.retries {
				d := c.backoff(attempt)
				if ra, ok := parseRetryAfter(res.header.Get("Retry-After"), c.now()); ok && ra <= c.maxWait {
					d = ra
				}
				c.logf("POST %s: %d. trying again in %s", gu, res.status, d.Round(time.Millisecond))
				if serr := c.sleep(ctx, d); serr != nil {
					return serr
				}
				continue
			}
		}
		return c.statusError(res)
	}
}

// ghHostname is the host the gh command knows an API host by: api.github.com is github.com, and the API of
// GitHub Enterprise Cloud with data residency, api.SUBDOMAIN.ghe.com, is SUBDOMAIN.ghe.com. The host of a
// GitHub Enterprise Server is the same for both.
func ghHostname(apiHost string) string {
	if apiHost == "api.github.com" {
		return "github.com"
	}
	if h, ok := strings.CutPrefix(apiHost, "api."); ok && strings.HasSuffix(h, ".ghe.com") {
		return h
	}
	return apiHost
}
