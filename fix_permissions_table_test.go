package jactionlint

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The fix of missing-permissions writes the scopes which the table of action_permissions.go lists, on the job.
func TestFixMissingPermissionsFromTheTable(t *testing.T) {
	tests := []struct {
		what   string
		src    string
		want   string
		unsafe bool
	}{
		{
			what: "an exact entry gets a block on its job",
			src: `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: softprops/action-gh-release@v2
`,
			want: `on: push

permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
      - uses: softprops/action-gh-release@v2
`,
			unsafe: true,
		},
		{
			what: "the scopes of several actions of a job are joined and sorted, and contents: read stays",
			src: `on: push
jobs:
  deploy:
    name: Deploy
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/deploy-pages@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::1:role/x
          aws-region: us-east-1
`,
			want: `on: push

permissions:
  contents: read
jobs:
  deploy:
    name: Deploy
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
      pages: write
    timeout-minutes: 10
    steps:
      - uses: actions/deploy-pages@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::1:role/x
          aws-region: us-east-1
`,
			unsafe: true,
		},
		{
			what: "a job with a token of its own needs nothing from the table",
			src: `on: push
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: softprops/action-gh-release@v2
        with:
          token: ${{ secrets.PAT }}
`,
			want: `on: push

permissions:
  contents: read
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: softprops/action-gh-release@v2
        with:
          token: ${{ secrets.PAT }}
`,
			unsafe: true,
		},
		{
			what: "a job with permissions of its own is left alone",
			src: `on: push
jobs:
  release:
    runs-on: ubuntu-latest
    permissions: {}
    steps:
      - run: echo
  other:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`,
			want: `on: push

permissions:
  contents: read
jobs:
  release:
    runs-on: ubuntu-latest
    permissions: {}
    steps:
      - run: echo
  other:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			w, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			f := fixMissingPermissions(w)
			if f == nil {
				t.Fatal("no fix")
			}
			if f.Unsafe != tc.unsafe {
				t.Errorf("unsafe: want %v got %v", tc.unsafe, f.Unsafe)
			}
			out, n := applyFixes([]byte(tc.src), []*Error{{ID: "missing-permissions", Fix: f}}, FixModeUnsafe)
			if n != 1 {
				t.Fatalf("applied %d fixes", n)
			}
			if d := cmp.Diff(tc.want, string(out)); d != "" {
				t.Errorf("(-want +got):\n%s", d)
			}
			// What the fix wrote is what the rules ask for: no finding of the permissions rules is left
			cfg := mustParseConfig(t, "profile: default\nrules:\n  missing-permissions: error\n  insufficient-permissions: error\n")
			for _, e := range lintFileWithConfig(t, cfg, "test.yaml", string(out)) {
				if e.ID == "missing-permissions" || e.ID == "insufficient-permissions" {
					t.Errorf("after the fix: %v", e)
				}
			}
		})
	}
}

// A job which needs a block but is not written in the block style cannot get one, and writing only the workflow level would
// break it, so there is no fix.
func TestFixMissingPermissionsWithAJobOfAnotherShape(t *testing.T) {
	src := "on: push\njobs:\n  release: {runs-on: ubuntu-latest, steps: [{uses: softprops/action-gh-release@v2}]}\n"
	w, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if f := fixMissingPermissions(w); f != nil {
		t.Errorf("want no fix: %+v", f)
	}
}

// An action which the table does not determine leaves no fix, with the reason: "contents: read" would break the job.
func TestFixMissingPermissionsWithAnActionTheTableDoesNotDetermine(t *testing.T) {
	for _, use := range []string{"github/codeql-action/upload-sarif@v3", "actions/stale@v9"} {
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + use + "\n"
		w, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		f, why := fixMissingPermissionsWhy(w)
		if f != nil || why == nil || why.Code != NoFixNeedsJudgment {
			t.Errorf("%s: want no fix and a reason: %+v %+v", use, f, why)
		}
		cfg := mustParseConfig(t, "profile: default\nrules:\n  missing-permissions: error\n")
		errs := errsWithID(lintFileWithConfig(t, cfg, "test.yaml", src), "missing-permissions")
		if len(errs) != 2 || errs[0].Fix != nil || errs[0].NoFix == nil || errs[0].NoFix.Code != NoFixNeedsJudgment {
			t.Errorf("%s: the findings carry the reason: %v", use, errs)
		}
	}
}

// A step whose inputs are an expression may give another token, so what its action needs is not told and there is no fix.
func TestFixMissingPermissionsWithUnknownInputs(t *testing.T) {
	for _, with := range []string{"        with: ${{ fromJSON(inputs.with) }}\n", "        with:\n          files: a\n          ${{ insert }}: ${{ matrix.inputs }}\n"} {
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: softprops/action-gh-release@v2\n" + with
		w, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		f, why := fixMissingPermissionsWhy(w)
		if f != nil || why == nil || why.Code != NoFixNeedsJudgment {
			t.Errorf("%q: want no fix and a reason: %+v %+v", with, f, why)
		}
	}
}
