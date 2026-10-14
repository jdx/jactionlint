---
layout: home
title: Static checker for GitHub Actions workflow files
description: jactionlint is a static checker for GitHub Actions workflow files, composite actions and dependabot.yml. It checks syntax and expressions, validates action inputs and reusable workflows, runs shellcheck and pyflakes on scripts, and reports security and policy findings.

hero:
  name: jactionlint
  text: Static checker for GitHub Actions workflow files
  image:
    src: /logo.svg
    alt: jactionlint
  tagline: A fork of rhysd/actionlint that merges its pull requests and adds security and policy checks, automatic fixes and a baseline.
  actions:
    - theme: brand
      text: Get started
      link: /install
    - theme: alt
      text: Try the playground
      link: /playground
    - theme: alt
      text: View on GitHub
      link: https://github.com/jdx/jactionlint

features:
  - title: Syntax check
    details: 'Reports unexpected or missing keys, event names, glob patterns, runner labels, cron syntax and <code>needs:</code> dependencies.'
    link: /checks#check-unexpected-keys
  - title: Typed expressions
    details: 'Type-checks <code>${{ }}</code> expressions and reports unknown properties and type mismatches.'
    link: /checks#check-type-check-expression
  - title: Action inputs
    details: 'Checks the <code>with:</code> inputs and the <code>steps.{id}.outputs</code> of the actions a step uses.'
    link: /checks#check-popular-action-inputs
  - title: Reusable workflows
    details: Checks the inputs, outputs and secrets of reusable workflows and of the jobs that call them.
    link: /checks#check-reusable-workflows
  - title: shellcheck and pyflakes
    details: 'Runs shellcheck on the shell scripts of <code>run:</code> and pyflakes on the Python ones. You install both separately.'
    link: /checks#check-shellcheck-integ
  - title: Security and policy
    details: Reports script injection, unpinned actions, excessive permissions, dangerous triggers and hard-coded credentials.
    link: /rules
  - title: Profiles
    details: 'Choose how much is checked: <code>correctness</code> (what actionlint checks), <code>default</code> or <code>pedantic</code>.'
    link: /config#profiles
  - title: Fixes
    details: 'Preview the automatic fixes with <code>--diff</code> and apply the safe ones with <code>--fix</code>.'
    link: /fix
  - title: CI and hooks
    details: Run it as a GitHub Action, with mise or Docker, from hk or pre-commit, and in your editor.
    link: /ci
---

<div class="home-section">

## Install

[mise](https://mise.jdx.dev/) installs jactionlint from the GitHub releases of this repository.

```sh
mise use -g jactionlint
jactionlint --version
```

Run `jactionlint` in a Git repository that has workflow files to check its workflows, composite actions, `dependabot.yml` and Actions lock file.

```sh
jactionlint
```

[Installation](./install.md) lists the other methods. To try jactionlint first, use the [playground](./playground.md): your browser runs it through WebAssembly.

## Example of a broken workflow

This workflow has a typo in a trigger, an untrusted value in a script, an undefined matrix property, an unpinned action, and no permissions, timeout or concurrency group. Save it as `test.yaml`:

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

Run it on the file:

```sh
jactionlint test.yaml
```

<a id="jactionlint-reports-7-errors"></a>
## What jactionlint reports

The default profile applies when you configure nothing. Each finding shows its position, its message, a code frame and the rule ID in brackets:

<!-- demo:output -->
```text
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

`jactionlint --profile correctness test.yaml` reports only `workflow-syntax`, `template-injection` and `undefined-property`. The other findings come from the security and policy rules of the default profile. `jactionlint --diff test.yaml` prints the safe fixes of `artipacked`, `missing-permissions` and `template-injection` as a diff and writes nothing.

## Where to go next

| You want to | Read |
| ----------- | ---- |
| Run jactionlint on a repository and read the result | [Run jactionlint](./usage.md) |
| Understand a finding | [Explain a rule](./usage.md#explain-a-rule) and the [rules](./rules.md) |
| Preview and apply the automatic fixes | [Fix findings automatically](./fix.md) |
| Silence a finding on purpose | [Ignore findings](./ignore.md) |
| Adopt the default profile on an existing repository | [Adopt new checks with a baseline](./baseline.md) |
| Run it in CI | [Run jactionlint in CI](./ci.md) |
| Run it before each commit or from an editor | [Git hooks and editors](./hooks.md) |
| Set profiles, rule levels and runner labels | [Configuration](./config.md) |
| Switch from actionlint | [Coming from actionlint](./actionlint.md) |
| Compare it with zizmor | [jactionlint and zizmor](./zizmor-parity.md) |
| Use it from a Go program | [Go API](./api.md) |

</div>
