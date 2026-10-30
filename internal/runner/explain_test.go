package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEveryExplainRefHasAnExplanation(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`explainRef\("([a-z-]+)"\)`)
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			found++
			if reasonByID(m[1]) == nil {
				t.Errorf("%s points at the explanation %q, which does not exist", f, m[1])
			}
		}
	}
	if found < 8 {
		t.Errorf("only %d messages point at an explanation", found)
	}
	for _, r := range reasons {
		if r.Title == "" || len(r.Text) < 40 || !regexp.MustCompile(`^[a-z-]+$`).MatchString(r.ID) {
			t.Errorf("reason %q is incomplete", r.ID)
		}
	}
}

func TestExplainCommand(t *testing.T) {
	var out bytes.Buffer
	if code := explain(&out, ""); code != ExitOK || !strings.Contains(out.String(), "sandbox-denied") || !strings.Contains(out.String(), "unpinned-action") {
		t.Fatalf("list: %d %s", code, out.String())
	}
	out.Reset()
	if code := explain(&out, "UNPINNED-ACTION"); code != ExitOK || !strings.Contains(out.String(), "--trust-actions") {
		t.Fatalf("one: %d %s", code, out.String())
	}
	out.Reset()
	if code := explain(&out, "nope"); code != ExitUsage {
		t.Fatalf("unknown: %d", code)
	}
	dir := repoWith(t, nil)
	var o, e bytes.Buffer
	if code := Main([]string{"-C", dir, "--explain", "sandbox-denied"}, strings.NewReader(""), &o, &e); code != ExitOK || !strings.Contains(o.String(), "--allow-write") {
		t.Fatalf("flag: %d %s %s", code, o.String(), e.String())
	}
	o.Reset()
	if code := Main([]string{"--version"}, strings.NewReader(""), &o, &e); code != ExitOK || !strings.Contains(o.String(), "runner behavior") {
		t.Fatalf("version: %d %s", code, o.String())
	}
}
