package jactionlint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The example of docs/checks.md has an output which is written by hand, because it needs two files. It is linted here, so
// that the output cannot go stale.
func TestConcurrencyDeadlockDocsExample(t *testing.T) {
	b, err := os.ReadFile("docs/checks.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	i := strings.Index(doc, `<a id="check-concurrency-deadlock"></a>`)
	if i < 0 {
		t.Fatal("the section is missing")
	}
	sec := doc[i:]
	if j := strings.Index(sec[10:], `<a id="`); j >= 0 {
		sec = sec[:10+j]
	}
	blocks := regexp.MustCompile("(?s)```[a-z]*\n(.*?)```").FindAllStringSubmatch(sec, -1)
	if len(blocks) != 3 {
		t.Fatalf("want three code blocks (the caller, the output and the called workflow) but got %d", len(blocks))
	}
	root := writeProject(t, map[string]string{
		".github/workflows/ci.yml":    blocks[0][1],
		".github/workflows/build.yml": blocks[2][1],
	})
	l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  concurrency-deadlock: error\n")
	errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "ci.yml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	es := errsWithID(errs, "concurrency-deadlock")
	if len(es) != 1 {
		t.Fatalf("want one finding: %v", es)
	}
	have := fmt.Sprintf("%s:%d:%d: %s [%s]", ".github/workflows/ci.yml", es[0].Line, es[0].Column, es[0].Message, es[0].Kind)
	if want := strings.TrimSpace(blocks[1][1]); have != want {
		t.Errorf("the output of the example in docs/checks.md is stale\nhave: %s\nwant: %s", have, want)
	}
}
