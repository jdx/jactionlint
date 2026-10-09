jactionlint
================
[![CI Status][ci-badge]][ci]
[![API Document][apidoc-badge]][apidoc]

[jactionlint][repo] is a static checker for GitHub Actions workflow files and an actively maintained fork of
[rhysd/actionlint][upstream], with upstream pull requests and fixes merged here. [Try it online!][playground]

Features:

- **Syntax check for workflow files** to check unexpected or missing keys following [workflow syntax][syntax-doc]
- **Strong type check for `${{ }}` expressions** to catch several semantic errors like access to not existing property,
  type mismatches, ...
- **Actions usage check** to check that inputs at `with:` and outputs in `steps.{id}.outputs` are correct
- **Reusable workflow check** to check inputs/outputs/secrets of reusable workflows and workflow calls
- **[shellcheck][] and [pyflakes][] integrations** for scripts at `run:`
- **Security and policy checks**; [script injection][script-injection-doc] by untrusted inputs, unpinned actions, excessive
  permissions, dangerous triggers, cache poisoning, hard-coded credentials, missing timeouts, ... Covers much of what
  [zizmor][zizmor] audits, with [rules and fixes of its own](docs/zizmor-parity.md)
- **Composite actions and `dependabot.yml`** are checked together with the workflows
- **Profiles** (`correctness`, `default`, `pedantic`) to choose how much is checked, and [the checks of actionlint][from-actionlint] with one line
- **Automatic fixes** (`--fix`, `--diff`), a **baseline** to adopt the checks step by step, **durable ignores** that survive Renovate, and
  SARIF, GCC and GitHub output formats
- **Online checks** (opt-in, `--online`) for impostor commits, known vulnerable actions, archived repositories and stale refs
- **Other several useful checks**; [glob syntax][filter-pattern-doc] validation, dependencies check for `needs:`,
  runner label validation, cron syntax validation, ...

See the [full list][checks] of checks done by jactionlint.

<img src="https://github.com/rhysd/ss/blob/master/actionlint/main.gif?raw=true" alt="jactionlint reports 7 errors" width="806" height="492"/>

**Example of broken workflow:**

```yaml
on:
  push:
    branch: main
    tags:
      - 'v\d+'
jobs:
  test:
    strategy:
      matrix:
        os: [macos-latest, linux-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - run: echo "Checking commit '${{ github.event.head_commit.message }}'"
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node_version: 18.x
      - uses: actions/cache@v4
        with:
          path: ~/.npm
          key: ${{ matrix.platform }}-node-${{ hashFiles('**/package-lock.json') }}
        if: ${{ github.repository.permissions.admin == true }}
      - run: npm install && npm test
```

**jactionlint reports 7 errors:**

```
test.yaml:3:5: unexpected key "branch" for "push" section. expected one of "branches", "branches-ignore", "paths", "paths-ignore", "tags", "tags-ignore", "types", "workflows" [workflow-syntax]
  |
3 |     branch: main
  |     ^~~~~~~
test.yaml:5:11: character '\' is invalid for branch and tag names. only special characters [, ?, +, *, \, ! can be escaped with \. see `man git-check-ref-format` for more details. note that regular expression is unavailable. note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [invalid-glob]
  |
5 |       - 'v\d+'
  |           ^~~~
test.yaml:10:28: label "linux-latest" is unknown. available labels are "windows-latest", "windows-latest-8-cores", "windows-2025", "windows-2025-vs2026", windows-2022", "windows-11-arm", "windows-11-vs2026-arm", "ubuntu-slim", "ubuntu-latest", "ubuntu-latest-4-cores", "ubuntu-latest-8-cores", "ubuntu-latest-16-cores", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-22.04", "ubuntu-22.04-arm", "macos-latest", "macos-latest-xlarge", "macos-latest-large", "macos-26-intel", "macos-26-xlarge", "macos-26-large", "macos-26", "macos-15-intel", "macos-15-xlarge", "macos-15-large", "macos-15", "macos-14-xlarge", "macos-14-large", "macos-14", "xcode-27", "xcode-27-xlarge", "self-hosted", "x64", "arm", "arm64", "linux", "macos", "windows". if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file [unknown-runner-label]
   |
10 |         os: [macos-latest, linux-latest]
   |                            ^~~~~~~~~~~~~
test.yaml:13:41: "github.event.head_commit.message" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
13 |       - run: echo "Checking commit '${{ github.event.head_commit.message }}'"
   |                                         ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:11: input "node_version" is not defined in action "actions/setup-node@v4". available inputs are "always-auth", "architecture", "cache", "cache-dependency-path", "check-latest", "node-version", "node-version-file", "registry-url", "scope", "token" [unknown-action-input]
   |
17 |           node_version: 18.x
   |           ^~~~~~~~~~~~~
test.yaml:21:20: property "platform" is not defined in object type {os: string} [undefined-property]
   |
21 |           key: ${{ matrix.platform }}-node-${{ hashFiles('**/package-lock.json') }}
   |                    ^~~~~~~~~~~~~~~
test.yaml:22:17: receiver of object dereference "permissions" must be type of object but got "string" [expression-type]
   |
22 |         if: ${{ github.repository.permissions.admin == true }}
   |                 ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

## Quick start

Install `jactionlint` command with [mise][mise] (recommended), by downloading [the released binary][releases], by Homebrew or by
`go install`. See [the installation document][install] for more details like how to manage the command with several package
managers or run via Docker container.

```sh
# With mise
mise use -g jactionlint

# Or with Go
go install github.com/jdx/jactionlint/v2/cmd/jactionlint@latest
```

`mise use jactionlint` without `-g` adds it to the `mise.toml` of the current project, so everyone working on the project and CI
use the same version. `mise upgrade jactionlint` updates it. Then run `jactionlint` in your repository. It finds the workflows,
the composite actions and `dependabot.yml` and checks them:

```sh
jactionlint
```

### Choose how much is checked

jactionlint organizes its checks in three tiers (correctness, security and policy, see [the contributing guide](CONTRIBUTING.md#policy-for-jactionlints-features))
and three profiles that each include the one before it. The profile decides what runs:

| Profile       | What it checks                                                                                                           |
| ------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `correctness` | What actionlint checks, plus jactionlint's bug detectors. No opinion about security or style.                           |
| `default`     | **What runs when you configure nothing.** `correctness` plus the security and policy rules worth failing a build on.     |
| `pedantic`    | `default` plus noisy and opinionated rules.                                                                              |

The default is stricter than a plain linter on purpose, so a repository that never configured anything gets the security checks too,
and it can fail on its first run. To get only the checks of actionlint, see [Coming from actionlint][from-actionlint]:

```sh
jactionlint --profile correctness
```

or put `profile: correctness` in `.github/jactionlint.yaml`. To adopt the `default` profile step by step, `jactionlint --baseline-write`
records today's findings and `jactionlint --baseline` fails only on new ones ([baseline](docs/usage.md#baseline)). `jactionlint --fix`
fixes what can be fixed mechanically. See [the configuration document][config] and [the migration guide](docs/v2-migration.md) if you used v1.

### In CI with mise

[`jdx/mise-action`](https://github.com/jdx/mise-action) installs the version in `mise.toml`, so CI runs what developers run. Pin the
actions by commit SHA:

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
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
      - run: jactionlint
```

### With hk

[hk](https://hk.jdx.dev) runs jactionlint as a Git hook and fixer. With `--format sarif` it gets the diagnostics with their rule IDs and
the fixes. Add this step to `hk.pkl` (see [the usage document][usage-hk] for baselines and the online checks):

```pkl
["jactionlint"] {
    glob = List(".github/workflows/*.yml", ".github/workflows/*.yaml")
    batch = true
    diagnostic_format = "sarif"
    check = "jactionlint --format sarif {{files}}"
    check_diff = "hk util sarif-diff -- jactionlint --format sarif {{files}}"
    fix = "jactionlint --fix {{files}}"
}
```

Another option to try jactionlint is [the online playground][playground]. Your browser can run jactionlint through WebAssembly.

See [the usage document][usage] for more details.

## Documents

- [Checks][checks]: Full list of all checks done by jactionlint with example inputs, outputs, and playground links.
- [Installation][install]: Installation instructions. mise (recommended), prebuilt binaries, a Docker image, building from source, a
  download script (for CI), supports by several package managers are available.
- [Usage][usage]: How to use `jactionlint` command locally or on GitHub Actions, the online playground, an official Docker image,
  and integrations with reviewdog, Problem Matchers, super-linter, pre-commit, VS Code.
- [Rules][rules]: Every rule ID with its group, default level and profile.
- [Configuration][config]: How to configure jactionlint behavior: profiles, the level of each rule, runner labels, configuration
  variables, ignores (by rule, by place, with an expiry), the baseline and the online options.
- [Coming from actionlint][from-actionlint]: How to get the checks of actionlint with `profile: correctness`, and what jactionlint adds.
- [Migrating to v2][migration]: What changed since v1, and what to do about it.
- [jactionlint and zizmor][zizmor-parity]: Where jactionlint stands relative to zizmor, audit by audit.
- [Go API][api]: How to use jactionlint as Go library.
- [References][refs]: Links to resources.

## Bug reporting

When you see some bugs or false positives, it is helpful to [file a new issue][issue-form] with a minimal example
of input. Giving me some feedbacks like feature requests or ideas of additional checks is also welcome.

See the [contribution guide](./CONTRIBUTING.md) for more details.

## License

jactionlint is distributed under [the MIT license](./LICENSE.txt).

[ci-badge]: https://github.com/jdx/jactionlint/actions/workflows/ci.yaml/badge.svg
[ci]: https://github.com/jdx/jactionlint/actions/workflows/ci.yaml
[apidoc-badge]: https://pkg.go.dev/badge/github.com/jdx/jactionlint/v2.svg
[apidoc]: https://pkg.go.dev/github.com/jdx/jactionlint/v2
[repo]: https://github.com/jdx/jactionlint
[mise]: https://mise.jdx.dev/
[upstream]: https://github.com/rhysd/actionlint
[playground]: https://jactionlint.jdx.dev/
[shellcheck]: https://github.com/koalaman/shellcheck
[pyflakes]: https://github.com/PyCQA/pyflakes
[syntax-doc]: https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions
[filter-pattern-doc]: https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet
[script-injection-doc]: https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks
[releases]: https://github.com/jdx/jactionlint/releases
[checks]: https://github.com/jdx/jactionlint/blob/main/docs/checks.md
[install]: https://github.com/jdx/jactionlint/blob/main/docs/install.md
[usage]: https://github.com/jdx/jactionlint/blob/main/docs/usage.md
[config]: https://github.com/jdx/jactionlint/blob/main/docs/config.md
[rules]: https://github.com/jdx/jactionlint/blob/main/docs/rules.md
[migration]: https://github.com/jdx/jactionlint/blob/main/docs/v2-migration.md
[zizmor-parity]: https://github.com/jdx/jactionlint/blob/main/docs/zizmor-parity.md
[zizmor]: https://docs.zizmor.sh/
[usage-hk]: https://github.com/jdx/jactionlint/blob/main/docs/usage.md#hk
[from-actionlint]: https://github.com/jdx/jactionlint/blob/main/docs/actionlint.md
[api]: https://github.com/jdx/jactionlint/blob/main/docs/api.md
[refs]: https://github.com/jdx/jactionlint/blob/main/docs/reference.md
[issue-form]: https://github.com/jdx/jactionlint/issues/new
