# jactionlint

[![CI status](https://github.com/jdx/jactionlint/actions/workflows/ci.yaml/badge.svg)](https://github.com/jdx/jactionlint/actions/workflows/ci.yaml)
[![Go reference](https://pkg.go.dev/badge/github.com/jdx/jactionlint/v2.svg)](https://pkg.go.dev/github.com/jdx/jactionlint/v2)

jactionlint is a static checker for GitHub Actions workflow files, composite actions and `dependabot.yml`. It is a fork of [rhysd/actionlint](https://github.com/rhysd/actionlint) that merges the pull requests of upstream and adds security and policy checks, automatic fixes and a baseline. Try it in the [playground](https://jactionlint.jdx.dev/playground) without installing anything.

## What it checks

- [Syntax and schema](https://jactionlint.jdx.dev/checks#check-unexpected-keys): unexpected or missing keys, event names, glob patterns, runner labels, cron syntax and `needs:` dependencies.
- [Typed expressions](https://jactionlint.jdx.dev/checks#check-type-check-expression): `${{ }}` expressions are type-checked, so an unknown property or a type mismatch is a finding.
- [Action inputs](https://jactionlint.jdx.dev/checks#check-popular-action-inputs) and [reusable workflows](https://jactionlint.jdx.dev/checks#check-reusable-workflows): the `with:` inputs and `steps.<id>.outputs` of actions, and the inputs, outputs and secrets of reusable workflows.
- [Scripts](https://jactionlint.jdx.dev/checks#check-shellcheck-integ): the shell scripts of `run:` go through shellcheck and the Python scripts (`shell: python`) through pyflakes, which you install yourself.
- [Security and policy](https://jactionlint.jdx.dev/rules): untrusted input in scripts, unpinned actions, excessive permissions, dangerous triggers, cache poisoning, hard-coded credentials and missing timeouts. [jactionlint and zizmor](https://jactionlint.jdx.dev/zizmor-parity) compares these rules with zizmor.
- [Online checks](https://jactionlint.jdx.dev/online): opt-in checks of the actions a workflow uses against the GitHub API. Without them, jactionlint does not use the network.

## Example

Save this workflow as `test.yaml`. It has a typo in a trigger, an untrusted value in a script, an undefined matrix property, an unpinned action, and no permissions, timeout or concurrency group:

<!-- demo:input -->
```yaml
on:
  push:
    branch: main
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo "${{ github.event.head_commit.message }}"
      - run: echo "${{ matrix.os }}"
```

Run `jactionlint test.yaml`. The default profile, which applies when you configure nothing, reports these findings (without colors):

<!-- demo:output -->
```
test.yaml:1:1: workflow has no "concurrency:", so every run of it executes at the same time even when a newer run supersedes the older ones. add a top-level "concurrency:" with a "group:" and "cancel-in-progress: true" [concurrency-limits]
  |
1 | on:
  | ^~~
test.yaml:3:5: unexpected key "branch" for "push" section. expected one of "branches", "branches-ignore", "paths", "paths-ignore", "tags", "tags-ignore", "types", "workflows" [workflow-syntax]
  |
3 |     branch: main
  |     ^~~~~~~
test.yaml:5:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set "default-minutes" in the "missing-timeout" rule options of the configuration to turn on the --fix fix [missing-timeout]
  |
5 |   test:
  |   ^~~~~
test.yaml:5:3: neither the workflow nor this job sets "permissions:" so the GITHUB_TOKEN gets the default permissions of the repository. set "permissions:" at workflow-level or job-level (use "permissions: {}" for no permissions) because the "missing-permissions" rule is enabled [missing-permissions]
  |
5 |   test:
  |   ^~~~~
test.yaml:8:15: action "actions/checkout@v4" must be pinned to a full-length commit SHA like "{owner}/{repo}@{sha}" because the "unpinned-uses" rule is enabled [unpinned-uses]
  |
8 |       - uses: actions/checkout@v4
  |               ^~~~~~~~~~~~~~~~~~~
test.yaml:8:15: actions/checkout leaves the GITHUB_TOKEN in the git config of the workspace, where a later step such as an artifact upload can publish it. set "persist-credentials: false" under "with:" unless a later step needs to push [artipacked]
  |
8 |       - uses: actions/checkout@v4
  |               ^~~~~~~~~~~~~~~~~~~
test.yaml:9:24: "github.event.head_commit.message" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
  |
9 |       - run: echo "${{ github.event.head_commit.message }}"
  |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:24: property "os" is not defined in object type {} [undefined-property]
   |
10 |       - run: echo "${{ matrix.os }}"
   |                        ^~~~~~~~~
```

`jactionlint --profile correctness test.yaml` reports only `workflow-syntax`, `template-injection` and `undefined-property`. `jactionlint --diff test.yaml` prints the safe fixes of `artipacked`, `missing-permissions` and `template-injection` as a diff and writes nothing. The [playground](https://jactionlint.jdx.dev/playground) turns off the default rules that a minimal example breaks, so it reports the findings of the `correctness` profile for this workflow.

<a id="quick-start"></a>
## Install

```sh
mise use -g jactionlint
jactionlint --version
```

Without `-g`, `mise use jactionlint` records the tool in the `mise.toml` of the current project. The other methods:

- Download an archive for your platform from the [releases](https://github.com/jdx/jactionlint/releases). It holds the executable, the man page and the docs.
- Install the Homebrew cask: `brew tap jdx/jactionlint https://github.com/jdx/jactionlint`, then `brew install --cask jdx/jactionlint/jactionlint`.
- Run the [download script](https://jactionlint.jdx.dev/install#download-script), which downloads the archive of your platform and extracts the executable. It does not verify the archive.
- Pull the Docker image `ghcr.io/jdx/jactionlint`, which also holds shellcheck and pyflakes.
- Build it with Go: `go install github.com/jdx/jactionlint/v2/cmd/jactionlint@latest`.

Distribution packages named `actionlint` install the original project, not this fork. These docs describe the main branch: `jactionlint --help` shows what the installed version supports. [Installation](https://jactionlint.jdx.dev/install) has the details and how to verify a download.

## Run it

```sh
jactionlint
```

With no arguments, jactionlint checks the workflows, composite actions, `dependabot.yml` and Actions lock file of the repository ([what a run checks](https://jactionlint.jdx.dev/usage#what-a-run-checks)). Run it in a Git or Jujutsu repository that has a `.github/workflows` directory: outside a repository it exits with status 3. Pass file names to check only those, wherever they are (for example the `test.yaml` above), or `-` to read standard input.

- The exit status is `0` for success, `1` when a finding of level `error` is left, `2` for an invalid command line and `3` for a failure. [Exit status](https://jactionlint.jdx.dev/cli#exit-status) lists every cause.
- The name in brackets at the end of a finding is its rule ID. `jactionlint --explain unpinned-uses` prints what the rule checks, its group, profile, level, options and fix. These pages describe the development version, so a release can lack an option such as `--explain`: when `jactionlint --help` of your install does not list an option, [build the main branch](https://jactionlint.jdx.dev/install#build-from-source) with `go install github.com/jdx/jactionlint/v2/cmd/jactionlint@main`.
- To silence a finding you accept, put `# jactionlint ignore=unpinned-uses` on the line above it ([Ignore findings](https://jactionlint.jdx.dev/ignore)).
- shellcheck and pyflakes are separate programs. When one is missing, jactionlint prints a note and skips the scripts it would have checked, and the missing program does not fail the run.
- `jactionlint --init-config` writes `.github/jactionlint.yaml` ([Configuration](https://jactionlint.jdx.dev/config)).

<a id="choose-how-much-is-checked"></a>
## Choose a profile

| Profile | Checks |
| ------- | ------ |
| `correctness` | What actionlint checks and the bug detectors of jactionlint. It has almost no security posture or policy rules. |
| `default` | `correctness` plus the security and policy rules worth failing a build on. It applies when you configure nothing. |
| `pedantic` | `default` plus the noisy and opinionated rules. |

Choose a profile with `--profile` or with `profile:` in the config file. The default profile can fail on the first run in a repository that never followed it. When a text run on a terminal or in CI reports many findings and applies no baseline, jactionlint prints a hint about the baseline; `--no-hints` turns it off.

To adopt the default profile step by step, fix what `--profile correctness` reports first, because those findings are mistakes and not debt to record. Then record the findings that are left and apply the record. A plain run applies the baseline only when the config file sets `baseline: auto`; `--baseline` applies it to one run.

```sh
# the checks of actionlint plus the bug detectors: fix what this reports first
jactionlint --profile correctness

# then record the findings of the default profile that are left, and apply the record
jactionlint --baseline-write
jactionlint --baseline
```

See [Profiles](https://jactionlint.jdx.dev/config#profiles), [Adopt new checks with a baseline](https://jactionlint.jdx.dev/baseline#write-and-apply-the-baseline), [Coming from actionlint](https://jactionlint.jdx.dev/actionlint) and [Migrating to v2](https://jactionlint.jdx.dev/v2-migration).

<a id="in-ci-with-mise"></a>
## Use it in CI

[`jdx/mise-action`](https://github.com/jdx/mise-action) installs the tools of the `mise.toml` of the repository, so add jactionlint with `mise use jactionlint` first. The step fails on a finding of level `error`. This workflow reports nothing under the default profile:

```yaml
name: Lint GitHub Actions workflows
on:
  push:
    branches: [main]
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
      - run: jactionlint
```

[Run jactionlint in CI](https://jactionlint.jdx.dev/ci#the-action) also covers the jactionlint action, which annotates pull requests or reports to code scanning, the [Docker image](https://jactionlint.jdx.dev/ci#docker) and [problem matchers](https://jactionlint.jdx.dev/ci#problem-matchers). [Git hooks and editors](https://jactionlint.jdx.dev/hooks) covers [hk](https://jactionlint.jdx.dev/hooks#hk) and [pre-commit](https://jactionlint.jdx.dev/hooks#pre-commit).

<a id="documents"></a>
## Documentation

The site [jactionlint.jdx.dev](https://jactionlint.jdx.dev) renders the pages in `docs/`, and the release archives hold the same files. Start with these pages:

| Page | Read it when |
| ---- | ------------ |
| [Installation](https://jactionlint.jdx.dev/install) | you choose how to install jactionlint (mise, Homebrew, a release archive, the download script, Docker or Go) and check that it works. |
| [Run jactionlint](https://jactionlint.jdx.dev/usage) | you run jactionlint on a repository, files or stdin, choose a profile, explain a rule or set up shellcheck, pyflakes and completion. |
| [Ignore findings](https://jactionlint.jdx.dev/ignore) | you accept a finding and silence it with a comment, an option, the config file or a zizmor comment. |
| [Adopt new checks with a baseline](https://jactionlint.jdx.dev/baseline) | you adopt a stricter profile before everything is fixed: record a baseline so that only new findings fail. |
| [Run jactionlint in CI](https://jactionlint.jdx.dev/ci) | you run jactionlint on GitHub Actions with the action, mise, Docker or the download script and show findings as annotations or code scanning alerts. |
| [Configuration](https://jactionlint.jdx.dev/config) | you create `.github/jactionlint.yaml` or look up a key, a profile or the level of a rule. |
| [Checks](https://jactionlint.jdx.dev/checks) | you hold a finding or a rule ID and want to know what is checked, with an example and the fix. |

The full list of pages is in [docs/README.md](docs/README.md).

<a id="bug-reporting"></a>
## Contributing and bugs

To report a bug, a false positive or an idea for a check, [open an issue](https://github.com/jdx/jactionlint/issues/new) with a minimal workflow that shows it. [CONTRIBUTING.md](CONTRIBUTING.md) explains how to build and test jactionlint and how to propose a change.

<a id="license"></a>
## License and acknowledgments

jactionlint is distributed under the [MIT license](LICENSE.txt). [NOTICE.md](NOTICE.md) names the projects it builds on, including actionlint, zizmor and ghlint.
