package jactionlint

import "testing"

// The pattern is normalized like the image: "node" and "docker.io/library/node" are the same image on both sides.
func TestImagePatternNormalizesBothSides(t *testing.T) {
	tests := []struct {
		pattern, ref string
		want         bool
	}{
		{"node", "node:20", true},
		{"node", "docker.io/library/node:20", true},
		{"docker.io/library/node", "node:20", true},
		{"docker.io/library/node", "docker.io/library/node:20", true},
		{"node:20", "docker.io/library/node:20", true},
		{"node:20", "docker.io/library/node:22", false},
		{"docker.io/library/*", "alpine", true},
		{"docker.io/library/*", "docker.io/library/alpine:3", true},
		{"grafana/*", "docker.io/grafana/grafana:11", true},
		{"docker.io/grafana/*", "grafana/grafana", true},
		{"index.docker.io/library/node", "node", true},
		{"node", "ghcr.io/o/node", false},
		{"node", "someone/node", false},
		{"*", "ghcr.io/o/x:1", true},
		{"ghcr.io/o/*", "ghcr.io/o/x/y:1", true},
		{"ghcr.io/o/*", "o/x", false},
	}
	for _, tc := range tests {
		p, err := parseImagePattern(tc.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.matches(tc.ref); got != tc.want {
			t.Errorf("pattern %q, image %q: want %v, got %v", tc.pattern, tc.ref, tc.want, got)
		}
	}
	// and in a policy
	policies := map[string]string{"node": "any"}
	if got := imagePolicyFor(policies, "docker.io/library/node:20", imagePolicyHashPin); got != imagePolicyAny {
		t.Errorf("policy of node for docker.io/library/node:20 = %v", got)
	}
}

// A script is read with the shell of its own job: an image built in a job on Linux is known to a job on Windows that runs
// a bash step.
func TestUnpinnedImagesLocalImagesUseTheShellOfTheirJob(t *testing.T) {
	src := "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: docker build -t app:ci .\n  test:\n    runs-on: windows-latest\n    steps:\n      - run: docker run --rm app:ci\n        shell: bash\n"
	if got := imageFindings(t, imagesCfg(t, ""), src); len(got) != 0 {
		t.Errorf("want nothing, got %q", got)
	}
	// the same image from a registry is reported
	src = "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: docker build -t other:ci .\n  test:\n    runs-on: windows-latest\n    steps:\n      - run: docker run --rm alpine:3\n        shell: bash\n"
	if got := imageFindings(t, imagesCfg(t, ""), src); len(got) != 1 {
		t.Errorf("want one finding, got %q", got)
	}
}
