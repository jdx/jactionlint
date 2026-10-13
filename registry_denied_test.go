//go:build !js

package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// What a status means depends on the registry and on the request (see the table in registry_http.go): a token service which refuses is
// a failure of the registry, a manifest which the registry refuses after it gave a token is an image which is private or absent.
func TestRegistryStatusesOfBothRegistriesAtBothStages(t *testing.T) {
	ctx := context.Background()
	status := func(code int) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }
	}
	type want struct {
		notFound bool        // ErrImageNotFound: the host answered, the image is not accessible to us
		kind     failureKind // of the failure otherwise
	}
	token := map[int]want{
		401: {false, failUnauthorized}, 403: {false, failForbidden}, 404: {false, failNotFound},
		429: {false, failRateLimit}, 500: {false, failServer},
	}
	manifest := map[int]want{
		401: {true, failNotFound}, 403: {true, failNotFound}, 404: {true, failNotFound},
		429: {false, failRateLimit}, 500: {false, failServer},
	}
	for _, reg := range []string{registryDockerHub, registryGHCR} {
		for stage, table := range map[string]map[int]want{"token service": token, "manifest": manifest} {
			for code, w := range table {
				t.Run(fmt.Sprintf("%s %s %d", reg, stage, code), func(t *testing.T) {
					f := newFakeRegistry(t)
					if stage == "manifest" {
						f.manifest = status(code)
					} else {
						f.token = status(code)
					}
					var hub, ghcr *fakeRegistry
					if reg == registryDockerHub {
						hub = f
					} else {
						ghcr = f
					}
					_, err := testRegistryClient(hub, ghcr, "").ImageDigest(ctx, reg, "acme/img", "1")
					if err == nil {
						t.Fatal("an error is expected")
					}
					if got := errors.Is(err, ErrImageNotFound); got != w.notFound {
						t.Errorf("not found = %v, want %v: %v", got, w.notFound, err)
					}
					if w.notFound {
						if kind := classifyRegistryFailure(err); kind != failNotFound {
							t.Errorf("kind %s", kind)
						}
						return
					}
					var se *registryStatusError
					if !errors.As(err, &se) || se.status != code {
						t.Errorf("the status %d is kept: %v", code, err)
					}
					if kind := classifyRegistryFailure(err); kind != w.kind {
						t.Errorf("kind %s, want %s (%v)", kind, w.kind, err)
					}
				})
			}
		}
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
