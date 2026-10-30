package runner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// nodeDist is where Node.js is downloaded from. Tests point it at a local server.
var nodeDist = "https://nodejs.org/dist"

// nodeProvider finds or installs the Node.js that a JavaScript action needs.
type nodeProvider struct {
	mu    sync.Mutex
	tools string // the tool cache of the run
	found map[int]string
}

func newNodeProvider(toolCache string) *nodeProvider {
	return &nodeProvider{tools: toolCache, found: map[int]string{}}
}

// majorOf maps `runs.using` to a Node.js major version: node20 is 20.
func majorOf(using string) (int, bool) {
	u := strings.ToLower(strings.TrimSpace(using))
	if !strings.HasPrefix(u, "node") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(u, "node"))
	return n, err == nil && n > 0
}

func nodeVersionOf(bin string) (string, error) {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ensure returns the path of a node binary of the major version: the one on PATH when it matches, otherwise one
// downloaded into the tool cache (and checked against the SHASUMS256.txt of that release).
func (n *nodeProvider) ensure(ctx context.Context, major int, pathDirs []string, say func(string)) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if p := n.found[major]; p != "" {
		return p, nil
	}
	for _, d := range pathDirs {
		bin := filepath.Join(d, "node")
		if st, err := os.Stat(bin); err != nil || st.IsDir() {
			continue
		}
		if v, err := nodeVersionOf(bin); err == nil && strings.HasPrefix(v, "v"+strconv.Itoa(major)+".") {
			n.found[major] = bin
			return bin, nil
		}
		break // the first node on PATH is the one that a step would find
	}
	// the tool cache layout of setup-node: node/<version>/<arch>
	arch := nodeArch()
	if matches, _ := filepath.Glob(filepath.Join(n.tools, "node", strconv.Itoa(major)+".*", arch, "bin", "node")); len(matches) > 0 {
		bin := matches[len(matches)-1]
		if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(bin)), ".jactionlint-complete")); err == nil {
			n.found[major] = bin
			return bin, nil
		}
	}
	say(fmt.Sprintf("no Node.js %d on PATH, downloading it into the tool cache", major))
	ver, err := latestNode(ctx, major)
	if err != nil {
		return "", err
	}
	bin, err := installNode(ctx, n.tools, ver, arch)
	if err != nil {
		return "", err
	}
	n.found[major] = bin
	return bin, nil
}

func nodeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	}
	return runtime.GOARCH
}

func nodeOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	}
	return runtime.GOOS
}

// latestNode returns the newest release (such as "20.18.1") of the major version.
func latestNode(ctx context.Context, major int) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", nodeDist+"/index.json", nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot list Node.js releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("cannot list Node.js releases: %s", resp.Status)
	}
	var rel []struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&rel); err != nil {
		return "", fmt.Errorf("cannot read the Node.js release list: %w", err)
	}
	prefix := "v" + strconv.Itoa(major) + "."
	for _, r := range rel { // newest first
		if strings.HasPrefix(r.Version, prefix) {
			return strings.TrimPrefix(r.Version, "v"), nil
		}
	}
	return "", fmt.Errorf("no Node.js release with the major version %d", major)
}

// installNode downloads node-v<ver>-<os>-<arch>.tar.gz, verifies its SHA-256 against SHASUMS256.txt of the release, and
// unpacks it to <tools>/node/<ver>/<arch>.
func installNode(ctx context.Context, tools, ver, arch string) (string, error) {
	file := fmt.Sprintf("node-v%s-%s-%s.tar.gz", ver, nodeOS(), arch)
	sums, err := httpGet(ctx, fmt.Sprintf("%s/v%s/SHASUMS256.txt", nodeDist, ver), 1<<20)
	if err != nil {
		return "", fmt.Errorf("cannot read the checksums of Node.js %s: %w", ver, err)
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == file {
			want = f[0]
		}
	}
	if want == "" {
		return "", fmt.Errorf("node.js %s has no build %s", ver, file)
	}
	dest := filepath.Join(tools, "node", ver, arch)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(dest), ".download-")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/v%s/%s", nodeDist, ver, file), nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot download %s: %w", file, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("cannot download %s: %s", file, resp.Status)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, h), io.LimitReader(resp.Body, 512<<20)); err != nil {
		return "", fmt.Errorf("cannot download %s: %w", file, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return "", fmt.Errorf("the checksum of %s is %s, not the %s that nodejs.org lists: refusing to run it", file, got, want)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".unpack-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := unpackNode(tmpFile, stage); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(stage, "bin", "node")); err != nil {
		return "", errors.New("the Node.js archive has no bin/node")
	}
	if err := os.WriteFile(filepath.Join(stage, ".jactionlint-complete"), nil, 0o644); err != nil {
		return "", err
	}
	os.RemoveAll(dest)
	if err := os.Rename(stage, dest); err != nil {
		return "", err
	}
	return filepath.Join(dest, "bin", "node"), nil
}

func httpGet(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// unpackNode extracts the release tarball without its top-level directory. Symlinks (npm, npx, corepack) stay inside.
func unpackNode(r io.Reader, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	root, _ := filepath.Abs(dst)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(h.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path %q in the Node.js archive", h.Name)
		}
		i := strings.IndexByte(name, filepath.Separator)
		if i < 0 {
			continue
		}
		name = name[i+1:]
		target := filepath.Join(root, name)
		if !inDir(root, target) {
			return fmt.Errorf("unsafe path %q in the Node.js archive", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
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
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			resolved := h.Linkname
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(target), h.Linkname)
			}
			if isAbsLink(h.Linkname) || !inDir(root, filepath.Clean(resolved)) {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			os.Remove(target)
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		}
	}
}
