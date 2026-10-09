Configuration
=============

This document describes how to configure [jactionlint](https://github.com/jdx/jactionlint) behavior.

Note that configuration file is optional. Running jactionlint without configuration file works fine in most cases:
correctness checks and high-confidence security checks are on by default. A configuration file is for the things that only
you can know (your self-hosted runner labels, your ignore patterns) and for opting in to the policy tier (for example
`require-permissions` or `timeout-minutes`), which is never enabled by default. See
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
    # List of regular expressions to filter errors by the error messages.
    ignore:
      # Ignore the specific error from shellcheck
      - 'shellcheck reported issue in this script: SC2086:.+'
  # This pattern only matches '.github/workflows/release.yaml' file.
  .github/workflows/release.yaml:
    ignore:
      # Ignore errors from the old runner check. This may be useful for (outdated) self-hosted runner environment.
      - 'the runner of ".+" action is too old to run on GitHub Actions'

# Configuration for the opt-in check of 'timeout-minutes' at jobs.
timeout-minutes:
  # Require every job to set 'timeout-minutes'.
  required: true
  # Maximum allowed value of 'timeout-minutes' in minutes.
  max: 60

# Require actions to be pinned to commit hashes instead of tags/branches
require-commit-hash: true
# Require explicit permissions at workflow-level or job-level
require-permissions: true
# Report local actions used before any checkout step in the same job
require-checkout-before-local-action: true
# Require `if:` conditions to be wrapped in `${{ }}`
require-expression-wrapping: true
# Report `cond && '' || other` where the value after `&&` is always falsy
check-falsy-ternary: true
# Report workflow names at 'on.workflow_run.workflows' which do not exist in the repository
check-workflow-run-names: true

# Require every 'run:' step to set a shell explicitly with 'shell:' (or 'defaults.run.shell'). (default: false)
require-shell: true

# Maximum number of non-blank lines allowed in a 'run:' script. 0 disables the check. (default: 0)
max-run-lines: 30
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
- `require-shell`: When `true`, every `run:` step must have an explicit shell by `shell:` of the step or `defaults.run.shell` of
  the job or the workflow. When omitted, GitHub Actions runs `bash -e {0}` on Linux/macOS, which differs from `shell: bash`
  (`bash --noprofile --norc -eo pipefail {0}`). See [the check](checks.md#check-run-policy). The default is `false`.
- `max-run-lines`: Maximum number of non-blank lines allowed in a `run:` script. Longer scripts are reported. `0` disables the
  check and negative values are rejected. See [the check](checks.md#check-run-policy). The default is `0`.
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
  treats a caller workflow that has no `permissions:` block at the workflow level *and* no `permissions:` block on
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
    - `ignore`: The configuration to ignore (filter) the errors by the error messages. This is an array of regular
      expressions. When one of the patterns matches the error message, the error will be ignored. It's similar to the
      `-ignore` command line option.
- `timeout-minutes`: Configuration for the [timeout check](checks.md#check-timeout-minutes). The check is disabled by
  default. It is enabled by setting `required: true` and/or `max`.
  - `required`: When `true`, every job must set `timeout-minutes`. Jobs calling a reusable workflow (`uses:`) are not
    checked since they do not support `timeout-minutes`. The default is `false`.
  - `max`: The maximum allowed value of `timeout-minutes` in minutes. A job with a larger value is reported. This is
    checked even when `required` is `false`. `0` (the default) means no limit. A negative value is a configuration error.
    Values given by expressions `${{ }}` are not checked.
- `require-commit-hash`: Optional lint to require actions to be pinned to commit hashes instead of tags/branches. Defaults to `false`
  (disabled). When `true`, `uses:` of GitHub-hosted actions and reusable workflows must have a full-length 40-digit commit SHA ref,
  and Docker actions must be pinned by digest (`docker://image@sha256:...`). Local actions (`./`, `$/`) are exempt.
  See [the check document](checks.md#check-action-format) for more details.
- `require-permissions`: Optional lint to require an explicit `permissions:` at workflow-level or job-level. Defaults to `false`
  (disabled). When `true`, every job not covered by a workflow-level `permissions:` and lacking its own is reported.
  `permissions: {}` counts as explicit. See [the check document](checks.md#check-permissions) for more details.
- `require-checkout-before-local-action`: Optional lint to report a local action (`uses: ./path`) used in a job before any step that
  checks out the repository. Defaults to `false` (disabled). See [the check document](checks.md#check-local-action-checkout) for
  what counts as a checkout and known limitations.
- `require-expression-wrapping`: Optional lint to require `if:` conditions (of jobs and steps) to be wrapped in `${{ }}`
  explicitly, though GitHub Actions makes the placeholder optional there. Defaults to `false` (disabled).
  See [the check document](checks.md#check-require-expression-wrapping) for more details.
- `check-falsy-ternary`: Optional lint to report `cond && b || c` when `b` is a literal which is always falsy (`''`, `0`,
  `false`, `null`). Defaults to `false` (disabled). See [the check document](checks.md#check-falsy-ternary) for more details.
- `check-workflow-run-names`: Optional lint to check that each workflow name at `on.workflow_run.workflows` exists in the
  repository. Defaults to `false` (disabled). A workflow is identified by its `name:`, or by its file path (like
  `.github/workflows/ci.yaml`) when it has no name; the comparison is case-insensitive. Names containing `${{ }}` or glob
  characters are skipped, and the check is skipped entirely when a workflow file in the repository cannot be parsed or has a
  dynamic `name:`. See [the check document](checks.md#check-workflow-run-names) for more details.

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

---

[Checks](checks.md) | [Installation](install.md) | [Usage](usage.md) | [Go API](api.md) | [References](reference.md)

[xdg]: https://specifications.freedesktop.org/basedir-spec/latest/
[Super-Linter]: https://github.com/super-linter/super-linter
[pat]: https://pkg.go.dev/path#Match
[vars]: https://docs.github.com/en/actions/learn-github-actions/variables
[secrets]: https://docs.github.com/en/actions/security-guides/using-secrets-in-github-actions
[doublestar]: https://github.com/bmatcuk/doublestar
