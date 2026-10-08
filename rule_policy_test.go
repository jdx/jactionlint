package jactionlint

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The goldens of the permission, pinning and checkout rules (excessive-permissions, undocumented-permissions,
// unpinned-uses, unpinned-images, self-repository, github-app, artipacked and cache-poisoning) live in
// testdata/policy. They are not in testdata/err because the shared golden test lints that directory with
// the default profile, which does not enable these rules, and they need options (a policy map, include-read)
// which a file name suffix cannot carry.
//
//	testdata/policy/<name>.yaml        the input
//	testdata/policy/<name>.out         the expected errors, one per line like testdata/err
//	testdata/policy/<name>.cfg.yaml    the configuration to lint it with
//	testdata/policy/ok/<name>.yaml     an input which must cause no error (the rule named by the file, or <name>.cfg.yaml)
//
// Run `go test -run TestPolicyGolden -update-policy-golden .` to write the .out files; review the diff.

var updatePolicyGolden = flag.Bool("update-policy-golden", false, "write the expected outputs of testdata/policy")

// policyRuleIDs are the rules whose goldens are in testdata/policy.
var policyRuleIDs = []string{
	"excessive-permissions", "undocumented-permissions", "unpinned-uses", "unpinned-images",
	"self-repository", "github-app", "artipacked", "cache-poisoning",
}

func lintPolicyFixture(t *testing.T, path string) []*Error {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg *Config
	cfgPath := strings.TrimSuffix(path, ".yaml") + ".cfg.yaml"
	if b, err := os.ReadFile(cfgPath); err == nil {
		cfg, err = parseConfig(b)
		if err != nil {
			t.Fatalf("%s: %v", cfgPath, err)
		}
	} else {
		// Only the rule which the file is named after
		cfg = &Config{Rules: map[string]RuleConfig{}}
		name := strings.ReplaceAll(strings.TrimSuffix(filepath.Base(path), ".yaml"), "_", "-")
		for _, id := range policyRuleIDs {
			if strings.HasPrefix(name, id) {
				cfg.Rules[id] = RuleConfig{Level: SeverityError}
			}
		}
	}
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withFixtureRules(cfg)
	errs, err := l.Lint("test.yaml", src, nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func policyFixtures(t *testing.T, dir string) []string {
	t.Helper()
	all, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var ret []string
	for _, f := range all {
		if !strings.HasSuffix(f, ".cfg.yaml") {
			ret = append(ret, f)
		}
	}
	if len(ret) == 0 {
		t.Fatalf("no fixture in %s", dir)
	}
	return ret
}

func TestPolicyGolden(t *testing.T) {
	for _, f := range policyFixtures(t, filepath.Join("testdata", "policy")) {
		base := strings.TrimSuffix(f, ".yaml")
		t.Run(filepath.Base(base), func(t *testing.T) {
			errs := lintPolicyFixture(t, f)
			if *updatePolicyGolden {
				slices.SortFunc(errs, compareErrors)
				var sb strings.Builder
				for _, e := range errs {
					sb.WriteString(e.Error())
					sb.WriteByte('\n')
				}
				if err := os.WriteFile(base+".out", []byte(sb.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			checkErrors(t, base+".out", errs)
		})
	}
}

func TestPolicyOK(t *testing.T) {
	for _, f := range policyFixtures(t, filepath.Join("testdata", "policy", "ok")) {
		t.Run(strings.TrimSuffix(filepath.Base(f), ".yaml"), func(t *testing.T) {
			if errs := lintPolicyFixture(t, f); len(errs) > 0 {
				for _, e := range errs {
					t.Errorf("unexpected error: %s", e.Error())
				}
			}
		})
	}
}

// TestPolicyEveryRuleHasGoldens checks that each rule of the batch has an error golden and an ok golden.
func TestPolicyEveryRuleHasGoldens(t *testing.T) {
	for _, id := range policyRuleIDs {
		stem := strings.ReplaceAll(id, "-", "_")
		if m, _ := filepath.Glob(filepath.Join("testdata", "policy", stem+"*.out")); len(m) == 0 {
			t.Errorf("rule %q has no golden in testdata/policy", id)
		}
		if m, _ := filepath.Glob(filepath.Join("testdata", "policy", "ok", stem+"*.yaml")); len(m) == 0 {
			t.Errorf("rule %q has no ok fixture in testdata/policy/ok", id)
		}
	}
}

func policyLint(t *testing.T, cfg *Config, src string) []*Error {
	t.Helper()
	return lintWithConfig(t, withFixtureRules(cfg), src)
}

func policyErrorsOf(errs []*Error, id string) []*Error {
	var ret []*Error
	for _, e := range errs {
		if e.ID == id {
			ret = append(ret, e)
		}
	}
	return ret
}

// applyPolicyFixes applies the fixes of the errors with the given rule ID and returns the new source.
func applyPolicyFixes(src string, errs []*Error, id string, mode FixMode) (string, int) {
	out, n := applyFixes([]byte(src), policyErrorsOf(errs, id), mode)
	return string(out), n
}

func TestArtipackedFix(t *testing.T) {
	cfg := fixtureConfig("artipacked")
	tests := []struct {
		name   string
		src    string
		want   string
		unsafe bool
		fixed  bool
	}{
		{
			name:  "no with",
			src:   "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - run: echo hi\n",
			want:  "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n      - run: echo hi\n",
			fixed: true,
		},
		{
			name:  "uses with a trailing comment and keys after it",
			src:   "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Checkout\n        uses: actions/checkout@v4 # v4\n        id: co\n",
			want:  "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Checkout\n        uses: actions/checkout@v4 # v4\n        with:\n          persist-credentials: false\n        id: co\n",
			fixed: true,
		},
		{
			name:  "with has inputs and a comment before the first",
			src:   "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          # the changelog needs history\n          fetch-depth: 0\n",
			want:  "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n          # the changelog needs history\n          fetch-depth: 0\n",
			fixed: true,
		},
		{
			name:  "empty with",
			src:   "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n      - run: echo hi\n",
			want:  "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n      - run: echo hi\n",
			fixed: true,
		},
		{
			name:  "windows line endings",
			src:   "on: push\r\njobs:\r\n  j:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - uses: actions/checkout@v4\r\n",
			want:  "on: push\r\njobs:\r\n  j:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - uses: actions/checkout@v4\r\n        with:\r\n          persist-credentials: false\r\n",
			fixed: true,
		},
		{
			name:  "no final newline",
			src:   "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4",
			want:  "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false",
			fixed: true,
		},
		{
			name:   "a later git push makes it unsafe",
			src:    "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - run: git push origin HEAD\n",
			want:   "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n      - run: git push origin HEAD\n",
			unsafe: true,
			fixed:  true,
		},
		{
			name:   "a later pushing action makes it unsafe",
			src:    "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - uses: stefanzweifel/git-auto-commit-action@v5\n",
			want:   "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n      - uses: stefanzweifel/git-auto-commit-action@v5\n",
			unsafe: true,
			fixed:  true,
		},
		{
			name:  "git before the checkout does not matter",
			src:   "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: git push\n      - uses: actions/checkout@v4\n",
			want:  "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: git push\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n",
			fixed: true,
		},
		{
			name: "flow mapping is not fixed",
			src:  "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - {uses: actions/checkout@v4}\n",
		},
		{
			name: "flow with is not fixed",
			src:  "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with: {fetch-depth: 0}\n",
		},
		{
			name:  "two checkouts",
			src:   "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          path: sub\n",
			want:  "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n          path: sub\n",
			fixed: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := policyLint(t, cfg, tc.src)
			found := policyErrorsOf(errs, "artipacked")
			if len(found) == 0 {
				t.Fatalf("no artipacked error: %v", errs)
			}
			hasFix := false
			for _, e := range found {
				if e.Fix != nil {
					hasFix = true
					if e.Fix.Unsafe != tc.unsafe {
						t.Errorf("unsafe = %v, want %v", e.Fix.Unsafe, tc.unsafe)
					}
				}
			}
			if hasFix != tc.fixed && tc.name != "two checkouts" {
				t.Fatalf("has fix = %v, want %v", hasFix, tc.fixed)
			}
			if !tc.fixed {
				for _, e := range found {
					if e.Fix != nil {
						t.Errorf("an unfixable finding must not carry a fix: %+v", e.Fix)
					}
				}
				return
			}

			// A safe-only run applies safe fixes only
			got, n := applyPolicyFixes(tc.src, errs, "artipacked", FixModeSafe)
			if tc.unsafe {
				if n != 0 || got != tc.src {
					t.Errorf("an unsafe fix was applied in safe mode: %d\n%s", n, got)
				}
			}
			got, n = applyPolicyFixes(tc.src, errs, "artipacked", FixModeUnsafe)
			if n == 0 {
				t.Fatal("no fix was applied")
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("fixed source mismatch (-want +got):\n%s", diff)
			}
			// Linting the result is clean and fixing again changes nothing
			again := policyLint(t, cfg, got)
			if left := policyErrorsOf(again, "artipacked"); len(left) != 0 {
				t.Errorf("the fixed source still has findings: %v", left)
			}
			if again2, n := applyPolicyFixes(got, again, "artipacked", FixModeUnsafe); n != 0 || again2 != got {
				t.Errorf("fixing is not idempotent")
			}
		})
	}
}

func TestSelfRepositoryFix(t *testing.T) {
	cfg := fixtureConfig("self-repository")
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/x\n      - uses: \"./quoted\"\n      - uses: './'\n      - uses: $/ok\n  b:\n    uses: ./.github/workflows/y.yml\n"
	want := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: $/.github/actions/x\n      - uses: \"$/quoted\"\n      - uses: '$/'\n      - uses: $/ok\n  b:\n    uses: $/.github/workflows/y.yml\n"
	errs := policyLint(t, cfg, src)
	if n := len(policyErrorsOf(errs, "self-repository")); n != 4 {
		t.Fatalf("want 4 findings but got %d: %v", n, errs)
	}
	if got, n := applyPolicyFixes(src, errs, "self-repository", FixModeSafe); n != 0 || got != src {
		t.Errorf("the fix is unsafe so it must not be applied in safe mode")
	}
	got, n := applyPolicyFixes(src, errs, "self-repository", FixModeUnsafe)
	if n != 4 {
		t.Fatalf("want 4 fixes but got %d", n)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("fixed source mismatch (-want +got):\n%s", diff)
	}
	if left := policyErrorsOf(policyLint(t, cfg, got), "self-repository"); len(left) != 0 {
		t.Errorf("the fixed source still has findings: %v", left)
	}
}

func TestPolicyConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		want string // part of the error, "" for valid
	}{
		{"valid", "rules:\n  unpinned-uses:\n    policies:\n      actions/*: ref-pin\n      '*': any\n      owner/repo/path: hash-pin\n", ""},
		{"bad policy", "rules:\n  unpinned-uses:\n    policies:\n      actions/*: pinned\n", "policy \"pinned\""},
		{"bad pattern", "rules:\n  unpinned-uses:\n    policies:\n      'actions/check*': ref-pin\n", "pattern \"actions/check*\""},
		{"pattern with ref", "rules:\n  unpinned-uses:\n    policies:\n      'actions/checkout@v4': ref-pin\n", "pattern \"actions/checkout@v4\""},
		{"not a mapping", "rules:\n  unpinned-uses:\n    policies: ref-pin\n", "must be a mapping"},
		{"not a string", "rules:\n  unpinned-uses:\n    policies:\n      actions/*: 1\n", "must be a string"},
		{"bool option", "rules:\n  undocumented-permissions:\n    include-read: true\n", ""},
		{"bool option wrong type", "rules:\n  undocumented-permissions:\n    include-read: 3\n", "true or false"},
		{"unknown option", "rules:\n  unpinned-images:\n    digest: true\n", "unknown key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig([]byte(tc.cfg))
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("want an error containing %q but got %v", tc.want, err)
			}
		})
	}
}

func TestUsesPolicyFor(t *testing.T) {
	policies := map[string]string{
		"actions/checkout":      "hash-pin",
		"actions/*":             "ref-pin",
		"Trusted/*":             "any",
		"trusted/strict/sub":    "hash-pin",
		"owner/repo/sub/deeper": "any",
	}
	tests := []struct {
		uses string
		want usesPolicy
	}{
		{"actions/checkout@v4", usesPolicyHashPin},
		{"actions/checkout/sub@v4", usesPolicyHashPin},
		{"ACTIONS/Checkout@v4", usesPolicyHashPin},
		{"actions/setup-node@v4", usesPolicyRefPin},
		{"trusted/tool@v1", usesPolicyAny},
		{"trusted/strict/sub@v1", usesPolicyHashPin},
		{"trusted/strict/sub/dir@v1", usesPolicyHashPin},
		{"trusted/strict/subdir@v1", usesPolicyAny},
		{"trusted/strict@v1", usesPolicyAny},
		{"other/repo@v1", usesPolicyHashPin},
		{"owner/repo/sub/deeper/x@v1", usesPolicyAny},
		{"owner/repo/sub@v1", usesPolicyHashPin},
	}
	for _, tc := range tests {
		if got := usesPolicyFor(policies, ParseUses(tc.uses)); got != tc.want {
			t.Errorf("%s: want %v but got %v", tc.uses, tc.want, got)
		}
	}
	if got := usesPolicyFor(map[string]string{"*": "ref-pin"}, ParseUses("anyone/anything@v1")); got != usesPolicyRefPin {
		t.Errorf("the catch-all pattern was not applied: %v", got)
	}
	if got := usesPolicyFor(nil, ParseUses("anyone/anything@v1")); got != usesPolicyHashPin {
		t.Errorf("the default must be hash-pin but got %v", got)
	}
}

// TestUnpinnedUsesKeepsLegacyBehavior checks that the rule reports what it did before it had policies when it
// has no configuration, including the messages.
func TestUnpinnedUsesKeepsLegacyBehavior(t *testing.T) {
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - uses: docker://alpine:3.20\n  c:\n    uses: owner/repo/.github/workflows/x.yml@main\n"
	errs := lintWithConfig(t, withFixtureRules(&Config{Rules: map[string]RuleConfig{"unpinned-uses": {Level: SeverityError}}}), src)
	var got []string
	for _, e := range policyErrorsOf(errs, "unpinned-uses") {
		got = append(got, e.Message)
	}
	want := []string{
		`action "actions/checkout@v4" must be pinned to a full-length commit SHA like "{owner}/{repo}@{sha}" because the "unpinned-uses" rule is enabled`,
		`docker image must be pinned to a digest like "docker://{image}@sha256:{digest}" because the "unpinned-uses" rule is enabled: "docker://alpine:3.20"`,
		`reusable workflow call "owner/repo/.github/workflows/x.yml@main" must be pinned to a full-length commit SHA like "owner/repo/path/to/workflow.yml@{sha}" because the "unpinned-uses" rule is enabled`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("messages mismatch (-want +got):\n%s", diff)
	}
}

func TestPolicyRulesAreOffByDefault(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "policy", "artipacked.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range lintWithConfig(t, withFixtureRules(&Config{}), string(src)) {
		for _, id := range policyRuleIDs {
			if e.ID == id {
				t.Errorf("%s is reported with the default profile: %s", id, e.Error())
			}
		}
	}
	// ... and the strict profile enables them
	cfg := withFixtureRules(&Config{Profile: ProfileStrict})
	seen := map[string]bool{}
	for _, f := range []string{"artipacked", "cache_poisoning_release", "excessive_permissions", "unpinned_images", "self_repository", "github_app"} {
		b, err := os.ReadFile(filepath.Join("testdata", "policy", f+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range lintWithConfig(t, cfg, string(b)) {
			seen[e.ID] = true
		}
	}
	for _, id := range []string{"artipacked", "cache-poisoning", "excessive-permissions", "unpinned-images", "self-repository", "github-app"} {
		if !seen[id] {
			t.Errorf("%s is not enabled by the strict profile", id)
		}
	}
	// undocumented-permissions is pedantic: only the all profile enables it
	b, err := os.ReadFile(filepath.Join("testdata", "policy", "undocumented_permissions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(policyErrorsOf(lintWithConfig(t, cfg, string(b)), "undocumented-permissions")); n != 0 {
		t.Errorf("undocumented-permissions is reported with the strict profile")
	}
	if n := len(policyErrorsOf(lintWithConfig(t, withFixtureRules(&Config{Profile: ProfileAll}), string(b)), "undocumented-permissions")); n == 0 {
		t.Errorf("undocumented-permissions is not enabled by the all profile")
	}
}

func TestCachePoisoningActions(t *testing.T) {
	cfg := fixtureConfig("cache-poisoning")
	const head = "on:\n  release:\n    types: [published]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		name string
		step string
		want int
	}{
		{"cache action", "      - uses: actions/cache@v4\n        with:\n          path: x\n          key: y\n", 1},
		{"cache restore", "      - uses: actions/cache/restore@v4\n        with:\n          path: x\n          key: y\n", 1},
		{"cache save is a write only", "      - uses: actions/cache/save@v4\n        with:\n          path: x\n          key: y\n", 0},
		{"lookup only", "      - uses: actions/cache@v4\n        with:\n          path: x\n          key: y\n          lookup-only: true\n", 0},
		{"setup-node without cache", "      - uses: actions/setup-node@v4\n", 0},
		{"setup-node with cache", "      - uses: actions/setup-node@v4\n        with:\n          cache: pnpm\n", 1},
		{"setup-node with cache false", "      - uses: actions/setup-node@v4\n        with:\n          cache: false\n", 0},
		{"setup-node v5 opt out of the automatic cache", "      - uses: actions/setup-node@v5\n        with:\n          package-manager-cache: false\n", 0},
		{"setup-node v5 explicit cache with the automatic one off", "      - uses: actions/setup-node@v5\n        with:\n          cache: npm\n          package-manager-cache: false\n", 1},
		{"setup-go caches by default", "      - uses: actions/setup-go@v5\n", 1},
		{"setup-go v3 does not", "      - uses: actions/setup-go@v3\n", 0},
		{"setup-go v3 with cache", "      - uses: actions/setup-go@v3\n        with:\n          cache: true\n", 1},
		{"setup-go opt out", "      - uses: actions/setup-go@v5\n        with:\n          cache: false\n", 0},
		{"setup-uv default", "      - uses: astral-sh/setup-uv@v6\n", 1},
		{"setup-uv opt out", "      - uses: astral-sh/setup-uv@v6\n        with:\n          enable-cache: false\n", 0},
		{"setup-uv conditional", "      - uses: astral-sh/setup-uv@v6\n        with:\n          enable-cache: ${{ github.event_name != 'release' }}\n", 0},
		{"rust-cache", "      - uses: Swatinem/rust-cache@v2\n", 1},
		{"rust-cache pinned by sha", "      - uses: swatinem/rust-cache@6323deb102c322ba6fcbdcafc7e3dddab59af2b6 # v2\n", 1},
		{"mise-action", "      - uses: jdx/mise-action@v3\n", 1},
		{"mise-action no cache", "      - uses: jdx/mise-action@v3\n        with:\n          cache: false\n", 0},
		{"step condition on the trigger", "      - uses: Swatinem/rust-cache@v2\n        if: github.event_name == 'push'\n", 0},
		{"gradle", "      - uses: gradle/actions/setup-gradle@v4\n", 1},
		{"docker cache-from gha", "      - uses: docker/build-push-action@v6\n        with:\n          cache-from: type=gha\n", 1},
		{"docker without cache", "      - uses: docker/build-push-action@v6\n", 0},
		{"unrelated action", "      - uses: actions/checkout@v4\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := policyLint(t, cfg, head+tc.step)
			if got := len(policyErrorsOf(errs, "cache-poisoning")); got != tc.want {
				t.Errorf("want %d findings but got %d: %v", tc.want, got, errs)
			}
		})
	}

	// Which workflows and jobs count as release workflows
	releases := []struct {
		name string
		src  string
		want int
	}{
		{"tag push", "on:\n  push:\n    tags: ['*']\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n", 1},
		{"tags-ignore only", "on:\n  push:\n    tags-ignore: ['x']\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n", 0},
		{"branch push", "on:\n  push:\n    branches: [main]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n", 0},
		{"publish command in another job", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", 0},
		{"publish command in the job", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n      - run: |\n          cargo build\n          cargo publish --token x\n", 1},
		{"workflow cache-mode none", "on:\n  release:\n    types: [published]\ncache-mode: none\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n", 0},
		{"job cache-mode read overrides none", "on:\n  release:\n    types: [published]\ncache-mode: none\njobs:\n  j:\n    runs-on: ubuntu-latest\n    cache-mode: read\n    steps:\n      - uses: Swatinem/rust-cache@v2\n", 1},
	}
	for _, tc := range releases {
		t.Run(tc.name, func(t *testing.T) {
			errs := policyLint(t, cfg, tc.src)
			if got := len(policyErrorsOf(errs, "cache-poisoning")); got != tc.want {
				t.Errorf("want %d findings but got %d: %v", tc.want, got, errs)
			}
		})
	}
}

func FuzzUsesPolicyPattern(f *testing.F) {
	for _, s := range []string{"*", "owner/*", "owner/repo", "owner/repo/path", "owner/repo/*", "a//b", "a/b@c", "/", "", "*/x", "a/b*/c", "docker://x"} {
		f.Add(s, "owner/repo/path/x@v1")
	}
	f.Fuzz(func(t *testing.T, pattern, uses string) {
		p, err := parseUsesPattern(pattern)
		if err != nil {
			return
		}
		u := ParseUses(uses)
		// Matching never panics, and a pattern naming a repository matches that repository
		_ = p.matches(u)
		if p.owner != "" && p.repo != "" && p.path == "" {
			own := ParseUses(p.owner + "/" + p.repo + "@v1")
			if !p.matches(own) {
				t.Errorf("pattern %q does not match its own repository", pattern)
			}
		}
		_ = usesPolicyFor(map[string]string{pattern: "ref-pin"}, u)
	})
}

func FuzzArtipackedFix(f *testing.F) {
	for _, s := range []string{
		"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n",
		"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          # c\n          fetch-depth: 0\n",
		"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n      - run: echo\n",
		"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - {uses: actions/checkout@v4, with: {path: x}}\n",
		"on: push\r\njobs:\r\n  j:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      -   uses:   actions/checkout@v4   # c\r\n",
		"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with: &w\n          path: x\n",
	} {
		f.Add(s)
	}
	cfg := fixtureConfig("artipacked")
	f.Fuzz(func(t *testing.T, src string) {
		l, err := NewLinter(io.Discard, &LinterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = withFixtureRules(cfg)
		errs, err := l.Lint("test.yaml", []byte(src), nil)
		if err != nil {
			return
		}
		before := policyErrorsOf(errs, "artipacked")
		out, n := applyFixes([]byte(src), before, FixModeUnsafe)
		if n == 0 {
			return
		}
		// A fix never breaks the file and removes the findings it was made for
		if _, perrs := Parse([]byte(src)); len(perrs) == 0 {
			if _, perrs := Parse(out); len(perrs) != 0 {
				t.Fatalf("the fixed source does not parse: %v\n%s", perrs, out)
			}
		}
		after, err := l.Lint("test.yaml", out, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(policyErrorsOf(after, "artipacked")); got > len(before)-n {
			t.Fatalf("%d fixes applied but findings went from %d to %d\n%s", n, len(before), got, out)
		}
	})
}

// TestPolicyDocsExamples checks that the examples of the sections of docs/checks.md for these rules show
// what the linter prints. The sections skip the generated output because the rules are not enabled by default,
// so scripts/check-checks cannot lint their examples. The configuration is the first "rules:" block of the section.
func TestPolicyDocsExamples(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	// A Windows checkout may convert the line breaks of the document
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	for _, id := range policyRuleIDs {
		if id == "unpinned-uses" {
			continue // documented in the action format section, which scripts/check-checks maintains
		}
		info, ok := LookupRule(id)
		if !ok || info.DocsAnchor == "" {
			t.Errorf("%s: rule or docs anchor is missing", id)
			continue
		}
		start := strings.Index(doc, `<a id="`+info.DocsAnchor+`"></a>`)
		if start < 0 {
			t.Errorf("%s: anchor %q not found", id, info.DocsAnchor)
			continue
		}
		section := doc[start+1:]
		if i := strings.Index(section, `<a id="`); i >= 0 {
			section = section[:i]
		}
		input, ok := docsBlock(section, "Example input:\n\n```yaml\n")
		if !ok {
			t.Errorf("%s: the section has no example input", id)
			continue
		}
		output, ok := docsBlock(section, "Output:\n<!-- Skip update output -->\n```\n")
		if !ok {
			t.Errorf("%s: the section has no output block", id)
			continue
		}
		cfgText, ok := docsBlock(section, "```yaml\nrules:\n")
		if !ok {
			t.Errorf("%s: the section has no rules block", id)
			continue
		}
		t.Run(id, func(t *testing.T) {
			cfg, err := parseConfig([]byte("rules:\n" + cfgText))
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			l, err := NewLinter(&out, &LinterOptions{})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = withFixtureRules(cfg)
			if _, err := l.Lint("test.yaml", []byte(input), nil); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(strings.TrimRight(output, "\n"), strings.TrimRight(out.String(), "\n")); diff != "" {
				t.Errorf("the output in docs/checks.md is outdated (-docs +linter):\n%s", diff)
			}
		})
	}
}

// docsBlock returns the text from after the marker to the closing code fence.
func docsBlock(section, marker string) (string, bool) {
	i := strings.Index(section, marker)
	if i < 0 {
		return "", false
	}
	rest := section[i+len(marker):]
	j := strings.Index(rest, "\n```")
	if j < 0 {
		return "", false
	}
	return rest[:j+1], true
}
