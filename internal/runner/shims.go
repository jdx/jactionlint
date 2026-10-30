package runner

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jdx/jactionlint/v2"
)

// shim replaces an action that only makes sense on a hosted runner. It returns handled=false when it cannot do what
// the inputs ask, and then the real action runs.
type shim func(jr *jobRun, n label, step *jactionlint.Step, inputs map[string]string, emit func(string), say func(string)) (res stepResult, eff *effects, handled bool)

func newEffects() *effects {
	return &effects{env: map[string]string{}, outputs: map[string]string{}, state: map[string]string{}}
}

// shimFor returns the built-in replacement of an action, unless the user turned it off (--no-shim).
func (e *Engine) shimFor(ref *jactionlint.UsesRef) shim {
	name := strings.ToLower(ref.Owner + "/" + ref.Repo)
	for _, n := range e.cfg.NoShim {
		if strings.EqualFold(n, name) {
			return nil
		}
	}
	if name == "actions/cache" {
		// off unless asked for: the real action works through the cache service
		on := false
		for _, n := range e.cfg.EnableShim {
			if strings.EqualFold(n, name) {
				on = true
			}
		}
		if !on {
			return nil
		}
		switch ref.Subpath {
		case "":
			return shimCacheMain
		case "restore":
			return shimCacheRestore
		case "save":
			return shimCacheSave
		}
		return nil
	}
	if ref.Subpath != "" {
		return nil
	}
	switch name {
	case "actions/checkout":
		return shimCheckout
	case "actions/upload-artifact":
		return shimUploadArtifact
	case "actions/download-artifact":
		return shimDownloadArtifact
	}
	return nil
}

// shimCheckout: the workspace of the job already is a copy of your checkout, which is what a checkout of this
// repository at the current commit gives. Anything else (another repository, a ref, a path) needs the real action.
func shimCheckout(jr *jobRun, n label, step *jactionlint.Step, in map[string]string, emit func(string), say func(string)) (stepResult, *effects, bool) {
	gh := jr.env.Contexts["github"].(map[string]any)
	cur, _ := gh["repository"].(string)
	sha, _ := gh["sha"].(string)
	ref, _ := gh["ref"].(string)
	refName, _ := gh["ref_name"].(string)
	repo := in["repository"]
	want := in["ref"]
	path := strings.TrimSuffix(strings.TrimPrefix(in["path"], "./"), "/")
	if (repo != "" && !strings.EqualFold(repo, cur)) || (want != "" && want != ref && want != refName && want != sha) || (path != "" && path != ".") {
		say("actions/checkout with repository, ref or path other than this checkout needs the real action, which runs now")
		return stepResult{}, nil, false
	}
	say("actions/checkout is replaced by the workspace copy of your checkout (" + shortSHA(sha) + "); use --no-shim actions/checkout to run the real action")
	eff := newEffects()
	eff.outputs["ref"] = ref
	eff.outputs["commit"] = sha
	return stepResult{Outcome: "success"}, eff, true
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

var artifactCounter atomic.Int64

// artifactsDir is where uploaded artifacts live during the run.
func (e *Engine) artifactsDir() string { return filepath.Join(e.rd.Root, "artifacts") }

// resolveInside resolves symlinks in p (relative to root) and reports an error when the result is not under root. The
// runner uses it before it reads or writes a path that a step could have turned into a link.
func resolveInside(root, p string) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p = filepath.Clean(p)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	// resolve the longest existing prefix, then append the rest
	existing, rest := p, ""
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	full := filepath.Join(realExisting, rest)
	if !inDir(realRoot, full) {
		return "", fmt.Errorf("%s resolves outside the workspace (a symlink?)", p)
	}
	return full, nil
}

// uploadFiles lists the regular files that the path patterns match, relative to work, without following links.
func uploadFiles(work string, patterns []string, hidden bool) (files []string, skippedLinks int) {
	include := map[string]bool{}
	var exclude []string
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if strings.HasPrefix(pat, "!") {
			exclude = append(exclude, rel(work, strings.TrimPrefix(pat, "!")))
			continue
		}
		pat = rel(work, pat)
		matches, _ := doublestar.Glob(os.DirFS(work), filepath.ToSlash(pat))
		for _, m := range matches {
			full := filepath.Join(work, filepath.FromSlash(m))
			st, err := os.Lstat(full)
			if err != nil {
				continue
			}
			switch {
			case st.Mode()&fs.ModeSymlink != 0:
				skippedLinks++
			case st.IsDir():
				filepath.WalkDir(full, func(p string, d fs.DirEntry, err error) error {
					if err != nil {
						return nil
					}
					if d.Type()&fs.ModeSymlink != 0 {
						skippedLinks++
						return nil
					}
					if !d.IsDir() {
						r, _ := filepath.Rel(work, p)
						include[filepath.ToSlash(r)] = true
					}
					return nil
				})
			case st.Mode().IsRegular():
				include[m] = true
			}
		}
	}
	for f := range include {
		skip := false
		for _, x := range exclude {
			if ok, _ := doublestar.Match(filepath.ToSlash(x), f); ok {
				skip = true
			}
			if ok, _ := doublestar.Match(filepath.ToSlash(x)+"/**", f); ok {
				skip = true
			}
		}
		if !hidden {
			for _, seg := range strings.Split(f, "/") {
				if strings.HasPrefix(seg, ".") {
					skip = true
				}
			}
		}
		if !skip {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	return files, skippedLinks
}

func rel(work, p string) string {
	p = strings.TrimPrefix(p, "./")
	if filepath.IsAbs(p) {
		if r, err := filepath.Rel(work, p); err == nil && !strings.HasPrefix(r, "..") {
			return filepath.ToSlash(r)
		}
	}
	return p
}

func shimUploadArtifact(jr *jobRun, n label, step *jactionlint.Step, in map[string]string, emit func(string), say func(string)) (stepResult, *effects, bool) {
	e := jr.e
	eff := newEffects()
	name := in["name"]
	if name == "" {
		name = "artifact"
	}
	if strings.ContainsAny(name, "\"<>|*?\r\n\\/:") {
		return stepResult{Outcome: "failure", Note: fmt.Sprintf("the artifact name %q contains a character that is not allowed", name)}, eff, true
	}
	patterns := strings.Split(strings.ReplaceAll(in["path"], "\r\n", "\n"), "\n")
	hidden := strings.EqualFold(in["include-hidden-files"], "true")
	files, links := uploadFiles(jr.work, patterns, hidden)
	if links > 0 {
		eff.warn("%d symbolic link(s) were not uploaded", links)
	}
	if len(files) == 0 {
		msg := "no files were found with the provided path: " + strings.TrimSpace(in["path"])
		switch strings.ToLower(in["if-no-files-found"]) {
		case "error":
			return stepResult{Outcome: "failure", Note: msg}, eff, true
		case "ignore":
		default:
			eff.warn("%s. No artifacts will be uploaded.", msg)
		}
		return stepResult{Outcome: "success"}, eff, true
	}
	dest := filepath.Join(e.artifactsDir(), name)
	e.mu.Lock()
	if _, err := os.Stat(dest); err == nil {
		if !strings.EqualFold(in["overwrite"], "true") {
			e.mu.Unlock()
			return stepResult{Outcome: "failure", Note: fmt.Sprintf("an artifact named %q already exists (pass overwrite: true to replace it)", name)}, eff, true
		}
		os.RemoveAll(dest)
	}
	e.mu.Unlock()
	// The paths are relative to the least common directory of the files, like the real action.
	base := commonDir(files)
	var total int64
	for _, f := range files {
		src, err := resolveInside(jr.work, f)
		if err != nil {
			eff.warn("%v: not uploaded", err)
			continue
		}
		r, _ := filepath.Rel(filepath.FromSlash(base), filepath.FromSlash(f))
		if err := copyEntry(src, filepath.Join(dest, r)); err != nil {
			return stepResult{Outcome: "failure", Note: err.Error()}, eff, true
		}
		if st, err := os.Stat(src); err == nil {
			total += st.Size()
		}
	}
	say(fmt.Sprintf("uploaded artifact %q: %d file(s), %d bytes (kept in .jactionlint/artifacts after the run)", name, len(files), total))
	eff.outputs["artifact-id"] = fmt.Sprint(artifactCounter.Add(1))
	eff.outputs["artifact-url"] = "file://" + dest
	return stepResult{Outcome: "success"}, eff, true
}

// commonDir returns the deepest directory (slash separated) that holds all the files; "" for the root.
func commonDir(files []string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Dir(files[0])), "/")
	if filepath.Dir(files[0]) == "." {
		return ""
	}
	for _, f := range files[1:] {
		d := strings.Split(filepath.ToSlash(filepath.Dir(f)), "/")
		if filepath.Dir(f) == "." {
			return ""
		}
		i := 0
		for i < len(parts) && i < len(d) && parts[i] == d[i] {
			i++
		}
		parts = parts[:i]
	}
	return strings.Join(parts, "/")
}

func shimDownloadArtifact(jr *jobRun, n label, step *jactionlint.Step, in map[string]string, emit func(string), say func(string)) (stepResult, *effects, bool) {
	e := jr.e
	eff := newEffects()
	if in["run-id"] != "" || in["repository"] != "" || in["github-token"] != "" && in["run-id"] != "" {
		say("actions/download-artifact with run-id or repository downloads from GitHub, so the real action runs now")
		return stepResult{}, nil, false
	}
	root := e.artifactsDir()
	target := in["path"]
	if target == "" {
		target = "."
	}
	dest, err := resolveInside(jr.work, target)
	if err != nil {
		return stepResult{Outcome: "failure", Note: err.Error()}, eff, true
	}
	ents, _ := os.ReadDir(root)
	var names []string
	for _, ent := range ents {
		if !ent.IsDir() {
			continue
		}
		if in["name"] != "" && ent.Name() != in["name"] {
			continue
		}
		if p := in["pattern"]; p != "" {
			if ok, _ := doublestar.Match(p, ent.Name()); !ok {
				continue
			}
		}
		names = append(names, ent.Name())
	}
	if in["name"] != "" && len(names) == 0 {
		return stepResult{Outcome: "failure", Note: fmt.Sprintf("unable to download artifact(s): artifact not found for name: %s", in["name"])}, eff, true
	}
	merge := strings.EqualFold(in["merge-multiple"], "true")
	for _, nm := range names {
		out := dest
		if in["name"] == "" && !merge {
			out = filepath.Join(dest, nm)
		}
		if err := copyDir(filepath.Join(root, nm), out, jr.work); err != nil {
			return stepResult{Outcome: "failure", Note: err.Error()}, eff, true
		}
	}
	say(fmt.Sprintf("downloaded %d artifact(s) to %s", len(names), target))
	eff.outputs["download-path"] = dest
	return stepResult{Outcome: "success"}, eff, true
}

// copyDir copies the files of src into dst, refusing to write through a link that leads out of the root.
func copyDir(src, dst, root string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		r, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, r)
		real, err := resolveInside(root, target)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(real, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		st, _ := in.Stat()
		out, err := os.OpenFile(real, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, st.Mode().Perm()|0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
