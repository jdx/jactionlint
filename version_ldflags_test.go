package jactionlint

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A stale "-X <module path>.version" in the release build settings silently blanks the output of
// "jactionlint --version", so check that they point at the real module path of go.mod.
func TestLdflagsUseModulePath(t *testing.T) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`).FindSubmatch(mod)
	if m == nil {
		t.Fatal("no module directive in go.mod")
	}
	path := string(m[1])
	if !strings.HasSuffix(path, "/v2") {
		t.Fatalf("module path %q must end with /v2", path)
	}

	for _, f := range []string{".goreleaser.yaml", "Dockerfile"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if want := "-X " + path + ".version="; !strings.Contains(src, want) {
			t.Errorf("%s must contain %q", f, want)
		}
		if strings.Contains(src, "-X github.com/jdx/jactionlint.") || strings.Contains(src, `-X "github.com/jdx/jactionlint.`) {
			t.Errorf("%s still sets variables of the v1 module path", f)
		}
	}
}
