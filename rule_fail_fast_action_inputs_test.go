package jactionlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const failFastConfig = "rules:\n  fail-fast-action-inputs: warn\n"

// The lines which end with "# want" are the ones the rule reports.
func TestRuleFailFastActionInputs(t *testing.T) {
	const head = "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		what string
		src  string
	}{
		{"upload-artifact", head + "      - uses: actions/upload-artifact@v4 # want\n        with:\n          path: dist\n"},
		{"upload-artifact pinned", head + "      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4.6.2 # want\n        with:\n          path: dist\n"},
		{"upload-artifact without a name", head + "      - uses: actions/upload-artifact@v3 # want\n        with:\n          name: x\n          path: dist\n"},
		{"gh-release with files", head + "      - uses: softprops/action-gh-release@v2 # want\n        with:\n          files: dist/*\n"},
		{"release-action with artifacts", head + "      - uses: ncipollo/release-action@v1 # want\n        with:\n          artifacts: dist/*\n"},
		{"test results", head + "      - uses: EnricoMi/publish-unit-test-result-action@v2 # want\n        with:\n          files: report.xml\n"},
		{"test results of a variant", head + "      - uses: EnricoMi/publish-unit-test-result-action/composite@v2 # want\n        with:\n          files: report.xml\n"},
		{"junit report", head + "      - uses: mikepenz/action-junit-report@v5 # want\n        with:\n          report_paths: '**/TEST-*.xml'\n"},
		{"a job that runs on a condition of its own", head + "      - uses: actions/upload-artifact@v4 # want\n        if: github.event_name == 'push'\n        with:\n          path: dist\n"},

		// The input is given, whatever its value: the person decided
		{"error", head + "      - uses: actions/upload-artifact@v4\n        with:\n          path: dist\n          if-no-files-found: error\n"},
		{"warn on purpose", head + "      - uses: actions/upload-artifact@v4\n        with:\n          path: dist\n          if-no-files-found: warn\n"},
		{"ignore on purpose", head + "      - uses: actions/upload-artifact@v4\n        with:\n          path: dist\n          if-no-files-found: ignore\n"},
		{"an expression", head + "      - uses: actions/upload-artifact@v4\n        with:\n          path: dist\n          if-no-files-found: ${{ inputs.strict && 'error' || 'warn' }}\n"},
		{"gh-release which does not fail on purpose", head + "      - uses: softprops/action-gh-release@v2\n        with:\n          files: dist/*\n          fail_on_unmatched_files: false\n"},

		// The input does not matter for the step
		{"gh-release without files", head + "      - uses: softprops/action-gh-release@v2\n        with:\n          tag_name: v1\n"},
		{"gh-release without a with", head + "      - uses: softprops/action-gh-release@v2\n"},
		{"release-action without artifacts", head + "      - uses: ncipollo/release-action@v1\n        with:\n          tag: v1\n"},

		// A step which is best effort
		{"after a failure", head + "      - uses: actions/upload-artifact@v4\n        if: failure()\n        with:\n          path: logs\n"},
		{"always", head + "      - uses: actions/upload-artifact@v4\n        if: ${{ always() }}\n        with:\n          path: logs\n"},
		{"cancelled", head + "      - uses: actions/upload-artifact@v4\n        if: cancelled()\n        with:\n          path: logs\n"},
		{"success and a condition is not best effort", head + "      - uses: actions/upload-artifact@v4 # want\n        if: success() && github.ref == 'refs/heads/main'\n        with:\n          path: dist\n"},
		{"continue-on-error", head + "      - uses: actions/upload-artifact@v4\n        continue-on-error: true\n        with:\n          path: dist\n"},
		{"a step that never runs", head + "      - uses: actions/upload-artifact@v4\n        if: false\n        with:\n          path: dist\n"},

		// The version does not have the input
		{"upload-artifact v1", head + "      - uses: actions/upload-artifact@v1\n        with:\n          name: x\n          path: dist\n"},
		{"upload-artifact v1 pinned", head + "      - uses: actions/upload-artifact@3446296876d12d4e3a0f3145a3c87e67bf0a16b5 # v1.0.0\n        with:\n          name: x\n          path: dist\n"},

		// Other actions are not in the table
		{"download-artifact", head + "      - uses: actions/download-artifact@v4\n"},
		{"a local action", head + "      - uses: ./.github/actions/upload\n"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			checkLines(t, lintBatchH(t, failFastConfig, tc.src, "fail-fast-action-inputs"), markedWantLines(tc.src)...)
		})
	}
}

// The rule is in the pedantic profile only.
func TestFailFastActionInputsProfile(t *testing.T) {
	const src = "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/upload-artifact@v4\n        with:\n          path: dist\n"
	for profile, want := range map[string]int{"correctness": 0, "default": 0, "pedantic": 1} {
		if got := lintBatchH(t, "profile: "+profile+"\n", src, "fail-fast-action-inputs"); len(got) != want {
			t.Errorf("%s: want %d findings: %v", profile, want, got)
		}
	}
	if got := lintBatchH(t, "profile: pedantic\n", src, "fail-fast-action-inputs"); len(got) == 1 && got[0].Severity != SeverityWarning {
		t.Errorf("the level is a warning: %v", got[0])
	}
}

func TestRuleFailFastActionInputsMessage(t *testing.T) {
	const src = "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/upload-artifact@v4\n        with:\n          path: dist\n"
	errs := lintBatchH(t, failFastConfig, src, "fail-fast-action-inputs")
	if len(errs) != 1 {
		t.Fatalf("want 1 finding: %v", errs)
	}
	want := `actions/upload-artifact does not fail by default: the step only warns when "path" matches no file, so a build which produced nothing stays green and the artifact is missing. set "if-no-files-found: error" (the default is "warn")`
	if errs[0].Message != want {
		t.Errorf("message:\n got %s\nwant %s", errs[0].Message, want)
	}
}

// The fix adds the input to the with of the step in every shape the step can be written in.
func TestFixFailFastActionInputs(t *testing.T) {
	tests := []struct {
		what string
		step string
		want string
	}{
		{
			"a with mapping",
			"      - uses: actions/upload-artifact@v4\n        with:\n          path: dist\n",
			"      - uses: actions/upload-artifact@v4\n        with:\n          if-no-files-found: error\n          path: dist\n",
		},
		{
			"a comment above the first input stays with it",
			"      - uses: actions/upload-artifact@v4\n        with:\n          # what to upload\n          path: dist\n",
			"      - uses: actions/upload-artifact@v4\n        with:\n          if-no-files-found: error\n          # what to upload\n          path: dist\n",
		},
		{
			"an empty with",
			"      - uses: mikepenz/action-junit-report@v5\n        with:\n",
			"      - uses: mikepenz/action-junit-report@v5\n        with:\n          require_tests: true\n",
		},
		{
			"no with for an input which always matters",
			"      - name: Report\n        uses: EnricoMi/publish-unit-test-result-action@v2\n        id: report\n",
			"      - name: Report\n        uses: EnricoMi/publish-unit-test-result-action@v2\n        with:\n          action_fail_on_inconclusive: true\n        id: report\n",
		},
		{
			"a flow with",
			"      - uses: actions/upload-artifact@v4\n        with: {path: dist}\n",
			"      - uses: actions/upload-artifact@v4\n        with: {if-no-files-found: error, path: dist}\n",
		},
		{
			"a flow step",
			"      - {uses: actions/upload-artifact@v4, with: {path: dist}}\n",
			"      - {uses: actions/upload-artifact@v4, with: {if-no-files-found: error, path: dist}}\n",
		},
		{
			"a flow step without a with",
			"      - {uses: mikepenz/action-junit-report@v5}\n",
			"      - {with: {require_tests: true}, uses: mikepenz/action-junit-report@v5}\n",
		},
	}
	cfg := mustParseConfig(t, failFastConfig)
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			const steps = "jobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n"
			src := "on: push\n" + steps + tc.step
			errs := errsWithID(lintFileWithConfig(t, cfg, "test.yaml", src), "fail-fast-action-inputs")
			if len(errs) != 1 {
				t.Fatalf("want 1 finding: %v", errs)
			}
			f := errs[0].Fix
			if f == nil || !f.Unsafe {
				t.Fatalf("want an unsafe fix: %+v (%+v)", f, errs[0].NoFix)
			}
			out, n := applyFixes([]byte(src), errs, FixModeSafe)
			if n != 0 || string(out) != src {
				t.Errorf("the safe mode must not apply the fix")
			}
			out, n = applyFixes([]byte(src), errs, FixModeUnsafe)
			if n != 1 {
				t.Fatalf("applied %d fixes", n)
			}
			if d := cmp.Diff("on: push\n"+steps+tc.want, string(out)); d != "" {
				t.Errorf("(-want +got):\n%s", d)
			}
			if left := errsWithID(lintFileWithConfig(t, cfg, "test.yaml", string(out)), "fail-fast-action-inputs"); len(left) != 0 {
				t.Errorf("the finding is still there after the fix: %v", left)
			}
		})
	}
}

// Several fixes of one file apply together.
func TestFixFailFastActionInputsTogether(t *testing.T) {
	const src = `on: push
jobs:
  t:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@v4
        with:
          path: a
      - uses: actions/upload-artifact@v4
        with:
          name: b
          path: b
      - uses: softprops/action-gh-release@v2
        with:
          files: dist/*
`
	const want = `on: push
jobs:
  t:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@v4
        with:
          if-no-files-found: error
          path: a
      - uses: actions/upload-artifact@v4
        with:
          if-no-files-found: error
          name: b
          path: b
      - uses: softprops/action-gh-release@v2
        with:
          fail_on_unmatched_files: true
          files: dist/*
`
	out, n := fixAndLint(t, mustParseConfig(t, failFastConfig), "test.yaml", src, FixModeUnsafe)
	if n == nil && out == want {
		return
	}
	if d := cmp.Diff(want, out); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
	for _, e := range n {
		if e.ID == "fail-fast-action-inputs" {
			t.Errorf("left: %v", e)
		}
	}
}

// What the docs show is what the rule prints.
func TestFailFastDocsExample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	start := strings.Index(doc, `<a id="check-fail-fast-action-inputs"></a>`)
	if start < 0 {
		t.Fatal("no section")
	}
	section := doc[start+1:]
	if i := strings.Index(section, `<a id="`); i >= 0 {
		section = section[:i]
	}
	input, ok := docsBlock(section, "Example input:\n\n```yaml\n")
	if !ok {
		t.Fatal("no example input")
	}
	output, ok := docsBlock(section, "Output:\n<!-- Skip update output -->\n```\n")
	if !ok {
		t.Fatal("no output block")
	}
	cfgText, ok := docsBlock(section, "```yaml\nrules:\n")
	if !ok {
		t.Fatal("no rules block")
	}
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
}

// Every entry of the table names its source and an input.
func TestFailFastTable(t *testing.T) {
	for name, e := range failFastTable {
		if name != strings.ToLower(name) || e.Name == "" || e.Value == "" || e.Default == "" || e.Problem == "" || !strings.HasPrefix(e.Source, "https://") {
			t.Errorf("%s: incomplete entry %+v", name, e)
		}
	}
}
