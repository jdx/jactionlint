package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func docURLWorkflow(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	wd := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "on: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	f := filepath.Join(wd, "ci.yaml")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func runDocURLs(t *testing.T, args ...string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	cmd.Main(append([]string{"jactionlint", "--no-color", "--profile", "default", "--shellcheck=", "--pyflakes="}, args...))
	return out.String()
}

func TestShowDocURLs(t *testing.T) {
	f := docURLWorkflow(t)
	const url = "https://jactionlint.jdx.dev/rules#missing-timeout"

	t.Run("off by default", func(t *testing.T) {
		for _, format := range []string{"text", "github", "oneline", "gcc"} {
			if out := runDocURLs(t, "--format", format, f); strings.Contains(out, "jactionlint.jdx.dev") {
				t.Errorf("%s: the URL is printed without --show-doc-urls:\n%s", format, out)
			}
		}
	})

	t.Run("text: under the snippet of the finding", func(t *testing.T) {
		out := runDocURLs(t, "--show-doc-urls", f)
		want := "  = docs: " + url + "\n"
		if !strings.Contains(out, want) {
			t.Fatalf("the output does not contain %q:\n%s", want, out)
		}
		// Each finding has its own, after its snippet
		findings := regexp.MustCompile(`(?m)^\S*ci\.yaml:\d+:\d+: `).FindAllString(out, -1)
		if len(findings) == 0 || strings.Count(out, "  = docs: ") != len(findings) {
			t.Errorf("%d findings and %d URLs:\n%s", len(findings), strings.Count(out, "  = docs: "), out)
		}
		if !regexp.MustCompile(`\^~+\n  = docs: `).MatchString(out) {
			t.Errorf("the URL does not follow the snippet:\n%s", out)
		}
	})

	t.Run("text: not in the one line format", func(t *testing.T) {
		if out := runDocURLs(t, "--show-doc-urls", "--format", "oneline", f); strings.Contains(out, "docs:") || strings.Count(out, "\n") != strings.Count(out, "ci.yaml:") {
			t.Errorf("a finding must stay one line:\n%s", out)
		}
	})

	t.Run("github: in the message", func(t *testing.T) {
		out := runDocURLs(t, "--show-doc-urls", "--format", "github", f)
		if !strings.Contains(out, "%0Adocs: "+url+"\n") {
			t.Errorf("the message has no URL:\n%s", out)
		}
		if strings.Count(out, "\n") != strings.Count(out, "::error ")+strings.Count(out, "::warning ")+strings.Count(out, "::notice ") {
			t.Errorf("an annotation must stay one line:\n%s", out)
		}
	})

	t.Run("formats which have the URL as data are unchanged", func(t *testing.T) {
		for _, format := range []string{"json", "sarif", "gcc"} {
			if runDocURLs(t, "--format", format, f) != runDocURLs(t, "--show-doc-urls", "--format", format, f) {
				t.Errorf("%s changes with --show-doc-urls", format)
			}
		}
	})
}
