package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// hashFiles implements hashFiles() for the files under root: the SHA-256 of the SHA-256 digests of the files that
// the patterns match, in path order. A pattern that starts with ! removes files. No match gives "".
func hashFiles(root string, patterns []string) (string, error) {
	var include, exclude []string
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "!") {
			exclude = append(exclude, strings.TrimPrefix(p, "!"))
		} else {
			include = append(include, p)
		}
	}
	matched := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		ok := false
		for _, p := range include {
			if m, _ := doublestar.Match(strings.TrimPrefix(p, "./"), rel); m {
				ok = true
				break
			}
		}
		for _, p := range exclude {
			if m, _ := doublestar.Match(strings.TrimPrefix(p, "./"), rel); m {
				ok = false
				break
			}
		}
		if ok {
			matched[rel] = true
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matched) == 0 {
		return "", nil
	}
	files := make([]string, 0, len(matched))
	for f := range matched {
		files = append(files, f)
	}
	sort.Strings(files)
	all := sha256.New()
	for _, f := range files {
		h := sha256.New()
		fh, err := os.Open(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		_, err = io.Copy(h, fh)
		fh.Close()
		if err != nil {
			continue
		}
		all.Write(h.Sum(nil))
	}
	return hex.EncodeToString(all.Sum(nil)), nil
}
