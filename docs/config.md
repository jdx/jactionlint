Configuration
=============

This document describes how to configure [jactionlint](https://github.com/jdx/jactionlint) behavior.

Note that configuration file is optional. Running jactionlint without configuration file works fine in most cases:
correctness checks and high-confidence security checks are on by default. A configuration file is for the things that only
you can know (your self-hosted runner labels, your ignore patterns) and for opting in to the policy tier (for example
`require-permissions` or `require-shell`), which is not enabled by default (`missing-timeout` is the one policy check that is). See
[the policy for new checks](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features)
for the three tiers.

## Configuration file

Configuration file `jactionlint.yaml` or `jactionlint.yml` can be put in `.github` directory.

> [!NOTE]
> The file names used by the original actionlint (`.github/actionlint.yaml` and `.github/actionlint.yml`) are also accepted, so
> an existing configuration keeps working. When both exist, `jactionlint.yaml` is used first. `jactionlint -init-config`
> generates `.github/jactionlint.yaml`.

Note: If you're using [Super-Linter][], the file should be placed in a different directory. Please check the project's document.

```yaml
# Configuration related to self-hosted runner.
self-hosted-runner:
  # Labels of self-hosted runner in array of strings.
  labels:
    - linux.2xlarge
    - windows-latest-xl
    - linux-multi-gpu
  # When true, only the labels listed above are accepted at 'runs-on:'. Built-in labels such as 'ubuntu-latest' and
  # 'self-hosted' are reported unless listed. (default: false)
  strict-labels: false

# Configuration variables in array of strings defined in your repository or organization.
config-variables:
  - DEFAULT_RUNNER
  - JOB_NAME
  - ENVIRONMENT_STAGE

# Actions (or reusable workflows) which must be used in every workflow. Opt-in; disabled by default.
required-actions:
  # Any version of the action is accepted when 'version' is omitted.
  - action: actions/checkout
  # 'version' is the exact ref after '@'. Tags, branches and commit SHAs are compared as-is.
  - action: github/codeql-action/init
    version: v3
# Controls what permissions are assumed for a caller workflow that declares no
# "permissions:" block at all when checking caller/callee permissions for local
# reusable workflow calls. "restricted" (default) assumes GitHub's restricted
# default token. "permissive" assumes write on every scope except "id-token",
# which always requires an explicit opt-in regardless of repo settings.
assume-default-permissions: restricted

# Secrets in array of strings defined in your repository or organization.
config-secrets:
  - DEPLOY_TOKEN
  - API_KEY
  - ACCESS_TOKEN

# Path-specific configurations.
paths:
  # Glob pattern relative to the repository root for matching files. The path separator is always '/'.
  # This example configures any YAML file under the '.github/workflows/' directory.
  .github/workflows/**/*.{yml,yaml}:
    # List of rule IDs and regular expressions to filter errors. A rule ID ignores all errors of the rule, and a
    # regular expression is matched to the error messages.
    ignore:
      # Ignore all errors of the rule
      - unpinned-uses
      # Ignore the specific error from shellcheck
      - "shellcheck reported issue in this script: SC2086:.+"
  # This pattern only matches '.github/workflows/release.yaml' file.
  .github/workflows/release.yaml:
    ignore:
      # Ignore errors from the old runner check. This may be useful for (outdated) self-hosted runner environment.
      - 'the runner of ".+" action is too old to run on GitHub Actions'

# Profile: the set of rules which are enabled. 'default', 'strict' or 'all'. (default: default)
profile: strict

# The level of each rule by its ID: 'error', 'warn', 'info' or 'off'. See https://jactionlint.jdx.dev/rules
rules:
  # Lower the level of a rule enabled by the profile
  unpinned-uses: warn
  # Turn a rule off
  missing-permissions: off
  # Turn on a rule which the profile does not enable
  require-shell: error
  # A rule with options takes a mapping
  max-run-lines:
    level: warn
    max: 30
  timeout-too-long:
    max: 60

# Config files to inherit from. Relative paths are resolved from this file. Later files win.
extends:
  - ../shared/jactionlint.yaml
```

- `self-hosted-runner`: Configuration for your self-hosted runner environment.
  - `labels`: Label names added to your self-hosted runners as list of pattern. Glob syntax supported by [`path.Match`][pat]
    is available.
  - `strict-labels`: When `true`, the [runner label check](checks.md#check-runner-labels) accepts only the labels listed in
    `labels` (glob patterns supported). The built-in labels (GitHub-hosted runner labels such as `ubuntu-latest` and the preset
    self-hosted labels such as `self-hosted`, `linux`, and `x64`) are reported as errors unless they are listed in `labels`.
    This is useful when all jobs must run on your own runners, or when your runners (e.g. Actions Runner Controller runner
    sets) do not have the default `self-hosted` label. Label conflict checks still apply to listed built-in labels. The default
    is `false`.
- `config-variables`: [Configuration variables][vars]. When an array is set, jactionlint will check `vars` properties strictly.
  An empty array means no variable is allowed. The default value `null` disables the check.
- `required-actions`: List of actions which must be used in each checked workflow. This check is disabled unless the list
  is non-empty. A missing action is reported once per workflow at the position of its first job.
  - `action`: Name of the action like `actions/checkout`, without `@version`. A sub-path is part of the name
    (`github/codeql-action/init`). Case-insensitive. Reusable workflow calls (`jobs.<id>.uses`) are also matched.
  - `version`: Optional ref (tag, branch or commit SHA) compared exactly. The requirement is satisfied when at least one
    use of the action has this version. When omitted any version is accepted.

  Local actions (`./...` and `$/...`) and Docker images (`docker://...`) never match. Only workflow files are checked; the steps of
  composite action files (`action.yml`) are not.

- `assume-default-permissions`: Controls how the caller/callee permissions check for local reusable workflow calls
  treats a caller workflow that has no `permissions:` block at the workflow level _and_ no `permissions:` block on
  the calling job. This mirrors the repository-level "Workflow permissions" setting (Settings → Actions → General),
  which jactionlint cannot read from the workflow file. Set to `restricted` (the default) to assume GitHub's
  restricted default token (`contents: read` and `packages: read`, everything else `none`). Set to `permissive` to
  assume the permissive default (write on every scope). Even under `permissive`, `id-token` is still treated as
  `none` because OIDC tokens always require an explicit opt-in regardless of the repo-level Workflow permissions
  setting. Note: this only affects callers with no `permissions:` block anywhere. Once a caller declares any
  `permissions:` block — even `permissions: {}` — the check always runs against that explicit block, because
  GitHub treats any scope omitted from an explicit block as `none`.
- `config-secrets`: [Secrets][secrets]. When an array is set, jactionlint will check `secrets` properties strictly against the
  list. An empty array means no secret is allowed. The default value `null` disables the check. `GITHUB_TOKEN` is always allowed. Note: this check only applies
  when secrets are not explicitly declared in the workflow (e.g. via `secrets:` in `on.workflow_call`), since declared secrets
  are already checked by their type.
- `paths`: Configurations for specific file path patterns. This is a mapping from a glob pattern and the corresponding
  configuration.
  - `{glob}`: A file path glob pattern to apply the configuration. The path separator is always '/'. It is matched to the
    relative path from the repository root. For example `.github/workflows/**/*.yaml` matches all the workflow files (with
    `.yaml` file extension). For the glob syntax, please read the [doublestar][] library's documentation.
    - `ignore`: The configuration to ignore (filter) the errors. This is an array of [rule IDs](rules.md) and regular
      expressions. A rule ID ignores all the errors of the rule. A regular expression ignores the errors whose message
      matches it. It's similar to the `-ignore` command line option.
- `profile`, `rules` and `extends`: See [Profiles](#profiles), [Rules](#rules) and [Extending config files](#extending-config-files).

Unknown keys are errors. jactionlint reports the key with its position and suggests the closest known key when it looks
like a typo:

```
unknown key "self-hosted-runnr" in the configuration at line:3,col:1. did you mean "self-hosted-runner"?
```

## Profiles

A profile is a named set of [rules](rules.md) which are enabled together. `profile` selects one of them:

| Profile   | Enables                                                                                                                                                                                                                                          |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `default` | All the correctness checks, the security errors with (almost) no false positives, the bug detectors `unsound-ternary`, `workflow-run-names` and `local-action-checkout`, and the policy check `missing-timeout`. Used when `profile` is omitted. |
| `strict`  | `default` plus the posture and policy checks: `unpinned-uses`, `missing-permissions` and `unused-ignore`.                                                                                                                                        |
| `all`     | `strict` plus the style checks: `require-shell`, `require-expression-wrapping` and `max-run-lines`.                                                                                                                                              |

The profile of each rule is in [the list of rules](rules.md). Some rules belong to no profile and run only when the
configuration turns them on: `required-actions` (when the `required-actions` list is not empty) and `timeout-too-long` (when
`max` is set).

## Rules

`rules` sets the level of each rule by its stable [rule ID](rules.md). The level is one of:

- `error`: The finding is printed and makes jactionlint exit with status 1.
- `warn` and `info`: The finding is printed with a `warning:` or `info:` prefix (or the corresponding level in `-format sarif`,
  `gcc` and `github`). It does not change the exit status unless `-strict-exit` is given.
- `off`: The rule is disabled.

A rule not listed in `rules` follows the profile: it runs at its default level if the profile includes it and is off otherwise.
Every rule is an `error` by default. Unknown rule IDs are errors with a suggestion for a similar ID.

A rule with options takes a mapping with `level` and the options. Giving options without `level` enables the rule at its
default level:

```yaml
rules:
  max-run-lines:
    level: warn
    max: 80
  timeout-too-long:
    max: 60
```

| Rule                  | Option            | Description                                                                                                                                                                     |
| --------------------- | ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `max-run-lines`       | `max`             | Maximum number of non-blank lines in a `run:` script. Default `100` when the rule is enabled by the `all` profile.                                                              |
| `missing-timeout`     | `default-minutes` | The `timeout-minutes` which `-fix` adds to a job without one. There is no default: without it the rule has no fix. Lowered to `max` of `timeout-too-long` when that is smaller. |
| `timeout-too-long`    | `max`             | Maximum allowed `timeout-minutes` of a job in minutes. Values given by `${{ }}` are not checked. The rule does nothing without `max`.                                           |
| `forbidden-uses`      | `allow`           | List of patterns of the only actions and reusable workflows that may be used, e.g. `actions/*`. See [forbidden actions](checks.md#check-forbidden-uses).                        |
| `forbidden-uses`      | `deny`            | List of patterns of actions and reusable workflows that must not be used. The rule does nothing without `allow` or `deny`.                                                      |
| `secrets-outside-env` | `allow`           | List of secret names that may be used by a job without an `environment:`. `GITHUB_TOKEN` is always allowed.                                                                     |
| `typosquat-uses`      | `allow`           | List of `owner/repo` slugs that are never reported, e.g. a legitimate fork of a popular action.                                                                                 |

## Extending config files

`extends` lists config files to inherit from. This makes an organization-wide configuration possible: keep a shared file in a
repository or a submodule and extend it from every project.

```yaml
extends:
  - ../shared/jactionlint.yaml
  - /etc/jactionlint/org.yaml
profile: strict
rules:
  require-shell: off
```

- Relative paths are resolved from the directory of the file which lists them. `extends` is read only from config files
  (not from `jactionlint` API calls that parse bytes).
- Later files win over earlier ones, and the file itself wins over all of them.
- `profile` and scalar keys are replaced. `rules` and `paths` are merged by key: the entry of the later file replaces the
  entry with the same rule ID or the same glob. Lists such as `labels`, `config-variables` and `required-actions` are replaced.
- A file can extend other files. A cycle, or a chain deeper than 10 files, is an error.

## Ignoring errors by rule ID

The `ignore` lists of `paths`, the `-ignore` command line option and the `# jactionlint ignore=` comments take rule IDs as
well as regular expressions. A pattern which is exactly a rule ID ignores all the errors of the rule; any other pattern is a
regular expression matched to the error messages. See [the usage document](usage.md#ignore-some-errors).

The [`unused-ignore`](rules.md#unused-ignore) rule (in the `strict` profile) reports ignore comments which did not suppress
anything.

## Deprecated keys

The following keys were replaced by `rules`. They still work for now: jactionlint translates them into rules and prints a
deprecation warning to stderr once per config file (in `-format sarif` it is in the log's `toolConfigurationNotifications`). `jactionlint -migrate-config` rewrites the file (keeping the comments
and the other keys) into the `rules` mapping.

| Deprecated key                               | Replacement                                                                                    |
| -------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| `require-commit-hash: true`                  | `rules: {unpinned-uses: error}`                                                                |
| `require-permissions: true`                  | `rules: {missing-permissions: error}`                                                          |
| `require-checkout-before-local-action: true` | `rules: {local-action-checkout: error}` (on by default now)                                    |
| `require-expression-wrapping: true`          | `rules: {require-expression-wrapping: error}`                                                  |
| `check-falsy-ternary: true`                  | `rules: {unsound-ternary: error}` (on by default now)                                          |
| `check-workflow-run-names: true`             | `rules: {workflow-run-names: error}` (on by default now)                                       |
| `require-shell: true`                        | `rules: {require-shell: error}`                                                                |
| `max-run-lines: N`                           | `rules: {max-run-lines: {level: error, max: N}}`                                               |
| `timeout-minutes: {required: true}`          | `rules: {missing-timeout: error}`                                                              |
| `timeout-minutes: {required: false}`         | `rules: {missing-timeout: off}`                                                                |
| `timeout-minutes: {max: N}`                  | `rules: {timeout-too-long: {level: error, max: N}}` (`missing-timeout` is left to the profile) |

Setting one of these keys to `false` (or `max-run-lines: 0`) turns the rule off, which is the way to disable the three rules
which are on by default in the old format. A rule written in `rules` wins over the deprecated key.

## Configuration file location and priority

jactionlint looks for a configuration file in the following order and uses the **first** one found. Configurations
are not merged:

1. The file passed via the `-config-file` command line option.
2. `jactionlint.yaml` (or `jactionlint.yml`) in the repository's `.github` directory, then `actionlint.yaml` (or
   `actionlint.yml`) in the same directory as used by the original actionlint. jactionlint locates the project by
   searching upwards from the linted file's directory.
3. The user-global configuration at `$XDG_CONFIG_HOME/jactionlint/jactionlint.yaml` (or `jactionlint.yml`). When
   `$XDG_CONFIG_HOME` is not set, `$HOME/.config/jactionlint/jactionlint.yaml` is used instead, following the
   [XDG Base Directory specification][xdg]. The location used by the original actionlint
   (`$XDG_CONFIG_HOME/actionlint/actionlint.yaml`) is checked after it.

The user-global configuration is useful for personal or organization-wide defaults shared across many repositories
(for example via dotfiles), or for CI base images that need a baseline configuration without injecting a config file
into every checkout. A repository's own `.github/jactionlint.yaml` always takes precedence over the global configuration,
so per-repository settings are never overridden by the global defaults.

`$XDG_CONFIG_HOME` must be an absolute path. A relative path is ignored as the specification requires. `$HOME/.config`
is used on all platforms including Windows and macOS (`%USERPROFILE%\.config` on Windows). The global
configuration is not used when `-config-file` is given.

## Generate the initial configuration

You don't need to write the first configuration file by your hand. `jactionlint` command can generate a default configuration
with `-init-config` flag.

```sh
jactionlint -init-config
vim .github/jactionlint.yaml
```

To rewrite an existing configuration which uses the [deprecated keys](#deprecated-keys):

```sh
jactionlint -migrate-config
```

---

[Checks](checks.md) | [Rules](rules.md) | [Installation](install.md) | [Usage](usage.md) | [Go API](api.md) | [References](reference.md)

[xdg]: https://specifications.freedesktop.org/basedir-spec/latest/
[Super-Linter]: https://github.com/super-linter/super-linter
[pat]: https://pkg.go.dev/path#Match
[vars]: https://docs.github.com/en/actions/learn-github-actions/variables
[secrets]: https://docs.github.com/en/actions/security-guides/using-secrets-in-github-actions
[doublestar]: https://github.com/bmatcuk/doublestar
