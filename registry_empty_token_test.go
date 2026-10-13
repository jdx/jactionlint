//go:build !js

package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// A token answer without a token is an error of the token service, not a public image which is "private": no manifest request
// goes out without credentials, and nothing is cached.
func TestRegistryTokenAnswerWithoutAToken(t *testing.T) {
	for _, body := range []string{`{}`, `{"token":""}`, `{"access_token":""}`, `{"expires_in":300}`} {
		hub := newFakeRegistry(t)
		hub.token = func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }
		c := testRegistryClient(hub, nil, "")
		_, err := c.ImageDigest(context.Background(), registryDockerHub, "library/alpine", "1")
		if err == nil || errors.Is(err, ErrImageNotFound) {
			t.Errorf("%s: got %v, want an error about the token", body, err)
		}
		if _, manifests := hub.requests(); len(manifests) != 0 {
			t.Errorf("%s: a manifest request without a token: %v", body, manifests)
		}
		if len(c.tokens) != 0 {
			t.Errorf("%s: an empty token was cached", body)
		}
	}
}
