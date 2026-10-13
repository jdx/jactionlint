//go:build !js

package jactionlint

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// 401 and 403 mean different things in different places: a manifest which is refused after a good anonymous token is an image which
// is private or absent, and a token service or a registry which refuses is a failure of the registry.
func TestRegistryDeniedStatuses(t *testing.T) {
	ctx := context.Background()
	status := func(code int) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }
	}
	tests := []struct {
		name     string
		token    int // status of the token service, 0 for a token
		manifest int
		notFound bool // ErrImageNotFound
		status   int  // the status of the registry error, 0 for none
		kind     failureKind
	}{
		{"manifest 401 after a token", 0, 401, true, 0, failNotFound},
		{"manifest 404", 0, 404, true, 0, failNotFound},
		{"manifest 403", 0, 403, false, 403, failForbidden},
		{"token service 401", 401, 0, false, 401, failUnauthorized},
		{"token service 403", 403, 0, false, 403, failForbidden},
		{"token service 429", 429, 0, false, 429, failRateLimit},
		{"manifest 429", 0, 429, false, 429, failRateLimit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hub := newFakeRegistry(t)
			if tc.token != 0 {
				hub.token = status(tc.token)
			}
			if tc.manifest != 0 {
				hub.manifest = status(tc.manifest)
			}
			_, err := testRegistryClient(hub, nil, "").ImageDigest(ctx, registryDockerHub, "library/alpine", "1")
			if err == nil {
				t.Fatal("an error is expected")
			}
			if got := errors.Is(err, ErrImageNotFound); got != tc.notFound {
				t.Errorf("not found = %v, want %v: %v", got, tc.notFound, err)
			}
			var se *registryStatusError
			if tc.status != 0 && (!errors.As(err, &se) || se.status != tc.status) {
				t.Errorf("the status %d is kept: %v", tc.status, err)
			}
			if kind := classifyRegistryFailure(err); kind != tc.kind {
				t.Errorf("kind %s, want %s (%v)", kind, tc.kind, err)
			}
		})
	}
}

func TestRegistryGHCRRetriesAForbiddenManifestWithTheToken(t *testing.T) {
	ghcr := newFakeRegistry(t)
	ghcr.manifest = func(w http.ResponseWriter, r *http.Request) {
		if len(ghcr.tokenReqs) < 2 { // The anonymous token cannot read the package
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Docker-Content-Digest", registryTestDigest)
	}
	d, err := testRegistryClient(nil, ghcr, "ghp_secret").ImageDigest(context.Background(), registryGHCR, "acme/private", "1")
	if err != nil || d != registryTestDigest {
		t.Errorf("got %q, %v", d, err)
	}
}

// A refused token service counts as a failure of the registry, which is backed off like the others, and says so.
func TestSessionRefusedTokenServiceIsAFailureOfTheRegistry(t *testing.T) {
	c := &scriptedImages{answers: map[string]error{registryDockerHub: &registryStatusError{registry: registryDockerHub, status: 401, token: true}}}
	s, _ := newImageSession(t, c)
	for i := 0; i < 5; i++ {
		_, _ = s.ImageDigest(imageOf(t, "img"+string(rune('a'+i))), "1")
	}
	if c.calls[registryDockerHub] != maxConsecutiveRegistryFailures {
		t.Errorf("%d lookups: the registry is backed off after %d failures", c.calls[registryDockerHub], maxConsecutiveRegistryFailures)
	}
	if by := s.skippedByKind(); by[string(failUnauthorized)] != 5 {
		t.Errorf("skipped by kind: %v", by)
	}
}
