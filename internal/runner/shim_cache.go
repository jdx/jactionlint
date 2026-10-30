package runner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/cache"
)

// The native actions/cache is opt-in (--shim actions/cache): the real action works through the cache service and is
// what runs by default. The shim needs no Node.js, and its archives are written and read by the runner itself, so they
// are bounded by this code and not by tar. Its entries use a version of their own, so they are not shared with the
// entries of the real action.
//
// An archive holds entries named w/<path> (under the workspace), h/<path> (under the home directory) and nothing else:
// restoring never writes anywhere else, whatever a poisoned entry says.

const shimCacheSalt = "jactionlint-shim-1"

type cachePlan struct {
	patterns []string
	key      string
	restore  []string
	version  string
}

func newCachePlan(in map[string]string) (*cachePlan, error) {
	p := &cachePlan{key: strings.TrimSpace(in["key"])}
	for _, l := range strings.Split(strings.ReplaceAll(in["path"], "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			p.patterns = append(p.patterns, l)
		}
	}
	for _, l := range strings.Split(strings.ReplaceAll(in["restore-keys"], "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			p.restore = append(p.restore, l)
		}
	}
	if len(p.patterns) == 0 {
		return nil, errors.New("input required and not supplied: path")
	}
	if p.key == "" {
		return nil, errors.New("input required and not supplied: key")
	}
	if strings.Contains(p.key, ",") {
		return nil, fmt.Errorf("key %q must not contain commas", p.key)
	}
	if len(p.key) > 512 {
		return nil, errors.New("key must be at most 512 characters")
	}
	sum := sha256.Sum256([]byte(strings.Join(p.patterns, "|") + "|gzip|" + shimCacheSalt))
	p.version = hex.EncodeToString(sum[:])
	return p, nil
}

func (e *Engine) cacheReady() (*cache.Store, []string, bool) {
	r := e.root()
	if r.cacheStore == nil {
		return nil, nil, false
	}
	return r.cacheStore, r.cacheScopes(), true
}

func shimCacheMain(jr *jobRun, n label, step *jactionlint.Step, in map[string]string, emit func(string), say func(string)) (stepResult, *effects, bool) {
	return runCacheShim(jr, n, step, in, say, true, true)
}

func shimCacheRestore(jr *jobRun, n label, step *jactionlint.Step, in map[string]string, emit func(string), say func(string)) (stepResult, *effects, bool) {
	return runCacheShim(jr, n, step, in, say, true, false)
}

func shimCacheSave(jr *jobRun, n label, step *jactionlint.Step, in map[string]string, emit func(string), say func(string)) (stepResult, *effects, bool) {
	return runCacheShim(jr, n, step, in, say, false, false)
}

func runCacheShim(jr *jobRun, n label, step *jactionlint.Step, in map[string]string, say func(string), restore, postSave bool) (stepResult, *effects, bool) {
	e := jr.e
	eff := newEffects()
	plan, err := newCachePlan(in)
	if err != nil {
		return stepResult{Outcome: "failure", Note: err.Error()}, eff, true
	}
	store, scopes, ok := e.cacheReady()
	if !ok {
		say("the cache is off (--cache=off), so nothing is restored or saved")
		return stepResult{Outcome: "success"}, eff, true
	}
	hit := false
	if restore {
		eff.outputs["cache-primary-key"] = plan.key
		ent, err := store.Lookup(scopes, plan.key, plan.restore, plan.version)
		if err != nil {
			return stepResult{Outcome: "failure", Note: err.Error()}, eff, true
		}
		switch {
		case ent == nil:
			if strings.EqualFold(in["fail-on-cache-miss"], "true") {
				return stepResult{Outcome: "failure", Note: "failed to restore: cache entry not found for input keys: " + plan.key}, eff, true
			}
			say("cache not found for input keys: " + strings.Join(append([]string{plan.key}, plan.restore...), ", "))
		default:
			hit = ent.Key == plan.key
			eff.outputs["cache-matched-key"] = ent.Key
			if hit {
				eff.outputs["cache-hit"] = "true"
			} else {
				eff.outputs["cache-hit"] = "false"
			}
			if strings.EqualFold(in["lookup-only"], "true") {
				say("cache found (" + ent.Key + "); lookup only, not restoring")
			} else {
				if err := extractCache(store.Path(ent), jr); err != nil {
					return stepResult{Outcome: "failure", Note: "cache restore failed: " + err.Error()}, eff, true
				}
				say(fmt.Sprintf("cache restored from key: %s (%d bytes)", ent.Key, ent.Size))
			}
		}
		if postSave {
			exact := hit
			jr.posts = append(jr.posts, &postStep{n: label("post " + string(n)), name: "Post Cache", step: step, run: func(ctx context.Context) (stepResult, *effects) {
				if exact {
					return stepResult{Outcome: "success", Note: "cache hit on the primary key, not saving"}, nil
				}
				if jr.failed {
					return stepResult{Outcome: "success", Note: "the job failed, not saving"}, nil
				}
				return saveCache(jr, store, scopes, plan, e)
			}})
		}
		return stepResult{Outcome: "success"}, eff, true
	}
	res, eff2 := saveCache(jr, store, scopes, plan, e)
	return res, eff2, true
}

func saveCache(jr *jobRun, store *cache.Store, scopes []string, plan *cachePlan, e *Engine) (stepResult, *effects) {
	eff := newEffects()
	if store.ReadOnly {
		return stepResult{Outcome: "success", Note: "the cache is read-only, not saving"}, eff
	}
	pend, err := store.Reserve(scopes[0], plan.key, plan.version)
	if err != nil {
		if errors.Is(err, cache.ErrExists) {
			e.pr.Note(2, "unable to reserve cache with key %s, another job may be creating this cache", plan.key)
			return stepResult{Outcome: "success"}, eff
		}
		return stepResult{Outcome: "failure", Note: err.Error()}, eff
	}
	files, err := writeCacheArchive(pend.Path, plan.patterns, jr)
	if err != nil {
		store.Abort(pend)
		return stepResult{Outcome: "failure", Note: err.Error()}, eff
	}
	if files == 0 {
		store.Abort(pend)
		eff.warn("path specified in the cache step does not match any files: %s", strings.Join(plan.patterns, ", "))
		return stepResult{Outcome: "success"}, eff
	}
	ent, err := store.Commit(pend, -1)
	if err != nil {
		store.Abort(pend)
		if errors.Is(err, cache.ErrExists) {
			return stepResult{Outcome: "success"}, eff
		}
		return stepResult{Outcome: "failure", Note: err.Error()}, eff
	}
	e.pr.Note(2, "cache saved with key: %s (%d files, %d bytes)", ent.Key, files, ent.Size)
	return stepResult{Outcome: "success"}, eff
}

// expandCachePaths turns the path patterns into files: ~ is the home directory, relative paths are relative to the
// workspace, ** and ! work, and symlinks are never followed.
func expandCachePaths(patterns []string, jr *jobRun) (files map[string]string, err error) {
	files = map[string]string{} // archive name -> path on disk
	var excludes []string
	type root struct{ prefix, dir string }
	roots := []root{{"w", jr.work}, {"h", jr.rd.Home}}
	nameOf := func(p string) (string, bool) {
		for _, r := range roots {
			if inDir(r.dir, p) {
				rel, _ := filepath.Rel(r.dir, p)
				if rel == "." {
					return r.prefix, true
				}
				return r.prefix + "/" + filepath.ToSlash(rel), true
			}
		}
		return "", false
	}
	abs := func(pat string) string {
		pat = strings.TrimSpace(pat)
		switch {
		case pat == "~" || strings.HasPrefix(pat, "~/"):
			return filepath.Join(jr.rd.Home, strings.TrimPrefix(strings.TrimPrefix(pat, "~"), "/"))
		case filepath.IsAbs(pat):
			return filepath.Clean(pat)
		}
		return filepath.Join(jr.work, filepath.FromSlash(strings.TrimPrefix(pat, "./")))
	}
	for _, pat := range patterns {
		if strings.HasPrefix(pat, "!") {
			excludes = append(excludes, abs(strings.TrimPrefix(pat, "!")))
		}
	}
	excluded := func(p string) bool {
		for _, x := range excludes {
			if ok, _ := doublestar.PathMatch(x, p); ok || inDir(x, p) {
				return true
			}
		}
		return false
	}
	for _, pat := range patterns {
		if strings.HasPrefix(pat, "!") {
			continue
		}
		matches, gerr := doublestar.FilepathGlob(abs(pat))
		if gerr != nil {
			return nil, fmt.Errorf("bad path pattern %q: %w", pat, gerr)
		}
		for _, m := range matches {
			filepath.WalkDir(m, func(p string, d fs.DirEntry, werr error) error {
				if werr != nil || excluded(p) {
					return nil
				}
				name, ok := nameOf(p)
				if !ok {
					return nil // outside the workspace and the home: not cached
				}
				files[name] = p
				return nil
			})
		}
	}
	return files, nil
}

func writeCacheArchive(dst string, patterns []string, jr *jobRun) (int, error) {
	files, err := expandCachePaths(patterns, jr)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, nil
	}
	f, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	count := 0
	for _, name := range names {
		p := files[name]
		st, err := os.Lstat(p)
		if err != nil {
			continue
		}
		hdr := &tar.Header{Name: name, Mode: int64(st.Mode().Perm()), ModTime: st.ModTime()}
		switch {
		case st.IsDir():
			hdr.Typeflag = tar.TypeDir
			hdr.Name += "/"
		case st.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				continue
			}
			hdr.Typeflag, hdr.Linkname = tar.TypeSymlink, target
		case st.Mode().IsRegular():
			hdr.Typeflag, hdr.Size = tar.TypeReg, st.Size()
		default:
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			f.Close()
			return 0, err
		}
		if hdr.Typeflag == tar.TypeReg {
			in, err := os.Open(p)
			if err != nil {
				f.Close()
				return 0, err
			}
			_, err = io.CopyN(tw, in, st.Size())
			in.Close()
			if err != nil {
				f.Close()
				return 0, fmt.Errorf("%s changed while it was being saved: %w", p, err)
			}
		}
		count++
	}
	if err := tw.Close(); err != nil {
		f.Close()
		return 0, err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return 0, err
	}
	return count, f.Close()
}

func extractCache(archive string, jr *jobRun) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	roots := map[string]string{"w": jr.work, "h": jr.rd.Home}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(strings.TrimSuffix(h.Name, "/"))
		prefix, rel, _ := strings.Cut(name, "/")
		base, ok := roots[prefix]
		if !ok || path.IsAbs(name) || rel == ".." || strings.HasPrefix(rel, "../") {
			return fmt.Errorf("the archive has an entry %q outside the workspace and the home directory", h.Name)
		}
		target := base
		if rel != "" {
			target = filepath.Join(base, filepath.FromSlash(rel))
		}
		// Never write through a link: resolve the directory that holds the entry.
		real, err := resolveInside(base, filepath.Dir(target))
		if err != nil {
			return fmt.Errorf("%s: %w", h.Name, err)
		}
		target = filepath.Join(real, filepath.Base(target))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			os.Remove(target)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fs.FileMode(h.Mode).Perm()|0o200)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			resolved := h.Linkname
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(target), h.Linkname)
			}
			if filepath.IsAbs(h.Linkname) || !inDir(base, filepath.Clean(resolved)) {
				continue // a link that leaves the directory is not restored
			}
			os.Remove(target)
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		}
	}
}
