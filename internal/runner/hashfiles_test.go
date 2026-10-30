package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestHashFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/go.sum", "one")
	write("b/go.sum", "two")
	write("c/skip.txt", "x")
	write(".git/go.sum", "ignored")

	sum := func(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }
	want := sha256.New()
	want.Write(sum("one"))
	want.Write(sum("two"))
	got, err := hashFiles(dir, []string{"**/go.sum"})
	if err != nil || got != hex.EncodeToString(want.Sum(nil)) {
		t.Fatalf("got %q, %v", got, err)
	}
	only := sha256.New()
	only.Write(sum("two"))
	got, _ = hashFiles(dir, []string{"**/go.sum", "!a/**"})
	if got != hex.EncodeToString(only.Sum(nil)) {
		t.Fatalf("negation: %q", got)
	}
	if got, _ := hashFiles(dir, []string{"**/nothing"}); got != "" {
		t.Fatalf("no match: %q", got)
	}
}
