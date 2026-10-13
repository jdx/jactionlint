package jactionlint

import (
	"strings"
	"testing"
)

// images-allow and images-deny of forbidden-uses check the container images like the actions.
func TestForbiddenUsesImages(t *testing.T) {
	const wf = `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    container: node:20
    services:
      db:
        image: postgres:16
      cache:
        image: ghcr.io/my-org/cache:1
    steps:
      - uses: docker://alpine:3
      - uses: docker://registry.example.com/team/tool:2
      - run: docker run --rm busybox:1 true
`
	tests := []struct {
		name string
		cfg  string
		want []string // a text of each message, in order
	}{
		{"no list", "", nil},
		{"deny a name", "    images-deny: [\"postgres\"]\n", []string{`image of service "db" "postgres:16" is forbidden by pattern "postgres" in the "images-deny" list`}},
		{"deny a registry", "    images-deny: [\"ghcr.io/*\", \"docker.io/library/alpine\"]\n", []string{`"ghcr.io/my-org/cache:1" is forbidden by pattern "ghcr.io/*"`, `"alpine:3" is forbidden by pattern "docker.io/library/alpine"`}},
		{"deny a tag", "    images-deny: [\"node:20\"]\n", []string{`container image "node:20" is forbidden by pattern "node:20"`}},
		{"deny another tag of it", "    images-deny: [\"node:18\"]\n", nil},
		{"allow only a registry", "    images-allow: [\"ghcr.io/my-org/*\", \"registry.example.com/*\"]\n", []string{
			`container image "node:20" is not allowed`, `image of service "db" "postgres:16" is not allowed`, `image "alpine:3" is not allowed`, `image of "docker run" "busybox:1" is not allowed`,
		}},
		{"the deny list wins", "    images-allow: [\"*\"]\n    images-deny: [\"busybox\"]\n", []string{`image of "docker run" "busybox:1" is forbidden by pattern "busybox"`}},
		{"allow the library and the registries", "    images-allow: [\"docker.io/library/*\", \"ghcr.io/*\", \"registry.example.com/*\"]\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mustParseConfig(t, "rules:\n  forbidden-uses:\n    level: error\n"+tc.cfg)
			var got []string
			for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", wf) {
				if e.ID == "forbidden-uses" {
					got = append(got, e.Message)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("want %d findings, got %q", len(tc.want), got)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("finding %d: want %q in %q", i, w, got[i])
				}
			}
		})
	}

	t.Run("expressions are not judged", func(t *testing.T) {
		cfg := mustParseConfig(t, "rules:\n  forbidden-uses:\n    level: error\n    images-allow: [\"ghcr.io/*\"]\n")
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    container: ${{ inputs.image }}\n    steps:\n      - run: docker run \"$IMAGE\"\n"
		for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", src) {
			if e.ID == "forbidden-uses" {
				t.Errorf("unexpected %q", e.Message)
			}
		}
	})
	t.Run("invalid pattern", func(t *testing.T) {
		if _, err := ParseConfig([]byte("rules:\n  forbidden-uses:\n    images-deny: [\"a b\"]\n")); err == nil {
			t.Error("want an error")
		}
	})
}
