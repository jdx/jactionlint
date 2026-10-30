# Run workflows locally

`jactionlint run` runs the jobs of a workflow on your machine, so that you can debug a `run:` script, an expression, a matrix or the `needs` wiring without pushing a commit. It needs no container engine and no runner image.

This page describes what the command does today. Each section says what is still missing.

## Run a workflow

```sh
jactionlint run ci                          # every job of .github/workflows/ci.yml
jactionlint run ci -j test                  # the job `test` and the jobs it needs
jactionlint run ci -e pull_request --secret TOKEN=abc --var REGION=eu
```

```text
[job] build
  [step 1] Run echo "hello from $GITHUB_REPOSITORY on $RUNNER_OS"
    hello from jdx/jactionlint on Linux
  [step 1] success
  [step 2] Run go test ./...
    ok  	github.com/jdx/jactionlint/v2	3.1s
  [step 2] success
[job] build: success
[job] test (macos-latest): skipped (runs-on macos-latest needs macOS, this machine is Linux)

Summary
  success   build
  skipped   test (macos-latest)  (runs-on macos-latest needs macOS, this machine is Linux)
```

Before it runs anything, `jactionlint run` lints the workflow with the `correctness` profile (what actionlint checks) and stops on an error, because GitHub would reject the file too. `--no-lint` skips that. The exit status is 0 when every job succeeded or was skipped, 1 when a job failed, 2 for a wrong command line and 3 when the run could not start.

What runs today:

- **`run:` steps**, in the order of the file, with `if:` (an implicit `success()` when the condition has no status function), `env:` (workflow, then job, then `GITHUB_ENV`, then step), `shell:` (`bash` runs `bash --noprofile --norc -eo pipefail {0}`, `sh`, `python`, `pwsh`, `powershell`, or a command line with `{0}`), `working-directory:`, `defaults.run`, `timeout-minutes:` and `continue-on-error:`. `steps.<id>.outputs`, `.outcome` and `.conclusion`, job `outputs:` and `needs.<job>.result` and `.outputs` work. A job whose `needs` did not succeed is skipped unless its `if:` says otherwise (`always()`, `failure()`).
- **Matrices**, also the ones that read `needs` outputs, with `fail-fast` (the remaining cells are skipped after a failure).
- **File commands:** `GITHUB_ENV`, `GITHUB_PATH`, `GITHUB_OUTPUT`, `GITHUB_STATE` and `GITHUB_STEP_SUMMARY`. `GITHUB_ENV` refuses variables that change what code a program loads (`NODE_OPTIONS`, `LD_*`, `DYLD_*`, `BASH_ENV`, `ENV`, `PYTHONPATH`, ...), and `GITHUB_PATH` accepts only directories that a step may run programs from.
- **Workflow commands** on standard output: `::error::`, `::warning::`, `::notice::` (with `file`, `line`, `col`, `title`), `::debug::` (with `--debug`), `::group::`/`::endgroup::`, `::add-mask::`, `::stop-commands::`, and the legacy `::set-output::` and `::save-state::` with a deprecation warning. `::set-env::` and `::add-path::` are refused, as on GitHub.
- **Secrets are masked** in all output: every `--secret` value and every `::add-mask::` value is shown as `***`. A reference to a secret you did not supply warns once.
- **Background steps:** `background: true`, `wait:`, `wait-all` and `cancel:`, and `parallel:` groups of `run` steps. The output of a background step is shown when the job waits for it.
- **`runs-on`:** a job runs on this machine when its label names this machine's operating system (`ubuntu-*` on Linux, `macos-*` on macOS) or says nothing about it; otherwise it is skipped with the reason. Cells of a matrix are decided one by one.
- **Ctrl-C** cancels the run: the process groups of the steps get `SIGTERM` and then `SIGKILL`, and steps with `if: always()` or `if: cancelled()` still run for up to five minutes.

Not modelled: `concurrency`, `permissions`, `environment` and OIDC; the output does not tell you that a workflow would pass on GitHub, only what happened here.

| Option | Meaning |
| --- | --- |
| `--secret NAME=VALUE`, `--secret-file FILE` | Secrets for `secrets.NAME`. They are masked in the output. |
| `--var NAME=VALUE` | Configuration variables for `vars.NAME`. |
| `--env-file FILE` | `NAME=VALUE` lines that become the default `env:` of the workflow. |
| `--pass-env NAME` | Let a variable of your environment reach the steps. Without it they get only `PATH`, `LANG`, `LC_*`, `TERM`, `TZ`. |
| `--sandbox on\|off` | `off` runs the steps without the sandbox, see [Isolation](#isolation). |
| `--in-place` | Work in your checkout instead of a copy. |
| `--allow-read DIR`, `--allow-write DIR` | Let steps read and run, or write, under `DIR`. |
| `--net open\|none\|proxy` | `open` (default), `none`, or `proxy` (see [Restricting the network](#restricting-the-network)). |
| `--allow-net HOST[,HOST...]`, `--learn` | With `--net=proxy`: hosts to let through, or print the hosts that steps contact. |
| `--cache on\|read-only\|off`, `--no-cache`, `--cache-dir DIR`, `--cache-size SIZE` | The local [actions cache](#the-cache). |
| `--shim ACTION` | Turn on a built-in replacement that is off by default (`actions/cache`). |
| `--trust-actions` | Run actions and reusable workflows that are not pinned to a commit SHA, see [Actions](#actions-and-reusable-workflows). |
| `--no-shim ACTION` | Run the real `actions/checkout`, `actions/upload-artifact` or `actions/download-artifact` instead of the built-in replacement. |
| `--container[=IMAGE]`, `--image LABEL=IMAGE`, `--platform P` | Run Linux jobs in a container, see [Containers](#containers). |
| `--container-engine docker\|podman`, `--no-containers` | Pick the engine; skip everything that needs a container. |
| `--trust-images`, `--unsafe-container-options` | Pull images that are not pinned by digest; pass `options:` as they are. |
| `--parallel N`, `--dry-run` | Run cells at once; print the commands and run nothing. |
| `--debug`, `--timings`, `--keep` | Show `::debug::` output, show the time of each step, keep `.jactionlint/run/<id>`. |

## See what would run

```sh
jactionlint run --list                     # every workflow of the repository
jactionlint run --list ci                  # .github/workflows/ci.yml (the extension is optional)
jactionlint run --list ci -j test          # the job `test` and the jobs it needs
```

The plan lists the jobs in the order in which they can run, grouped in stages (the jobs of a stage do not depend on each other), and expands each matrix into the cells that GitHub would create:

```text
.github/workflows/ci.yml  (CI)
  stage 1
    test (ubuntu-latest, 1.22)   runs-on: ${{ matrix.os }}
    test (ubuntu-latest, 1.23, true)   runs-on: ${{ matrix.os }}
    test (macos-latest, 1.23)   runs-on: ${{ matrix.os }}
    lint   runs-on: ubuntu-latest   if: github.event_name == 'push'   name: Lint
  stage 2
    shards (matrix known at run time: ${{ fromJSON(needs.lint.outputs.matrix) }})   needs: lint
```

The expansion follows the rules of the runner: the product of the rows, then `exclude` (a partial match removes the combination), then each `include` entry, which is added to every combination whose original values it does not overwrite and otherwise becomes a combination of its own. The name of a cell is the job ID and the scalar values of the combination, so a YAML `3.10` shows as `3.1`, like on GitHub.

A matrix that is built from the output of another job (`fromJSON(needs.setup.outputs.matrix)`) cannot be expanded before the run. The plan says so instead of guessing. A matrix that only needs `github`, `vars`, `inputs` or `env` (`fromJSON(inputs.oses)`) is expanded.

## Expressions

The plan evaluates the `${{ }}` expressions it can: the `name:` and `runs-on:` of each cell, and the `if:` of a job. An `if:` that reads only `github`, `vars`, `inputs`, `env`, `runner` or `matrix` is marked `=> if is true` or `=> if is false`, and `jactionlint run` will skip a job whose condition is false. A condition that reads `needs` or `steps` is left as it is written.

The evaluator follows the rules of GitHub: comparisons and property names ignore case, values of different types compare as numbers (`'1' == 1`, `null == 0`), `&&` and `||` return one of their operands, `a.*.b` filters an array or the values of an object, and `0`, `''`, `null` and `NaN` are false. It implements `contains`, `startsWith`, `endsWith`, `format`, `join`, `toJSON`, `fromJSON`, `hashFiles` and the status functions `success`, `failure`, `always` and `cancelled`.

The `github` context is filled from git: the repository comes from the `origin` remote, `sha` and `ref` from `HEAD`, and the actor from `git config user.name`. The `runner` context describes this machine, with `runner.environment` set to `self-hosted`.

Differences from GitHub that you can see in the plan: a matrix built from JSON lists its keys in alphabetical order instead of the order of the JSON text, which changes the order of the values in a job name, not which cells exist.

| Option | Meaning |
| --- | --- |
| `-l`, `--list` | Print the plan and exit. |
| `--json` | Print the plan as JSON (with `--list`). |
| `-j`, `--job ID` | Select a job and the jobs it needs. Repeat the option for more jobs. |
| `-e`, `--event NAME` | The event that triggers the workflow (default `push`). The plan notes a workflow that has no such trigger. |
| `--event-file FILE` | A JSON file with the event payload, which is `github.event`. Without it, jactionlint makes a small payload for the event (a `push` has `ref`, `after` and `head_commit`; a `pull_request` has `number` and `head` and `base` with `ref` and `sha`). |
| `-i`, `--input NAME=VALUE` | An input of `workflow_dispatch`, which is `inputs.NAME`. Repeat the option for more inputs. |
| `-C`, `--directory DIR` | Run as if started in `DIR`. |

Running the steps is not implemented yet.

## Actions and reusable workflows

`uses:` steps run, in the sandbox like every other step.

- **Local actions** (`uses: ./.github/actions/setup`) are read from the workspace copy of the job.
- **Repository actions** (`uses: owner/repo/path@sha`) are downloaded at the commit into `~/.cache/jactionlint/actions/owner/repo/<sha>` and reused by every later run; nothing a step does can change that copy. **They must be pinned to a full commit SHA**, because running an action executes someone else's code with your secrets. For a tag or a branch, `jactionlint run` stops and says so; `--trust-actions` resolves the ref now, prints the commit it chose, and runs it. Set `GITHUB_TOKEN` (or `GH_TOKEN`) in your shell for private repositories and to avoid rate limits; the token goes to GitHub only and never reaches a step.
- **Composite actions** run their steps in a scope of their own (`inputs`, `steps`, `github.action_path`), nested in the transcript as `[step 3.1]`. `GITHUB_ENV` and `GITHUB_PATH` of an inner step reach the job, and the outputs are the `outputs:` of `action.yml`.
- **JavaScript actions** (`runs.using: node12` to `node24`) run `pre`, `main` and `post` with `INPUT_*` and `STATE_*`; `post` runs at the end of the job, last action first, under its `post-if`. The Node.js comes from `PATH` when its major version matches; otherwise the matching release is downloaded from nodejs.org into the tool cache and **checked against the `SHASUMS256.txt` of that release** before it is unpacked.
- **Reusable workflows** (`uses:` on a job, a file of this repository or `owner/repo/.github/workflows/x.yml@sha`) get `with:` (booleans and numbers are typed, defaults apply, an undeclared input is an error), `secrets:` or `secrets: inherit`, and their outputs become the outputs of the calling job. Their jobs show as `caller / job`. Nesting is limited to 10 levels, as on GitHub.
- **Docker actions** (`runs.using: docker`, `uses: docker://image`) run through the container engine, see [Containers](#containers).

Three actions only make sense on a hosted runner, so `jactionlint run` replaces them (`--no-shim` runs the real one):

| Action | What happens locally |
| --- | --- |
| `actions/checkout` | The job's workspace already is a copy of your checkout, so it does nothing. A `repository`, `ref` (other than the current one) or `path` falls back to the real action. |
| `actions/upload-artifact` | Copies the matched files into `.jactionlint/artifacts/<run>/<name>/` after the run. Symbolic links, hidden files (unless `include-hidden-files`) and `!` excluded paths are left out; an existing name is an error unless `overwrite: true`. |
| `actions/download-artifact` | Copies an artifact of the same run into `path` (`pattern` and `merge-multiple` work). `run-id` and `repository` need the real action. |

The runner reads and writes files that a step could have replaced by a link (the artifact copies, `GITHUB_ENV` and the other file commands). It never follows a link out of the workspace there, so a step cannot use the runner to read a file it is not allowed to read.

## Containers

`jactionlint run` never uses a container engine unless a workflow asks for one or you pass `--container`. The engine is `docker`, or `podman` when that is what is installed (`--container-engine` picks). With no engine, or with `--no-containers`, only the jobs and steps that need one are skipped, with the reason; the rest runs.

**What a workflow can ask for:**

- `container:` on a job runs its steps inside that image.
- `services:` start next to the job on a private network, with the `env`, `ports` and health check of the workflow. The job waits until a service with a health check is healthy. `job.services.<id>.ports['5432']` is the port on this machine (published on `127.0.0.1` only); a job in a container reaches the service by its ID as host name. Services and the network are removed at the end of the job, also after Ctrl-C.
- A Docker action (`runs.using: docker`, an image or a `Dockerfile`, or `uses: docker://image`) runs with the paths of a hosted runner (`/github/workspace`, `/github/home`, `/github/file_commands`) and `INPUT_*` set.

**What a container gets.** Only the run directories of the job are mounted: the workspace copy, the fake home, the temp directory, the tool cache (and the downloaded actions, read-only), at the same paths as on this machine. No host home, no Docker socket, no privileged mode, no host network or PID namespace. Every capability is dropped except the nine a root userland needs (`CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `FSETID`, `KILL`, `SETGID`, `SETUID`, `SETPCAP`, `NET_BIND_SERVICE`) and `no-new-privileges` is set. The container is its own boundary, so the steps inside get that hardening and not the Landlock sandbox of the host; `--sandbox=off` does not relax it.

**`options:` is not passed through.** It is parsed against a list: `--init`, `--user`, `--memory`, `--cpus`, `--shm-size`, `--hostname`, `--ulimit`, the `--health-*` options, `-e`, `--label`, `--dns`, `--add-host`, `--entrypoint` and a few more. Anything that widens access is refused with the reason (`--privileged`, `--cap-add`, `-v`/`--mount`, `--network host`, `--pid`, `--device`, `--security-opt`, ...), and so is an option that is not on the list. `--unsafe-container-options` passes the string through as it is.

**Images must be pinned by digest** (`ubuntu:24.04@sha256:…`), because pulling an image runs someone else's code. An unpinned image stops the job with a message; `--trust-images` (or `--trust-actions`) pulls it anyway. Each pull is printed. A pinned image that is already local is not pulled again.

**`--container` for cross-platform testing.** Run every job whose `runs-on` is a Linux label in a container instead of on this machine, for example the Linux cells of a matrix from a Mac:

```sh
jactionlint run ci --container                         # the default image of each ubuntu-* label
jactionlint run ci --container=ubuntu:22.04@sha256:…   # one image for everything
jactionlint run ci --container --image ubuntu-latest=ghcr.io/me/runner@sha256:…
jactionlint run ci --container --platform linux/amd64  # emulate another architecture
```

`ubuntu-latest` and `ubuntu-24.04` map to `ubuntu:24.04`, `ubuntu-22.04` to `ubuntu:22.04`, both pinned by digest. These images are minimal on purpose: they lack the tools of a hosted image (`jq`, `gh`, compilers, language runtimes), so a workflow that installs its tools through `setup-*` actions or `apt-get` works, and anything heavier needs an image you choose with `--image`. The job is root in its container and `sudo` runs the command directly, so `sudo apt-get install` works as on a hosted runner (under the host sandbox it does not, by design). `macos-*` and `windows-*` cells cannot run in a container and are skipped.

JavaScript actions run inside the container with the Node.js of the tool cache, which is a glibc build: images based on musl (Alpine) cannot run them.

**The engine must see this machine's files.** The directories of the run are bind-mounted, so the engine has to run on this machine (Docker Desktop and Colima share the home directory; a remote daemon does not). `jactionlint run` checks that the container sees a marker file before the first step and says so when it does not.

`--list` shows where each cell would run (`=> runs in a container: ubuntu:24.04@sha256:…`, `=> skipped: needs macOS`).

## Parallel runs and dry runs

`--parallel N` runs up to N matrix cells and independent jobs at once, respecting `needs` and `strategy.max-parallel`. When `fail-fast` is on, a failing cell cancels the ones that are still running. The output of each job is printed in one piece when the job ends. Jobs that run at once each get a home directory of their own (the first keeps the path that caches were saved with, so caches saved by the others do not match).

`--dry-run` prints what would run and runs nothing: for every cell of every selected job it shows the steps with their `${{ }}` replaced, the `shell`, the `working-directory`, the `env` (secrets as `***`), the `with:` of actions, and what each `if:` evaluates to when no earlier step has produced an output. It copies no workspace, starts no cache service and needs neither the sandbox nor an engine.

## The cache

Without a cache every run downloads the toolchains again, and the throwaway home starts empty. `jactionlint run` therefore starts a **local cache service** on `127.0.0.1` (random port, random token for each run) and points every step at it, the way a hosted runner does (`ACTIONS_CACHE_URL`, `ACTIONS_RESULTS_URL`, `ACTIONS_CACHE_SERVICE_V2`, `ACTIONS_RUNTIME_TOKEN`). Actions that cache through `@actions/cache` work **unmodified**: `actions/cache`, the `setup-*` actions, `Swatinem/rust-cache`, `jdx/mise-action`.

```console
$ jactionlint run ci          # first run
    mise cache not found for mise-v0-linux-x64-975071a9...
    Cache saved from ...
$ jactionlint run ci          # second run
    Cache hit for: mise-v0-linux-x64-975071a9...
    mise cache restored from key: mise-v0-linux-x64-975071a9...
```

The service speaks both protocols of the library, because actions bundle different versions of it: the REST protocol of `@actions/cache` 3.x (`actions/cache@v3`, and the fallback of every newer version), and the Twirp protocol of 4.x and later, whose upload goes to a "signed URL" with the three blob operations of the Azure SDK (Put Blob up to 128 MiB, Put Block and Put Block List above). It was checked with the real library, 3.3.0, 4.0.5 and 6.3.0, on archives of 300 KB and 140 MiB (`TestRealActionsCache` in `internal/runner/cache`).

- **Matching** follows GitHub: the exact `key` first, then each `restore-keys` entry as a prefix, in order, newest entry of a prefix first. An entry matches only the same *version* (a hash of the cached paths and the compression method that the library computes). Keys are immutable: saving an existing key is skipped, as on GitHub.
- **Scope:** a cache is saved for the ref of the run (`refs/heads/feature`) and read from that ref and then from the default branch, so a throwaway branch cannot poison the cache of `main`.
- **Eviction:** entries unread for 7 days expire, and the least recently used entries go when the store is over 10 GB (`--cache-size`).
- **Where:** `~/.cache/jactionlint/cache/<owner>-<repo>/` (`--cache-dir`). The store is not in the sandbox's allow lists: a step reaches it only through the service, and a cache entry is only ever unpacked by a later sandboxed step.
- **The home keeps its path.** The fake home is emptied before every job but lives at `~/.cache/jactionlint/home/<repo>/home` each time, because the version of a cache entry includes the absolute paths it holds (`~/.cargo`, `~/.local/share/mise`). Jobs of one repository share that directory one at a time.
- **`RUNNER_TOOL_CACHE`** is a persistent directory, `~/.cache/jactionlint/toolcache/<os>-<arch>`, where `setup-*` actions and the Node.js downloads go.

```sh
jactionlint run ci --cache=read-only        # restore but never save
jactionlint run ci --no-cache               # no cache service at all
jactionlint cache ls                        # the entries of this repository (--all: every repository)
jactionlint cache rm 'rust-*'               # remove by key or prefix
jactionlint cache prune                     # expire entries and apply the size limit
jactionlint cache path
```

The service is reachable with `--net=open` and `--net=proxy`; with `--net=none` steps cannot reach it, so nothing is cached.

**A native `actions/cache`** is available with `--shim actions/cache`: it needs no Node.js and the runner itself writes and reads its archives (gzip, with entries under the workspace and the home only, so a poisoned entry cannot write anywhere else). It has the inputs and outputs of the real action, including `actions/cache/restore` and `actions/cache/save`, and saves in a post step when the job succeeded and the primary key missed. Its entries have a version of their own and are not shared with the real action's.

## Restricting the network

`--net=open` (the default) does not restrict the network, which keeps `npm install` and `cargo build` working, but a hostile step can then send away whatever it can read. `--net=proxy` is the strict mode: on Linux with Landlock ABI 4 (kernel 6.7) or newer, a step may connect only to the runner's proxy and the cache service, UDP and raw sockets are denied, and tools that ignore `HTTP_PROXY` fail. The proxy lets through the hosts GitHub needs and the common package registries (npm, PyPI, crates.io, the Go proxy, RubyGems, Maven Central, nodejs.org, mise.jdx.dev), refuses the rest with a message, never connects to loopback or private addresses (unless you named the host), and lists the refused hosts at the end of the run.

```sh
jactionlint run ci --learn                         # nothing refused; prints the hosts the steps contacted
jactionlint run ci --net=proxy --allow-net registry.example.com,cdn.example.com
```

`--learn` is how to move from `open` to `proxy`: it prints the ready-to-paste `--allow-net` list. `--net=none` denies internet sockets altogether; unix sockets keep working.

## Isolation

Third-party actions and the scripts of the repository under test run as you, so they could read `~/.ssh`, `~/.aws` or your cloud credentials, write outside the repository, or send files away. `jactionlint run` therefore runs every step in a sandbox that the operating system enforces, and it refuses to run a step when it cannot apply the sandbox. There is no container engine and no image to pull.

`jactionlint run --check-sandbox` says what this machine supports and tests it:

```console
$ jactionlint run --check-sandbox
Linux: Landlock ABI 10, seccomp-bpf
ok: a command can write to its workspace
ok: a command cannot read a file in your home directory that is outside the workspace
```

What a step gets:

- **A scrubbed environment.** Only `PATH`, `LANG`, `LC_*`, `TERM`, `TZ`, the variables the runner sets (`GITHUB_*`, `RUNNER_*`, `CI`) and the step's own `env:` reach it. Your `AWS_*`, `GITHUB_TOKEN` and `SSH_AUTH_SOCK` do not. No token is injected: `secrets.*` and `github.token` are empty unless you supply them.
- **A throwaway home.** `HOME`, `XDG_*` and `TMPDIR` point into `.jactionlint/run/<id>/`. Your real home directory cannot be read or written.
- **A copy of the workspace.** Steps work in `.jactionlint/run/<id>/work`: the tracked files and the untracked files that git does not ignore, plus `.git` (cloned copy-on-write where the file system can, copied otherwise, never hard-linked). Your checkout is not touched.
- **Files it may read and run:** the system directories (`/usr`, `/bin`, `/lib*`, `/etc`, `/opt`, `/nix`, `/snap`), the directories on `PATH`, and the tool installs of mise, asdf, rustup, nvm, pyenv, sdkman and volta under your home (`~/.local/share/mise/installs` and similar).
- **Files it may write:** the workspace copy, the fake home, the temp directories, the tool cache, `/tmp` and `/var/tmp`.

### Linux

The filesystem rules are Landlock (Linux 5.13 or newer; the newest ABI the kernel offers is used), applied with seccomp-bpf by a helper process that the runner starts from its own binary (`/proc/self/exe __sandbox-exec`) just before the step, so the step cannot undo it. The step gets `no_new_privs` (`sudo` and setuid programs do not work), and these system calls fail with `EPERM`: `ptrace`, `process_vm_readv/writev`, `pidfd_getfd`, `kcmp`, `bpf`, `perf_event_open`, `userfaultfd`, `io_uring_*`, `keyctl`, `add_key`, `request_key`, `kexec_*`, `reboot`, module loading, the mount family, `unshare`, `setns`, `swapon`, `swapoff`, `acct`, `syslog`, `sethostname` and `setdomainname`. Calls made through another ABI (x32) are refused.

Limits you should know about:

- `/proc` is not open as a whole, because `/proc/<pid>/environ` of your other processes would hold your secrets. A step can read `/proc/cpuinfo`, `/proc/meminfo`, `/proc/stat` and the like, and the `/proc/<pid>` entry of the process the sandbox started. A child process of the step cannot read its own `/proc/self`, so `ps`, `pgrep` and `lsof` do not work.
- `/tmp` and `/var/tmp` are shared with the host, `/etc` is readable, and a file system rule does not hide a path from `stat`.
- Connecting to a unix socket in an allowed directory (such as an `ssh-agent` socket in `/tmp`) is restricted only from Landlock ABI 9 (Linux 6.15); signals and abstract unix sockets of other processes only from ABI 6 (Linux 6.12). Older kernels still run, and `--check-sandbox` names what is missing.
- A process that leaves the process group of the step (`setsid`) is not killed when the step times out.
- Hosts that disable Landlock (`lsm=` without `landlock`) or run on a kernel older than 5.13 cannot run steps in the default mode. The runner says so and does not fall back; `--sandbox=off` is the explicit opt-out.

### macOS

The sandbox is a Seatbelt profile that `sandbox-exec` applies. It starts from "allow everything" and closes what a step must not touch: nothing is written except the workspace copy, the fake home, the temp directories and the tool cache; the contents of `/Users` (every home directory, with `~/.ssh`, `~/.aws` and the user keychain), `/Volumes` and the system keychains cannot be read, and their directories cannot be listed, except for the directories the step may use; unix sockets outside the allow list (ssh-agent, `docker.sock`) cannot be connected to; `--net=none` denies IP sockets and `--net=proxy` allows only the ports of the proxy and the cache service. A profile that denies everything first has to name every Mach service and system file that a dynamically linked program needs, and a missing one makes `bash` abort at start; the price of the shape used here is that a place the profile does not name, such as `/etc`, is readable, as on Linux.

The macOS backend is new: the profile text is unit-tested everywhere and the escape test runs on macOS CI, but it has had far less use than the Linux one.

### Windows

Not enforced yet: the runner refuses to run steps. Use WSL2, where the Linux sandbox applies.
