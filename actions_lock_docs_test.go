package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The example of "GitHub Actions lock file" in docs/checks.md cannot be checked by scripts/check-checks, which lints one file,
// so this test lints the repository in testdata/actions_lock/docs_example and compares the output with the block of the
// document, which follows "The output of `jactionlint` for this repository:" in the section.
func TestActionsLockDocsExample(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join("testdata", "actions_lock", "docs_example")
	for _, rel := range []string{"ci.yml", "actions.lock"} {
		b, err := os.ReadFile(filepath.Join(src, ".github", "workflows", rel))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, ".github", "workflows", rel), string(b))
	}
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")

	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{WorkingDir: root, Color: ColorOptionKindNever})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig("actions-lock")
	if _, err := l.LintRepository(root); err != nil {
		t.Fatal(err)
	}

	doc, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	sec := string(doc)
	i := strings.Index(sec, "<a id=\"check-actions-lock\"></a>")
	if i < 0 {
		t.Fatal("no section for the lock file in docs/checks.md")
	}
	sec = sec[i:]
	const marker = "The output of `jactionlint` for this repository:\n\n```\n"
	i = strings.Index(sec, marker)
	if i < 0 {
		t.Fatal("the section has no example output")
	}
	sec = sec[i+len(marker):]
	end := strings.Index(sec, "\n```")
	if end < 0 {
		t.Fatal("the output block is not closed")
	}
	want := strings.TrimSpace(sec[:end])
	// Windows writes the paths with backslashes and the document may be checked out with CRLF
	got := strings.ReplaceAll(strings.ReplaceAll(out.String(), "\r\n", "\n"), `\`, "/")
	want = strings.ReplaceAll(want, "\r\n", "\n")
	if got := strings.TrimSpace(got); got != want {
		t.Errorf("the output of the example in docs/checks.md is stale.\ngot:\n%s\nwant:\n%s", got, want)
	}
}
