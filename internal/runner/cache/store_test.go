package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func save(t *testing.T, s *Store, scope, key, version, body string) *Entry {
	t.Helper()
	p, err := s.Reserve(scope, key, version)
	if err != nil {
		t.Fatalf("reserve %s: %v", key, err)
	}
	if err := os.WriteFile(p.Path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e, err := s.Commit(p, int64(len(body)))
	if err != nil {
		t.Fatalf("commit %s: %v", key, err)
	}
	return e
}

func newStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	return s, &now
}

func TestLookupSemantics(t *testing.T) {
	s, now := newStore(t)
	main, feat := "refs/heads/main", "refs/heads/feature"
	save(t, s, main, "npm-linux-abc", "v1", "main-abc")
	*now = now.Add(time.Hour)
	save(t, s, main, "npm-linux-def", "v1", "main-def")
	*now = now.Add(time.Hour)
	save(t, s, feat, "npm-linux-ghi", "v1", "feat-ghi")
	save(t, s, main, "npm-linux-abc", "v2", "other version")

	for _, tc := range []struct {
		name    string
		scopes  []string
		key     string
		restore []string
		version string
		want    string // body of the entry found, "" for none
	}{
		{"exact key", []string{main}, "npm-linux-abc", nil, "v1", "main-abc"},
		{"exact key, other version", []string{main}, "npm-linux-abc", nil, "v2", "other version"},
		{"version mismatch never matches", []string{main}, "npm-linux-abc", []string{"npm-"}, "v3", ""},
		{"miss", []string{main}, "npm-linux-zzz", nil, "v1", ""},
		{"restore key prefix takes the newest", []string{main}, "npm-linux-zzz", []string{"npm-linux-"}, "v1", "main-def"},
		{"restore keys in order", []string{main}, "x", []string{"nothing-", "npm-linux-a", "npm-"}, "v1", "main-abc"},
		{"exact key beats a newer prefix match", []string{main}, "npm-linux-abc", []string{"npm-linux-d"}, "v1", "main-abc"},
		{"feature scope sees its own save", []string{feat, main}, "npm-linux-ghi", nil, "v1", "feat-ghi"},
		{"feature falls back to the default branch", []string{feat, main}, "npm-linux-abc", nil, "v1", "main-abc"},
		{"the default branch does not see a feature save", []string{main}, "npm-linux-ghi", nil, "v1", ""},
		{"a prefix prefers the current scope", []string{feat, main}, "x", []string{"npm-linux-"}, "v1", "feat-ghi"},
		{"keys are case sensitive", []string{main}, "NPM-LINUX-ABC", nil, "v1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := s.Lookup(tc.scopes, tc.key, tc.restore, tc.version)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if e != nil {
					t.Fatalf("found %s", e.Key)
				}
				return
			}
			if e == nil {
				t.Fatal("not found")
			}
			if b, _ := os.ReadFile(s.Path(e)); string(b) != tc.want {
				t.Fatalf("found %q (%s), want %q", b, e.Key, tc.want)
			}
		})
	}
}

func TestImmutableKeys(t *testing.T) {
	s, _ := newStore(t)
	save(t, s, "main", "k", "v", "first")
	if _, err := s.Reserve("main", "k", "v"); err != ErrExists {
		t.Fatalf("a saved key must be refused, got %v", err)
	}
	// the same key in another scope or version is a different cache
	if p, err := s.Reserve("feature", "k", "v"); err != nil {
		t.Fatalf("other scope: %v", err)
	} else {
		s.Abort(p)
	}
	if _, err := s.Reserve("main", "k", "v2"); err != nil {
		t.Fatalf("other version: %v", err)
	}
	// a second reservation of a key being uploaded is refused too
	if _, err := s.Reserve("main", "k", "v2"); err != ErrExists {
		t.Fatalf("a key being uploaded must be refused, got %v", err)
	}
	ro, _ := Open(t.TempDir())
	ro.ReadOnly = true
	if _, err := ro.Reserve("main", "x", "v"); err != ErrReadOnly {
		t.Fatalf("read-only: %v", err)
	}
}

func TestCommitChecksSize(t *testing.T) {
	s, _ := newStore(t)
	p, _ := s.Reserve("main", "k", "v")
	os.WriteFile(p.Path, []byte("12345"), 0o600)
	if _, err := s.Commit(p, 99); err == nil {
		t.Fatal("a wrong size must fail")
	}
	if _, err := s.Commit(p, 5); err != nil {
		t.Fatal(err)
	}
	p2, _ := s.Reserve("main", "empty", "v")
	if _, err := s.Commit(p2, 0); err == nil {
		t.Fatal("nothing uploaded must fail")
	}
}

func TestExpiryAndEviction(t *testing.T) {
	s, now := newStore(t)
	s.TTL = 7 * 24 * time.Hour
	save(t, s, "main", "old", "v", "o")
	*now = now.Add(6 * 24 * time.Hour)
	if e, _ := s.Lookup([]string{"main"}, "old", nil, "v"); e == nil {
		t.Fatal("an entry unread for 6 days must still be there")
	}
	// the lookup refreshed it
	*now = now.Add(6 * 24 * time.Hour)
	if e, _ := s.Lookup([]string{"main"}, "old", nil, "v"); e == nil {
		t.Fatal("a read refreshes the expiry")
	}
	*now = now.Add(8 * 24 * time.Hour)
	if e, _ := s.Lookup([]string{"main"}, "old", nil, "v"); e != nil {
		t.Fatal("an entry unread for 8 days must be gone")
	}
	// an expired key can be saved again
	save(t, s, "main", "old", "v", "new")

	// size cap: the least recently used entries go first
	s.MaxBytes = 10
	save(t, s, "main", "a", "v", "aaaa")
	*now = now.Add(time.Minute)
	save(t, s, "main", "b", "v", "bbbb")
	*now = now.Add(time.Minute)
	s.Lookup([]string{"main"}, "a", nil, "v") // a is now newer than b
	*now = now.Add(time.Minute)
	save(t, s, "main", "c", "v", "cccc")
	keys := map[string]bool{}
	es, _ := s.List()
	for _, e := range es {
		keys[e.Key] = true
	}
	if keys["b"] || !keys["a"] || !keys["c"] {
		t.Fatalf("eviction kept %v, want a and c", keys)
	}
	if n, _ := s.TotalSize(); n > 10 {
		t.Fatalf("size %d over the cap", n)
	}
	files, _ := os.ReadDir(filepath.Join(s.Dir(), "entries"))
	if len(files) != len(es) {
		t.Errorf("%d archives for %d entries", len(files), len(es))
	}
}

func TestRemoveAndPrune(t *testing.T) {
	s, _ := newStore(t)
	save(t, s, "main", "go-1", "v", "x")
	save(t, s, "main", "go-2", "v", "x")
	save(t, s, "main", "rust-1", "v", "x")
	if n, _ := s.Remove("go-*"); n != 2 {
		t.Fatalf("removed %d", n)
	}
	if n, _ := s.Remove("rust-1"); n != 1 {
		t.Fatalf("removed %d", n)
	}
	if n, _ := s.Remove("nope"); n != 0 {
		t.Fatal(n)
	}
	// a stray archive is removed by prune
	stray := filepath.Join(s.Dir(), "entries", "stray.bin")
	os.WriteFile(stray, []byte("x"), 0o600)
	if _, err := s.Prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stray); err == nil {
		t.Fatal("prune left a stray archive")
	}
}

func TestStoreSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	save(t, s, "main", "k", "v", "body")
	s2, _ := Open(dir)
	e, err := s2.Lookup([]string{"main"}, "k", nil, "v")
	if err != nil || e == nil {
		t.Fatalf("%v %v", e, err)
	}
	if b, _ := os.ReadFile(s2.Path(e)); string(b) != "body" {
		t.Fatal(string(b))
	}
	os.WriteFile(filepath.Join(dir, "index.json"), []byte("{broken"), 0o600)
	if _, err := s2.Lookup([]string{"main"}, "k", nil, "v"); err == nil {
		t.Fatal("a damaged index must be reported")
	}
}
