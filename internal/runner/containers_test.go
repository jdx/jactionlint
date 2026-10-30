package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeDocker is a docker command that records its arguments, answers the queries jactionlint makes, and runs `exec`
// on this machine, so that the whole path of a container job can be tested without an engine. The real engine was used
// by hand (see the pull request).
const fakeDocker = `#!/bin/bash
log="$FAKE_DOCKER_LOG"
echo "$*" >> "$log"
cmd=$1; shift
case "$cmd" in
  version) echo 29.0.0 ;;
  build) echo sha256:abc ;;
  image)
    if [ "$1" = inspect ]; then echo '["PATH=/usr/local/bin:/usr/bin:/bin","LANG=C.UTF-8"]'; fi ;;
  inspect)
    fmt=""; while [ $# -gt 0 ]; do case "$1" in --format) fmt="$2"; shift 2 ;; *) shift ;; esac; done
    case "$fmt" in
      *Ports*) echo '{"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"49153"}],"6379/tcp":[{"HostIp":"127.0.0.1","HostPort":"49154"}]}' ;;
      *"if .State.Health"*) echo yes ;;
      *Health.Status*) echo healthy ;;
    esac ;;
  run)
    for a in "$@"; do
      case "$prev" in --env-file) echo "ENVFILE: $(cat "$a")" >> "$log"; cat "$a" >> "$log.cenv" ;; esac
      prev=$a
    done
    echo "docker-action-output" ;;
  exec)
    if [ -n "$FAKE_DOCKER_NO_MOUNTS" ] && [ "$2" = test ]; then exit 1; fi
    # the environment of the container, as the engine keeps it for every exec
    if [ -f "$log.cenv" ]; then while IFS= read -r l; do [ -n "$l" ] && export "$l"; done < "$log.cenv"; fi
    while [ $# -gt 0 ]; do
      case "$1" in
        -i) shift ;;
        -w) cd "$2"; shift 2 ;;
        --env-file) while IFS= read -r l; do [ -n "$l" ] && export "$l"; done < "$2"; shift 2 ;;
        -e) shift 2 ;;
        *) break ;;
      esac
    done
    shift
    exec "$@" ;;
esac
exit 0
`

func fakeEngine(t *testing.T) (bin, log string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte(fakeDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	log = filepath.Join(dir, "log")
	t.Setenv("FAKE_DOCKER_LOG", log)
	return bin, log
}

func readLog(t *testing.T, log string) string {
	t.Helper()
	b, _ := os.ReadFile(log)
	return string(b)
}

const digestImage = "ubuntu:24.04@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852"

func TestContainerJob(t *testing.T) {
	bin, log := fakeEngine(t)
	wf := `on: push
jobs:
  j:
    runs-on: self-hosted
    container:
      image: ` + digestImage + `
      env:
        FROM_CONTAINER: from-the-container
      options: --memory 512m --cpus 1
    steps:
      - run: |
          echo "env=$FROM_CONTAINER path=$PATH"
          echo "out" > made.txt
      - run: echo "job context ${{ job.container.network }}" | sed -E 's/[0-9]+-[0-9]+/N-N/g'
`
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 0, "--container-engine", bin)
	for _, want := range []string{"env=from-the-container path=/usr/local/bin:/usr/bin:/bin", "job context jactionlint-N-N-net", "[job] j: success"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	l := readLog(t, log)
	for _, want := range []string{
		"run -d --init --name jactionlint-",
		"--cap-drop=ALL --security-opt no-new-privileges --cap-add CHOWN",
		"--entrypoint tail " + digestImage + " -f /dev/null",
		"--memory 512m --cpus 1",
		"--label jactionlint=1",
		"exec -i -w ",
		"rm -f -v jactionlint-",
		"network rm jactionlint-",
	} {
		if !strings.Contains(l, want) {
			t.Errorf("the engine log lacks %q\n%s", want, l)
		}
	}
	for _, bad := range []string{"--privileged", "docker.sock", " -v /:/", "--network host", "--pid"} {
		if strings.Contains(l, bad) {
			t.Errorf("the container command line has %q\n%s", bad, l)
		}
	}
	if strings.Contains(l, "tracked.txt") {
		t.Error("nothing but the run directories is mounted")
	}
}

func TestContainerOptionsAreFiltered(t *testing.T) {
	for opts, want := range map[string]string{
		"--privileged":                      "--privileged is refused",
		"--cap-add SYS_ADMIN":               "--cap-add is refused",
		"-v /:/host":                        "-v is refused",
		"-v/etc:/etc":                       "-v is refused",
		"--volume=/home:/h":                 "--volume is refused",
		"--network=host":                    "--network is refused",
		"--net host":                        "--net is refused",
		"--pid host":                        "--pid is refused",
		"--device /dev/kvm":                 "--device is refused",
		"--security-opt seccomp=unconfined": "--security-opt is refused",
		"--userns host":                     "--userns is refused",
		"--mount type=bind,src=/,dst=/m":    "--mount is refused",
		"-p 80:80":                          "-p is refused",
		"--name evil":                       "--name is refused",
		"--surprise":                        "not on the list",
		"--memory":                          "needs a value",
	} {
		_, err := parseOptions(opts, false)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %q", opts, err, want)
		}
	}
	got, err := parseOptions("--init --user 1000:1000 -m512m --health-cmd 'pg_isready -U postgres' --health-interval=5s --cpus 1.5 -e FOO=bar", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "--init|--user|1000:1000|-m512m|--health-cmd|pg_isready -U postgres|--health-interval=5s|--cpus|1.5|-e|FOO=bar" {
		t.Errorf("got %q", got)
	}
	if got, err := parseOptions("--privileged -v /:/h", true); err != nil || len(got) != 3 {
		t.Errorf("unsafe: %q %v", got, err)
	}
}

func TestRefusedOptionStopsTheJob(t *testing.T) {
	bin, log := fakeEngine(t)
	wf := `on: push
jobs:
  j:
    runs-on: self-hosted
    container:
      image: ` + digestImage + `
      options: --privileged
    steps:
      - run: echo never
`
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 1, "--container-engine", bin)
	if !strings.Contains(out, "--privileged is refused") || !strings.Contains(out, "--unsafe-container-options") || strings.Contains(out, "never") {
		t.Fatalf("output:\n%s", out)
	}
	if strings.Contains(readLog(t, log), "--privileged") {
		t.Fatal("the option reached the engine")
	}
	// with --unsafe-container-options it is passed through
	mustRun(t, dir, 0, "--container-engine", bin, "--unsafe-container-options")
	if !strings.Contains(readLog(t, log), "--privileged") {
		t.Fatal("--unsafe-container-options did not pass the option")
	}
}

func TestImageTrust(t *testing.T) {
	bin, log := fakeEngine(t)
	wf := `on: push
jobs:
  j:
    runs-on: self-hosted
    container: ubuntu:24.04
    steps:
      - run: echo ran
`
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 1, "--container-engine", bin)
	if !strings.Contains(out, "is not pinned by digest") || !strings.Contains(out, "--trust-images") {
		t.Fatalf("an unpinned image was not refused:\n%s", out)
	}
	if strings.Contains(readLog(t, log), "pull") {
		t.Fatal("an unpinned image was pulled")
	}
	out = mustRun(t, dir, 0, "--container-engine", bin, "--trust-images")
	if !strings.Contains(out, "pulling ubuntu:24.04") || !strings.Contains(out, "ran") {
		t.Fatalf("--trust-images:\n%s", out)
	}
}

func TestServices(t *testing.T) {
	bin, log := fakeEngine(t)
	wf := `on: push
jobs:
  j:
    runs-on: self-hosted
    services:
      pg:
        image: postgres:16@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
        env:
          POSTGRES_PASSWORD: secret
        ports:
          - 5432
        options: --health-cmd pg_isready --health-interval 5s
      cache:
        image: redis@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
        ports:
          - 6000:6379
    steps:
      - run: |
          echo "postgres on localhost:${{ job.services.pg.ports['5432'] }}"
          echo "redis on localhost:${{ job.services.cache.ports['6379'] }}"
`
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 0, "--container-engine", bin)
	for _, want := range []string{"postgres on localhost:49153", "redis on localhost:49154", "starting the services of the job"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	l := readLog(t, log)
	for _, want := range []string{
		"--network-alias pg", "--network-alias cache", "-p 127.0.0.1::5432", "-p 127.0.0.1:6000:6379",
		"--health-cmd pg_isready --health-interval 5s", "--cap-drop=ALL",
		"rm -f -v jactionlint-", "network rm",
	} {
		if !strings.Contains(l, want) {
			t.Errorf("the engine log lacks %q\n%s", want, l)
		}
	}
	if strings.Contains(l, "-p 5432") || strings.Contains(l, "-p 0.0.0.0") {
		t.Errorf("a port was published on every interface\n%s", l)
	}
	// the steps ran on this machine: no job container
	if strings.Contains(l, "--entrypoint tail") {
		t.Errorf("a job container was started for a job that only has services\n%s", l)
	}
}

func TestContainerModeLinuxCells(t *testing.T) {
	bin, log := fakeEngine(t)
	wf := `on: push
jobs:
  j:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, ubuntu-22.04, macos-latest, windows-latest]
    steps:
      - run: echo "cell ${{ matrix.os }} sudo=$(sudo -n echo works)"
`
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 0, "--container-engine", bin, "--container", "--trust-images")
	for _, want := range []string{
		"cell ubuntu-latest sudo=works", "cell ubuntu-22.04 sudo=works",
		"j (macos-latest): skipped (runs-on macos-latest needs macOS",
		"j (windows-latest): skipped (runs-on windows-latest needs Windows",
	} {
		if strings.Contains(want, "macos-latest") && runtime.GOOS == "darwin" {
			continue // on a Mac that cell runs on the host
		}
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	l := readLog(t, log)
	for _, want := range []string{
		"ubuntu:24.04@sha256:fe81560990fae86d", "ubuntu:22.04@sha256:076e7bf8", "/usr/local/bin/sudo:ro",
	} {
		if !strings.Contains(l, want) {
			t.Errorf("the engine log lacks %q\n%s", want, l)
		}
	}
	// one image for everything, and an image per label
	log2 := readLog(t, log)
	_ = log2
	mustRun(t, dir, 0, "--container-engine", bin, "--container="+digestImage)
	mustRun(t, dir, 0, "--container-engine", bin, "--container", "--image", "ubuntu-22.04=alpine@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	if !strings.Contains(readLog(t, log), "alpine@sha256:cccc") {
		t.Error("--image LABEL=IMAGE was not used")
	}
	// a label without a default image
	wf2 := strings.ReplaceAll(wf, "[ubuntu-latest, ubuntu-22.04, macos-latest, windows-latest]", "[ubuntu-18.04]")
	os.WriteFile(filepath.Join(dir, ".github", "workflows", "w.yml"), []byte(wf2), 0o644)
	out = mustRun(t, dir, 0, "--container-engine", bin, "--container")
	if !strings.Contains(out, "no default image for runs-on ubuntu-18.04") || !strings.Contains(out, "--image") {
		t.Errorf("missing default image:\n%s", out)
	}
}

func TestContainerFlagsConflict(t *testing.T) {
	dir := repoWith(t, map[string]string{"w.yml": "on: push\njobs:\n  j:\n    runs-on: x\n    steps: [{run: e}]\n"})
	code, _, errOut := runIn(t, dir, "w", "--no-lint", "--container", "--no-containers")
	if code != ExitUsage || !strings.Contains(errOut, "contradict") {
		t.Fatalf("code %d %q", code, errOut)
	}
}

func TestNoEngineSkipsOnlyContainerJobs(t *testing.T) {
	wf := `on: push
jobs:
  plain:
    runs-on: self-hosted
    steps:
      - run: echo plain job ran
  boxed:
    runs-on: self-hosted
    container: ` + digestImage + `
    steps:
      - run: echo never
  after:
    needs: [plain]
    runs-on: self-hosted
    steps:
      - run: echo after ran
`
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 0, "--container-engine", "no-such-engine-installed")
	for _, want := range []string{"plain job ran", "after ran", `[job] boxed: skipped (the container engine "no-such-engine-installed" is not installed)`} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	out = mustRun(t, dir, 0, "--no-containers")
	if !strings.Contains(out, "boxed: skipped (it needs a container and --no-containers is set)") {
		t.Errorf("--no-containers:\n%s", out)
	}
}

func TestContainerNeedsToSeeTheRunDirectory(t *testing.T) {
	bin, _ := fakeEngine(t)
	t.Setenv("FAKE_DOCKER_NO_MOUNTS", "1")
	wf := "on: push\njobs:\n  j:\n    runs-on: self-hosted\n    container: " + digestImage + "\n    steps:\n      - run: echo never\n"
	dir := actionRepo(t, wf, nil)
	out := mustRun(t, dir, 1, "--container-engine", bin)
	if !strings.Contains(out, "cannot see the directories of this run") || strings.Contains(out, "never") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestDockerActions(t *testing.T) {
	bin, log := fakeEngine(t)
	wf := `on: push
jobs:
  j:
    runs-on: self-hosted
    steps:
      - uses: ./act
        with:
          who: Mona
      - uses: docker://alpine@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
        with:
          args: ignored
      - uses: docker://alpine:3.20
`
	files := map[string]string{
		"act/action.yml": `name: d
inputs:
  who:
    default: nobody
runs:
  using: docker
  image: Dockerfile
  entrypoint: /entry.sh
  args:
    - hello
    - ${{ inputs.who }}
  env:
    FROM_ACTION: yes
`,
		"act/Dockerfile": "FROM scratch\n",
	}
	dir := actionRepo(t, wf, files)
	out := mustRun(t, dir, 1, "--container-engine", bin)
	l := readLog(t, log)
	for _, want := range []string{
		"build -q -t jactionlint-action:", "-f ",
		"run --rm --init --label jactionlint=1 --workdir /github/workspace",
		"--cap-drop=ALL",
		":/github/workspace", ":/github/home", ":/github/file_commands", ":/github/workflow/event.json:ro", ":/opt/hostedtoolcache",
		"--entrypoint /entry.sh jactionlint-action:", " hello Mona",
		"ENVFILE:", "INPUT_WHO=Mona", "FROM_ACTION=yes", "GITHUB_WORKSPACE=/github/workspace", "HOME=/github/home",
		"GITHUB_OUTPUT=/github/file_commands/output", "GITHUB_EVENT_PATH=/github/workflow/event.json", "RUNNER_TEMP=/github/runner_temp",
		"alpine@sha256:dddd",
	} {
		if !strings.Contains(l, want) {
			t.Errorf("the engine log lacks %q\n%s", want, l)
		}
	}
	if strings.Contains(l, "PATH=/home") || strings.Contains(l, "\nPATH=") {
		t.Errorf("the PATH of this machine reached a Docker action\n%s", l)
	}
	if !strings.Contains(out, "docker-action-output") {
		t.Errorf("output of the action is missing\n%s", out)
	}
	// the unpinned docker:// image is refused
	if !strings.Contains(out, "alpine:3.20 is not pinned by digest") {
		t.Errorf("an unpinned docker:// image was not refused\n%s", out)
	}
}
