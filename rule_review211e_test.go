package jactionlint

import "testing"

// A policy of a short Docker Hub name is as specific as the same name with the registry written out (Bugbot on #211).
func TestImagePolicySpecificityOfShortNames(t *testing.T) {
	policies := map[string]string{"node": "any", "docker.io/library/*": "hash-pin"}
	if got := imagePolicyFor(policies, "node:20", imagePolicyTagPin); got != imagePolicyAny {
		t.Errorf("node:20 = %v, want any", got)
	}
	if got := imagePolicyFor(policies, "docker.io/library/node:20", imagePolicyTagPin); got != imagePolicyAny {
		t.Errorf("docker.io/library/node:20 = %v, want any", got)
	}
	if got := imagePolicyFor(policies, "alpine:3", imagePolicyTagPin); got != imagePolicyHashPin {
		t.Errorf("alpine:3 = %v, want hash-pin", got)
	}
	// the order of writing does not matter and a longer glob of another registry does not beat a name
	policies = map[string]string{"grafana/grafana": "tag-pin", "*": "hash-pin", "ghcr.io/*": "any"}
	if got := imagePolicyFor(policies, "grafana/grafana:11", imagePolicyHashPin); got != imagePolicyTagPin {
		t.Errorf("grafana/grafana:11 = %v, want tag-pin", got)
	}
}

// The transports of podman other than docker:// name a file, a directory or a store, not an image of a registry; the options of run
// are read with their aliases.
func TestUnpinnedImagesTransportsAndOptionAliases(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   int
	}{
		{"docker-archive with a path", "podman run docker-archive:/tmp/image.tar", 0},
		{"docker-archive with a relative path", "podman run docker-archive:./image.tar", 0},
		{"oci with a directory", "podman pull oci:./dir:tag", 0},
		{"oci-archive", "podman run oci-archive:/tmp/x.tar", 0},
		{"dir", "podman run dir:/x", 0},
		{"docker-daemon", "podman run docker-daemon:alpine:3", 0},
		{"containers-storage", "podman run containers-storage:alpine", 0},
		{"sif", "podman pull sif:/x.sif", 0},
		{"tarball", "podman pull tarball:/x.tar", 0},
		{"the docker transport is a registry", "podman run docker://alpine:3", 1},
		{"net is network", "docker run --net host alpine:3", 1},
		{"net with an equal sign", "docker run --net=host alpine:3", 1},
		{"net-alias", "docker run --network-alias a --net-alias b alpine:3", 1},
		{"short options of the reference", "docker run -c 512 -m 1g -h box -l a=b -a stdout alpine:3", 1},
		{"podman options", "podman run --uidmap 0:1:1 --unsetenv A --no-hosts alpine:3", 1},
		{"an option which is not known stays unread", "docker run --not-an-option x alpine:3", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := imageFindings(t, imagesCfg(t, ""), runWorkflow(tc.script))
			if len(got) != tc.want {
				t.Errorf("want %d findings, got %q", tc.want, got)
			}
		})
	}
	t.Run("images-deny does not judge a transport", func(t *testing.T) {
		cfg := mustParseConfig(t, "rules:\n  forbidden-uses:\n    level: error\n    images-deny: [\"*\"]\n")
		var n int
		for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", runWorkflow("podman run docker-archive:/tmp/image.tar")) {
			if e.ID == "forbidden-uses" {
				n++
			}
		}
		if n != 0 {
			t.Errorf("want none, got %d", n)
		}
	})
}
