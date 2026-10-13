package jactionlint

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSessionRegistryFailureKinds(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind string
	}{
		{"rate limit", fmt.Errorf("%w: %w", errRegistryRateLimited, &registryStatusError{registry: registryDockerHub, status: 429}), string(failRateLimit)},
		{"server error", &registryStatusError{registry: registryDockerHub, status: 502}, string(failServer)},
		{"refused", &registryStatusError{registry: registryDockerHub, status: 403}, string(failForbidden)},
		{"another status", &registryStatusError{registry: registryDockerHub, status: 418}, string(failOther)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &scriptedImages{answers: map[string]error{registryDockerHub: tc.err}}
			s, _ := newImageSession(t, c)
			for i := 0; i < 5; i++ { // The first ones are asked, the others are skipped after the registry is marked down
				_, _ = s.ImageDigest(imageOf(t, fmt.Sprintf("img%d", i)), "1")
			}
			by := s.skippedByKind()
			if by[tc.kind] != 5 || len(by) != 1 {
				t.Errorf("skipped by kind %v, want 5 of %q", by, tc.kind)
			}
		})
	}
}

func TestSessionRegistryWarnings(t *testing.T) {
	c := &scriptedImages{answers: map[string]error{registryDockerHub: &registryStatusError{registry: registryDockerHub, status: 502}}}
	s, warnings := newImageSession(t, c)
	_, _ = s.ImageDigest(imageOf(t, "img0"), "1")
	if len(*warnings) != 1 || !strings.Contains((*warnings)[0], "still tried") || strings.Contains((*warnings)[0], "not asked again") {
		t.Fatalf("one failure does not stop the registry, and the warning must say what happened: %v", *warnings)
	}
	for i := 1; i < maxConsecutiveRegistryFailures; i++ {
		_, _ = s.ImageDigest(imageOf(t, fmt.Sprintf("img%d", i)), "1")
	}
	if len(*warnings) != 2 || !strings.Contains((*warnings)[1], "not asked again") {
		t.Errorf("the registry is marked down: one more warning says it: %v", *warnings)
	}
	_, _ = s.ImageDigest(imageOf(t, "img9"), "1")
	if len(*warnings) != 2 {
		t.Errorf("no warning for the skipped images: %v", *warnings)
	}
	if err := s.registryDown[registryDockerHub]; err == nil || errors.Is(err, errImagesUnsupported) {
		t.Errorf("down: %v", err)
	}
}
