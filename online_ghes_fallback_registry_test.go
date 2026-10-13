//go:build !js

package jactionlint

import (
	"context"
	"encoding/base64"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// ghcrRequestsOf asks ghcr.io (a fake which denies every manifest, so that the token is tried) for the digest of a private image with the
// image client of the GitHub client and returns the credentials which reached the registry.
func ghcrRequestsOf(t *testing.T, c GitHubClient) (credentials []string) {
	t.Helper()
	ghcr := newFakeRegistry(t)
	ghcr.manifest = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }
	old := defaultRegistryEndpoints
	defaultRegistryEndpoints.ghcr = ghcr.srv.URL
	t.Cleanup(func() { defaultRegistryEndpoints = old })
	img := defaultImageClient(c, false)
	if img == nil {
		t.Fatal("no image client")
	}
	_, _ = img.ImageDigest(context.Background(), registryGHCR, "acme/private", "1")
	tokens, manifests := ghcr.requests()
	for _, r := range append(tokens, manifests...) {
		_, auth, _ := strings.Cut(r, "|")
		auth, _, _ = strings.Cut(auth, "|")
		if after, ok := strings.CutPrefix(auth, "Basic "); ok {
			b, _ := base64.StdEncoding.DecodeString(after)
			auth = string(b)
		}
		if auth != "" && !strings.HasPrefix(auth, "Bearer t-") {
			credentials = append(credentials, auth)
		}
	}
	return credentials
}

func tokenClient(t *testing.T, baseURL, token string) *httpGitHubClient {
	t.Helper()
	c, err := newHTTPGitHubClient(httpGitHubOptions{BaseURL: baseURL, Token: token, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// ghcr.io is the registry of github.com. An Enterprise Server without the fallback gives it no token, as before, and with the fallback
// it gets the token of github.com, never the one of the Server.
func TestGHCRNeverGetsTheTokenOfTheServer(t *testing.T) {
	server := func() *httpGitHubClient { return tokenClient(t, "https://ghes.example.com/api/v3", "server-token") }

	t.Run("a Server without the fallback", func(t *testing.T) {
		if got := ghcrRequestsOf(t, server()); len(got) != 0 {
			t.Errorf("credentials reached ghcr.io: %v", got)
		}
	})
	t.Run("the fallback with a token for github.com", func(t *testing.T) {
		c := newFallbackGitHubClient(server(), tokenClient(t, "https://api.github.com", "dotcom-token"))
		got := ghcrRequestsOf(t, c)
		if len(got) == 0 {
			t.Fatal("the token of github.com was not used for the private image")
		}
		for _, cred := range got {
			if strings.Contains(cred, "server-token") || !strings.Contains(cred, "dotcom-token") {
				t.Errorf("wrong credentials at ghcr.io: %q", cred)
			}
		}
	})
	t.Run("the fallback without a token for github.com", func(t *testing.T) {
		c := newFallbackGitHubClient(server(), tokenClient(t, "https://api.github.com", ""))
		if got := ghcrRequestsOf(t, c); len(got) != 0 {
			t.Errorf("credentials reached ghcr.io: %v", got)
		}
	})
	t.Run("a client which cannot tell", func(t *testing.T) {
		c := newFallbackGitHubClient(server(), &stubClient{})
		if got := ghcrRequestsOf(t, c); len(got) != 0 {
			t.Errorf("credentials reached ghcr.io: %v", got)
		}
	})
}

// Every field of the answers is added up, so that a field which is added later cannot be left out of the report of the fallback.
func TestAnswersOfBothHostsAreAddedUpFieldByField(t *testing.T) {
	var a, b OnlineAnswers
	va, vb := reflect.ValueOf(&a).Elem(), reflect.ValueOf(&b).Elem()
	for i := 0; i < va.NumField(); i++ {
		switch va.Field(i).Kind() {
		case reflect.Int:
			va.Field(i).SetInt(1)
			vb.Field(i).SetInt(2)
		case reflect.String:
			va.Field(i).SetString("2026-02-01T00:00:00Z")
			vb.Field(i).SetString("2026-01-01T00:00:00Z")
		default:
			t.Fatalf("the field %s has a kind which this test does not know: add the sum of it to add and here", va.Type().Field(i).Name)
		}
	}
	a.add(b)
	for i := 0; i < va.NumField(); i++ {
		name := va.Type().Field(i).Name
		switch name {
		case "OldestFetched":
			if a.OldestFetched != "2026-01-01T00:00:00Z" {
				t.Errorf("OldestFetched is the oldest: %q", a.OldestFetched)
			}
		case "CacheTTLSeconds":
			if a.CacheTTLSeconds != 2 {
				t.Errorf("CacheTTLSeconds is the longest: %d", a.CacheTTLSeconds)
			}
		default:
			if va.Field(i).Int() != 3 {
				t.Errorf("%s is %d, want the sum 3", name, va.Field(i).Int())
			}
		}
	}
}
