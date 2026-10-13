//go:build !js

package jactionlint

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const registryTestDigest = "sha256:6baf43584bcb78f2e5847d1de515f23499913ac9f12bdf834811a3145eb11ca1"

// fakeRegistry serves the token service and the v2 API of one registry.
type fakeRegistry struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	tokenReqs []string // "scope|Authorization"
	manifests []string // "path|Authorization|Accept"
	// manifest decides the answer for a manifest request. The default is the digest.
	manifest func(w http.ResponseWriter, r *http.Request)
	// token decides the answer of the token service. The default is a token.
	token func(w http.ResponseWriter, r *http.Request)
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	f := &fakeRegistry{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			f.tokenReqs = append(f.tokenReqs, r.URL.Query().Get("scope")+"|"+r.Header.Get("Authorization"))
			if f.token != nil {
				f.token(w, r)
				return
			}
			fmt.Fprint(w, `{"token":"t-`+r.URL.Query().Get("scope")+`"}`)
		case strings.Contains(r.URL.Path, "/manifests/"):
			f.manifests = append(f.manifests, r.Method+" "+r.URL.Path+"|"+r.Header.Get("Authorization")+"|"+r.Header.Get("Accept"))
			if f.manifest != nil {
				f.manifest(w, r)
				return
			}
			w.Header().Set("Docker-Content-Digest", registryTestDigest)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeRegistry) requests() (tokens, manifests []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tokenReqs...), append([]string(nil), f.manifests...)
}

func testRegistryClient(hub, ghcr *fakeRegistry, token string) *registryClient {
	ep := registryEndpoints{}
	if hub != nil {
		ep.hubAuth, ep.hubRegistry = hub.srv.URL+"/token", hub.srv.URL
	}
	if ghcr != nil {
		ep.ghcr = ghcr.srv.URL
	}
	var tok func() string
	if token != "" {
		tok = func() string { return token }
	}
	return newRegistryClient(ep, tok, false)
}

func TestRegistryClientDockerHub(t *testing.T) {
	hub := newFakeRegistry(t)
	c := testRegistryClient(hub, nil, "ghp_secret")
	ctx := context.Background()
	d, err := c.ImageDigest(ctx, registryDockerHub, "library/alpine", "3.19")
	if err != nil || d != registryTestDigest {
		t.Fatalf("got %q, %v", d, err)
	}
	tokens, manifests := hub.requests()
	if len(tokens) != 1 || tokens[0] != "repository:library/alpine:pull|" {
		t.Errorf("an anonymous token for the repository: %v", tokens)
	}
	if len(manifests) != 1 || !strings.HasPrefix(manifests[0], "HEAD /v2/library/alpine/manifests/3.19|Bearer t-repository:library/alpine:pull|") {
		t.Errorf("manifest request: %v", manifests)
	}
	for _, mt := range []string{"application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json"} {
		if !strings.Contains(manifests[0], mt) {
			t.Errorf("the request must accept %s", mt)
		}
	}

	// The token of the repository is used again
	if _, err := c.ImageDigest(ctx, registryDockerHub, "library/alpine", "3.20"); err != nil {
		t.Fatal(err)
	}
	if tokens, _ := hub.requests(); len(tokens) != 1 {
		t.Errorf("one token for the repository: %v", tokens)
	}
}

func TestRegistryClientNeverSendsCredentialsToDockerHub(t *testing.T) {
	hub := newFakeRegistry(t)
	hub.manifest = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }
	c := testRegistryClient(hub, nil, "ghp_secret")
	_, err := c.ImageDigest(context.Background(), registryDockerHub, "acme/private", "1")
	if !errors.Is(err, ErrImageNotFound) {
		t.Errorf("a private image looks like a missing one: %v", err)
	}
	tokens, manifests := hub.requests()
	if len(tokens) != 1 || len(manifests) != 1 {
		t.Errorf("no second try with credentials on Docker Hub: %v %v", tokens, manifests)
	}
	for _, r := range append(tokens, manifests...) {
		if strings.Contains(r, "Basic") || strings.Contains(r, "ghp_secret") {
			t.Errorf("credentials sent to Docker Hub: %s", r)
		}
	}
}

func TestRegistryClientGHCR(t *testing.T) {
	ctx := context.Background()
	public := func(t *testing.T) {
		ghcr := newFakeRegistry(t)
		c := testRegistryClient(nil, ghcr, "ghp_secret")
		d, err := c.ImageDigest(ctx, registryGHCR, "astral-sh/uv", "0.5.0")
		if err != nil || d != registryTestDigest {
			t.Fatalf("got %q, %v", d, err)
		}
		tokens, _ := ghcr.requests()
		if len(tokens) != 1 || strings.Contains(tokens[0], "Basic") {
			t.Errorf("a public image is asked for anonymously, the token stays home: %v", tokens)
		}
	}
	t.Run("public", public)

	t.Run("private falls back to the token", func(t *testing.T) {
		ghcr := newFakeRegistry(t)
		ghcr.manifest = func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "Bearer t-repository:acme/private:pull" && len(ghcr.tokenReqs) == 1 {
				w.WriteHeader(http.StatusUnauthorized) // The anonymous token cannot read it
				return
			}
			w.Header().Set("Docker-Content-Digest", registryTestDigest)
		}
		c := testRegistryClient(nil, ghcr, "ghp_secret")
		d, err := c.ImageDigest(ctx, registryGHCR, "acme/private", "1")
		if err != nil || d != registryTestDigest {
			t.Fatalf("got %q, %v", d, err)
		}
		tokens, _ := ghcr.requests()
		if len(tokens) != 2 || !strings.HasSuffix(tokens[0], "|") {
			t.Fatalf("an anonymous request first: %v", tokens)
		}
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("jactionlint:ghp_secret"))
		if !strings.HasSuffix(tokens[1], "|"+want) {
			t.Errorf("the second request carries the token as the password: %v", tokens[1])
		}
	})

	t.Run("not visible with the token either", func(t *testing.T) {
		ghcr := newFakeRegistry(t)
		ghcr.manifest = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }
		c := testRegistryClient(nil, ghcr, "ghp_secret")
		_, err := c.ImageDigest(ctx, registryGHCR, "acme/nothing", "1")
		if !errors.Is(err, ErrImageNotFound) {
			t.Errorf("got %v", err)
		}
		if strings.Contains(fmt.Sprint(err), "ghp_secret") {
			t.Errorf("the token is in the message: %v", err)
		}
	})

	t.Run("without a token there is no second try", func(t *testing.T) {
		ghcr := newFakeRegistry(t)
		ghcr.manifest = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }
		c := testRegistryClient(nil, ghcr, "")
		if _, err := c.ImageDigest(ctx, registryGHCR, "acme/private", "1"); !errors.Is(err, ErrImageNotFound) {
			t.Errorf("got %v", err)
		}
		if tokens, _ := ghcr.requests(); len(tokens) != 1 {
			t.Errorf("%v", tokens)
		}
	})
}

func TestRegistryClientErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("not found", func(t *testing.T) {
		hub := newFakeRegistry(t)
		hub.manifest = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }
		if _, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/nothing", "1"); !errors.Is(err, ErrImageNotFound) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("rate limit", func(t *testing.T) {
		hub := newFakeRegistry(t)
		hub.manifest = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }
		if _, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1"); !errors.Is(err, errRegistryRateLimited) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("rate limit of the token service", func(t *testing.T) {
		hub := newFakeRegistry(t)
		hub.token = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }
		if _, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1"); !errors.Is(err, errRegistryRateLimited) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("a server error", func(t *testing.T) {
		hub := newFakeRegistry(t)
		hub.manifest = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }
		_, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1")
		var se *registryStatusError
		if !errors.As(err, &se) || se.status != http.StatusBadGateway || errors.Is(err, ErrImageNotFound) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("a digest which is not sha256", func(t *testing.T) {
		for _, d := range []string{"", "sha256:abc", "md5:" + strings.Repeat("a", 32), "sha256:" + strings.Repeat("g", 64), registryTestDigest + "\nX-Evil: 1"} {
			hub := newFakeRegistry(t)
			hub.manifest = func(w http.ResponseWriter, r *http.Request) {
				if d != "" && !strings.Contains(d, "\n") {
					w.Header().Set("Docker-Content-Digest", d)
				}
			}
			if _, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1"); err == nil {
				t.Errorf("%q must not be accepted", d)
			}
		}
	})
	t.Run("the digest is lower case", func(t *testing.T) {
		hub := newFakeRegistry(t)
		hub.manifest = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Docker-Content-Digest", strings.ToUpper(registryTestDigest[:7])+registryTestDigest[7:])
		}
		d, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1")
		if err != nil || d != registryTestDigest {
			t.Errorf("got %q, %v", d, err)
		}
	})
	t.Run("a token answer which is not a token", func(t *testing.T) {
		hub := newFakeRegistry(t)
		hub.token = func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html>") }
		if _, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1"); err == nil {
			t.Error("an error is expected")
		}
	})
	t.Run("a huge token answer is cut", func(t *testing.T) {
		hub := newFakeRegistry(t)
		hub.token = func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"token":"`+strings.Repeat("a", 1<<20)+`"}`)
		}
		if _, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1"); err == nil {
			t.Error("the answer is over the limit and cannot be a valid document")
		}
	})
	t.Run("a registry which does not answer", func(t *testing.T) {
		hub := newFakeRegistry(t)
		hub.manifest = func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) }
		c := testRegistryClient(hub, nil, "")
		c.hc.Timeout = 50 * time.Millisecond
		_, err := c.ImageDigest(ctx, registryDockerHub, "library/alpine", "1")
		if err == nil || classifyFailure(err) != failTimeout {
			t.Errorf("a timeout is expected: %v (%s)", err, classifyFailure(err))
		}
	})
	t.Run("a canceled run", func(t *testing.T) {
		hub := newFakeRegistry(t)
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := testRegistryClient(hub, nil, "").ImageDigest(cctx, registryDockerHub, "library/alpine", "1"); !errors.Is(err, context.Canceled) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("a redirect of an authenticated request to another host is refused", func(t *testing.T) {
		other := newFakeRegistry(t)
		hub := newFakeRegistry(t)
		hub.manifest = func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.srv.URL+"/v2/library/alpine/manifests/1", http.StatusTemporaryRedirect)
		}
		if _, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1"); err == nil {
			t.Error("the redirect carries the bearer token and must be refused")
		}
		if _, m := other.requests(); len(m) != 0 {
			t.Errorf("the other host was asked: %v", m)
		}
	})
}

func TestRegistryClientOfflineAndNames(t *testing.T) {
	hub := newFakeRegistry(t)
	c := testRegistryClient(hub, nil, "")
	c.offline = true
	if _, err := c.ImageDigest(context.Background(), registryDockerHub, "library/alpine", "1"); !errors.Is(err, errImagesOffline) {
		t.Errorf("the mode which never uses the network: %v", err)
	}
	c.offline = false
	for _, bad := range [][2]string{{"Library/Alpine", "1"}, {"library/alpine", ""}, {"library/alpine", "a b"}, {"library/../x", "1"}, {"library/alpine", "../1"}, {"library//alpine", "1"}} {
		if _, err := c.ImageDigest(context.Background(), registryDockerHub, bad[0], bad[1]); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if _, err := c.ImageDigest(context.Background(), "quay.io", "a/b", "1"); err == nil {
		t.Error("another registry must be refused")
	}
	if tokens, manifests := hub.requests(); len(tokens)+len(manifests) != 0 {
		t.Errorf("requests for refused names: %v %v", tokens, manifests)
	}
}

func TestParseRegistryImage(t *testing.T) {
	tests := []struct {
		image  string
		want   string // "registry/repository", or "" when the image is not asked
		reason string
	}{
		{"ubuntu", "docker.io/library/ubuntu", ""},
		{"alpine", "docker.io/library/alpine", ""},
		{"postgres", "docker.io/library/postgres", ""},
		{"bitnami/redis", "docker.io/bitnami/redis", ""},
		{"docker.io/library/alpine", "docker.io/library/alpine", ""},
		{"docker.io/alpine", "docker.io/library/alpine", ""},
		{"index.docker.io/bitnami/redis", "docker.io/bitnami/redis", ""},
		{"registry-1.docker.io/library/alpine", "docker.io/library/alpine", ""},
		{"ghcr.io/astral-sh/uv", "ghcr.io/astral-sh/uv", ""},
		{"GHCR.IO/astral-sh/uv", "ghcr.io/astral-sh/uv", ""},
		{"ghcr.io/org/team/tool", "ghcr.io/org/team/tool", ""},
		{"quay.io/org/tool", "", "quay.io is not supported"},
		{"mcr.microsoft.com/dotnet/sdk", "", "mcr.microsoft.com is not supported"},
		{"gcr.io/proj/img", "", "not supported"},
		{"localhost:5000/img", "", "not supported"},
		{"localhost/img", "", "not supported"},
		{"registry.example.com:8443/img", "", "not supported"},
		{"ghcr.io/Org/Tool", "", "not a name"},
		{"Ubuntu", "", "not a name"},
		{"ghcr.io/org//tool", "", "not a name"},
		{"ghcr.io/org/-tool", "", "not a name"},
	}
	for _, tc := range tests {
		t.Run(tc.image, func(t *testing.T) {
			img, reason := parseRegistryImage(tc.image)
			if tc.want == "" {
				if reason == "" || !strings.Contains(reason, tc.reason) {
					t.Errorf("got %+v, %q. want a reason with %q", img, reason, tc.reason)
				}
				return
			}
			if reason != "" || img.registry+"/"+img.repository != tc.want {
				t.Errorf("got %s/%s, %q. want %s", img.registry, img.repository, reason, tc.want)
			}
		})
	}
}

func TestRegistryImageSlug(t *testing.T) {
	for in, want := range map[string]string{"docker.io/library/ubuntu": "library/ubuntu", "ghcr.io/org/team/tool": "org/team", "ghcr.io/org/tool": "org/tool"} {
		img, reason := parseRegistryImage(in)
		if reason != "" || img.slug() != want {
			t.Errorf("%s: %q, %q", in, img.slug(), reason)
		}
	}
}
