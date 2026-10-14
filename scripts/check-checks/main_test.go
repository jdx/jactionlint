package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v4"
)

// Do not mess up the test output. Temporarily commenting out this function may be helpful when debugging some test cases.
func init() {
	log.SetOutput(io.Discard)
	stderr = io.Discard
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func testErr(t *testing.T, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("error did not occur")
	}
	msg := err.Error()
	for _, w := range want {
		if !strings.Contains(msg, w) {
			t.Errorf("error message %q does not contain expected text %q", msg, w)
		}
	}
}

func TestMainGenerateOK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-checks doesn't support Windows")
	}
	root := t.TempDir()

	in := must(os.Open(filepath.FromSlash("testdata/ok/minimal.in")))
	path := filepath.FromSlash(root + "/minimal.in")
	tmp := must(os.Create(path))
	must(io.Copy(tmp, in))
	in.Close()
	tmp.Close()

	if err := Main([]string{"exe", "--fix", path}); err != nil {
		t.Fatal(err)
	}

	want := must(os.ReadFile(filepath.FromSlash("testdata/ok/minimal.out")))
	have := must(os.ReadFile(path))
	if diff := cmp.Diff(want, have); diff != "" {
		t.Fatal(diff)
	}
}

func TestMainCheckOK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-checks doesn't support Windows")
	}
	path := filepath.FromSlash("testdata/ok/minimal.out")
	if err := Main([]string{"exe", path}); err != nil {
		t.Fatal(err)
	}
}

func TestMainCheckQuietOK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-checks doesn't support Windows")
	}
	path := filepath.FromSlash("testdata/ok/minimal.out")
	if err := Main([]string{"exe", "-quiet", path}); err != nil {
		t.Fatal(err)
	}
}

func TestMainPrintHelp(t *testing.T) {
	if err := Main([]string{"exe", "--help"}); err != nil {
		t.Fatal(err)
	}
}

func TestMainCheckError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-checks doesn't support Windows")
	}
	path := filepath.FromSlash("testdata/ok/minimal.in")
	testErr(t, Main([]string{"exe", path}), "checks document has some update")
}

func TestMainFileNotFound(t *testing.T) {
	testErr(t, Main([]string{"exe", "this-file-does-not-exist.md"}), "could not read the document file")
}

func TestMainTooManyArgs(t *testing.T) {
	testErr(t, Main([]string{"exe", "a", "b", "c"}), "this command should take exact one file path but got")
}

func TestMainInvalidCheckFlag(t *testing.T) {
	testErr(t, Main([]string{"exe", "-c", "foo.md"}), "flag provided but not defined")
}

func TestMainUpdateError(t *testing.T) {
	path := filepath.FromSlash("testdata/err/no_playground_link.md")
	if err := Main([]string{"exe", path}); err == nil {
		t.Fatal("no error occurred")
	}
}

func TestUpdateOK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-checks doesn't support Windows")
	}

	dir := filepath.FromSlash("testdata/ok")

	tests := []string{}
	for _, e := range must(os.ReadDir(dir)) {
		n := e.Name()
		if !strings.HasSuffix(n, ".in") {
			continue
		}
		tests = append(tests, strings.TrimSuffix(n, filepath.Ext(n)))
	}

	for _, tc := range tests {
		in := filepath.Join(dir, tc+".in")
		out := filepath.Join(dir, tc+".out")
		t.Run(tc, func(t *testing.T) {
			have, err := Update(must(os.ReadFile(in)))
			if err != nil {
				t.Fatal(err)
			}
			want := must(os.ReadFile(out))
			if diff := cmp.Diff(want, have); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestUpdateError(t *testing.T) {
	dir := filepath.FromSlash("testdata/err")

	tests := []string{}
	for _, e := range must(os.ReadDir(dir)) {
		n := e.Name()
		if !strings.HasSuffix(n, ".md") {
			continue
		}
		tests = append(tests, strings.TrimSuffix(n, filepath.Ext(n)))
	}

	for _, tc := range tests {
		path := filepath.Join(dir, tc+".md")
		t.Run(tc, func(t *testing.T) {
			if _, err := Update(must(os.ReadFile(path))); err == nil {
				t.Fatal("no error occurred")
			}
		})
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state state
		want  string
	}{
		{stateInit, "init"},
		{stateAnchor, "anchor"},
		{stateHeading, "heading"},
		{stateInputHeader, "input header"},
		{stateInputBlock, "input block"},
		{stateAfterInput, "after input"},
		{stateOutputHeader, "output header"},
		{stateAfterOutput, "after output"},
		{stateOutputBlock, "output block"},
		{stateEnd, "end"},
	}

	for _, tc := range tests {
		have := tc.state.String()
		if have != tc.want {
			t.Errorf("wanted %q for state %d but have %q", tc.want, tc.state, have)
		}
	}
}

func TestMergeConfig(t *testing.T) {
	base := "rules:\n  a: off\n  b:\n    pedantic: true\nprofile: default\n"
	b, err := mergeConfig(base, "rules:\n  a: warn\n  c: error\nignores: []\n")
	if err != nil {
		t.Fatal(err)
	}
	var have map[string]any
	if err := yaml.Unmarshal(b, &have); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"rules": map[string]any{
			"a": "warn",
			"b": map[string]any{"pedantic": true},
			"c": "error",
		},
		"profile": "default",
		"ignores": []any{},
	}
	if diff := cmp.Diff(want, have); diff != "" {
		t.Fatal(diff)
	}

	b, err = mergeConfig(base, "  \n")
	if err != nil || string(b) != base {
		t.Fatalf("an empty config must leave the base alone: %q, %v", b, err)
	}
	if _, err := mergeConfig(base, "rules: ["); err == nil {
		t.Fatal("an invalid config did not fail")
	}
}

func TestParseMarkers(t *testing.T) {
	m, err := parseMarkers([]byte("# requires --online\n# select: a, b,c\n# profile: pedantic\n# file: .github/workflows/ci.yml\non: push\n# select: ignored\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := markers{online: true, select_: []string{"a", "b", "c"}, profile: "pedantic", file: ".github/workflows/ci.yml"}
	if diff := cmp.Diff(want, m, cmp.AllowUnexported(markers{})); diff != "" {
		t.Fatal(diff)
	}

	// A comment of the example is not a marker, and ends the head
	m, err = parseMarkers([]byte("# ERROR: oops\n# select: a\n"))
	if err != nil || len(m.select_) != 0 {
		t.Fatalf("a comment of the example must end the head: %+v, %v", m, err)
	}

	for _, src := range []string{"# select:\n", "# profile:\n", "# file: ../x\n", "# file:\n"} {
		if _, err := parseMarkers([]byte(src)); err == nil {
			t.Errorf("%q did not fail", src)
		}
	}
}

// The checks page has no hand-written output: nothing verifies one, and twenty of them had gone stale. An example that
// needs another configuration, rules or files says so with the markers and the blocks that the README describes.
func TestChecksPageSkipsNoOutput(t *testing.T) {
	b, err := os.ReadFile(filepath.FromSlash("../../docs/checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if l == "<!-- Skip update output -->" {
			t.Errorf("docs/checks.md:%d: the output is written by hand, so nothing checks it. Use `# select:`, `Example config:` and `Example file:` instead (scripts/check-checks/README.md)", i+1)
		}
	}
}
