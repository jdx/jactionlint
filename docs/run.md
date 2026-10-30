# Run workflows locally

`jactionlint run` runs the jobs of a workflow on your machine, so that you can debug a `run:` script, an expression, a matrix or the `needs` wiring without pushing a commit. It needs no container engine and no runner image.

This page describes what the command does today. Each section says what is still missing.

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

The sandbox is a Seatbelt profile that `sandbox-exec` applies (`(deny default)` plus the same allow lists). The profile is generated and unit-tested on every platform; the macOS backend has not been run on a Mac yet, so treat it as unverified until the macOS CI job of the release lands.

### Windows

Not enforced yet: the runner refuses to run steps. Use WSL2, where the Linux sandbox applies.
