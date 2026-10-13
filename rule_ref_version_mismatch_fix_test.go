package jactionlint

import (
	"strings"
	"testing"
)

const (
	rvmCheckout = "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683" // v4.2.2
	rvmNoTags   = "actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529" // a commit no tag points to
)

func lintRefVersionMismatch(t *testing.T, client GitHubClient, src string) []*Error {
	t.Helper()
	errs, _ := lintOnline(t, client, nil, src)
	var ret []*Error
	for _, e := range errs {
		if e.ID == "ref-version-mismatch" {
			ret = append(ret, e)
		}
	}
	return ret
}

func TestRefVersionMismatchFix(t *testing.T) {
	tests := []struct {
		name string
		step string
		want string // The step after the unsafe fix
	}{
		{"another version", "uses: " + rvmCheckout + " # v3.0.0", "uses: " + rvmCheckout + " # v4.2.2"},
		{"a moving tag which moved on", "uses: " + rvmCheckout + " # v4", "uses: " + rvmCheckout + " # v4.2.2"},
		{"a version which does not exist", "uses: " + rvmCheckout + " # v4.9.9", "uses: " + rvmCheckout + " # v4.2.2"},
		{"the words around the version stay", "uses: " + rvmCheckout + " # tag=v3 (updated by a bot)", "uses: " + rvmCheckout + " # tag=v4.2.2 (updated by a bot)"},
		{"a following comment stays attached", "uses: " + rvmCheckout + " # v3 # jactionlint ignore=unpinned-uses", "uses: " + rvmCheckout + " # v4.2.2 # jactionlint ignore=unpinned-uses"},
		{"quoted value", `uses: "` + rvmCheckout + `" # v3`, `uses: "` + rvmCheckout + `" # v4.2.2`},
		{"no white space after the hash", "uses: " + rvmCheckout + " #v3", "uses: " + rvmCheckout + " #v4.2.2"},
		{"ratchet", "uses: " + rvmCheckout + " # ratchet:actions/checkout@v3", "uses: " + rvmCheckout + " # ratchet:actions/checkout@v4.2.2"},
		{"ratchet, other case", "uses: " + rvmCheckout + " # Ratchet:Actions/Checkout@v3", "uses: " + rvmCheckout + " # Ratchet:Actions/Checkout@v4.2.2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := workflowWith(tc.step)
			errs := lintRefVersionMismatch(t, onlineFixtureClient(t), src)
			if len(errs) != 1 {
				t.Fatalf("%d findings", len(errs))
			}
			e := errs[0]
			if e.Fix == nil || !e.Fix.Unsafe {
				t.Fatalf("an unsafe fix is expected: %+v (no fix: %+v)", e.Fix, e.NoFix)
			}
			if !strings.Contains(e.Message, "the SHA is wrong") || !strings.Contains(e.Fix.Description, "SHA is wrong") {
				t.Errorf("the message and the fix must say that the SHA can be the wrong part:\n%s\n%s", e.Message, e.Fix.Description)
			}

			// Plain --fix leaves it, --fix=unsafe applies it
			if got, n := applyFixes([]byte(src), errs, FixModeSafe); n != 0 || string(got) != src {
				t.Errorf("the safe mode applied %d fixes", n)
			}
			got, n := applyFixes([]byte(src), errs, FixModeUnsafe)
			if n != 1 {
				t.Fatalf("%d fixes applied", n)
			}
			if want := workflowWith(tc.want); string(got) != want {
				t.Errorf("got:\n%swant:\n%s", got, want)
			}
			if rest := lintRefVersionMismatch(t, onlineFixtureClient(t), string(got)); len(rest) != 0 {
				t.Errorf("still reported after the fix: %s", rest[0].Message)
			}
		})
	}
}

func TestRefVersionMismatchFixKeepsCRLFAndMultibyte(t *testing.T) {
	src := "on: push\r\njobs:\r\n  test:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - name: \"é日本\"\r\n        uses: " + rvmCheckout + " # v3 é日本\r\n"
	errs := lintRefVersionMismatch(t, onlineFixtureClient(t), src)
	if len(errs) != 1 || errs[0].Fix == nil {
		t.Fatalf("no fix: %+v", errs)
	}
	got, n := applyFixes([]byte(src), errs, FixModeUnsafe)
	want := strings.Replace(src, "# v3 ", "# v4.2.2 ", 1)
	if n != 1 || string(got) != want {
		t.Errorf("got %d fixes:\n%q\nwant:\n%q", n, got, want)
	}
}

func TestRefVersionMismatchNoFix(t *testing.T) {
	tests := []struct {
		name   string
		client func(*testing.T) GitHubClient
		step   string
		code   NoFixCode
		reason string
	}{
		{"the commit has no tag", func(t *testing.T) GitHubClient { return onlineFixtureClient(t) }, "uses: " + rvmNoTags + " # v4", NoFixNeedsJudgment, "no tag"},
		{"two versions of the same specificity", func(t *testing.T) GitHubClient { return tagsClient(t) }, "uses: example/tags@" + tagsSHA2 + " # v1.0.0", NoFixNeedsJudgment, "several versions"},
		{"only tags which are not versions", func(t *testing.T) GitHubClient {
			c, _ := NewFixtureGitHubClient([]byte(`{"repos":{"example/names":{"tags":{"tags":[{"name":"v1.0.0","sha":"` + tagsSHA1 + `"},{"name":"stable","sha":"` + tagsSHA2 + `"}]}}}}`))
			return c
		}, "uses: example/names@" + tagsSHA2 + " # v1.0.0", NoFixNeedsJudgment, "no version tag"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintRefVersionMismatch(t, tc.client(t), workflowWith(tc.step))
			if len(errs) != 1 {
				t.Fatalf("%d findings", len(errs))
			}
			e := errs[0]
			if e.Fix != nil {
				t.Fatalf("no fix expected: %+v", e.Fix)
			}
			if e.NoFix == nil || e.NoFix.Code != tc.code || !strings.Contains(e.NoFix.Reason, tc.reason) {
				t.Errorf("no-fix reason: %+v, want %s containing %q", e.NoFix, tc.code, tc.reason)
			}
		})
	}
}

func TestRatchetComments(t *testing.T) {
	// ratchet names the action and its version: the comment is read as the version of the action
	tests := []struct {
		name    string
		step    string
		finding bool
	}{
		{"matches the commit", "uses: " + rvmCheckout + " # ratchet:actions/checkout@v4.2.2", false},
		{"names another version", "uses: " + rvmCheckout + " # ratchet:actions/checkout@v3.0.0", true},
		{"names another action", "uses: " + rvmCheckout + " # ratchet:actions/cache@v3", false},
		{"excludes the pin", "uses: " + rvmCheckout + " # ratchet:exclude", false},
		{"names a branch", "uses: " + rvmCheckout + " # ratchet:actions/checkout@main", false},
		{"is not at the start", "uses: " + rvmCheckout + " # see ratchet:actions/checkout@v3", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintRefVersionMismatch(t, onlineFixtureClient(t), workflowWith(tc.step))
			if got := len(errs) > 0; got != tc.finding {
				t.Errorf("reported = %v, want %v: %v", got, tc.finding, errs)
			}
		})
	}
}

func TestCommentVersionAt(t *testing.T) {
	checkout := ParseUses("actions/checkout@" + strings.Repeat("a", 40))
	sub := ParseUses("github/codeql-action/analyze@" + strings.Repeat("a", 40))
	tests := []struct {
		text string
		ref  *UsesRef
		want string // "" when the comment has no version
		at   string // the text which the span covers
	}{
		{"v4.2.2", checkout, "v4.2.2", "v4.2.2"},
		{"  tag=v4 (x)", checkout, "v4", "v4"},
		{"tag= v4", checkout, "v4", "v4"},
		{"(v4)", checkout, "v4", "v4"},
		{"ratchet:actions/checkout@v4", checkout, "v4", "v4"},
		{"ratchet:github/codeql-action/analyze@v3.1.0 note", sub, "v3.1.0", "v3.1.0"},
		{"ratchet:github/codeql-action@v3", sub, "", ""},
		{"ratchet:actions/checkout@v4", nil, "", ""},
		{"ratchet:exclude", checkout, "", ""},
		{"keep v4", checkout, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			v, s, e, ok := commentVersionAt(tc.text, tc.ref)
			if ok != (tc.want != "") || v != tc.want {
				t.Fatalf("got %q, %v. want %q", v, ok, tc.want)
			}
			if ok && tc.text[s:e] != tc.at {
				t.Errorf("span %q, want %q", tc.text[s:e], tc.at)
			}
		})
	}
}
