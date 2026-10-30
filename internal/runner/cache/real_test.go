package cache

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealActionsCache checks the service against the real @actions/cache library. It needs Node.js and the library,
// so it only runs when JACTIONLINT_ACTIONS_CACHE_DIRS lists installs of it (a directory with
// node_modules/@actions/cache each), separated by the OS path list separator:
//
//	mkdir v3 v6 && (cd v3 && npm i --ignore-scripts @actions/cache@3.3.0) && (cd v6 && npm i --ignore-scripts @actions/cache@6.3.0)
//	JACTIONLINT_ACTIONS_CACHE_DIRS=$PWD/v3:$PWD/v6 JACTIONLINT_BIG=1 go test ./internal/runner/cache -run RealActionsCache -v
//
// v3 uses the REST protocol; 4 and later use Twirp and the Azure blob SDK. JACTIONLINT_BIG=1 adds a 140 MiB archive, which
// is above the 128 MiB limit of a single Put Blob and so goes through Put Block and Put Block List.
func TestRealActionsCache(t *testing.T) {
	dirs := filepath.SplitList(os.Getenv("JACTIONLINT_ACTIONS_CACHE_DIRS"))
	if len(dirs) == 0 {
		t.Skip("JACTIONLINT_ACTIONS_CACHE_DIRS is not set")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	for _, dir := range dirs {
		for _, big := range []bool{false, true} {
			if big && os.Getenv("JACTIONLINT_BIG") == "" {
				continue
			}
			name := fmt.Sprintf("%s/big=%v", filepath.Base(dir), big)
			t.Run(name, func(t *testing.T) {
				svc, _ := newService(t, false)
				work := t.TempDir()
				data := filepath.Join(work, "data")
				os.MkdirAll(data, 0o755)
				write := func(name string, n int64) {
					f, _ := os.Create(filepath.Join(data, name))
					io.CopyN(f, rand.Reader, n)
					f.Close()
				}
				write("small.bin", 300000)
				if big {
					write("big.bin", 140<<20)
				}
				before := tree(t, data)
				env := append(os.Environ(),
					"RUNNER_TEMP="+filepath.Join(work, "tmp"), "GITHUB_WORKSPACE="+work, "GITHUB_REF=refs/heads/main",
					"GITHUB_SERVER_URL=https://github.com", "RUNNER_OS=Linux")
				os.MkdirAll(filepath.Join(work, "tmp"), 0o755)
				for k, v := range svc.Env() {
					env = append(env, k+"="+v)
				}
				if strings.Contains(filepath.Base(dir), "v3") {
					env = append(env, "ACTIONS_CACHE_SERVICE_V2=") // libraries up to 3.x only know REST
				}
				run := func(args ...string) string {
					cmd := exec.Command(node, append([]string{"testdata/client.mjs", dir}, args...)...)
					cmd.Env = env
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("node %v: %v\n%s", args, err, out)
					}
					return string(out)
				}
				if out := run("save", data, "key-1"); !strings.Contains(out, "RESULT save") || strings.Contains(out, "RESULT save -1") {
					t.Fatalf("save: %s", out)
				}
				os.RemoveAll(data)
				if out := run("restore", data, "key-2", "key-"); !strings.Contains(out, `RESULT restore "key-1"`) {
					t.Fatalf("restore by prefix: %s", out)
				}
				if after := tree(t, data); after != before {
					t.Fatalf("the restored files differ\nbefore: %s\nafter:  %s", before, after)
				}
				if out := run("restore", data, "unknown"); !strings.Contains(out, "RESULT restore undefined") {
					t.Fatalf("a miss: %s", out)
				}
				if out := run("save", data, "key-1"); !strings.Contains(out, "RESULT save -1") {
					t.Fatalf("a saved key must not be saved again: %s", out)
				}
			})
		}
	}
}

func tree(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.New()
		io.Copy(h, f)
		f.Close()
		fmt.Fprintf(&b, "%s:%s;", e.Name(), hex.EncodeToString(h.Sum(nil))[:16])
	}
	return b.String()
}
