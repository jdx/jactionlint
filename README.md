<div align="center">

<a href="https://jactionlint.jdx.dev"><img src="https://jactionlint.jdx.dev/logo.svg" alt="" width="88" height="88"></a>

# jactionlint

**Catch broken and insecure GitHub Actions workflows before they run.**

[![CI status](https://github.com/jdx/jactionlint/actions/workflows/ci.yaml/badge.svg)](https://github.com/jdx/jactionlint/actions/workflows/ci.yaml)
[![Latest release](https://img.shields.io/github/v/release/jdx/jactionlint?label=release&color=0f766e)](https://github.com/jdx/jactionlint/releases/latest)
[![Go reference](https://pkg.go.dev/badge/github.com/jdx/jactionlint/v2.svg)](https://pkg.go.dev/github.com/jdx/jactionlint/v2)

[Documentation](https://jactionlint.jdx.dev) · [Playground](https://jactionlint.jdx.dev/playground) · [Rules](https://jactionlint.jdx.dev/rules) · [Changelog](https://github.com/jdx/jactionlint/blob/main/CHANGELOG.md)

</div>

jactionlint is a static checker for GitHub Actions. It reads your workflows, composite actions and `dependabot.yml` and reports two kinds of problems: the mistakes that make a run fail or misbehave, and the patterns that let an outsider run code in your repository or steal its token. It started as a fork of [actionlint](https://github.com/rhysd/actionlint) and covers its checks, and its security rules follow the audits of [zizmor](https://github.com/zizmorcore/zizmor).

- **Correctness.** Unknown keys and events, type errors in `${{ }}` expressions, wrong inputs of actions and reusable workflows, and broken `needs:`, cron and glob patterns. [shellcheck](https://github.com/koalaman/shellcheck) and [pyflakes](https://github.com/PyCQA/pyflakes) check the scripts of `run:` steps.
- **Security.** Script injection, excessive token permissions, dangerous triggers such as `pull_request_target`, cache poisoning, credentials left in the checkout and AI agent actions that outsiders can steer.
- **Policy.** Actions used by a tag instead of a commit, and workflows without `permissions:`, a job timeout or a concurrency group.
- **Fixes.** `jactionlint --fix` repairs what has a mechanical fix, and `--diff` shows the change first.
- **Gradual adoption.** Profiles choose how much is checked, and a baseline fails the build only on new findings.
- **Local runs.** `jactionlint run ci` runs the jobs of a workflow on your machine, every step in an OS sandbox, with a local actions cache and no container engine unless the workflow asks for one. See [Run workflows locally](https://jactionlint.jdx.dev/run).
- **No network unless you ask.** The [online checks](https://jactionlint.jdx.dev/online), which ask GitHub about archived repositories, impostor commits and published advisories, run only with `--online`.

Try it without installing anything in the [playground](https://jactionlint.jdx.dev/playground), which runs jactionlint in your browser.

<a id="quick-start"></a>
## Quick start

Install jactionlint with [mise](https://mise.jdx.dev) and run it at the root of a repository:

```sh
mise use -g jactionlint
jactionlint
```

With no arguments, jactionlint checks the workflows, composite actions, `dependabot.yml` and Actions lock file of the repository. Pass file names to check only those, or `-` to read standard input. It exits with status 1 when a finding of level `error` is left.

Other ways to install it:

- **Homebrew:** `brew tap jdx/jactionlint https://github.com/jdx/jactionlint`, then `brew install --cask jdx/jactionlint/jactionlint`.
- **PyPI:** `uvx jactionlint` runs it without installing it, and `pipx install jactionlint` or `uv tool install jactionlint` installs it.
- **Docker:** the image `ghcr.io/jdx/jactionlint` also holds shellcheck and pyflakes. [Run the image](https://jactionlint.jdx.dev/ci#run-the-image) shows the command.
- **Go:** `go install github.com/jdx/jactionlint/v2/cmd/jactionlint@latest`.
- **Release archives:** download the archive for your platform from the [releases](https://github.com/jdx/jactionlint/releases) and [verify it](https://jactionlint.jdx.dev/install#verify-the-download).

Packages named `actionlint`, as in Homebrew core, install the original project, not this fork. [Installation](https://jactionlint.jdx.dev/install) covers every method.

## Example

This workflow has a typo in a trigger, untrusted input in a script and an undefined matrix property. It also uses an action by tag and sets no permissions, timeout or concurrency group. Save it as `test.yaml`:

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

`jactionlint test.yaml` prints each finding with its position, a message, the line it points at and, in brackets, the ID of its rule:

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

`jactionlint --explain template-injection` prints what a rule checks, why it matters and how to fix it. With `--profile correctness`, jactionlint reports only `workflow-syntax`, `template-injection` and `undefined-property`. The other five findings come from the security and policy rules of the default profile, which applies when you configure nothing.

`jactionlint --diff test.yaml` previews the safe fixes, and `jactionlint --fix test.yaml` applies them. Here they stop `actions/checkout` from persisting the token, give the token read-only permissions and pass the untrusted message through an environment variable:

<!-- demo:diff -->
```diff
--- a/test.yaml
+++ b/test.yaml
@@ -1,10 +1,17 @@
 on:
   push:
     branch: main
+
+permissions:
+  contents: read
 jobs:
   test:
     runs-on: ubuntu-latest
     steps:
       - uses: actions/checkout@v4
-      - run: echo "${{ github.event.head_commit.message }}"
+        with:
+          persist-credentials: false
+      - run: echo "${HEAD_COMMIT_MESSAGE}"
+        env:
+          HEAD_COMMIT_MESSAGE: ${{ github.event.head_commit.message }}
       - run: echo "${{ matrix.os }}"
```

<a id="in-ci-with-mise"></a>
## Run it in CI

The jactionlint action installs jactionlint, checks the repository and annotates the pull request with the findings. This workflow passes the default profile itself:

```yaml
name: jactionlint
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
      - uses: jdx/jactionlint@baa8d685e2b27ff5821558c9eeabb2051c1da61b # v2.4.0
```

Set `advanced-security: true` under `with:` to upload the findings to code scanning instead, which needs `security-events: write`. [Run jactionlint in CI](https://jactionlint.jdx.dev/ci) also covers mise, which verifies the download, the Docker image and problem matchers. [Git hooks and editors](https://jactionlint.jdx.dev/hooks) covers hk and pre-commit.

<a id="choose-how-much-is-checked"></a>
## Adopt it on an existing repository

The default profile can fail the first run on a repository that never followed it. Choose how much is checked with `--profile`, or with `profile:` in `.github/jactionlint.yaml`:

| Profile | What it checks |
| --- | --- |
| `correctness` | The checks of actionlint and the bug detectors of jactionlint. |
| `default` | `correctness`, plus the security and policy rules worth failing a build on. It applies when you configure nothing. |
| `pedantic` | `default`, plus the noisier and opinionated rules. |

Fix what the `correctness` profile reports first, because those findings are mistakes, not debt. Then record what the default profile still finds, and let the build fail only on new findings:

```sh
jactionlint --profile correctness   # fix these first
jactionlint --baseline-write        # record the findings that are left
jactionlint --baseline              # fail only on new findings
```

Silence a finding that you accept with a comment such as `# jactionlint ignore=unpinned-uses` on the line above it. `jactionlint --init-config` writes a config file to start from. See [Profiles](https://jactionlint.jdx.dev/config#profiles), [Adopt new checks with a baseline](https://jactionlint.jdx.dev/baseline) and [Ignore findings](https://jactionlint.jdx.dev/ignore).

## Coming from actionlint or zizmor

- **From actionlint.** jactionlint reads the config file of actionlint. `--profile correctness` keeps the checks that you have, and the default profile adds the security and policy rules when you are ready for them. [Coming from actionlint](https://jactionlint.jdx.dev/actionlint) lists the renamed options and the differences.
- **From zizmor.** Each audit of zizmor 1.30.1 has a rule, under the same ID in every case but one, and `# zizmor: ignore[...]` comments keep working. [jactionlint and zizmor](https://jactionlint.jdx.dev/zizmor-parity) maps the audits, the personas and the settings of `zizmor.yml`.

<a id="documents"></a>
## Documentation

[jactionlint.jdx.dev](https://jactionlint.jdx.dev) has the full documentation, and the release archives hold the same pages. These are the ones to start with:

- [Installation](https://jactionlint.jdx.dev/install) and [Run jactionlint](https://jactionlint.jdx.dev/usage): install it, run it and read a finding.
- [Checks](https://jactionlint.jdx.dev/checks) and [Rules](https://jactionlint.jdx.dev/rules): what each rule finds, with an example and the fix.
- [Fix findings automatically](https://jactionlint.jdx.dev/fix): safe and unsafe fixes, and why a finding has none.
- [Configuration](https://jactionlint.jdx.dev/config): `.github/jactionlint.yaml`, profiles and rule levels.
- [Go API](https://jactionlint.jdx.dev/api): use jactionlint from a Go program.

The site documents the `main` branch. `jactionlint --help` lists what your installed version supports.

<a id="bug-reporting"></a>
## Contributing

To report a bug, a false positive or an idea for a check, [open an issue](https://github.com/jdx/jactionlint/issues/new) with a minimal workflow that shows it. [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md) explains how to build and test jactionlint and how to propose a change.

<a id="license"></a>
## License and acknowledgments

jactionlint is distributed under the [MIT license](https://github.com/jdx/jactionlint/blob/main/LICENSE.txt). It is a fork of [actionlint](https://github.com/rhysd/actionlint) by [rhysd](https://github.com/rhysd), and its security rules follow the audits of [zizmor](https://github.com/zizmorcore/zizmor). [NOTICE.md](https://github.com/jdx/jactionlint/blob/main/NOTICE.md) names every project that it builds on.
