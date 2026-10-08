package jactionlint

import (
	"strings"
	"testing"
)

func TestCanonLocalUsesSpec(t *testing.T) {
	tests := []struct {
		what string
		spec string
		want string
		ok   bool
	}{
		{"workspace relative action", "./action", "./action", true},
		{"workspace relative nested path", "./.github/actions/my-action", "./.github/actions/my-action", true},
		{"workspace relative workflow", "./.github/workflows/ci.yml", "./.github/workflows/ci.yml", true},
		{"self repository action", "$/action", "./action", true},
		{"self repository nested path", "$/.github/actions/my-action", "./.github/actions/my-action", true},
		{"self repository workflow", "$/.github/workflows/ci.yml", "./.github/workflows/ci.yml", true},
		{"self repository with empty path", "$/", "./", true},
		{"repository action", "owner/repo@v1", "", false},
		{"repository action with path", "owner/repo/path@v1", "", false},
		{"docker action", "docker://alpine:3.18", "", false},
		{"dollar without slash", "$action", "", false},
		{"dollar alone", "$", "", false},
		{"parent relative path", "../action", "", false},
		{"empty", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			have, ok := canonLocalUsesSpec(tc.spec)
			if ok != tc.ok {
				t.Fatalf("wanted ok=%v but have ok=%v for spec %q", tc.ok, ok, tc.spec)
			}
			if have != tc.want {
				t.Fatalf("wanted %q but have %q for spec %q", tc.want, have, tc.spec)
			}
		})
	}
}

// Both spellings must land on the same cache key, which is the property the metadata caches depend
// on: LocalReusableWorkflowCache.WriteWorkflowCallEvent writes its keys in the "./" form, and a
// "$/" caller that looked up anything else would never find them.
func TestCanonLocalUsesSpecAgreesBetweenForms(t *testing.T) {
	for _, path := range []string{"action", ".github/actions/my-action", ".github/workflows/ci.yml"} {
		local, ok := canonLocalUsesSpec("./" + path)
		if !ok {
			t.Fatalf("%q was not recognised as a local spec", "./"+path)
		}
		self, ok := canonLocalUsesSpec("$/" + path)
		if !ok {
			t.Fatalf("%q was not recognised as a local spec", "$/"+path)
		}
		if local != self {
			t.Errorf("%q and %q canonicalise differently: %q vs %q", "./"+path, "$/"+path, local, self)
		}
	}
}

func TestParseUses(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	digest := "sha256:" + strings.Repeat("ab", 32)
	tests := []struct {
		what string
		spec string
		want UsesRef // Raw is filled in by the test
	}{
		{"action with tag", "actions/checkout@v4", UsesRef{Kind: UsesAction, Owner: "actions", Repo: "checkout", Ref: "v4", HasRef: true, RefKind: RefSemverTag}},
		{"action with full sha", "actions/checkout@" + sha, UsesRef{Kind: UsesAction, Owner: "actions", Repo: "checkout", Ref: sha, HasRef: true, RefKind: RefFullSHA}},
		{"action with uppercase sha", "a/b@" + strings.ToUpper(sha), UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: strings.ToUpper(sha), HasRef: true, RefKind: RefFullSHA}},
		{"action with short sha", "a/b@abc1234", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: "abc1234", HasRef: true, RefKind: RefShortSHA}},
		{"39 hex digits is short", "a/b@" + sha[:39], UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: sha[:39], HasRef: true, RefKind: RefShortSHA}},
		{"41 hex digits is not a sha", "a/b@" + sha + "0", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: sha + "0", HasRef: true, RefKind: RefOther}},
		{"six hex digits is a branch", "a/b@abc123", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: "abc123", HasRef: true, RefKind: RefOther}},
		{"branch", "a/b@main", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: "main", HasRef: true, RefKind: RefOther}},
		{"branch with slash", "a/b@release/v1", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: "release/v1", HasRef: true, RefKind: RefOther}},
		{"full semver", "a/b@v1.2.3", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: "v1.2.3", HasRef: true, RefKind: RefSemverTag}},
		{"prerelease semver", "a/b@1.2.3-rc.1", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: "1.2.3-rc.1", HasRef: true, RefKind: RefSemverTag}},
		{"action with subpath", "github/codeql-action/init@v3", UsesRef{Kind: UsesAction, Owner: "github", Repo: "codeql-action", Subpath: "init", Ref: "v3", HasRef: true, RefKind: RefSemverTag}},
		{"action with nested subpath", "a/b/c/d@v1", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Subpath: "c/d", Ref: "v1", HasRef: true, RefKind: RefSemverTag}},
		{"trailing slash after repo", "a/b/@v1", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: "v1", HasRef: true, RefKind: RefSemverTag}},
		{"trailing slash in subpath", "a/b/c/@v1", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Subpath: "c/", Ref: "v1", HasRef: true, RefKind: RefSemverTag}},
		{"uppercase owner", "Actions/Checkout@v4", UsesRef{Kind: UsesAction, Owner: "Actions", Repo: "Checkout", Ref: "v4", HasRef: true, RefKind: RefSemverTag}},
		{"ref containing at", "a/b@v1@v2", UsesRef{Kind: UsesAction, Owner: "a", Repo: "b", Ref: "v1@v2", HasRef: true, RefKind: RefOther}},
		{"reusable workflow", "o/r/.github/workflows/x.yml@v1", UsesRef{Kind: UsesReusableWorkflow, Owner: "o", Repo: "r", Subpath: ".github/workflows/x.yml", Ref: "v1", HasRef: true, RefKind: RefSemverTag}},
		{"reusable workflow .yaml", "o/r/.github/workflows/X.YAML@" + sha, UsesRef{Kind: UsesReusableWorkflow, Owner: "o", Repo: "r", Subpath: ".github/workflows/X.YAML", Ref: sha, HasRef: true, RefKind: RefFullSHA}},

		{"ref missing", "actions/checkout", UsesRef{Kind: UsesInvalid, Problem: "ref is missing", Owner: "actions", Repo: "checkout"}},
		{"owner missing", "checkout@v4", UsesRef{Kind: UsesInvalid, Problem: "owner is missing", Ref: "v4", HasRef: true, RefKind: RefSemverTag}},
		{"empty ref", "a/b@", UsesRef{Kind: UsesInvalid, Problem: "owner and repo and ref should not be empty", Owner: "a", Repo: "b", HasRef: true, RefKind: RefOther}},
		{"empty owner", "/b@v1", UsesRef{Kind: UsesInvalid, Problem: "owner and repo and ref should not be empty", Repo: "b", Ref: "v1", HasRef: true, RefKind: RefSemverTag}},
		{"empty repo", "a/@v1", UsesRef{Kind: UsesInvalid, Problem: "owner and repo and ref should not be empty", Owner: "a", Ref: "v1", HasRef: true, RefKind: RefSemverTag}},
		{"empty", "", UsesRef{Kind: UsesInvalid, Problem: "ref is missing"}},
		{"only at", "@", UsesRef{Kind: UsesInvalid, Problem: "owner is missing", HasRef: true, RefKind: RefOther}},

		{"expression in ref", "a/b@${{ inputs.ref }}", UsesRef{Kind: UsesAction, Dynamic: true, Owner: "a", Repo: "b", Ref: "${{ inputs.ref }}", HasRef: true, RefKind: RefOther}},
		{"expression in owner", "${{ env.O }}/b@v1", UsesRef{Kind: UsesAction, Dynamic: true, Owner: "${{ env.O }}", Repo: "b", Ref: "v1", HasRef: true, RefKind: RefOther}},

		{"local", "./.github/actions/x", UsesRef{Kind: UsesLocal, Path: "./.github/actions/x"}},
		{"self repository", "$/.github/actions/x", UsesRef{Kind: UsesLocal, Path: "./.github/actions/x"}},
		{"local with at", "./x@v1", UsesRef{Kind: UsesLocal, Path: "./x@v1"}},

		{"docker tag", "docker://alpine:3.18", UsesRef{Kind: UsesDocker, Image: "alpine", Tag: "3.18", HasTag: true}},
		{"docker no tag", "docker://alpine", UsesRef{Kind: UsesDocker, Image: "alpine"}},
		{"docker empty tag", "docker://alpine:", UsesRef{Kind: UsesDocker, Image: "alpine", HasTag: true}},
		{"docker digest", "docker://alpine@" + digest, UsesRef{Kind: UsesDocker, Image: "alpine", Digest: digest, RefKind: RefDigest}},
		{"docker tag and digest", "docker://alpine:3.18@" + digest, UsesRef{Kind: UsesDocker, Image: "alpine", Tag: "3.18", HasTag: true, Digest: digest, RefKind: RefDigest}},
		{"docker registry port", "docker://host:5000/img@" + digest, UsesRef{Kind: UsesDocker, Image: "host:5000/img", Digest: digest, RefKind: RefDigest}},
		{"docker registry port and tag", "docker://host:5000/img:v1", UsesRef{Kind: UsesDocker, Image: "host:5000/img", Tag: "v1", HasTag: true}},
		{"docker registry port only", "docker://host:5000/img", UsesRef{Kind: UsesDocker, Image: "host:5000/img"}},
		{"docker short digest", "docker://alpine@sha256:abc", UsesRef{Kind: UsesDocker, Image: "alpine", Digest: "sha256:abc"}},
		{"docker uppercase digest", "docker://alpine@SHA256:" + strings.ToUpper(strings.Repeat("ab", 32)), UsesRef{Kind: UsesDocker, Image: "alpine", Digest: "SHA256:" + strings.ToUpper(strings.Repeat("ab", 32)), RefKind: RefDigest}},
		{"docker no image", "docker://", UsesRef{Kind: UsesInvalid, Problem: "image is missing"}},
	}

	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			want := tc.want
			want.Raw = tc.spec
			if want.Dynamic == false && strings.Contains(tc.spec, "${{") {
				t.Fatal("test case must set Dynamic")
			}
			have := ParseUses(tc.spec)
			if *have != want {
				t.Fatalf("parse %q\nwant: %+v\nhave: %+v", tc.spec, want, *have)
			}
		})
	}
}

func TestUsesRefHelpers(t *testing.T) {
	sha := strings.Repeat("a", 40)
	digest := "@sha256:" + strings.Repeat("0", 64)

	pinned := []struct {
		spec string
		want bool
	}{
		{"a/b@" + sha, true},
		{"a/b/c@" + sha, true},
		{"a/b/.github/workflows/x.yml@" + sha, true},
		{"a/b@v1", false},
		{"a/b@main", false},
		{"a/b@abc1234", false},
		{"a/b", false},
		{"a/b@" + "${{ inputs.sha }}", false},
		{"./x", true},
		{"$/x", true},
		{"docker://alpine" + digest, true},
		{"docker://host:5000/img" + digest, true},
		{"docker://alpine:3", false},
		{"docker://alpine:3@sha256:abc", false},
		{"", false},
	}
	for _, tc := range pinned {
		if have := ParseUses(tc.spec).IsPinned(); have != tc.want {
			t.Errorf("IsPinned(%q): want %v, have %v", tc.spec, tc.want, have)
		}
	}

	names := []struct{ spec, want string }{
		{"actions/checkout@v4", "actions/checkout"},
		{"Actions/Checkout@v4", "actions/checkout"},
		{"github/codeql-action/init@v3", "github/codeql-action/init"},
		{"github/codeql-action/init/@v3", "github/codeql-action/init"},
		{"a/b/.github/workflows/X.yml@v1", "a/b/.github/workflows/X.yml"},
		{"a/b/@v1", "a/b"},
		{"$/x", "./x"},
		{"./x", "./x"},
		{"docker://host:5000/img:v1", "host:5000/img"},
		{"nope", ""},
	}
	for _, tc := range names {
		if have := ParseUses(tc.spec).CanonicalName(); have != tc.want {
			t.Errorf("CanonicalName(%q): want %q, have %q", tc.spec, tc.want, have)
		}
	}

	same := []struct {
		a, b string
		want bool
	}{
		{"actions/checkout@v4", "actions/checkout/sub@v3", true},
		{"Actions/Checkout@v4", "actions/checkout@v4", true},
		{"o/r/.github/workflows/x.yml@v1", "o/r@v1", true},
		{"o/r@v1", "o/other@v1", false},
		{"o/r@v1", "p/r@v1", false},
		{"o/r@v1", "./x", false},
		{"./x", "./x", false},
		{"docker://o/r", "docker://o/r", false},
		{"o/r", "o/r@v1", false}, // invalid is not a repository reference
	}
	for _, tc := range same {
		if have := ParseUses(tc.a).SameRepo(ParseUses(tc.b)); have != tc.want {
			t.Errorf("SameRepo(%q, %q): want %v, have %v", tc.a, tc.b, tc.want, have)
		}
	}
	if ParseUses("o/r@v1").SameRepo(nil) {
		t.Error("SameRepo(nil) must be false")
	}
}

func TestUsesKindStrings(t *testing.T) {
	for _, k := range []UsesKind{UsesInvalid, UsesAction, UsesReusableWorkflow, UsesDocker, UsesLocal} {
		if k.String() == "" {
			t.Errorf("empty name for kind %d", k)
		}
	}
	for _, k := range []RefKind{RefNone, RefFullSHA, RefShortSHA, RefSemverTag, RefOther, RefDigest} {
		if k.String() == "" {
			t.Errorf("empty name for ref kind %d", k)
		}
	}
}

func TestWorkflowCallUsesFormats(t *testing.T) {
	tests := []struct {
		spec        string
		local, repo bool
	}{
		{"./.github/workflows/x.yml", true, false},
		{"$/.github/workflows/x.yml", true, false},
		{"./", false, false},
		{"./x.yml@v1", false, false},
		{"o/r/.github/workflows/x.yml@v1", false, true},
		{"o/r/x.yml@v1", false, true},
		{"o/r/x@v1", false, true},
		{"o/r@v1", false, false},
		{"o/r/@v1", false, false},
		{"o/r/x@", false, false},
		{"/r/x@v1", false, false},
		{"o//x@v1", false, false},
		{".o/r/x@v1", false, false},
		{"docker://o/r/x@v1", false, false},
	}
	for _, tc := range tests {
		u := ParseUses(tc.spec)
		if u.isLocalWorkflowCall() != tc.local || u.isRepoWorkflowCall() != tc.repo {
			t.Errorf("%q: want local=%v repo=%v, have local=%v repo=%v", tc.spec, tc.local, tc.repo, u.isLocalWorkflowCall(), u.isRepoWorkflowCall())
		}
	}
}
