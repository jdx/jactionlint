// Package cache is the local actions cache: a store of archives with the matching rules of GitHub's cache, and a
// service that speaks the protocols of @actions/cache so that actions which cache work unmodified.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Defaults of GitHub's cache.
const (
	DefaultMaxBytes = 10 << 30
	DefaultTTL      = 7 * 24 * time.Hour
)

// ErrExists is returned when a key is already saved: keys are immutable.
var ErrExists = errors.New("a cache with this key already exists")

// ErrReadOnly is returned by writes to a store that was opened read-only.
var ErrReadOnly = errors.New("the cache is read-only")

// Entry is a saved cache.
type Entry struct {
	ID       int64     `json:"id"`
	Key      string    `json:"key"`
	Version  string    `json:"version"`
	Scope    string    `json:"scope"`
	Size     int64     `json:"size"`
	File     string    `json:"file"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
}

type index struct {
	NextID  int64    `json:"next_id"`
	Entries []*Entry `json:"entries"`
}

// Store keeps the entries of one repository under a directory. It is safe for use from several goroutines and, through a
// lock file, from several processes.
type Store struct {
	dir      string
	MaxBytes int64
	TTL      time.Duration
	ReadOnly bool
	Now      func() time.Time

	mu      sync.Mutex
	pending map[int64]*Pending
}

// Open opens (and creates) the store in dir.
func Open(dir string) (*Store, error) {
	for _, d := range []string{dir, filepath.Join(dir, "entries"), filepath.Join(dir, "pending")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{dir: dir, MaxBytes: DefaultMaxBytes, TTL: DefaultTTL, Now: time.Now, pending: map[int64]*Pending{}}, nil
}

// Dir returns the directory of the store.
func (s *Store) Dir() string { return s.dir }

func (s *Store) indexPath() string { return filepath.Join(s.dir, "index.json") }

// withIndex runs f with the index loaded under the process lock, and saves it when f returns true.
func (s *Store) withIndex(f func(ix *index) (save bool, err error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := LockDir(s.dir)
	if err != nil {
		return err
	}
	defer unlock()
	ix := &index{NextID: 1}
	if b, err := os.ReadFile(s.indexPath()); err == nil {
		if err := json.Unmarshal(b, ix); err != nil {
			return fmt.Errorf("the cache index %s is damaged (remove it to start over): %w", s.indexPath(), err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	save, err := f(ix)
	if err != nil {
		return err
	}
	if !save {
		return nil
	}
	b, err := json.MarshalIndent(ix, "", " ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".index-")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.indexPath())
}

func (s *Store) expired(e *Entry) bool { return s.TTL > 0 && s.Now().Sub(e.LastUsed) > s.TTL }

// Lookup finds the entry to restore. scopes are in order of preference (the scope of the current ref, then the default
// branch). The exact key wins; then each restore key is a prefix, in order, and within a prefix the newest entry. Only
// entries with the same version match. The last-used time of the entry is refreshed.
func (s *Store) Lookup(scopes []string, key string, restoreKeys []string, version string) (*Entry, error) {
	var found *Entry
	err := s.withIndex(func(ix *index) (bool, error) {
		valid := func(e *Entry) bool { return e.Version == version && !s.expired(e) }
		for _, sc := range scopes {
			for _, e := range ix.Entries {
				if e.Scope == sc && e.Key == key && valid(e) {
					found = e
					break
				}
			}
			if found != nil {
				break
			}
		}
		if found == nil {
		prefixes:
			for _, p := range restoreKeys {
				for _, sc := range scopes {
					var best *Entry
					for _, e := range ix.Entries {
						if e.Scope == sc && strings.HasPrefix(e.Key, p) && valid(e) && (best == nil || e.Created.After(best.Created)) {
							best = e
						}
					}
					if best != nil {
						found = best
						break prefixes
					}
				}
			}
		}
		if found == nil {
			return false, nil
		}
		found.LastUsed = s.Now()
		cp := *found
		found = &cp
		return true, nil
	})
	return found, err
}

// Path returns where the archive of an entry is.
func (s *Store) Path(e *Entry) string { return filepath.Join(s.dir, "entries", e.File) }

// Pending is a cache that is being uploaded.
type Pending struct {
	ID      int64
	Key     string
	Version string
	Scope   string
	Path    string // where the service writes the upload
}

// Reserve starts saving a cache. It fails with ErrExists when the key is already saved in the scope (with any
// version: GitHub rejects the key, not the pair) or being saved by someone else.
func (s *Store) Reserve(scope, key, version string) (*Pending, error) {
	if s.ReadOnly {
		return nil, ErrReadOnly
	}
	var p *Pending
	err := s.withIndex(func(ix *index) (bool, error) {
		for _, e := range ix.Entries {
			if e.Scope == scope && e.Key == key && e.Version == version && !s.expired(e) {
				return false, ErrExists
			}
		}
		for _, q := range s.pending {
			if q.Scope == scope && q.Key == key && q.Version == version {
				return false, ErrExists
			}
		}
		id := ix.NextID
		ix.NextID++
		p = &Pending{ID: id, Key: key, Version: version, Scope: scope, Path: filepath.Join(s.dir, "pending", fmt.Sprintf("%d", id))}
		s.pending[id] = p
		return true, nil
	})
	return p, err
}

// Pending returns an upload in progress.
func (s *Store) Pending(id int64) *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[id]
}

// Abort forgets an upload.
func (s *Store) Abort(p *Pending) {
	s.mu.Lock()
	delete(s.pending, p.ID)
	s.mu.Unlock()
	os.Remove(p.Path)
	os.RemoveAll(p.Path + ".blocks")
}

// Commit finishes an upload: the file must have the size the client announced.
func (s *Store) Commit(p *Pending, size int64) (*Entry, error) {
	st, err := os.Stat(p.Path)
	if err != nil {
		return nil, fmt.Errorf("nothing was uploaded for %s", p.Key)
	}
	if size >= 0 && st.Size() != size {
		return nil, fmt.Errorf("the upload of %s has %d bytes, the client said %d", p.Key, st.Size(), size)
	}
	sum := sha256.Sum256([]byte(p.Scope + "\x00" + p.Key + "\x00" + p.Version))
	name := hex.EncodeToString(sum[:16]) + ".bin"
	var out *Entry
	err = s.withIndex(func(ix *index) (bool, error) {
		var live []*Entry
		for _, e := range ix.Entries {
			if e.Scope == p.Scope && e.Key == p.Key && e.Version == p.Version {
				if !s.expired(e) {
					return false, ErrExists
				}
				os.Remove(s.Path(e))
				continue
			}
			live = append(live, e)
		}
		ix.Entries = live
		if err := os.Rename(p.Path, filepath.Join(s.dir, "entries", name)); err != nil {
			return false, err
		}
		now := s.Now()
		out = &Entry{ID: p.ID, Key: p.Key, Version: p.Version, Scope: p.Scope, Size: st.Size(), File: name, Created: now, LastUsed: now}
		ix.Entries = append(ix.Entries, out)
		s.evict(ix)
		return true, nil
	})
	if err == nil {
		s.mu.Lock()
		delete(s.pending, p.ID)
		s.mu.Unlock()
		os.RemoveAll(p.Path + ".blocks")
	}
	return out, err
}

// evict removes expired entries, then the least recently used until the store fits MaxBytes.
func (s *Store) evict(ix *index) (removed int) {
	var keep []*Entry
	for _, e := range ix.Entries {
		if s.expired(e) {
			os.Remove(s.Path(e))
			removed++
			continue
		}
		keep = append(keep, e)
	}
	ix.Entries = keep
	if s.MaxBytes <= 0 {
		return removed
	}
	var total int64
	for _, e := range ix.Entries {
		total += e.Size
	}
	if total <= s.MaxBytes {
		return removed
	}
	sorted := append([]*Entry(nil), ix.Entries...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].LastUsed.Before(sorted[j].LastUsed) })
	drop := map[*Entry]bool{}
	for _, e := range sorted {
		if total <= s.MaxBytes {
			break
		}
		drop[e] = true
		total -= e.Size
	}
	keep = keep[:0]
	for _, e := range ix.Entries {
		if drop[e] {
			os.Remove(s.Path(e))
			removed++
			continue
		}
		keep = append(keep, e)
	}
	ix.Entries = keep
	return removed
}

// List returns the entries, newest first.
func (s *Store) List() ([]*Entry, error) {
	var out []*Entry
	err := s.withIndex(func(ix *index) (bool, error) {
		out = append(out, ix.Entries...)
		return false, nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, err
}

// Remove deletes the entries whose key matches (exactly, or by prefix when it ends in *). It returns how many.
func (s *Store) Remove(pattern string) (int, error) {
	n := 0
	err := s.withIndex(func(ix *index) (bool, error) {
		var keep []*Entry
		for _, e := range ix.Entries {
			match := e.Key == pattern
			if p, ok := strings.CutSuffix(pattern, "*"); ok {
				match = strings.HasPrefix(e.Key, p)
			}
			if match {
				os.Remove(s.Path(e))
				n++
				continue
			}
			keep = append(keep, e)
		}
		ix.Entries = keep
		return n > 0, nil
	})
	return n, err
}

// Prune removes expired entries and applies the size limit, and deletes archives that no entry refers to.
func (s *Store) Prune() (removed int, err error) {
	err = s.withIndex(func(ix *index) (bool, error) {
		removed = s.evict(ix)
		known := map[string]bool{}
		for _, e := range ix.Entries {
			known[e.File] = true
		}
		if ents, err := os.ReadDir(filepath.Join(s.dir, "entries")); err == nil {
			for _, f := range ents {
				if !known[f.Name()] {
					os.Remove(filepath.Join(s.dir, "entries", f.Name()))
					removed++
				}
			}
		}
		if ents, err := os.ReadDir(filepath.Join(s.dir, "pending")); err == nil {
			for _, f := range ents {
				if st, err := f.Info(); err == nil && s.Now().Sub(st.ModTime()) > time.Hour {
					os.RemoveAll(filepath.Join(s.dir, "pending", f.Name()))
				}
			}
		}
		return true, nil
	})
	return removed, err
}

// TotalSize is the number of bytes the entries occupy.
func (s *Store) TotalSize() (int64, error) {
	es, err := s.List()
	var n int64
	for _, e := range es {
		n += e.Size
	}
	return n, err
}
