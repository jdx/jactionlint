//go:build !js

package jactionlint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// defaultGitHubCacheMaxBytes is the size the on-disk cache of GitHub answers may grow to.
	defaultGitHubCacheMaxBytes = 32 << 20
	// maxCachedBodyBytes is the largest answer which is cached. Bigger ones are used and forgotten.
	maxCachedBodyBytes = 4 << 20
	// cacheFileExt is the extension of cache files. Other files in the directory are never touched.
	cacheFileExt = ".json"
	// staleTempAge is how old a leftover temporary file must be before it is removed.
	staleTempAge = time.Hour
)

// cacheEntry is one cached answer of the GitHub API.
type cacheEntry struct {
	URL     string    `json:"url"`
	ETag    string    `json:"etag,omitempty"`
	Fetched time.Time `json:"fetched"`
	Status  int       `json:"status"`
	Link    string    `json:"link,omitempty"`
	Body    []byte    `json:"body,omitempty"`
}

// diskCache stores answers of the GitHub API in a directory, one file per URL, for the ETag
// revalidation and the time-to-live of githubHTTPClient.
//
// Many processes can use the same directory at once (hk runs one jactionlint per batch of files):
// a file is written under a temporary name and renamed into place, so a reader sees a whole file
// or none, and a file which cannot be decoded is just a miss. The size of the directory is
// bounded: when it exceeds the limit the least recently written files are removed.
type diskCache struct {
	dir      string
	maxBytes int64

	mu      sync.Mutex
	size    int64
	scanned bool
}

// newDiskCache returns a cache in the directory. The directory is created on the first write.
func newDiskCache(dir string, maxBytes int64) *diskCache {
	if maxBytes <= 0 {
		maxBytes = defaultGitHubCacheMaxBytes
	}
	return &diskCache{dir: dir, maxBytes: maxBytes}
}

// defaultGitHubCacheDir returns $XDG_CACHE_HOME/jactionlint ($HOME/.cache/jactionlint when unset). It
// is empty when no cache directory can be determined.
func defaultGitHubCacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" && filepath.IsAbs(d) {
		return filepath.Join(d, "jactionlint")
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".cache", "jactionlint")
	}
	return ""
}

func (c *diskCache) path(key string) string {
	return filepath.Join(c.dir, key+cacheFileExt)
}

// cacheKey derives the file name for a request. The scope tells apart answers fetched with different
// credentials, which can differ for private repositories.
func cacheKey(scope, url string) string {
	h := sha256.Sum256([]byte(scope + "\x00" + url))
	return hex.EncodeToString(h[:16])
}

// get returns the entry or nil when there is none, or it is unreadable or belongs to another URL.
func (c *diskCache) get(key, url string) *cacheEntry {
	if c == nil {
		return nil
	}
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil
	}
	var e cacheEntry
	if json.Unmarshal(b, &e) != nil || e.URL != url || e.Status == 0 {
		return nil
	}
	return &e
}

// put writes the entry. Failures are ignored: the cache is an optimization.
func (c *diskCache) put(key string, e *cacheEntry) {
	if c == nil || len(e.Body) > maxCachedBodyBytes {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.dir, ".tmp-*")
	if err != nil {
		return
	}
	name := tmp.Name()
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(name)
		return
	}
	if err := os.Rename(name, c.path(key)); err != nil {
		os.Remove(name)
		return
	}
	c.grew(int64(len(b)))
}

// grew accounts for written bytes and prunes the directory when it became too big. The first call
// measures what is already there.
func (c *diskCache) grew(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.scanned {
		c.scanned = true
		c.size = c.pruneLocked(true) // Counts the files, including the one just written
		return
	}
	c.size += n
	if c.size > c.maxBytes {
		c.size = c.pruneLocked(true)
	}
}

type cacheFile struct {
	path string
	size int64
	mod  time.Time
}

// pruneLocked removes the oldest cache files until the directory is down to three quarters of the
// limit, and returns the size that remains. It also removes abandoned temporary files.
func (c *diskCache) pruneLocked(enforce bool) int64 {
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return 0
	}
	var files []cacheFile
	var total int64
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(c.dir, e.Name())
		switch {
		case strings.HasPrefix(e.Name(), ".tmp-"):
			if time.Since(info.ModTime()) > staleTempAge {
				os.Remove(p)
			}
		case strings.HasSuffix(e.Name(), cacheFileExt):
			files = append(files, cacheFile{p, info.Size(), info.ModTime()})
			total += info.Size()
		}
	}
	if !enforce || total <= c.maxBytes {
		return total
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	target := c.maxBytes / 4 * 3
	for _, f := range files {
		if total <= target {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
	return total
}
