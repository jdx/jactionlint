---
layout: home
title: Static checker for GitHub Actions workflow files
description: jactionlint is a static checker for GitHub Actions workflow files. It checks syntax, type-checks ${{ }} expressions, validates action inputs and reusable workflows, runs shellcheck and pyflakes, and finds security problems.

hero:
  name: jactionlint
  text: Static checker for GitHub Actions workflow files
  image:
    src: /logo.svg
    alt: jactionlint
  tagline: An actively maintained fork of rhysd/actionlint. Catch mistakes in your workflows before they run, with as few false positives as possible.
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
    details: Checks workflow files for unexpected or missing keys following the workflow syntax.
    link: /checks#check-unexpected-keys
  - title: Strong type check for expressions
    details: 'Catches access to not existing properties, type mismatches and other semantic errors in <code>${{ }}</code> expressions.'
    link: /checks#check-type-check-expression
  - title: Actions usage check
    details: 'Checks that inputs at <code>with:</code> and outputs in <code>steps.{id}.outputs</code> are correct.'
    link: /checks#check-action-format
  - title: Reusable workflow check
    details: Checks inputs, outputs and secrets of reusable workflows and workflow calls.
    link: /checks
  - title: shellcheck and pyflakes
    details: 'Integrates with shellcheck and pyflakes for scripts at <code>run:</code>.'
    link: /checks#check-shellcheck-integ
  - title: Security and policy checks
    details: Detects script injection by untrusted inputs, unpinned actions, excessive permissions, dangerous triggers and hard-coded credentials.
    link: /checks#untrusted-inputs
  - title: Profiles
    details: 'Choose how much is checked: <code>correctness</code> (what actionlint checks), <code>default</code> or <code>pedantic</code>.'
    link: /config#profiles
  - title: Fixes and a baseline
    details: 'Apply the safe fixes with <code>--fix</code> and adopt the stricter checks step by step with a baseline.'
    link: /usage#fix-errors-automatically
  - title: Other useful checks
    details: 'Glob syntax validation, dependencies check for <code>needs:</code>, runner label validation, cron syntax validation and more.'
    link: /checks
---

<div class="home-section">

## Install

[mise](https://mise.jdx.dev/) installs jactionlint from the GitHub releases of this repository.

```sh
mise use -g jactionlint
jactionlint --version
```

Then run it in your repository. jactionlint finds all workflow files and checks them.

```sh
jactionlint
```

Other ways to install are described in the [installation document](/install). You can also try it in the
[online playground](/playground); your browser runs jactionlint through WebAssembly.

## Example of a broken workflow

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

## jactionlint reports 7 errors

```text
test.yaml:3:5: unexpected key "branch" for "push" section. expected one of "branches", "branches-ignore", "paths", "paths-ignore", "tags", "tags-ignore", "types", "workflows" [syntax-check]
  |
3 |     branch: main
  |     ^~~~~~~
test.yaml:5:11: character '\' is invalid for branch and tag names. only special characters [, ?, +, *, \, ! can be escaped with \. see `man git-check-ref-format` for more details. note that regular expression is unavailable. note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [glob]
  |
5 |       - 'v\d+'
  |           ^~~~
test.yaml:10:28: label "linux-latest" is unknown. available labels are "windows-latest", "windows-latest-8-cores", "windows-2025", "windows-2025-vs2026", windows-2022", "windows-11-arm", "windows-11-vs2026-arm", "ubuntu-slim", "ubuntu-latest", "ubuntu-latest-4-cores", "ubuntu-latest-8-cores", "ubuntu-latest-16-cores", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-22.04", "ubuntu-22.04-arm", "macos-latest", "macos-latest-xlarge", "macos-latest-large", "macos-26-intel", "macos-26-xlarge", "macos-26-large", "macos-26", "macos-15-intel", "macos-15-xlarge", "macos-15-large", "macos-15", "macos-14-xlarge", "macos-14-large", "macos-14", "xcode-27", "xcode-27-xlarge", "self-hosted", "x64", "arm", "arm64", "linux", "macos", "windows". if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file [runner-label]
   |
10 |         os: [macos-latest, linux-latest]
   |                            ^~~~~~~~~~~~~
test.yaml:13:41: "github.event.head_commit.message" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
13 |       - run: echo "Checking commit '${{ github.event.head_commit.message }}'"
   |                                         ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:11: input "node_version" is not defined in action "actions/setup-node@v4". available inputs are "always-auth", "architecture", "cache", "cache-dependency-path", "check-latest", "node-version", "node-version-file", "registry-url", "scope", "token" [action]
   |
17 |           node_version: 18.x
   |           ^~~~~~~~~~~~~
test.yaml:21:20: property "platform" is not defined in object type {os: string} [expression]
   |
21 |           key: ${{ matrix.platform }}-node-${{ hashFiles('**/package-lock.json') }}
   |                    ^~~~~~~~~~~~~~~
test.yaml:22:17: receiver of object dereference "permissions" must be type of object but got "string" [expression]
   |
22 |         if: ${{ github.repository.permissions.admin == true }}
   |                 ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

See the [full list of checks](/checks), or learn how to [use jactionlint](/usage) locally and on GitHub Actions.

</div>
