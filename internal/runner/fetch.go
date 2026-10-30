package runner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Where actions come from. Tests point these at a local server.
var (
	apiBase      = "https://api.github.com"
	codeloadBase = "https://codeload.github.com"
)

var reSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

const (
	maxArchiveBytes = 512 << 20
	maxArchiveFiles = 200000
)

// repoToken is the token used to read action repositories: from the environment of the runner itself (never from a step).
func repoToken() string {
	for _, k := range []string{"JACTIONLINT_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func httpClient() *http.Client { return &http.Client{Timeout: 5 * time.Minute} }

// resolveRef returns the commit SHA a ref of a repository points at.
func resolveRef(ctx context.Context, owner, repo, ref string) (string, error) {
	if reSHA.MatchString(ref) {
		return ref, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/repos/%s/%s/commits/%s", apiBase, owner, repo, ref), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.sha")
	req.Header.Set("User-Agent", "jactionlint")
	if t := repoToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s/%s@%s: %w", owner, repo, ref, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("cannot resolve %s/%s@%s: GitHub answered %s", owner, repo, ref, resp.Status)
	}
	sha := strings.TrimSpace(string(body))
	if !reSHA.MatchString(sha) {
		return "", fmt.Errorf("cannot resolve %s/%s@%s: unexpected answer", owner, repo, ref)
	}
	return sha, nil
}

// fetchRepo makes sure the files of the repository at the commit are in the cache and returns their directory. cached
// says that nothing was downloaded. The directory is content-addressed (owner/repo/sha), never written by a step, and
// shared between runs.
func fetchRepo(ctx context.Context, stateDir, owner, repo, sha string) (dir string, cached bool, err error) {
	if !reSHA.MatchString(sha) {
		return "", false, fmt.Errorf("%q is not a commit SHA", sha)
	}
	if strings.ContainsAny(owner+repo, "/\\") || owner == "" || repo == "" || strings.Contains(owner+repo, "..") {
		return "", false, fmt.Errorf("invalid repository %s/%s", owner, repo)
	}
	dir = filepath.Join(stateDir, "actions", strings.ToLower(owner), strings.ToLower(repo), sha)
	if st, err := os.Stat(filepath.Join(dir, ".jactionlint-complete")); err == nil && !st.IsDir() {
		return dir, true, nil
	}
	url := fmt.Sprintf("%s/%s/%s/tar.gz/%s", codeloadBase, owner, repo, sha)
	token := repoToken()
	if token != "" {
		url = fmt.Sprintf("%s/repos/%s/%s/tarball/%s", apiBase, owner, repo, sha)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", "jactionlint")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// do not send the token to another host after a redirect
	client := httpClient()
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return errors.New("too many redirects")
		}
		r.Header.Del("Authorization")
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("cannot download %s/%s@%s: %w", owner, repo, sha, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", false, fmt.Errorf("cannot download %s/%s@%s: %s (is it private? set GITHUB_TOKEN for jactionlint)", owner, repo, sha, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", false, err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".incoming-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(tmp)
	if err := extractTarGz(io.LimitReader(resp.Body, maxArchiveBytes+1), tmp); err != nil {
		return "", false, fmt.Errorf("cannot unpack %s/%s@%s: %w", owner, repo, sha, err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".jactionlint-complete"), nil, 0o644); err != nil {
		return "", false, err
	}
	os.RemoveAll(dir) // an interrupted earlier attempt
	if err := os.Rename(tmp, dir); err != nil {
		if st, e2 := os.Stat(filepath.Join(dir, ".jactionlint-complete")); e2 == nil && !st.IsDir() {
			return dir, true, nil // another run finished first
		}
		return "", false, err
	}
	return dir, false, nil
}

// extractTarGz unpacks a GitHub tarball (one top-level directory, which is dropped) into dst. It refuses entries
// that would land outside dst, hard links, devices, and symlinks that point outside dst.
func extractTarGz(r io.Reader, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	files := 0
	root, _ := filepath.Abs(dst)
	var links []struct{ name, target string }
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		files++
		if files > maxArchiveFiles {
			return errors.New("too many files")
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\x00") {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		// drop the top-level directory
		if i := strings.Index(name, "/"); i >= 0 {
			name = name[i+1:]
		} else {
			continue // the top-level directory itself, or pax headers
		}
		if name == "" || name == "." {
			continue
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\x00") {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if !inDir(root, target) {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += h.Size
			if total > maxArchiveBytes {
				return errors.New("archive too large")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, h.Size)); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			links = append(links, struct{ name, target string }{name, h.Linkname})
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
		default:
			return fmt.Errorf("%q is a kind of file that an action repository must not contain", h.Name)
		}
	}
	// Symlinks last, and only when they stay inside the directory.
	for _, l := range links {
		target := filepath.Join(root, filepath.FromSlash(l.name))
		resolved := l.target
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(target), l.target)
		}
		if isAbsLink(l.target) || !inDir(root, filepath.Clean(resolved)) {
			continue // dropped: a link out of the repository is never needed to run an action
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(l.target, target); err != nil {
			return err
		}
	}
	return nil
}

// isAbsLink reports whether a link target is absolute on any OS: a rooted path counts on Windows too, where filepath.IsAbs
// wants a drive letter.
func isAbsLink(target string) bool {
	return filepath.IsAbs(target) || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "\\")
}
