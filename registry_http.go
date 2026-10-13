//go:build !js

package jactionlint

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// registryRequestTimeout bounds one request to a registry, and maxRegistryTokenBody the answer of a token
// service. A manifest is asked for with HEAD: its digest is in the headers, and no body is read.
const (
	registryRequestTimeout = 20 * time.Second
	maxRegistryTokenBody   = 64 << 10
	maxRegistryErrorBody   = 4 << 10
)

// manifestAccept lists the formats of manifests, so that the registry answers with the digest of what
// `docker pull` resolves: the index of a multi-platform image, else the manifest.
const manifestAccept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"

// registryTokenMargin is how long before its end a token is not used any more.
const registryTokenMargin = 10 * time.Second

type cachedRegistryToken struct {
	value   string
	expires time.Time
}

// registryEndpoints are the URLs of the two registries. Tests replace them.
type registryEndpoints struct {
	hubAuth, hubRegistry string
	ghcr                 string
}

var defaultRegistryEndpoints = registryEndpoints{
	hubAuth:     "https://auth.docker.io/token",
	hubRegistry: "https://registry-1.docker.io",
	ghcr:        "https://ghcr.io",
}

// registryClient asks Docker Hub and ghcr.io for the digest of an image (the registry v2 API). It sends no request
// to another host: the registry is one of the two. It asks anonymously, which works for every public image, and
// only when that is refused does it send the GitHub token to ghcr.io (the token of GITHUB_TOKEN reads the
// packages of the repository it belongs to). Docker Hub gets no credentials. The tokens which the registries
// give are kept in memory for the run.
type registryClient struct {
	hc *http.Client
	ep registryEndpoints
	// githubToken returns the token for ghcr.io, or "". It can be nil.
	githubToken func() string
	// offline makes every request fail: the mode --online=cache never uses the network, and the digest of a tag is
	// not cached on disk because a tag moves.
	offline bool

	mu     sync.Mutex
	tokens map[string]cachedRegistryToken // by registry, repository and whether credentials were sent
	now    func() time.Time
}

func newRegistryClient(ep registryEndpoints, githubToken func() string, offline bool) *registryClient {
	hc := &http.Client{Timeout: registryRequestTimeout}
	// A request with credentials is never redirected to another host
	hc.CheckRedirect = authRedirectPolicy(nil)
	return &registryClient{hc: hc, ep: ep, githubToken: githubToken, offline: offline, tokens: map[string]cachedRegistryToken{}, now: time.Now}
}

// ImageDigest implements ImageDigestClient.
func (c *registryClient) ImageDigest(ctx context.Context, registry, repository, tag string) (string, error) {
	if c.offline {
		return "", errImagesOffline
	}
	if !imageRepoRe.MatchString(repository) || !imageTagRe.MatchString(tag) {
		return "", fmt.Errorf("%q is not a name of an image", repository+":"+tag)
	}
	var base string
	switch registry {
	case registryDockerHub:
		base = c.ep.hubRegistry
	case registryGHCR:
		base = c.ep.ghcr
	default:
		return "", fmt.Errorf("the registry %q is not supported", registry)
	}

	d, err := c.manifestDigest(ctx, registry, base, repository, tag, false)
	// ghcr.io answers 401, 403 or 404 for a package which is private, or does not exist. The token of GitHub can read
	// the packages it has access to, so it is sent only now, to that host only.
	if registry == registryGHCR && c.githubToken != nil && isDeniedStatus(err) {
		if tok := c.githubToken(); tok != "" {
			d, err = c.manifestDigest(ctx, registry, base, repository, tag, true)
		}
	}
	if isDeniedStatus(err) && !errors.Is(err, ErrImageNotFound) {
		// Refused to everyone we could be: the image is private, or does not exist
		err = fmt.Errorf("%s/%s:%s is private or does not exist: %w", registry, repository, tag, ErrImageNotFound)
	}
	return d, err
}

func isDeniedStatus(err error) bool {
	var se *registryStatusError
	if !errors.As(err, &se) {
		return errors.Is(err, ErrImageNotFound)
	}
	return se.status == http.StatusUnauthorized || se.status == http.StatusForbidden
}

func (c *registryClient) manifestDigest(ctx context.Context, registry, base, repository, tag string, withCreds bool) (string, error) {
	token, err := c.token(ctx, registry, repository, withCreds)
	if err != nil {
		return "", err
	}
	target := strings.TrimRight(base, "/") + "/v2/" + repository + "/manifests/" + url.PathEscape(tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", manifestAccept)
	req.Header.Set("User-Agent", "jactionlint")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return "", redactURLError(err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxRegistryErrorBody))
	switch {
	case res.StatusCode == http.StatusOK:
		d := res.Header.Get("Docker-Content-Digest")
		if !isSHA256Digest(d) {
			return "", fmt.Errorf("the registry %s did not answer with a sha256 digest for %s:%s", registry, repository, tag)
		}
		return strings.ToLower(d), nil
	case res.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("%s/%s:%s: %w", registry, repository, tag, ErrImageNotFound)
	case res.StatusCode == http.StatusTooManyRequests:
		return "", fmt.Errorf("%w: %w", errRegistryRateLimited, &registryStatusError{registry: registry, status: res.StatusCode})
	}
	return "", &registryStatusError{registry: registry, status: res.StatusCode}
}

// token asks the registry for a token to pull the repository. Docker Hub and ghcr.io both give an anonymous token for
// a public repository. With credentials, ghcr.io is asked with the GitHub token as the password.
func (c *registryClient) token(ctx context.Context, registry, repository string, withCreds bool) (string, error) {
	key := fmt.Sprintf("%s|%s|%t", registry, repository, withCreds)
	c.mu.Lock()
	ct, ok := c.tokens[key]
	c.mu.Unlock()
	if ok && c.now().Before(ct.expires) {
		return ct.value, nil
	}
	var endpoint, service string
	switch registry {
	case registryDockerHub:
		endpoint, service = c.ep.hubAuth, "registry.docker.io"
	case registryGHCR:
		endpoint, service = strings.TrimRight(c.ep.ghcr, "/")+"/token", "ghcr.io"
	}
	q := url.Values{"service": {service}, "scope": {"repository:" + repository + ":pull"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "jactionlint")
	if withCreds {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("jactionlint:"+c.githubToken())))
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return "", redactURLError(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxRegistryTokenBody))
	if err != nil {
		return "", err
	}
	if res.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("%w: %w", errRegistryRateLimited, &registryStatusError{registry: registry, status: res.StatusCode})
	}
	if res.StatusCode != http.StatusOK {
		return "", &registryStatusError{registry: registry, status: res.StatusCode}
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("the token service of %s answered with something else than a token", registry)
	}
	t := tok.Token
	if t == "" {
		t = tok.AccessToken
	}
	// The tokens are short-lived (about 5 minutes on Docker Hub): keep one for the time it says, less a margin, and 60 seconds
	// (what the specification says) when it says nothing. A token about to expire is not kept.
	life := time.Duration(tok.ExpiresIn) * time.Second
	if tok.ExpiresIn <= 0 {
		life = time.Minute
	}
	if life -= registryTokenMargin; life > 0 {
		c.mu.Lock()
		c.tokens[key] = cachedRegistryToken{value: t, expires: c.now().Add(life)}
		c.mu.Unlock()
	}
	return t, nil
}

// redactURLError returns the error of a request without the URL in it, which has the name of the repository.
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}
