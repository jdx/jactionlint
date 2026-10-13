package jactionlint

import "testing"

// The order of the specificity of the patterns of a policy is total and documented (docs/checks.md): the literal characters before
// the first wildcard of the normalized pattern, then fewer wildcards, then a tag, then the longer pattern as written, then
// the lexical order of the patterns.
func TestImagePolicySpecificityOrder(t *testing.T) {
	// each case: two patterns which both match the image, the policy of the first one is the one which has to win
	tests := []struct {
		name         string
		winner, rest string
		image        string
	}{
		{"a name beats the glob of its registry", "node", "docker.io/library/*", "node:20"},
		{"a name beats docker.io/*", "node", "docker.io/*", "node:20"},
		{"docker.io/library/* beats docker.io/*", "docker.io/library/*", "docker.io/*", "alpine:3"},
		{"a tag beats the same name", "node:20", "node", "node:20"},
		{"a tag of the same name with a wildcard", "ghcr.io/org/*:v1", "ghcr.io/org/*", "ghcr.io/org/app:v1"},
		{"a name beats the glob of its organization", "ghcr.io/org/app", "ghcr.io/org/*", "ghcr.io/org/app:1"},
		{"the organization beats the registry", "ghcr.io/org/*", "ghcr.io/*", "ghcr.io/org/app:1"},
		{"the registry beats everything", "ghcr.io/*", "*", "ghcr.io/org/app:1"},
		{"fewer wildcards", "ghcr.io/org/app*", "ghcr.io/org/*app*", "ghcr.io/org/app-1:1"},
		{"a longer prefix beats a tag", "ghcr.io/org/*", "*:1", "ghcr.io/org/app:1"},
		{"a tag with a prefix of the same length", "node:20", "node", "docker.io/library/node:20"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, order := range [][2]string{{tc.winner, tc.rest}, {tc.rest, tc.winner}} {
				policies := map[string]string{order[0]: "hash-pin", order[1]: "hash-pin"}
				policies[tc.winner] = "any"
				if got := imagePolicyFor(policies, tc.image, imagePolicyTagPin); got != imagePolicyAny {
					t.Errorf("%q should win over %q for %q, got %v", tc.winner, tc.rest, tc.image, got)
				}
			}
		})
	}
	t.Run("patterns that tie give the same answer every time", func(t *testing.T) {
		policies := map[string]string{"node": "hash-pin", "docker.io/library/node": "any"}
		first := imagePolicyFor(policies, "node:20", imagePolicyTagPin)
		for i := 0; i < 20; i++ {
			if got := imagePolicyFor(policies, "node:20", imagePolicyTagPin); got != first {
				t.Fatalf("answer changed: %v, then %v", first, got)
			}
		}
	})
	t.Run("a pattern which does not match does not count", func(t *testing.T) {
		policies := map[string]string{"ghcr.io/org/app": "any", "*": "hash-pin"}
		if got := imagePolicyFor(policies, "alpine:3", imagePolicyTagPin); got != imagePolicyHashPin {
			t.Errorf("got %v, want hash-pin", got)
		}
	})
}
