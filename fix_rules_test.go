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
	"go.yaml.in/yaml/v4"
)

// updateFixGoldens rewrites the expected files in testdata/fix.
var updateFixGoldens = flag.Bool("update-fix-goldens", false, "rewrite the expected files of testdata/fix")

// fixerIDs are the rules whose fixes TestFixers applies. Fixes of other rules are ignored so that the
// goldens do not change when another rule gets a fixer.
var fixerIDs = []string{"missing-timeout", "missing-permissions", "unused-ignore"}

func fixerConfig(t *testing.T, extra string) *Config {
	t.Helper()
	return mustParseConfig(t, "profile: default\nrules:\n  missing-permissions: error\n  missing-timeout:\n    default-minutes: 30\n  unused-ignore: error\n"+extra)
}

// fixWith lints the source and applies the fixes of fixerIDs until nothing changes, like FixFiles does. It
// returns the new source, the number of the fixes applied and the errors left from fixerIDs.
func fixWith(t *testing.T, src []byte, cfg *Config, mode FixMode) ([]byte, int, []*Error) {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	total := 0
	for pass := 0; ; pass++ {
		errs, err := l.Lint("test.yaml", src, nil)
		if err != nil {
			t.Fatal(err)
		}
		var mine []*Error
		for _, e := range errs {
			if slices.Contains(fixerIDs, e.ID) {
				mine = append(mine, e)
			}
		}
		out, n := applyFixes(src, mine, mode)
		if n == 0 || pass >= maxFixPasses {
			return src, total, mine
		}
		total += n
		src = out
	}
}

// docWithoutAdditions parses the workflow and removes what the fixers add: the timeout-minutes of the
// jobs and the workflow-level permissions, when the original did not have them.
func docWithoutAdditions(t *testing.T, fixed, orig []byte) (any, any) {
	t.Helper()
	var o, f map[string]any
	if err := yaml.Unmarshal(orig, &o); err != nil {
		t.Fatalf("original: %v", err)
	}
	if err := yaml.Unmarshal(fixed, &f); err != nil {
		t.Fatalf("fixed workflow is not valid YAML: %v\n%s", err, fixed)
	}
	if _, ok := o["permissions"]; !ok {
		delete(f, "permissions")
	}
	ojobs, _ := o["jobs"].(map[string]any)
	fjobs, _ := f["jobs"].(map[string]any)
	for id, fj := range fjobs {
		fjm, _ := fj.(map[string]any)
		ojm, _ := ojobs[id].(map[string]any)
		if fjm == nil {
			continue
		}
		if _, ok := ojm["timeout-minutes"]; !ok {
			delete(fjm, "timeout-minutes")
		}
	}
	return o, f
}

func toCRLF(b []byte) []byte {
	return []byte(strings.ReplaceAll(string(b), "\n", "\r\n"))
}

func readGolden(t *testing.T, path string) ([]byte, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return b, true
}

func writeGolden(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// unfixable are the cases which keep findings after fixing, because the fixers skip shapes they do
// not understand instead of guessing. The value is the number of findings left by ID.
var unfixable = map[string]map[string]int{
	"crafted_flow_style.yaml": {"missing-timeout": 3},
	// Their zizmor comments name cache-poisoning, adhoc-packages and use-trusted-publishing, which the default
	// profile enables. jactionlint reports nothing where the cache-poisoning comment sits, and the adhoc-packages
	// comment of the first one is about local tarballs, which jactionlint does not call ad hoc, so these
	// comments are stale for it. There is no fix for the comment of another tool
	"aube-bun-lock-import_node-addon-impl.yaml": {"unused-ignore": 2},
	"communique-cloudflare_release-plz.yaml":    {"unused-ignore": 1},
}

// TestFixers applies the fixes of missing-timeout, missing-permissions and unused-ignore to real
// workflows and to crafted ones in testdata/fix. The expected files are NAME.fixed.yaml for the safe fixes, and
// NAME.unsafe.yaml when applying the unsafe fixes too gives another result.
func TestFixers(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "fix", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	files = slices.DeleteFunc(files, func(f string) bool {
		return strings.HasSuffix(f, ".fixed.yaml") || strings.HasSuffix(f, ".unsafe.yaml")
	})
	if len(files) < 30 {
		t.Fatalf("testdata/fix must have the workflows to fix but got %d", len(files))
	}
	cfg := fixerConfig(t, "")

	for _, f := range files {
		base := strings.TrimSuffix(f, ".yaml")
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}

			safe, nSafe, _ := fixWith(t, src, cfg, FixModeSafe)
			unsafe, nUnsafe, left := fixWith(t, src, cfg, FixModeUnsafe)

			if *updateFixGoldens {
				writeGolden(t, base+".fixed.yaml", safe)
				if string(unsafe) != string(safe) {
					writeGolden(t, base+".unsafe.yaml", unsafe)
				} else {
					os.Remove(base + ".unsafe.yaml")
				}
			}
			wantSafe, ok := readGolden(t, base+".fixed.yaml")
			if !ok {
				t.Fatalf("%s.fixed.yaml is missing. run go test -run TestFixers -update-fix-goldens .", base)
			}
			if diff := cmp.Diff(string(wantSafe), string(safe)); diff != "" {
				t.Errorf("safe fixes (-want +got):\n%s", diff)
			}
			wantUnsafe, ok := readGolden(t, base+".unsafe.yaml")
			if !ok {
				wantUnsafe = wantSafe
			}
			if diff := cmp.Diff(string(wantUnsafe), string(unsafe)); diff != "" {
				t.Errorf("unsafe fixes (-want +got):\n%s", diff)
			}
			if nUnsafe < nSafe {
				t.Errorf("unsafe mode applied %d fixes, less than %d of the safe mode", nUnsafe, nSafe)
			}

			// What the fixers leave are the findings they cannot fix, and they have no fix at all
			wantLeft := unfixable[name]
			gotLeft := map[string]int{}
			for _, e := range left {
				if e.Fix != nil {
					t.Errorf("a finding with a fix is left: %v", e)
				}
				gotLeft[e.ID]++
			}
			if wantLeft == nil {
				wantLeft = map[string]int{}
			}
			if diff := cmp.Diff(wantLeft, gotLeft); diff != "" {
				t.Errorf("findings left after fixing (-want +got):\n%s", diff)
			}

			// Fixing is idempotent
			for mode, out := range map[FixMode][]byte{FixModeSafe: safe, FixModeUnsafe: unsafe} {
				again, n, _ := fixWith(t, out, cfg, mode)
				if n != 0 || string(again) != string(out) {
					t.Errorf("fixing again changed the file (%d fixes):\n%s", n, again)
				}
			}

			// Only the lines of the fixes were added to the workflow: the rest means the same
			for _, out := range [][]byte{safe, unsafe} {
				orig, fixed := docWithoutAdditions(t, out, src)
				if diff := cmp.Diff(orig, fixed); diff != "" {
					t.Errorf("the fix changed more than it adds (-orig +fixed):\n%s", diff)
				}
			}

			// A file with CRLF line endings is fixed with CRLF line endings
			crlf := toCRLF(src)
			gotCRLF, _, _ := fixWith(t, crlf, cfg, FixModeUnsafe)
			if diff := cmp.Diff(string(toCRLF(unsafe)), string(gotCRLF)); diff != "" {
				t.Errorf("CRLF (-want +got):\n%s", diff)
			}
			if strings.Contains(strings.ReplaceAll(string(gotCRLF), "\r\n", ""), "\n") {
				t.Errorf("the CRLF file got a bare LF")
			}
		})
	}
}

func TestFixTimeoutOptions(t *testing.T) {
	src := []byte("on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n")
	tests := []struct {
		what string
		cfg  string
		want string
	}{
		{"default-minutes", "  missing-timeout:\n    default-minutes: 12\n", "    timeout-minutes: 12\n"},
		{"lowered to the maximum", "  missing-timeout:\n    default-minutes: 45\n  timeout-too-long:\n    max: 20\n", "    timeout-minutes: 20\n"},
		{"the maximum is not a limit for a smaller default", "  missing-timeout:\n    default-minutes: 5\n  timeout-too-long:\n    max: 20\n", "    timeout-minutes: 5\n"},
		{"a fractional maximum is rounded down", "  missing-timeout:\n    default-minutes: 45\n  timeout-too-long:\n    max: 20.5\n", "    timeout-minutes: 20\n"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			cfg := mustParseConfig(t, "rules:\n"+tc.cfg+"  unused-ignore: error\n")
			out, n, left := fixWith(t, src, cfg, FixModeSafe)
			if n != 1 || len(left) != 0 {
				t.Fatalf("%d fixes, left %v", n, left)
			}
			want := strings.Replace(string(src), "    steps:", tc.want+"    steps:", 1)
			if diff := cmp.Diff(want, string(out)); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
			// The result does not trigger timeout-too-long
			l, _ := NewLinter(io.Discard, &LinterOptions{})
			l.defaultConfig = cfg
			errs, err := l.Lint("test.yaml", out, nil)
			if err != nil || len(errs) != 0 {
				t.Errorf("the fixed workflow must be clean: %v, %v", errs, err)
			}
		})
	}

	for _, bad := range []string{"-1", "1.5", "abc"} {
		if _, err := ParseConfig([]byte("rules:\n  missing-timeout:\n    default-minutes: " + bad + "\n")); err == nil {
			t.Errorf("default-minutes: %s must be rejected", bad)
		}
	}
}

func TestNoFixForMissingTimeoutWithoutDefaultMinutes(t *testing.T) {
	src := []byte("on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n")
	for _, rules := range []string{"", "  timeout-too-long:\n    max: 20\n"} {
		cfg := mustParseConfig(t, "rules:\n"+rules+"  missing-timeout: error\n  unused-ignore: error\n")
		out, n, left := fixWith(t, src, cfg, FixModeUnsafe)
		if n != 0 || string(out) != string(src) {
			t.Errorf("%q: the file must not be changed without default-minutes: %d fixes\n%s", rules, n, out)
		}
		if len(left) != 1 || left[0].ID != "missing-timeout" || left[0].Fix != nil {
			t.Errorf("%q: want one missing-timeout finding without a fix, got %v", rules, left)
		}
		if !strings.Contains(left[0].Message, "timeout-minutes") {
			t.Errorf("%q: the message must tell to set timeout-minutes: %q", rules, left[0].Message)
		}
	}
}

func TestFixOnlyWhenTheRuleIsEnabled(t *testing.T) {
	src := []byte("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n")
	cfg := mustParseConfig(t, "rules:\n  missing-timeout: off\n")
	out, n, _ := fixWith(t, src, cfg, FixModeUnsafe)
	if n != 0 || string(out) != string(src) {
		t.Errorf("no rule is enabled but %d fixes were applied:\n%s", n, out)
	}
}

func TestFixerErrorsCarryTheFix(t *testing.T) {
	src := []byte("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: gh pr list\n")
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixerConfig(t, "")
	errs, err := l.Lint("test.yaml", src, nil)
	if err != nil {
		t.Fatal(err)
	}
	var timeouts, perms []*Error
	for _, e := range errs {
		switch e.ID {
		case "missing-timeout":
			timeouts = append(timeouts, e)
		case "missing-permissions":
			perms = append(perms, e)
		}
	}
	if len(timeouts) != 2 || len(perms) != 2 {
		t.Fatalf("want 2 findings of each rule: %v", errs)
	}
	for _, e := range timeouts {
		if e.Fix == nil || e.Fix.Unsafe || len(e.Fix.Edits) != 1 {
			t.Errorf("missing-timeout fix: %+v", e.Fix)
		}
	}
	// Every job of the workflow reports the same fix, so applying one applies all
	if perms[0].Fix == nil || perms[1].Fix == nil || !perms[0].Fix.Unsafe || cmp.Diff(perms[0].Fix, perms[1].Fix) != "" {
		t.Errorf("missing-permissions fixes: %+v %+v", perms[0].Fix, perms[1].Fix)
	}
	if perms[0].Fix.Description != "Add permissions: contents: read" {
		t.Errorf("description: %q", perms[0].Fix.Description)
	}
}

func TestFixNoTrailingNewline(t *testing.T) {
	src := []byte("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest")
	out, n, _ := fixWith(t, src, fixerConfig(t, ""), FixModeUnsafe)
	want := "on: push\npermissions:\n  contents: read\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 30"
	if diff := cmp.Diff(want, string(out)); n != 2 || diff != "" {
		t.Errorf("%d fixes (-want +got):\n%s", n, diff)
	}
}

func TestFixUnusedIgnoreKeepsTheUsedPatterns(t *testing.T) {
	src := "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n" +
		"      # jactionlint ignore=template-injection, no-match\n" +
		"      - run: echo ${{ github.event.issue.title }}\n"
	out, n, left := fixWith(t, []byte(src), fixerConfig(t, ""), FixModeSafe)
	want := strings.Replace(src, "template-injection, no-match", "template-injection", 1)
	if diff := cmp.Diff(want, string(out)); n != 1 || len(left) != 0 || diff != "" {
		t.Errorf("%d fixes, left %v (-want +got):\n%s", n, left, diff)
	}
}

// FuzzFixers checks that the fixers never break a workflow whatever it looks like: the result of a
// valid YAML file is valid YAML, and fixing it again changes nothing.
func FuzzFixers(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "fix", "*.yaml"))
	for _, file := range files {
		if strings.HasSuffix(file, ".fixed.yaml") || strings.HasSuffix(file, ".unsafe.yaml") {
			continue
		}
		if b, err := os.ReadFile(file); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte("on: push\njobs:\n  a:\n    runs-on: x\n    steps: []\n"))
	f.Add([]byte("on: push\r\njobs:\r\n  a: {runs-on: x}\r\n"))
	f.Add([]byte("jobs:\n a:\n  runs-on: |\n   x\n # c\n"))
	cfg := mustParseConfig(f, "rules:\n  missing-permissions: error\n  missing-timeout:\n    default-minutes: 30\n  unused-ignore: error\n")
	f.Fuzz(func(t *testing.T, src []byte) {
		var in any
		valid := yaml.Unmarshal(src, &in) == nil
		out, _, _ := fixWith(t, src, cfg, FixModeUnsafe)
		if valid {
			var o any
			if err := yaml.Unmarshal(out, &o); err != nil {
				t.Fatalf("the fix broke the YAML: %v\n%q\n->\n%q", err, src, out)
			}
		}
		again, n, _ := fixWith(t, out, cfg, FixModeUnsafe)
		if n != 0 || string(again) != string(out) {
			t.Fatalf("fixing again changed the result (%d fixes):\n%q\n->\n%q", n, out, again)
		}
	})
}
