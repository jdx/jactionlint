package jactionlint

import (
	"strings"
	"testing"
)

func imagesCfg(t *testing.T, opts string) *Config {
	t.Helper()
	return mustParseConfig(t, "rules:\n  unpinned-images:\n    level: error\n"+opts)
}

func runWorkflow(script string) string {
	return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n" + indentScript(script) + "\n"
}

func indentScript(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "          " + l
	}
	return strings.Join(lines, "\n")
}

func imageFindings(t *testing.T, cfg *Config, src string) []string {
	t.Helper()
	var got []string
	for _, e := range lintFileWithConfig(t, cfg, "ci.yaml", src) {
		if e.ID == "unpinned-images" {
			got = append(got, e.Message)
		}
	}
	return got
}

// The images of docker run, docker create and docker pull in a script are checked like the ones of container:.
func TestUnpinnedImagesInScripts(t *testing.T) {
	const digest = "@sha256:0000000000000000000000000000000000000000000000000000000000000000"
	tests := []struct {
		name   string
		script string
		want   []string // the images which are reported, in order
	}{
		{"run without a tag", "docker run --rm alpine echo hi", []string{"alpine"}},
		{"run latest", "docker run alpine:latest", []string{"alpine:latest"}},
		{"run with a tag", "docker run --rm -v \"$PWD:/w\" -w /w node:20 npm test", []string{"node:20"}},
		{"run with a digest", "docker run --rm node:20" + digest + " npm test", nil},
		{"run with the options of a cluster", "docker run -it --rm -e A=b -p 80:80 -v /a:/b --name x ghcr.io/o/tool:1", []string{"ghcr.io/o/tool:1"}},
		{"value attached to a short option", "docker run -eA=b -p8080:80 alpine:3", []string{"alpine:3"}},
		{"cluster ending in a value option", "docker run -itv /a:/b alpine:3", []string{"alpine:3"}},
		{"an option with =", "docker run --network=host --entrypoint=sh alpine:3 -c true", []string{"alpine:3"}},
		{"end of options", "docker run --rm -- alpine:3 true", []string{"alpine:3"}},
		{"container run", "docker container run alpine:3", []string{"alpine:3"}},
		{"create", "docker create --name c alpine:3", []string{"alpine:3"}},
		{"pull", "docker pull alpine", []string{"alpine"}},
		{"pull with a platform", "docker image pull --platform linux/arm64 -q alpine:3", []string{"alpine:3"}},
		{"podman", "podman run --rm quay.io/o/img:1", []string{"quay.io/o/img:1"}},
		{"sudo", "sudo docker run alpine:3", []string{"alpine:3"}},
		{"global options", "docker -H unix:///x run alpine:3", []string{"alpine:3"}},
		{"two commands", "docker pull alpine:3\ndocker run alpine:3 true", []string{"alpine:3", "alpine:3"}},
		{"in a pipeline", "echo x | docker run -i alpine:3 cat", []string{"alpine:3"}},
		{"registry with a port", "docker run localhost:5000/img:1", []string{"localhost:5000/img:1"}},
		// not reported
		{"an option which is not known", "docker run --frobnicate alpine:3", nil},
		{"a variable", "docker run \"$IMAGE\"", nil},
		{"an expression", "docker run ${{ matrix.image }}", nil},
		{"a command substitution", "docker run $(docker build -q .)", nil},
		{"an image id", "docker run 3f2a1b9c8d7e", nil},
		{"scratch", "docker run scratch", nil},
		{"another verb", "docker exec c alpine:3", nil},
		{"build", "docker build -t alpine:3 .", nil},
		{"the image that the script builds", "docker build -t tool:ci .\ndocker run --rm tool:ci", nil},
		{"the image that the script builds with --tag", "docker buildx build --tag=ghcr.io/o/tool:ci --load .\ndocker run ghcr.io/o/tool:ci", nil},
		{"the image that the script tags", "docker tag a b\ndocker run b", nil},
		{"the same image run as a terminal is still an image", "docker run -t alpine:3", []string{"alpine:3"}},
		{"an image that another command gets as an argument", "./utils/docker/build.sh --amd64 app:localbuild\ndocker run --rm app:localbuild", nil},
		{"an image that is pushed", "docker push ghcr.io/o/app:ci\ndocker run ghcr.io/o/app:ci", nil},
		{"another image next to the one of a build", "docker build -t app:ci .\ndocker run alpine:3", []string{"alpine:3"}},
		{"loaded images", "docker load -i image.tar\ndocker run alpine:3", nil},
		{"not docker", "echo docker run alpine:3", nil},
		{"a comment", "# docker run alpine:3\ntrue", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := imageFindings(t, imagesCfg(t, ""), runWorkflow(tc.script))
			if len(got) != len(tc.want) {
				t.Fatalf("want findings for %q, got %q", tc.want, got)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], "image \""+w+"\" of \"docker ") && !strings.Contains(got[i], "image \""+w+"\" of \"podman ") {
					t.Errorf("finding %d: want the image %q, got %q", i, w, got[i])
				}
			}
		})
	}

	t.Run("an image of another step which builds it", func(t *testing.T) {
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/build-push-action@v6\n        with:\n          tags: |\n            ghcr.io/o/app:test\n          load: true\n      - run: docker run ghcr.io/o/app:test\n"
		if got := imageFindings(t, imagesCfg(t, ""), src); len(got) != 0 {
			t.Errorf("want nothing, got %q", got)
		}
	})
	t.Run("the position is the image", func(t *testing.T) {
		errs := lintFileWithConfig(t, imagesCfg(t, ""), "ci.yaml", runWorkflow("echo start\ndocker run --rm alpine:3 true"))
		var found bool
		for _, e := range errs {
			if e.ID == "unpinned-images" {
				found = true
				if e.Line != 8 || e.Column != 27 {
					t.Errorf("position %d:%d, want 8:27", e.Line, e.Column)
				}
			}
		}
		if !found {
			t.Error("no finding")
		}
	})
	t.Run("composite action", func(t *testing.T) {
		// the steps of a composite action are checked too (see TestActionScopeIsComplete)
		if scope := actionRuleScope["unpinned-images"]; scope.scope != actionApplies {
			t.Errorf("scope = %v", scope.scope)
		}
	})
}

func TestUnpinnedImagesPolicies(t *testing.T) {
	const opts = "    policies:\n      \"ghcr.io/my-org/*\": tag-pin\n      \"registry.example.com/*\": any\n      \"docker.io/library/*\": hash-pin\n      \"node:20\": any\n"
	tests := []struct {
		name  string
		cfg   string
		image string
		want  bool
	}{
		{"default, a tag", "", "alpine:3", true},
		{"tag-pin accepts a tag", opts, "ghcr.io/my-org/tool:1", false},
		{"tag-pin rejects latest", opts, "ghcr.io/my-org/tool:latest", true},
		{"tag-pin rejects no tag", opts, "ghcr.io/my-org/tool", true},
		{"nested path", opts, "ghcr.io/my-org/team/tool:1", false},
		{"another org", opts, "ghcr.io/other/tool:1", true},
		{"any", opts, "registry.example.com/team/tool", false},
		{"hash-pin of Docker Hub", opts, "alpine:3", true},
		{"hash-pin of Docker Hub with a registry", opts, "docker.io/library/alpine:3", true},
		{"a pattern with a tag wins over the registry", opts, "node:20", false},
		{"another tag of it", opts, "node:22", true},
		{"policy of * and require-digest", "    require-digest: false\n    policies:\n      \"*\": hash-pin\n", "alpine:3", true},
		{"require-digest false", "    require-digest: false\n", "alpine:3", false},
		{"require-digest false still rejects latest", "    require-digest: false\n", "alpine:latest", true},
		{"a policy of a pattern wins over require-digest", "    require-digest: false\n    policies:\n      \"alpine\": hash-pin\n", "alpine:3", true},
		{"case", "    policies:\n      \"GHCR.io/My-Org/*\": any\n", "ghcr.io/my-org/tool", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    container: " + tc.image + "\n    steps:\n      - run: echo\n"
			got := imageFindings(t, imagesCfg(t, tc.cfg), src)
			if (len(got) > 0) != tc.want {
				t.Errorf("want a finding: %v, got %q", tc.want, got)
			}
			// the same policy for a script
			got = imageFindings(t, imagesCfg(t, tc.cfg), runWorkflow("docker run "+tc.image))
			if (len(got) > 0) != tc.want {
				t.Errorf("script: want a finding: %v, got %q", tc.want, got)
			}
		})
	}

	for name, cfg := range map[string]string{
		"invalid policy":  "    policies:\n      \"*\": nope\n",
		"invalid pattern": "    policies:\n      \"a b\": any\n",
		"empty pattern":   "    policies:\n      \"\": any\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig([]byte("rules:\n  unpinned-images:\n    level: error\n" + cfg)); err == nil {
				t.Error("want an error for the configuration")
			}
		})
	}
}
