package jactionlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The rule is in the pedantic profile, so scripts/check-checks cannot lint the example of its section. This test lints it
// with the configuration of the section and compares the output with the block written in docs/checks.md.
func TestEventNameNeverReceivedDocsExample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	info, ok := LookupRule(eventNameID)
	if !ok {
		t.Fatal("the rule is not registered")
	}
	start := strings.Index(doc, `<a id="`+info.DocsAnchor+`"></a>`)
	if start < 0 {
		t.Fatalf("anchor %q not found", info.DocsAnchor)
	}
	section := doc[start+1:]
	if i := strings.Index(section, `<a id="`); i >= 0 {
		section = section[:i]
	}
	input, ok := docsBlock(section, "Example input:\n\n```yaml\n")
	if !ok {
		t.Fatal("the section has no example input")
	}
	output, ok := docsBlock(section, "Output:\n<!-- Skip update output -->\n\n```\n")
	if !ok {
		t.Fatal("the section has no output block")
	}
	cfgText, ok := docsBlock(section, "```yaml\nrules:\n")
	if !ok {
		t.Fatal("the section has no rules block")
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
