//go:build !js

package jactionlint

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestRegistryTokensExpire(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		expiresIn string // the field of the answer
		after     time.Duration
		tokenReqs int // token requests for two lookups, the second one after the time
	}{
		{"within the time it says", `,"expires_in":300`, 200 * time.Second, 1},
		{"after the time it says", `,"expires_in":300`, 295 * time.Second, 2},
		{"nothing said: 60 seconds", ``, 30 * time.Second, 1},
		{"nothing said, later", ``, 55 * time.Second, 2},
		{"too short to keep", `,"expires_in":5`, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub := newFakeRegistry(t)
			hub.token = func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"token":"t"`+tc.expiresIn+`}`)
			}
			c := testRegistryClient(hub, nil, "")
			c.now = func() time.Time { return now }
			if _, err := c.ImageDigest(ctx, registryDockerHub, "library/alpine", "3.19"); err != nil {
				t.Fatal(err)
			}
			now = now.Add(tc.after)
			if _, err := c.ImageDigest(ctx, registryDockerHub, "library/alpine", "3.20"); err != nil {
				t.Fatal(err)
			}
			if tokens, _ := hub.requests(); len(tokens) != tc.tokenReqs {
				t.Errorf("%d token requests, want %d", len(tokens), tc.tokenReqs)
			}
		})
	}
}
