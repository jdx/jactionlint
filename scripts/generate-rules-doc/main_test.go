package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jdx/jactionlint/v2"
)

// docs/rules.md must be regenerated whenever the registry changes: go run ./scripts/generate-rules-doc
func TestRulesDocIsUpToDate(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "rules.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(string(b), "\r\n", "\n")

	var got strings.Builder
	if err := generate(&got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got.String()); diff != "" {
		t.Fatalf("docs/rules.md is outdated. run \"go run ./scripts/generate-rules-doc\" (-want +got):\n%s", diff)
	}
}

func TestRulesDocMentionsEveryRule(t *testing.T) {
	var out strings.Builder
	if err := run([]string{"-"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, r := range jactionlint.Rules() {
		if !strings.Contains(out.String(), "\n## "+r.ID+"\n") {
			t.Errorf("rule %q has no section", r.ID)
		}
	}
	if err := run([]string{"a", "b"}, &out); err == nil {
		t.Error("too many arguments must be an error")
	}
}

func TestRunWritesFile(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "rules.md")
	if err := run([]string{dst}, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "<!-- Generated") {
		t.Errorf("unexpected content: %s", b[:40])
	}
	if err := run([]string{filepath.Join(t.TempDir(), "no", "such", "dir", "x.md")}, nil); err == nil {
		t.Error("writing to a missing directory must be an error")
	}
}
