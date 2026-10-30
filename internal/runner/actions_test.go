package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// actionRepo builds a repository in the home directory (not /tmp, which the sandbox lets steps write) with the
// given files, and a workflow.
func actionRepo(t *testing.T, workflow string, files map[string]string) string {
	t.Helper()
	needSteps(t)
	if err := sandbox.Available(sandbox.NetOpen); err != nil {
		t.Skip(err)
	}
	dir := homeRepo(t, workflow)
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustRun(t *testing.T, dir string, wantCode int, args ...string) string {
	t.Helper()
	code, out, errOut := runIn(t, dir, append([]string{"w", "--no-lint"}, args...)...)
	if code != wantCode {
		t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", code, wantCode, out, errOut)
	}
	return out + errOut
}

func needNode(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	return p
}

func nodePathRoot(t *testing.T) []string {
	t.Helper()
	p := needNode(t)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return []string{"--allow-read", filepath.Dir(filepath.Dir(p))}
}

func TestLocalCompositeAction(t *testing.T) {
	wf := `on: push
jobs:
  j:
    runs-on: self-hosted
    steps:
      - id: greet
        uses: ./.github/actions/greet
        with:
          who: world
        env:
          FROM_STEP_ENV: visible
      - run: echo "outer sees ${{ steps.greet.outputs.message }} and $EXPORTED"
`
	action := `name: greet
inputs:
  who:
    default: nobody
  punct:
    default: "!"
outputs:
  message:
    value: ${{ steps.make.outputs.msg }}
runs:
  using: composite
  steps:
    - id: make
      shell: bash
      run: |
        echo "msg=hello ${{ inputs.who }}${{ inputs.punct }}" >> "$GITHUB_OUTPUT"
        echo "EXPORTED=from-composite" >> "$GITHUB_ENV"
        echo "path=$GITHUB_ACTION_PATH" | sed 's|.*/\.github|.github|'
        echo "step env: $FROM_STEP_ENV"
    - shell: bash
      run: echo "inner steps see ${{ steps.make.outputs.msg }}"
    - shell: bash
      if: failure()
      run: echo never
`
	dir := actionRepo(t, wf, map[string]string{".github/actions/greet/action.yml": action})
	out := mustRun(t, dir, 0)
	for _, want := range []string{
		"[step 1] Run ./.github/actions/greet",
		"[step 1.1] Run echo \"msg=hello",
		".github/actions/greet",
		"step env: visible",
		"inner steps see hello world!",
		"[step 1.3] Run echo never: skipped",
		"outer sees hello world! and from-composite",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
}

func TestCompositeFailureAndContinueOnError(t *testing.T) {
	wf := `on: push
jobs:
  j:
    runs-on: self-hosted
    steps:
      - id: a
        uses: ./act
        continue-on-error: true
      - run: echo "outcome=${{ steps.a.outcome }} conclusion=${{ steps.a.conclusion }}"
      - uses: ./act
      - if: failure()
        run: echo "failure() after composite"
`
	act := `runs:
  using: composite
  steps:
    - shell: bash
      run: exit 4
    - shell: bash
      run: echo not-run
`
	dir := actionRepo(t, wf, map[string]string{"act/action.yml": act})
	out := mustRun(t, dir, 1)
	for _, want := range []string{"outcome=failure conclusion=success", "failure() after composite", "[step 1.2] Run echo not-run: skipped"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
}

func TestNodeActionPrePostState(t *testing.T) {
	wf := `on: push
jobs:
  j:
    runs-on: self-hosted
    steps:
      - id: n
        uses: ./act
        with:
          who: Mona
          multi-word input: spaces
      - run: echo "main output ${{ steps.n.outputs.greeting }}"
`
	files := map[string]string{
		"act/action.yml": `name: n
inputs:
  who:
    default: nobody
  multi-word input:
    default: x
runs:
  using: node24
  pre: pre.js
  main: main.js
  post: post.js
`,
		"act/pre.js":  `console.log("pre: state not set yet", process.env.STATE_started || "(none)"); console.log("::save-state name=started::yes")`,
		"act/main.js": `const fs=require("fs"); console.log("main: INPUT_WHO=" + process.env.INPUT_WHO + " INPUT_MULTI_WORD_INPUT=" + process.env["INPUT_MULTI_WORD INPUT"]+"|"+process.env.INPUT_MULTI_WORD_INPUT + " HOME_FAKE=" + /jactionlint/.test(process.env.HOME)); fs.appendFileSync(process.env.GITHUB_OUTPUT, "greeting=Hello " + process.env.INPUT_WHO + "\n"); console.log("::save-state name=value::from-main"); console.log("secret is", process.env.GITHUB_TOKEN || "unset")`,
		"act/post.js": `console.log("post: STATE_started=" + process.env.STATE_started + " STATE_value=" + process.env.STATE_value)`,
	}
	dir := actionRepo(t, wf, files)
	out := mustRun(t, dir, 0, nodePathRoot(t)...)
	for _, want := range []string{
		"pre: state not set yet (none)",
		"main: INPUT_WHO=Mona",
		"HOME_FAKE=true",
		"secret is unset",
		"main output Hello Mona",
		"[post step 1] Post ./act",
		"post: STATE_started=yes STATE_value=from-main",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if strings.Index(out, "main output") > strings.Index(out, "post: STATE_started") {
		t.Errorf("the post step must run after the later steps of the job\n%s", out)
	}
}

func TestRemoteActionPinningAndTrust(t *testing.T) {
	// a repository served over HTTP: owner/repo at one commit
	sha := strings.Repeat("a", 40)
	tarball := makeTarGz(t, "owner-repo-"+sha[:7], map[string]string{
		"action.yml":     "runs:\n  using: composite\n  steps:\n    - shell: bash\n      run: echo remote action ran, input=${{ inputs.x }}\ninputs:\n  x:\n    default: dflt\n",
		"sub/action.yml": "runs:\n  using: composite\n  steps:\n    - shell: bash\n      run: echo subpath action ran\n",
		"link":           "",
	})
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		switch r.URL.Path {
		case "/repos/owner/repo/commits/v1":
			w.Write([]byte(sha))
		case "/owner/repo/tar.gz/" + sha:
			w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	apiBase, codeloadBase = srv.URL+"/api-unused", srv.URL
	defer func(a, c string) { apiBase, codeloadBase = a, c }("https://api.github.com", "https://codeload.github.com")
	apiBase = srv.URL
	t.Setenv("JACTIONLINT_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	wf := fmt.Sprintf(`on: push
jobs:
  j:
    runs-on: self-hosted
    steps:
      - uses: owner/repo@%s
        with:
          x: given
      - uses: owner/repo/sub@%s
`, sha, sha)
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 0)
	if !strings.Contains(out, "remote action ran, input=given") || !strings.Contains(out, "subpath action ran") || !strings.Contains(out, "downloaded owner/repo@aaaaaaaaaaaa") {
		t.Fatalf("remote action did not run:\n%s", out)
	}
	if hits["/owner/repo/tar.gz/"+sha] != 1 {
		t.Errorf("the second use should come from the cache: %d downloads", hits["/owner/repo/tar.gz/"+sha])
	}
	// the second run uses the cache and does not download
	mustRun(t, dir, 0)
	if hits["/owner/repo/tar.gz/"+sha] != 1 {
		t.Errorf("a second run downloaded again")
	}

	// an unpinned ref is refused unless --trust-actions
	os.WriteFile(filepath.Join(dir, ".github", "workflows", "w.yml"), []byte("on: push\njobs:\n  j:\n    runs-on: self-hosted\n    steps:\n      - uses: owner/repo@v1\n"), 0o644)
	out = mustRun(t, dir, 1)
	if !strings.Contains(out, "is not pinned to a commit SHA") || !strings.Contains(out, "--trust-actions") {
		t.Fatalf("unpinned action was not refused:\n%s", out)
	}
	if strings.Contains(out, "remote action ran") {
		t.Fatal("an unpinned action ran")
	}
	out = mustRun(t, dir, 0, "--trust-actions")
	if !strings.Contains(out, "owner/repo@v1 is "+sha) || !strings.Contains(out, "remote action ran") {
		t.Fatalf("--trust-actions:\n%s", out)
	}
}

func makeTarGz(t *testing.T, top string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: top + "/", Typeflag: tar.TypeDir, Mode: 0o755})
	for name, body := range files {
		if name == "link" {
			tw.WriteHeader(&tar.Header{Name: top + "/link", Typeflag: tar.TypeSymlink, Linkname: "../../etc/passwd"})
			continue
		}
		tw.WriteHeader(&tar.Header{Name: top + "/" + name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestExtractRefusesEscapes(t *testing.T) {
	for name, hdr := range map[string]tar.Header{
		"dotdot":   {Name: "top/../../evil", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1},
		"absolute": {Name: "/etc/evil", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1},
		"hardlink": {Name: "top/h", Typeflag: tar.TypeLink, Linkname: "/etc/passwd"},
		"device":   {Name: "top/dev", Typeflag: tar.TypeChar},
	} {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		h := hdr
		tw.WriteHeader(&h)
		if h.Size > 0 {
			tw.Write([]byte("x"))
		}
		tw.Close()
		gz.Close()
		dst := t.TempDir()
		err := extractTarGz(&buf, dst)
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
		if _, e := os.Stat(filepath.Join(filepath.Dir(dst), "evil")); e == nil {
			t.Errorf("%s: a file escaped", name)
		}
	}
	// a link out of the tree is dropped, one inside is kept
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "top/a.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1})
	tw.Write([]byte("a"))
	tw.WriteHeader(&tar.Header{Name: "top/in", Typeflag: tar.TypeSymlink, Linkname: "a.txt"})
	tw.WriteHeader(&tar.Header{Name: "top/out", Typeflag: tar.TypeSymlink, Linkname: "../../../etc/passwd"})
	tw.WriteHeader(&tar.Header{Name: "top/abs", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.Close()
	gz.Close()
	dst := t.TempDir()
	if err := extractTarGz(&buf, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "in")); err != nil {
		t.Error("the inner link was dropped")
	}
	for _, n := range []string{"out", "abs"} {
		if _, err := os.Lstat(filepath.Join(dst, n)); err == nil {
			t.Errorf("the link %s was kept", n)
		}
	}
}

func TestNodeInstallChecksum(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("no node downloads on this OS")
	}
	ver := "20.99.0"
	file := fmt.Sprintf("node-v%s-%s-%s.tar.gz", ver, nodeOS(), nodeArch())
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	top := strings.TrimSuffix(file, ".tar.gz")
	script := "#!/bin/sh\necho v" + ver + "\n"
	tw.WriteHeader(&tar.Header{Name: top + "/bin/node", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(script))})
	tw.Write([]byte(script))
	tw.Close()
	gz.Close()
	sum := sha256.Sum256(buf.Bytes())
	good := hex.EncodeToString(sum[:])

	serve := func(listed string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/index.json":
				fmt.Fprintf(w, `[{"version":"v21.0.0"},{"version":"v%s"},{"version":"v20.1.0"},{"version":"v18.0.0"}]`, ver)
			case "/v" + ver + "/SHASUMS256.txt":
				fmt.Fprintf(w, "%s  %s\n%s  other.tar.gz\n", listed, file, strings.Repeat("0", 64))
			case "/v" + ver + "/" + file:
				w.Write(buf.Bytes())
			default:
				http.NotFound(w, r)
			}
		}))
	}
	old := nodeDist
	defer func() { nodeDist = old }()

	srv := serve(good)
	nodeDist = srv.URL
	tools := t.TempDir()
	np := newNodeProvider(tools)
	var said []string
	bin, err := np.ensure(t.Context(), 20, nil, func(s string) { said = append(said, s) })
	srv.Close()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := nodeVersionOf(bin); err != nil || v != "v"+ver {
		t.Fatalf("installed node: %q %v", v, err)
	}
	if len(said) == 0 || !strings.Contains(said[0], "downloading") {
		t.Errorf("no note about the download: %v", said)
	}
	// the second time, from the tool cache, without a server
	bin2, err := newNodeProvider(tools).ensure(t.Context(), 20, nil, func(string) {})
	if err != nil || bin2 != bin {
		t.Fatalf("cached node: %v %q", err, bin2)
	}

	// a wrong checksum is refused and nothing is installed
	srv = serve(strings.Repeat("f", 64))
	defer srv.Close()
	nodeDist = srv.URL
	tools2 := t.TempDir()
	_, err = newNodeProvider(tools2).ensure(t.Context(), 20, nil, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want a checksum error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(tools2, "node", ver, nodeArch())); err == nil {
		t.Errorf("a download with a wrong checksum was installed")
	}
	// PATH wins when its node has the right major version
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\necho v22.1.0\n"), 0o755)
	got, err := newNodeProvider(t.TempDir()).ensure(t.Context(), 22, []string{dir}, func(string) {})
	if err != nil || got != filepath.Join(dir, "node") {
		t.Fatalf("PATH node: %v %q", err, got)
	}
}

func TestReusableWorkflow(t *testing.T) {
	caller := `on: push
jobs:
  prep:
    runs-on: self-hosted
    outputs:
      tag: ${{ steps.t.outputs.tag }}
    steps:
      - id: t
        run: echo "tag=v9" >> "$GITHUB_OUTPUT"
  call:
    needs: prep
    uses: ./.github/workflows/called.yml
    with:
      version: ${{ needs.prep.outputs.tag }}
      verbose: true
    secrets:
      token: ${{ secrets.OUTER }}
  inherit:
    uses: ./.github/workflows/called.yml
    with:
      version: inherited
    secrets: inherit
  after:
    needs: [call, inherit]
    runs-on: self-hosted
    steps:
      - run: echo "call result ${{ needs.call.result }} output ${{ needs.call.outputs.built }} and ${{ needs.inherit.outputs.built }}"
`
	called := `on:
  workflow_call:
    inputs:
      version:
        type: string
        required: true
      verbose:
        type: boolean
        default: false
      count:
        type: number
        default: 3
    secrets:
      token:
        required: false
    outputs:
      built:
        value: ${{ jobs.build.outputs.artifact }}
jobs:
  build:
    runs-on: self-hosted
    outputs:
      artifact: ${{ steps.b.outputs.name }}
    steps:
      - id: b
        run: |
          echo "version=${{ inputs.version }} verbose=${{ inputs.verbose }} count=${{ inputs.count }} token=${{ secrets.token }}"
          echo "name=pkg-${{ inputs.version }}" >> "$GITHUB_OUTPUT"
`
	dir := actionRepo(t, caller, map[string]string{".github/workflows/called.yml": called})
	out := mustRun(t, dir, 0, "--secret", "OUTER=s3cret", "--secret", "token=inherited-token")
	for _, want := range []string{
		"[job] call / build",
		"version=v9 verbose=true count=3 token=***",
		"[job] inherit / build",
		"version=inherited verbose=false count=3 token=***",
		"call result success output pkg-v9 and pkg-inherited",
		"success   call / build",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "s3cret") || strings.Contains(out, "inherited-token") {
		t.Errorf("a secret leaked\n%s", out)
	}
}

func TestArtifactsAndCheckoutShim(t *testing.T) {
	wf := `on: push
jobs:
  produce:
    runs-on: self-hosted
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
      - run: |
          mkdir -p dist/sub
          echo one > dist/a.txt
          echo two > dist/sub/b.txt
          echo hidden > dist/.secret
          ln -s /etc/passwd dist/link
          test -f tracked.txt
      - uses: actions/upload-artifact@v4
        with:
          name: bundle
          path: |
            dist/
            !dist/sub/skip
      - uses: actions/upload-artifact@v4
        with:
          name: bundle
          path: dist/a.txt
        continue-on-error: true
  consume:
    needs: produce
    runs-on: self-hosted
    steps:
      - uses: actions/download-artifact@v4
        with:
          name: bundle
          path: got
      - run: |
          find got -type f | sort
          cat got/sub/b.txt
      - uses: actions/download-artifact@v4
        with:
          name: missing
        continue-on-error: true
`
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 0)
	for _, want := range []string{
		"actions/checkout is replaced by the workspace copy",
		"uploaded artifact \"bundle\": 2 file(s)",
		"warning: 1 symbolic link(s) were not uploaded",
		"already exists",
		"got/a.txt",
		"got/sub/b.txt",
		"two",
		"artifact not found for name: missing",
		"artifacts kept in .jactionlint/artifacts/",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if strings.Contains(out, ".secret") || strings.Contains(out, "link\n") {
		t.Errorf("hidden file or link uploaded:\n%s", out)
	}
	ents, _ := filepath.Glob(filepath.Join(dir, ".jactionlint", "artifacts", "*", "bundle", "a.txt"))
	if len(ents) != 1 {
		t.Errorf("artifact not kept: %v", ents)
	}
}
