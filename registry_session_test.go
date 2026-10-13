package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// plainGitHubClient hides the optional interfaces of the client it wraps.
type plainGitHubClient struct{ GitHubClient }

// scriptedImages answers the digest of an image from a table, or with the error of the table.
type scriptedImages struct {
	calls   map[string]int
	answers map[string]error // by registry; nil means the digest
}

func (c *scriptedImages) ImageDigest(_ context.Context, registry, repository, tag string) (string, error) {
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[registry]++
	if err := c.answers[registry]; err != nil {
		return "", err
	}
	return "sha256:" + strings.Repeat("a", 64), nil
}

func imageOf(t *testing.T, name string) registryImage {
	t.Helper()
	img, reason := parseRegistryImage(name)
	if reason != "" {
		t.Fatal(reason)
	}
	return img
}

func newImageSession(t *testing.T, images *scriptedImages) (*onlineSession, *[]string) {
	t.Helper()
	var warnings []string
	s := newOnlineSession(context.Background(), onlineFixtureClient(t), func(m string) { warnings = append(warnings, m) })
	s.images = images
	return s, &warnings
}

func TestSessionImageDigestIsAskedOnce(t *testing.T) {
	c := &scriptedImages{}
	s, _ := newImageSession(t, c)
	for i := 0; i < 3; i++ {
		if _, err := s.ImageDigest(imageOf(t, "alpine"), "3.19"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ImageDigest(imageOf(t, "alpine"), "3.20"); err != nil {
		t.Fatal(err)
	}
	if c.calls[registryDockerHub] != 2 {
		t.Errorf("%d lookups for two tags", c.calls[registryDockerHub])
	}
}

func TestSessionImageDigestAllowAndDeny(t *testing.T) {
	c := &scriptedImages{}
	s, _ := newImageSession(t, c)
	s.deny = []string{"library/*"}
	if _, err := s.ImageDigest(imageOf(t, "alpine"), "3.19"); !errors.Is(err, errOnlineExcluded) {
		t.Errorf("a denied namespace: %v", err)
	}
	if _, err := s.ImageDigest(imageOf(t, "ghcr.io/acme/tool"), "1"); err != nil {
		t.Errorf("another namespace: %v", err)
	}
	s.deny = nil
	s.allow = []string{"acme/*"}
	if _, err := s.ImageDigest(imageOf(t, "postgres"), "16"); !errors.Is(err, errOnlineExcluded) {
		t.Errorf("outside of the allow list: %v", err)
	}
	if _, err := s.ImageDigest(imageOf(t, "ghcr.io/acme/other"), "1"); err != nil {
		t.Errorf("inside of the allow list: %v", err)
	}
	if c.calls[registryDockerHub] != 0 {
		t.Error("Docker Hub was asked for an image which is left out")
	}
}

func TestSessionRegistryFailuresStayInTheRegistry(t *testing.T) {
	c := &scriptedImages{answers: map[string]error{registryDockerHub: errors.New("boom")}}
	s, warnings := newImageSession(t, c)
	for i := 0; i < 6; i++ {
		if _, err := s.ImageDigest(imageOf(t, fmt.Sprintf("img%d", i)), "1"); err == nil {
			t.Fatal("an error is expected")
		}
	}
	if c.calls[registryDockerHub] != maxConsecutiveRegistryFailures {
		t.Errorf("%d lookups, want %d: the registry is not asked again after that", c.calls[registryDockerHub], maxConsecutiveRegistryFailures)
	}
	if s.skippedLookups() != 6 {
		t.Errorf("%d skipped lookups", s.skippedLookups())
	}
	if len(*warnings) != 1 {
		t.Errorf("one warning for the kind of failure: %v", *warnings)
	}
	// The other registry and GitHub itself are not affected
	if _, err := s.ImageDigest(imageOf(t, "ghcr.io/acme/tool"), "1"); err != nil {
		t.Errorf("ghcr.io: %v", err)
	}
	if _, err := s.Tags("actions", "checkout"); err != nil {
		t.Errorf("GitHub lookups must keep working: %v", err)
	}
	if err := s.blockedErr(); err != nil {
		t.Errorf("the session of GitHub is blocked: %v", err)
	}
}

func TestSessionRegistryRateLimit(t *testing.T) {
	c := &scriptedImages{answers: map[string]error{registryDockerHub: fmt.Errorf("%w: status 429", errRegistryRateLimited)}}
	s, _ := newImageSession(t, c)
	for i := 0; i < 3; i++ {
		_, _ = s.ImageDigest(imageOf(t, fmt.Sprintf("img%d", i)), "1")
	}
	if c.calls[registryDockerHub] != 1 {
		t.Errorf("a rate limit stops the lookups of the registry at once: %d", c.calls[registryDockerHub])
	}
}

func TestSessionImageNotFoundIsNotAnOutage(t *testing.T) {
	c := &scriptedImages{answers: map[string]error{registryDockerHub: fmt.Errorf("x: %w", ErrImageNotFound)}}
	s, _ := newImageSession(t, c)
	for i := 0; i < 5; i++ {
		_, _ = s.ImageDigest(imageOf(t, fmt.Sprintf("img%d", i)), "1")
	}
	if c.calls[registryDockerHub] != 5 {
		t.Errorf("a missing image does not stop the registry: %d lookups", c.calls[registryDockerHub])
	}
}

func TestSessionWithoutAnImageClient(t *testing.T) {
	s := newOnlineSession(context.Background(), plainGitHubClient{onlineFixtureClient(t)}, nil)
	if _, err := s.ImageDigest(imageOf(t, "alpine"), "3.19"); !errors.Is(err, errImagesUnsupported) {
		t.Errorf("got %v", err)
	}
	// The fixtures answer images too
	s = newOnlineSession(context.Background(), onlineFixtureClient(t), nil)
	if d, err := s.ImageDigest(imageOf(t, "alpine"), "3.19"); err != nil || d != digestAlpine {
		t.Errorf("got %q, %v", d, err)
	}
}

func TestRecordedImagesAreServed(t *testing.T) {
	fx := `{"images":{"docker.io/library/alpine:3.19":"` + digestAlpine + `","docker.io/example/gone:1":"not-found"}}`
	c, err := NewFixtureGitHubClient([]byte(fx))
	if err != nil {
		t.Fatal(err)
	}
	rec := NewRecordingGitHubClient(c)
	ctx := context.Background()
	if d, err := rec.ImageDigest(ctx, registryDockerHub, "library/alpine", "3.19"); err != nil || d != digestAlpine {
		t.Fatalf("got %q, %v", d, err)
	}
	if _, err := rec.ImageDigest(ctx, registryDockerHub, "example/gone", "1"); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := rec.ImageDigest(ctx, registryDockerHub, "library/alpine", "9"); err == nil {
		t.Error("an image which was not recorded fails the test")
	}
	b, err := rec.Fixtures()
	if err != nil {
		t.Fatal(err)
	}
	again, err := NewFixtureGitHubClient(b)
	if err != nil {
		t.Fatal(err)
	}
	if d, err := again.ImageDigest(ctx, registryDockerHub, "library/alpine", "3.19"); err != nil || d != digestAlpine {
		t.Errorf("got %q, %v", d, err)
	}
}
