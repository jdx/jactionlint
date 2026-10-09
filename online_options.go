package jactionlint

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

// OnlineMode selects how the online checks use the network.
type OnlineMode string

const (
	// OnlineModeDefault asks GitHub for what the checks need. A lookup which fails is skipped with one
	// warning per kind of failure, and the exit status does not change.
	OnlineModeDefault OnlineMode = ""
	// OnlineModeCache never uses the network. Only the answers already in the disk cache are used,
	// whatever their age. A lookup with no cached answer is skipped.
	OnlineModeCache OnlineMode = "cache"
	// OnlineModeStrict is like the default mode but a skipped lookup makes the exit status 3: use it
	// where a check that could not run must not pass silently.
	OnlineModeStrict OnlineMode = "strict"
)

// OnlineModeCacheStrict is OnlineModeCache that also fails when a lookup has no cached answer.
const OnlineModeCacheStrict OnlineMode = "cache,strict"

// Offline reports whether the mode never uses the network.
func (m OnlineMode) Offline() bool { return m == OnlineModeCache || m == OnlineModeCacheStrict }

// Strict reports whether a skipped lookup fails the run.
func (m OnlineMode) Strict() bool { return m == OnlineModeStrict || m == OnlineModeCacheStrict }

// ParseOnlineMode parses the value of -online and of "online-options.mode": "true" (or "on", or
// empty), "cache", "strict", or "cache,strict" for both. Order does not matter.
func ParseOnlineMode(s string) (OnlineMode, error) {
	var cache, strict bool
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ',' || r == '+' || r == ' ' }) {
		switch w {
		case "true", "on", "1":
		case "cache":
			cache = true
		case "strict":
			strict = true
		default:
			return "", fmt.Errorf("invalid online mode %q. available values are \"true\", \"cache\", \"strict\" and \"cache,strict\"", s)
		}
	}
	switch {
	case cache && strict:
		return OnlineModeCacheStrict, nil
	case cache:
		return OnlineModeCache, nil
	case strict:
		return OnlineModeStrict, nil
	}
	return OnlineModeDefault, nil
}

// OnlineOptions tunes how the online checks reach GitHub. The zero value of every field means "use the
// default". The same options are read from the "online-options" key of the configuration file; the
// values of LinterOptions.OnlineOptions win over it.
type OnlineOptions struct {
	// Mode selects how the network is used. See OnlineMode. Setting it to "cache" or "strict" in the
	// configuration also turns the online checks on.
	Mode OnlineMode `yaml:"mode"`
	// APIURL is the URL of the REST API of a GitHub Enterprise Server such as
	// "https://ghe.example.com/api/v3". The default is $GITHUB_API_URL, else derived from
	// $GITHUB_SERVER_URL or $GH_HOST, else https://api.github.com.
	//
	// A token is sent only to hosts which the user chose: github.com, a host named by the
	// environment, or a host set by -online-api-url, the user-global config file or -config-file. An
	// APIURL in the config file of a repository gets no token, because a pull request could
	// otherwise send your token to any server.
	APIURL string `yaml:"api-url"`
	// TokenEnv names the environment variable which holds the token. It is read before $GITHUB_TOKEN
	// and $GH_TOKEN.
	TokenEnv string `yaml:"token-env"`
	// TokenFile is a file which holds the token (surrounding white space is ignored). It is read
	// before $GITHUB_TOKEN and $GH_TOKEN, after TokenEnv.
	TokenFile string `yaml:"token-file"`
	// Allow limits the lookups to the repositories matching one of the patterns "owner/repo". "*"
	// matches within a part ("owner/*", "*/setup-*"). Case does not matter. Empty allows all.
	Allow []string `yaml:"allow"`
	// Deny skips the lookups for the repositories matching one of the patterns, for private or
	// internal actions GitHub cannot answer for. It wins over Allow.
	Deny []string `yaml:"deny"`
	// CacheTTL is how long a cached answer is used without asking GitHub whether it changed. Zero
	// revalidates every answer (which costs no rate limit when nothing changed). Nil means one hour.
	CacheTTL *time.Duration `yaml:"cache-ttl"`
	// MaxRateLimitWait is the longest the checks wait for a rate limit to reset. A limit which resets
	// later skips the lookups, with a message naming the time. Zero never waits. Nil means 30 seconds.
	MaxRateLimitWait *time.Duration `yaml:"max-rate-limit-wait"`
	// Retries is how often a request is repeated after a server error or a secondary rate limit,
	// with exponential backoff and jitter. Nil means 2.
	Retries *int `yaml:"retries"`
	// Concurrency is how many requests are in flight at most. Zero means 6.
	Concurrency int `yaml:"concurrency"`
	// GitHubCLI is whether "gh auth token" is asked for a token when no variable or file has one.
	// Nil means true.
	GitHubCLI *bool `yaml:"gh-cli"`
}

const (
	defaultOnlineCacheTTL = time.Hour
	defaultOnlineMaxWait  = 30 * time.Second
	defaultOnlineRetries  = 2
	defaultOnlineInFlight = 6
	maxOnlineInFlight     = 32
	maxOnlineRetries      = 10
)

// validate checks the values which can be checked without the environment.
func (o *OnlineOptions) validate() error {
	if m, err := ParseOnlineMode(string(o.Mode)); err != nil {
		return err
	} else if m != o.Mode {
		return fmt.Errorf("invalid online mode %q. available values are \"cache\", \"strict\" and \"cache,strict\"", o.Mode)
	}
	if o.APIURL != "" {
		if _, err := parseAPIURL(o.APIURL); err != nil {
			return err
		}
	}
	for _, l := range []struct {
		key  string
		pats []string
	}{{"allow", o.Allow}, {"deny", o.Deny}} {
		for _, p := range l.pats {
			if err := validateRepoPattern(p); err != nil {
				return fmt.Errorf("invalid pattern %q in \"online-options.%s\": %w", p, l.key, err)
			}
		}
	}
	if o.CacheTTL != nil && *o.CacheTTL < 0 {
		return fmt.Errorf("\"online-options.cache-ttl\" must not be negative")
	}
	if o.MaxRateLimitWait != nil && *o.MaxRateLimitWait < 0 {
		return fmt.Errorf("\"online-options.max-rate-limit-wait\" must not be negative")
	}
	if o.Retries != nil && (*o.Retries < 0 || *o.Retries > maxOnlineRetries) {
		return fmt.Errorf("\"online-options.retries\" must be between 0 and %d", maxOnlineRetries)
	}
	if o.Concurrency < 0 || o.Concurrency > maxOnlineInFlight {
		return fmt.Errorf("\"online-options.concurrency\" must be between 1 and %d", maxOnlineInFlight)
	}
	return nil
}

// parseAPIURL parses and checks the URL of a GitHub API. The credentials in an URL are refused, and
// the error does not repeat the URL, which could hold them.
func parseAPIURL(s string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(s), "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("invalid GitHub API URL: it must be like https://api.github.com")
	}
	if u.User != nil {
		return nil, fmt.Errorf("invalid GitHub API URL: it must not contain credentials")
	}
	u.RawQuery, u.Fragment = "", ""
	return u, nil
}

// validateRepoPattern checks an allow or deny pattern "owner/repo" with "*" wildcards.
func validateRepoPattern(p string) error {
	if strings.Count(p, "/") != 1 || strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return fmt.Errorf("it must be like \"owner/repo\", \"owner/*\" or \"*/repo\"")
	}
	if strings.ContainsAny(p, "?[]\\") {
		return fmt.Errorf("only \"*\" is a wildcard")
	}
	if _, err := path.Match(p, ""); err != nil {
		return err
	}
	return nil
}

// matchRepoPattern reports whether the "owner/repo" slug matches the pattern. Case is ignored.
func matchRepoPattern(pat, slug string) bool {
	ok, err := path.Match(strings.ToLower(pat), strings.ToLower(slug))
	return err == nil && ok
}

// overlay returns o with the fields set in over replacing its own.
func (o OnlineOptions) overlay(over OnlineOptions) OnlineOptions {
	if over.Mode != OnlineModeDefault {
		o.Mode = over.Mode
	}
	if over.APIURL != "" {
		o.APIURL = over.APIURL
	}
	if over.TokenEnv != "" {
		o.TokenEnv = over.TokenEnv
	}
	if over.TokenFile != "" {
		o.TokenFile = over.TokenFile
	}
	if over.Allow != nil {
		o.Allow = over.Allow
	}
	if over.Deny != nil {
		o.Deny = over.Deny
	}
	if over.CacheTTL != nil {
		o.CacheTTL = over.CacheTTL
	}
	if over.MaxRateLimitWait != nil {
		o.MaxRateLimitWait = over.MaxRateLimitWait
	}
	if over.Retries != nil {
		o.Retries = over.Retries
	}
	if over.Concurrency != 0 {
		o.Concurrency = over.Concurrency
	}
	if over.GitHubCLI != nil {
		o.GitHubCLI = over.GitHubCLI
	}
	return o
}

// onlineAPIDefaults derives the URL of the REST API from the environment: $GITHUB_API_URL (set by GitHub
// Actions on github.com and on Enterprise Server), then $GITHUB_SERVER_URL, then $GH_HOST (the host gh
// uses). The empty string means api.github.com.
func onlineAPIDefaults(getenv func(string) string) string {
	if v := strings.TrimSpace(getenv("GITHUB_API_URL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(getenv("GITHUB_SERVER_URL")); v != "" {
		if u, err := url.Parse(v); err == nil && u.Host != "" {
			return apiURLForHost(u.Host, u.Scheme)
		}
	}
	if v := strings.TrimSpace(getenv("GH_HOST")); v != "" {
		return apiURLForHost(v, "https")
	}
	return ""
}

// apiURLForHost returns the REST API URL of a GitHub host: api.github.com for github.com, the
// api.<name>.ghe.com of GitHub Enterprise Cloud with data residency, and <host>/api/v3 otherwise.
func apiURLForHost(host, scheme string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if scheme != "http" {
		scheme = "https"
	}
	switch {
	case host == "" || host == "github.com" || host == "www.github.com":
		return ""
	case strings.HasSuffix(host, ".ghe.com") && !strings.HasPrefix(host, "api."):
		return scheme + "://api." + host
	}
	return scheme + "://" + host + "/api/v3"
}

// defaultClientOptions is what newDefaultGitHubClient needs to know.
type defaultClientOptions struct {
	// Options are the resolved options of the run (the flags over the configuration file).
	Options OnlineOptions
	// OptionsTrusted is false when the API URL came from the config file of a repository.
	OptionsTrusted bool
	// TTL is the cache time-to-live. Zero revalidates every answer.
	TTL    time.Duration
	Notify func(string)
	Debug  func(string, ...any)
	// Verbose receives which token source is used.
	Verbose func(string, ...any)
	// Context bounds "gh auth token".
	Context context.Context
}
