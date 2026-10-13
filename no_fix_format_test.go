package jactionlint

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var updateNoFixGoldens = flag.Bool("update-nofix-goldens", false, "rewrite the expected files of testdata/nofix")

// noFixGoldenConfig turns on the rules whose findings have no applicable fix for a different reason each: a
// missing option (missing-timeout), a lookup that is off (unpinned-uses), an unsafe fix (insecure-commands),
// and one that needs a person (template-injection), next to findings with a safe fix and with none by design.
func noFixGoldenConfig(t *testing.T) *Config {
	return mustParseConfig(t, "rules:\n  missing-timeout: error\n  unpinned-uses: error\n  insecure-commands: error\n  template-injection: error\n")
}

func lintNoFixGolden(t *testing.T, format string) string {
	t.Helper()
	var b bytes.Buffer
	l, err := NewLinter(&b, &LinterOptions{Format: format, Config: noFixGoldenConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.LintFile(filepath.Join("testdata", "nofix", "w.yaml"), &Project{root: filepath.Join("testdata", "nofix")}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if runtime.GOOS == "windows" {
		out = strings.ReplaceAll(out, `testdata\\nofix\\`, "testdata/nofix/")
	}
	return out
}

// TestNoFixGoldens pins the JSON and the SARIF log of findings that have no applicable fix. Regenerate with
// go test -run TestNoFixGoldens -update-nofix-goldens . and review the diff.
func TestNoFixGoldens(t *testing.T) {
	for format, file := range map[string]string{FormatJSON: "w.json", FormatSARIF: "w.sarif"} {
		have := lintNoFixGolden(t, format)
		path := filepath.Join("testdata", "nofix", file)
		if *updateNoFixGoldens {
			if err := os.WriteFile(path, []byte(have), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(string(want), have); diff != "" {
			t.Errorf("%s (-want +got): %s", format, diff)
		}
	}
}

// TestNoFixSARIFContract checks what hk's sarif-diff and code scanning rely on: a result which has no fix to
// offer has no "fixes" entry at all, and says why in its properties; a finding which is advice only has no
// reason; and the names of the existing properties are unchanged.
func TestNoFixSARIFContract(t *testing.T) {
	run := sarifRunOf(t, lintNoFixGolden(t, FormatSARIF))
	want := map[string]string{
		"missing-timeout":    "option-required",
		"unpinned-uses":      "online-required",
		"insecure-commands":  "unsafe-edit", // the fix exists but SARIF never offers an unsafe one
		"template-injection": "needs-judgment",
	}
	seen := map[string]bool{}
	for _, r := range run["results"].([]any) {
		res := r.(sarifDoc)
		id := res["ruleId"].(string)
		props, _ := res["properties"].(sarifDoc)
		code, _ := props["noFixCode"].(string)
		if code != "" {
			if _, has := res["fixes"]; has {
				t.Errorf("%s has both fixes and noFixCode", id)
			}
			if props["noFixReason"] == "" {
				t.Errorf("%s has a code and no reason", id)
			}
		}
		if w, ok := want[id]; ok && code == w {
			seen[id] = true
		} else if !ok && code != "" {
			t.Errorf("%s: unexpected noFixCode %q", id, code)
		}
		if _, has := res["fixes"]; !has && id == "missing-permissions" {
			t.Errorf("a finding with a safe fix lost its fixes")
		}
		if id == "concurrency-limits" && code != "" {
			t.Errorf("an advice-only finding must have no reason: %v", props)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("%s: noFixCode %q not found", id, want[id])
		}
	}
}

// An unsafe fix is a fix in the JSON (marked unsafe, with its edits) and has no no_fix: only the SARIF log,
// which cannot carry an unsafe fix, needs the reason.
func TestNoFixJSONUnsafeFixStaysAFix(t *testing.T) {
	var have []map[string]any
	if err := json.Unmarshal([]byte(lintNoFixGolden(t, FormatJSON)), &have); err != nil {
		t.Fatal(err)
	}
	for _, e := range have {
		if e["id"] != "insecure-commands" {
			continue
		}
		fix, _ := e["fix"].(map[string]any)
		if fix == nil || fix["unsafe"] != true || e["no_fix"] != nil {
			t.Errorf("insecure-commands: %v", e)
		}
		return
	}
	t.Error("no insecure-commands finding")
}
