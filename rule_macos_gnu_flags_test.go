package jactionlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestMacosGnuFlagsOKFixture runs the rule on the workflow of testdata/ok, which the golden test does not do because
// the rule is pedantic.
func TestMacosGnuFlagsOKFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "ok", "macos_gnu_flags.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range lintBatchH(t, pedanticCfg, string(b), "macos-gnu-flags") {
		t.Errorf("unexpected: %s", e)
	}
}

func TestMacosGnuFlagsIsPedantic(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    runs-on: macos-latest\n    steps:\n      - run: sed -i 's/a/b/' f\n"
	if errs := lintBatchH(t, "", src, "macos-gnu-flags"); len(errs) != 0 {
		t.Errorf("the rule is on without the pedantic profile: %v", errs)
	}
	errs := lintBatchH(t, pedanticCfg, src, "macos-gnu-flags")
	if len(errs) != 1 {
		t.Fatalf("want one finding, have %v", errs)
	}
	if errs[0].Severity != SeverityWarning {
		t.Errorf("level %v, want warning", errs[0].Severity)
	}
}

func TestIsMacOSRunnerLabel(t *testing.T) {
	for l, want := range map[string]bool{
		"macos-latest": true, "macos-15": true, "macOS-14": true, "macos-15-intel": true, "macos-26-xlarge": true,
		// Labels of runners of the owner, and other systems
		"macOS-ARM64": false, "macos": false, "macos-custom": false, "ubuntu-latest": false, "windows-latest": false, "self-hosted": false,
	} {
		if have := isMacOSRunnerLabel(l); have != want {
			t.Errorf("%q: have %v, want %v", l, have, want)
		}
	}
}

// TestMacosGnuFlagsDocsExample checks the example of docs/checks.md: the rule is pedantic, so scripts/check-checks
// cannot lint it.
func TestMacosGnuFlagsDocsExample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	info, ok := LookupRule("macos-gnu-flags")
	if !ok {
		t.Fatal("the rule is not registered")
	}
	start := strings.Index(doc, `<a id="`+info.DocsAnchor+`"></a>`)
	if start < 0 {
		t.Fatal("the section is missing")
	}
	section := doc[start+1:]
	if i := strings.Index(section, `<a id="`); i >= 0 {
		section = section[:i]
	}
	input, ok1 := docsBlock(section, "Example input:\n\n```yaml\n")
	output, ok2 := docsBlock(section, "Output:\n<!-- Skip update output -->\n```\n")
	cfgText, ok3 := docsBlock(section, "```yaml\nrules:\n")
	if !ok1 || !ok2 || !ok3 {
		t.Fatal("the section has no example input, output or rules block")
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
