package jactionlint

import (
	"context"
	"io"
	"strings"
	"testing"
)

const (
	digestAlpine  = "sha256:6baf43584bcb78f2e5847d1de515f23499913ac9f12bdf834811a3145eb11ca1"
	digestPG16    = "sha256:ca0bd484cb98bf4b24eb1010e73fb3fcbd6714d240fbc1a10eea5b7dbecb641d"
	digestUbuntu  = "sha256:5ec03bb3441e8b0bf3b4f9cd4629a1ae763010dc3035bb8da3ae6cf026486401"
	digestNode    = "sha256:32fa97f3363975684b08bf4e8a68a47c7905175cc20275b50b974bbd02aba731"
	digestUV050   = "sha256:0e0fb77970aceaa106c1fee7103ec3e4885f6c4289e32a596123af06d2e9db9d"
	imageTestHead = "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n"
)

func imagesConfig(t *testing.T, extra string) *Config {
	t.Helper()
	return mustParseConfig(t, "rules:\n  unpinned-images: error\n  unpinned-uses: error\n"+extra)
}

func lintImages(t *testing.T, client GitHubClient, cfg *Config, src string) []*Error {
	t.Helper()
	errs, _ := lintOnline(t, client, cfg, src)
	return errs
}

func TestImageFixes(t *testing.T) {
	tests := []struct {
		name string
		job  string // the body of the job after runs-on
		want string // the body after the unsafe fix
	}{
		{"container", "    container: alpine:3.19\n    steps:\n      - run: echo\n", "    container: alpine:3.19@" + digestAlpine + "\n    steps:\n      - run: echo\n"},
		{"container with an image key", "    container:\n      image: postgres:16\n      options: --user root\n    steps:\n      - run: echo\n", "    container:\n      image: postgres:16@" + digestPG16 + "\n      options: --user root\n    steps:\n      - run: echo\n"},
		{"quoted", "    container: \"ubuntu:22.04\"\n    steps:\n      - run: echo\n", "    container: \"ubuntu:22.04@" + digestUbuntu + "\"\n    steps:\n      - run: echo\n"},
		{"single quoted with a comment", "    container: 'ubuntu:22.04' # the base\n    steps:\n      - run: echo\n", "    container: 'ubuntu:22.04@" + digestUbuntu + "' # the base\n    steps:\n      - run: echo\n"},
		{"trailing white space", "    container: alpine:3.19  \n    steps:\n      - run: echo\n", "    container: alpine:3.19@" + digestAlpine + "  \n    steps:\n      - run: echo\n"},
		{"service", "    services:\n      db:\n        image: postgres:16\n    steps:\n      - run: echo\n", "    services:\n      db:\n        image: postgres:16@" + digestPG16 + "\n    steps:\n      - run: echo\n"},
		{"no tag is latest", "    container: node\n    steps:\n      - run: echo\n", "    container: node:latest@" + digestNode + "\n    steps:\n      - run: echo\n"},
		{"the tag latest", "    container: node:latest\n    steps:\n      - run: echo\n", "    container: node:latest@" + digestNode + "\n    steps:\n      - run: echo\n"},
		{"docker hub by name", "    container: docker.io/library/alpine:3.19\n    steps:\n      - run: echo\n", "    container: docker.io/library/alpine:3.19@" + digestAlpine + "\n    steps:\n      - run: echo\n"},
		{"ghcr", "    container: ghcr.io/astral-sh/uv:0.5.0\n    steps:\n      - run: echo\n", "    container: ghcr.io/astral-sh/uv:0.5.0@" + digestUV050 + "\n    steps:\n      - run: echo\n"},
		{"uses docker", "    steps:\n      - uses: docker://alpine:3.19\n", "    steps:\n      - uses: docker://alpine:3.19@" + digestAlpine + "\n"},
		{"uses docker quoted", "    steps:\n      - uses: \"docker://alpine:3.19\"\n", "    steps:\n      - uses: \"docker://alpine:3.19@" + digestAlpine + "\"\n"},
		{"matrix values", "    strategy:\n      matrix:\n        image: [alpine:3.19]\n    container: ${{ matrix.image }}\n    steps:\n      - run: echo\n", ""},
		{"matrix include values", "    strategy:\n      matrix:\n        include:\n          - image: alpine:3.19\n          - image: ubuntu:22.04\n    container: ${{ matrix.image }}\n    steps:\n      - run: echo\n", "    strategy:\n      matrix:\n        include:\n          - image: alpine:3.19@" + digestAlpine + "\n          - image: ubuntu:22.04@" + digestUbuntu + "\n    container: ${{ matrix.image }}\n    steps:\n      - run: echo\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := imageTestHead + tc.job
			errs := lintImages(t, onlineFixtureClient(t), imagesConfig(t, ""), src)
			got, n := applyFixes([]byte(src), errs, FixModeUnsafe)
			want := imageTestHead + tc.want
			if tc.want == "" {
				want = src // A flow sequence: not edited
				if n != 0 {
					t.Fatalf("%d fixes applied: %s", n, got)
				}
			} else if n == 0 {
				t.Fatalf("no fix applied: %v", errs)
			}
			if string(got) != want {
				t.Errorf("got:\n%swant:\n%s", got, want)
			}
			// The plain mode leaves it
			if tc.want != "" {
				if out, n := applyFixes([]byte(src), errs, FixModeSafe); n != 0 || string(out) != src {
					t.Errorf("the fix is unsafe and --fix leaves it: %d applied", n)
				}
				// And what is left is clean
				for _, e := range lintImages(t, onlineFixtureClient(t), imagesConfig(t, ""), string(got)) {
					if e.ID == "unpinned-images" || e.ID == "unpinned-uses" {
						t.Errorf("after the fix: %s: %s", e.ID, e.Message)
					}
				}
			}
		})
	}
}

func TestImageFixesAreUnsafeAndSaySo(t *testing.T) {
	src := imageTestHead + "    container: alpine:3.19\n    steps:\n      - run: echo\n"
	errs := lintImages(t, onlineFixtureClient(t), imagesConfig(t, ""), src)
	var e *Error
	for _, x := range errs {
		if x.ID == "unpinned-images" {
			e = x
		}
	}
	if e == nil || e.Fix == nil {
		t.Fatalf("no fix: %v", errs)
	}
	if !e.Fix.Unsafe || !strings.Contains(e.Fix.Description, "Unsafe") || !strings.Contains(e.Fix.Description, "sha256:6baf43584bc") {
		t.Errorf("fix: %+v", e.Fix)
	}
}

func TestImageFixesNoFix(t *testing.T) {
	tests := []struct {
		name   string
		job    string
		code   NoFixCode
		reason string
	}{
		{"another registry", "    container: quay.io/example/tool:1\n    steps:\n      - run: echo\n", NoFixNeedsJudgment, "quay.io is not supported"},
		{"a registry with a port", "    container: localhost:5000/tool:1\n    steps:\n      - run: echo\n", NoFixNeedsJudgment, "not supported"},
		{"an image which is not there", "    container: example/gone:1\n    steps:\n      - run: echo\n", NoFixNeedsJudgment, "has no image"},
		{"a name with capitals", "    container: ghcr.io/Example/Tool:1\n    steps:\n      - run: echo\n", NoFixNeedsJudgment, "not a name of an image"},
		{"an expression", "    container: ${{ vars.REGISTRY }}/tool:1.2\n    steps:\n      - run: echo\n", NoFixUnsupportedShape, "expression"},
		{"an invalid digest", "    container: alpine:3.19@sha256:abc\n    steps:\n      - run: echo\n", NoFixNeedsJudgment, "not a valid sha256 digest"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintImages(t, onlineFixtureClient(t), imagesConfig(t, ""), imageTestHead+tc.job)
			var e *Error
			for _, x := range errs {
				if x.ID == "unpinned-images" {
					e = x
				}
			}
			if e == nil {
				t.Fatalf("no finding: %v", lineIDsOf(errs))
			}
			if e.Fix != nil {
				t.Fatalf("no fix expected: %+v", e.Fix)
			}
			if e.NoFix == nil || e.NoFix.Code != tc.code || !strings.Contains(e.NoFix.Reason, tc.reason) {
				t.Errorf("no-fix reason %+v, want %s containing %q", e.NoFix, tc.code, tc.reason)
			}
		})
	}
}

func TestImageFixNeedsTheOnlineChecks(t *testing.T) {
	src := imageTestHead + "    container: alpine:3.19\n    steps:\n      - uses: docker://alpine:3.19\n"
	errs := lintWithConfig(t, imagesConfig(t, ""), src)
	n := 0
	for _, e := range errs {
		if e.ID != "unpinned-images" && e.ID != "unpinned-uses" {
			continue
		}
		n++
		if e.Fix != nil || e.NoFix == nil || e.NoFix.Code != NoFixOnlineRequired {
			t.Errorf("%s: offline there is no fix and the reason says so: %+v %+v", e.ID, e.Fix, e.NoFix)
		}
	}
	if n != 2 {
		t.Errorf("%d findings", n)
	}
}

func TestImageFixDoesNotTouchTheTagsOfActions(t *testing.T) {
	// The tag of an action gets the pin fix, and an unfixable one keeps its own reason
	src := workflowWith("uses: actions/checkout@v4", "uses: docker://alpine:3.19")
	errs := lintImages(t, onlineFixtureClient(t), imagesConfig(t, ""), src)
	got, n := applyFixes([]byte(src), errs, FixModeUnsafe)
	want := workflowWith("uses: actions/checkout@"+pinCheckoutV4+" # v4", "uses: docker://alpine:3.19@"+digestAlpine)
	if n != 2 || string(got) != want {
		t.Errorf("%d fixes:\n%s\nwant:\n%s", n, got, want)
	}
}

func TestImageFixKeepsCRLF(t *testing.T) {
	src := "on: push\r\njobs:\r\n  test:\r\n    runs-on: ubuntu-latest\r\n    container: alpine:3.19\r\n    steps:\r\n      - name: \"é日本\"\r\n        uses: docker://alpine:3.19\r\n"
	errs := lintImages(t, onlineFixtureClient(t), imagesConfig(t, ""), src)
	got, n := applyFixes([]byte(src), errs, FixModeUnsafe)
	want := strings.ReplaceAll(src, "alpine:3.19\r\n", "alpine:3.19@"+digestAlpine+"\r\n")
	if n != 2 || string(got) != want {
		t.Errorf("%d fixes:\n%q\nwant:\n%q", n, got, want)
	}
}

func TestImageFixOfDockerAction(t *testing.T) {
	src := "name: x\ndescription: x\nruns:\n  using: docker\n  image: docker://alpine:3.19\n"
	l, err := NewLinter(io.Discard, &LinterOptions{Online: true, GitHubClient: onlineFixtureClient(t), LogWriter: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withFixtureRules(imagesConfig(t, ""))
	errs, err := l.Lint(".github/actions/x/action.yml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	got, n := applyFixes([]byte(src), errs, FixModeUnsafe)
	want := strings.Replace(src, "alpine:3.19", "alpine:3.19@"+digestAlpine, 1)
	if n != 1 || string(got) != want {
		t.Errorf("%d fixes:\n%s\nwant:\n%s", n, got, want)
	}
}

func TestImageFixRespectsTheDenyList(t *testing.T) {
	cfg := mustParseConfig(t, "online-options:\n  deny: [\"library/*\"]\nrules:\n  unpinned-images: error\n")
	c := &imageCountingClient{GitHubClient: onlineFixtureClient(t)}
	src := imageTestHead + "    container: alpine:3.19\n    services:\n      cache:\n        image: ghcr.io/astral-sh/uv:0.5.0\n    steps:\n      - run: echo\n"
	errs, l := lintOnline(t, c, cfg, src)
	for _, e := range errs {
		if e.ID == "unpinned-images" && strings.Contains(e.Message, "alpine") {
			if e.Fix != nil || e.NoFix == nil {
				t.Errorf("a denied image gets no fix: %+v %+v", e.Fix, e.NoFix)
			}
		}
		if e.ID == "unpinned-images" && strings.Contains(e.Message, "uv") && e.Fix == nil {
			t.Errorf("an image which is not denied is pinned: %+v", e.NoFix)
		}
	}
	if c.calls != 1 {
		t.Errorf("%d registry lookups, want 1 (the image of ghcr.io)", c.calls)
	}
	if l.online.sess.excludedRepos() != 1 {
		t.Errorf("%d excluded", l.online.sess.excludedRepos())
	}
}

type imageCountingClient struct {
	GitHubClient
	calls int
}

func (c *imageCountingClient) ImageDigest(_ context.Context, registry, repository, tag string) (string, error) {
	c.calls++
	return digestUV050, nil
}
