package jactionlint

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

const (
	pinCheckoutV4   = "11d5960a326750d5838078e36cf38b85af677262"
	pinSetupNodeV4  = "49933ea5288caeca8642d1e84afbd3f7d6820020"
	pinChangedV45   = "48d8f15b2aaa3d255ca5af3eba4870f807ce6b3c"
	pinCheckoutV422 = "11bd71901bbe5b1630ceea73d27597364c9af683"
)

func unpinnedConfig() *Config {
	return &Config{Rules: map[string]RuleConfig{"unpinned-uses": {Level: SeverityError, levelSet: true}}}
}

func TestPinFixes(t *testing.T) {
	tests := []struct {
		name string
		step string
		want string // The step after the fix. Empty when no fix is offered
	}{
		{"tag", "uses: actions/checkout@v4", "uses: actions/checkout@" + pinCheckoutV4 + " # v4"},
		{"exact version tag", "uses: actions/checkout@v4.2.2", "uses: actions/checkout@" + pinCheckoutV422 + " # v4.2.2"},
		{"double quotes", `uses: "actions/checkout@v4"`, `uses: "actions/checkout@` + pinCheckoutV4 + `" # v4`},
		{"single quotes", `uses: 'actions/checkout@v4'`, `uses: 'actions/checkout@` + pinCheckoutV4 + `' # v4`},
		{"trailing white space", "uses: actions/checkout@v4   ", "uses: actions/checkout@" + pinCheckoutV4 + " # v4"},
		{"comment naming the ref", "uses: actions/checkout@v4 # v4", "uses: actions/checkout@" + pinCheckoutV4 + " # v4"},
		{"comment naming the ref among words", "uses: actions/checkout@v4 # tag=v4 (see docs)", "uses: actions/checkout@" + pinCheckoutV4 + " # tag=v4 (see docs)"},
		{"another action", "uses: actions/setup-node@v4", "uses: actions/setup-node@" + pinSetupNodeV4 + " # v4"},
		{"a tag that is also a version of other tags", "uses: tj-actions/changed-files@v45", "uses: tj-actions/changed-files@" + pinChangedV45 + " # v45"},
		{"with other keys after", "uses: actions/checkout@v4\n        with:\n          fetch-depth: 0", "uses: actions/checkout@" + pinCheckoutV4 + " # v4\n        with:\n          fetch-depth: 0"},

		{"a branch", "uses: actions/upload-artifact@main", ""},
		{"a ref which is a tag and a branch", "uses: example/confusing@v1", ""},
		{"an unknown tag", "uses: actions/checkout@v99", ""},
		{"a comment about something else", "uses: actions/checkout@v4 # needed for the cache", ""},
		{"an unknown repository", "uses: example/forgotten@v1", ""},
		{"a docker image", "uses: docker://alpine:3.19", ""},
		{"an abbreviated sha", "uses: actions/checkout@abc1234", ""},
		{"a dynamic ref", "uses: actions/checkout@${{ matrix.ref }}", ""},
		{"a flow mapping", "{uses: actions/checkout@v4, name: x}", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := workflowWith(tc.step)
			errs, _ := lintOnline(t, onlineFixtureClient(t), unpinnedConfig(), src)
			var e *Error
			for _, x := range errs {
				if x.ID == "unpinned-uses" {
					e = x
				}
			}
			if e == nil {
				if tc.want == "" && strings.Contains(tc.step, "{") {
					return // Not reported at all
				}
				t.Fatalf("no unpinned-uses error: %v", idsOf(errs))
			}
			if tc.want == "" {
				if e.Fix != nil {
					t.Fatalf("an unfixable error must carry no fix: %+v", e.Fix)
				}
				return
			}
			if e.Fix == nil {
				t.Fatal("no fix")
			}
			if e.Fix.Unsafe {
				t.Error("pinning an exact tag is safe")
			}
			got, n := applyFixes([]byte(src), errs, FixModeSafe)
			if n != 1 {
				t.Fatalf("%d fixes applied", n)
			}
			if want := workflowWith(tc.want); string(got) != want {
				t.Errorf("got:\n%swant:\n%s", got, want)
			}

			// The result lints clean of the rule, and with the online checks too
			errs2, _ := lintOnline(t, onlineFixtureClient(t), unpinnedConfig(), string(got))
			for _, x := range errs2 {
				switch x.ID {
				case "unpinned-uses", "ref-version-mismatch", "impostor-commit", "stale-action-refs":
					t.Errorf("after the fix: %s: %s", x.ID, x.Message)
				}
			}
		})
	}
}

func TestPinFixOfReusableWorkflow(t *testing.T) {
	src := "on: push\njobs:\n  call:\n    uses: actions/checkout/.github/workflows/ci.yml@v4\n"
	errs, _ := lintOnline(t, onlineFixtureClient(t), unpinnedConfig(), src)
	got, n := applyFixes([]byte(src), errs, FixModeSafe)
	want := "on: push\njobs:\n  call:\n    uses: actions/checkout/.github/workflows/ci.yml@" + pinCheckoutV4 + " # v4\n"
	if n != 1 || string(got) != want {
		t.Errorf("got %d fixes:\n%s", n, got)
	}
}

func TestPinFixesOnlyWithTheOnlineChecks(t *testing.T) {
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{LogWriter: io.Discard}) // Not online
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = unpinnedConfig()
	errs, err := l.Lint("test.yaml", []byte(workflowWith("uses: actions/checkout@v4")), &Project{root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].Fix != nil {
		t.Errorf("offline there is no fix: %+v", errs)
	}
}

func TestPinFixKeepsCRLFAndMultibyteLines(t *testing.T) {
	src := "on: push\r\njobs:\r\n  test:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - name: \"é日本\" \r\n        uses: actions/checkout@v4\r\n"
	errs, _ := lintOnline(t, onlineFixtureClient(t), unpinnedConfig(), src)
	got, n := applyFixes([]byte(src), errs, FixModeSafe)
	want := strings.Replace(src, "@v4\r\n", "@"+pinCheckoutV4+" # v4\r\n", 1)
	if n != 1 || string(got) != want {
		t.Errorf("got %d fixes:\n%q\nwant:\n%q", n, got, want)
	}

	src = "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - { name: \"é日本\", uses: actions/checkout@v4 }\n"
	errs, _ = lintOnline(t, onlineFixtureClient(t), unpinnedConfig(), src)
	for _, e := range errs {
		if e.ID == "unpinned-uses" && e.Fix != nil {
			t.Errorf("a flow mapping is not edited: %+v", e.Fix)
		}
	}
}

func TestPinFixNeedsResolvableTag(t *testing.T) {
	// A client that fails: no fix and no crash
	c := &failingClient{err: &GitHubStatusError{Status: 500}}
	errs, _ := lintOnline(t, c, unpinnedConfig(), workflowWith("uses: actions/checkout@v4"))
	for _, e := range errs {
		if e.Fix != nil {
			t.Errorf("no fix is possible without GitHub: %+v", e.Fix)
		}
	}
}

func FuzzPinFix(f *testing.F) {
	for _, s := range []string{
		"      - uses: actions/checkout@v4\n",
		"      - uses: \"actions/checkout@v4\" # v4\n",
		"      - {uses: actions/checkout@v4}\n",
		"\xff\xfe uses: a/b@v1",
		"",
	} {
		f.Add([]byte(s), 1, 15)
	}
	client := func() *FixtureGitHubClient {
		b := []byte(`{"repos":{"actions/checkout":{"tags":{"tags":[{"name":"v4","sha":"` + pinCheckoutV4 + `"}]},"refs":{"heads/v4":{"found":false}}}}}`)
		c, _ := NewFixtureGitHubClient(b)
		return c
	}
	f.Fuzz(func(t *testing.T, src []byte, line, col int) {
		starts := lineStarts(src)
		if line < 1 || line > len(starts) {
			return
		}
		sess := newOnlineSession(context.Background(), client(), nil)
		fix, err := pinFix(sess, src, starts[line-1], col)
		if err != nil {
			return
		}
		if !fix.validFor(src) {
			t.Fatalf("an invalid fix %+v for %q", fix, src)
		}
		out, _ := applyFixes(src, []*Error{{Fix: fix}}, FixModeSafe)
		if bytes.Equal(out, src) {
			t.Fatalf("a fix which changes nothing for %q", src)
		}
	})
}
