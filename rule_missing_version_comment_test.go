package jactionlint

import (
	"io"
	"strings"
	"testing"
)

const (
	mvcCheckout    = "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683" // v4.2.2
	mvcCheckoutSHA = "11bd71901bbe5b1630ceea73d27597364c9af683"
	mvcNoTag       = "actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529" // no tag points to it
)

func TestMissingVersionCommentProfile(t *testing.T) {
	src := workflowWith("uses: " + mvcCheckout)
	if got := len(lintBatchH(t, pedanticCfg, src, "missing-version-comment")); got != 1 {
		t.Errorf("the pedantic profile reports the pin without a comment once, got %d", got)
	}
	for _, cfg := range []string{"profile: default\n", "profile: correctness\n", ""} {
		if got := lintBatchH(t, cfg, src, "missing-version-comment"); len(got) != 0 {
			t.Errorf("%q: the rule is pedantic: %v", cfg, got)
		}
	}
	// Turned on by name, in any profile
	if got := lintBatchH(t, "rules:\n  missing-version-comment: warning\n", src, "missing-version-comment"); len(got) != 1 {
		t.Errorf("the rule can be enabled by its name: %v", got)
	}
}

func TestMissingVersionCommentOfActionFile(t *testing.T) {
	src := "name: x\ndescription: x\nruns:\n  using: composite\n  steps:\n    - uses: " + mvcCheckout + "\n      shell: bash\n"
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withFixtureRules(mustParseConfig(t, "rules:\n  missing-version-comment: warning\n"))
	errs, err := l.Lint(".github/actions/x/action.yml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, e := range errs {
		if e.ID == "missing-version-comment" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d findings in the composite action", n)
	}
}

func TestMissingVersionCommentFix(t *testing.T) {
	tests := []struct {
		name string
		step string
		want string // the step after the fix, "" for no fix
		code NoFixCode
	}{
		{"no comment", "uses: " + mvcCheckout, "uses: " + mvcCheckout + " # v4.2.2", ""},
		{"trailing white space", "uses: " + mvcCheckout + "   ", "uses: " + mvcCheckout + " # v4.2.2", ""},
		{"quoted", `uses: "` + mvcCheckout + `"`, `uses: "` + mvcCheckout + `" # v4.2.2`, ""},
		{"single quoted with other keys after", "uses: '" + mvcCheckout + "'\n        with:\n          fetch-depth: 0", "uses: '" + mvcCheckout + "' # v4.2.2\n        with:\n          fetch-depth: 0", ""},
		{"upper case SHA", "uses: actions/checkout@11BD71901BBE5B1630CEEA73D27597364C9AF683", "uses: actions/checkout@11BD71901BBE5B1630CEEA73D27597364C9AF683 # v4.2.2", ""},
		{"a comment without a version", "uses: " + mvcCheckout + " # needed", "", NoFixNeedsJudgment},
		{"a commit which no tag points to", "uses: " + mvcNoTag, "", NoFixNeedsJudgment},
		{"a flow mapping", "{uses: " + mvcCheckout + ", name: x}", "", NoFixUnsupportedShape},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := workflowWith(tc.step)
			errs, _ := lintOnline(t, onlineFixtureClient(t), mustParseConfig(t, "rules:\n  missing-version-comment: warning\n"), src)
			var e *Error
			for _, x := range errs {
				if x.ID == "missing-version-comment" {
					e = x
				}
			}
			if e == nil {
				t.Fatalf("no finding: %v", lineIDsOf(errs))
			}
			if tc.want == "" {
				if e.Fix != nil {
					t.Fatalf("no fix expected: %+v", e.Fix)
				}
				if e.NoFix == nil || e.NoFix.Code != tc.code {
					t.Errorf("no-fix reason %+v, want %s", e.NoFix, tc.code)
				}
				return
			}
			if e.Fix == nil || e.Fix.Unsafe {
				t.Fatalf("a safe fix is expected: %+v (%+v)", e.Fix, e.NoFix)
			}
			got, n := applyFixes([]byte(src), errs, FixModeSafe)
			if n != 1 {
				t.Fatalf("%d fixes applied", n)
			}
			if want := workflowWith(tc.want); string(got) != want {
				t.Errorf("got:\n%swant:\n%s", got, want)
			}
			// Linting the result: no finding of this rule, and the comment is the version of the commit
			errs2, _ := lintOnline(t, onlineFixtureClient(t), mustParseConfig(t, "rules:\n  missing-version-comment: warning\n"), string(got))
			for _, x := range errs2 {
				if x.ID == "missing-version-comment" || x.ID == "ref-version-mismatch" {
					t.Errorf("after the fix: %s: %s", x.ID, x.Message)
				}
			}
		})
	}
}

func TestMissingVersionCommentNeedsOnlineForTheFix(t *testing.T) {
	src := workflowWith("uses: " + mvcCheckout)
	errs := lintBatchH(t, "rules:\n  missing-version-comment: warning\n", src, "missing-version-comment")
	if len(errs) != 1 {
		t.Fatalf("%d findings", len(errs))
	}
	if errs[0].Fix != nil || errs[0].NoFix == nil || errs[0].NoFix.Code != NoFixOnlineRequired {
		t.Errorf("offline there is no fix and the reason says so: %+v %+v", errs[0].Fix, errs[0].NoFix)
	}
}

func TestMissingVersionCommentAmbiguousVersion(t *testing.T) {
	// The commit is v2.0.1 and v2.0.2: nothing says which one to name
	src := workflowWith("uses: example/tags@" + tagsSHA2)
	errs, _ := lintOnline(t, tagsClient(t), mustParseConfig(t, "rules:\n  missing-version-comment: warning\n"), src)
	var found bool
	for _, e := range errs {
		if e.ID != "missing-version-comment" {
			continue
		}
		found = true
		if e.Fix != nil || e.NoFix == nil || !strings.Contains(e.NoFix.Reason, "several versions") {
			t.Errorf("ambiguous: %+v %+v", e.Fix, e.NoFix)
		}
	}
	if !found {
		t.Error("no finding")
	}

	// A commit with a moving tag and an exact one gets the exact one
	src = workflowWith("uses: example/tags@" + tagsSHA1)
	errs, _ = lintOnline(t, tagsClient(t), mustParseConfig(t, "rules:\n  missing-version-comment: warning\n"), src)
	got, n := applyFixes([]byte(src), errs, FixModeSafe)
	if want := workflowWith("uses: example/tags@" + tagsSHA1 + " # v1.2.3"); n != 1 || string(got) != want {
		t.Errorf("got %d fixes:\n%s", n, got)
	}
}

func TestMissingVersionCommentFixKeepsCRLF(t *testing.T) {
	src := "on: push\r\njobs:\r\n  test:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - name: \"é日本\"\r\n        uses: " + mvcCheckout + "\r\n"
	errs, _ := lintOnline(t, onlineFixtureClient(t), mustParseConfig(t, "rules:\n  missing-version-comment: warning\n"), src)
	got, n := applyFixes([]byte(src), errs, FixModeSafe)
	want := strings.Replace(src, mvcCheckoutSHA+"\r\n", mvcCheckoutSHA+" # v4.2.2\r\n", 1)
	if n != 1 || string(got) != want {
		t.Errorf("got %d fixes:\n%q\nwant:\n%q", n, got, want)
	}
}
