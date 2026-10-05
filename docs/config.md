Configuration
=============

This document describes how to configure [actionlint](..) behavior.

Note that configuration file is optional. The author tries to keep configuration file as minimal as possible not to
bother users to configure behavior of actionlint. Running actionlint without configuration file would work fine in most
cases.

## Configuration file

Configuration file `actionlint.yaml` or `actionlint.yml` can be put in `.github` directory.

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
# Require actions to be pinned to commit hashes instead of tags/branches
require-commit-hash: true
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
- `config-variables`: [Configuration variables][vars]. When an array is set, actionlint will check `vars` properties strictly.
  An empty array means no variable is allowed. The default value `null` disables the check.
- `config-secrets`: [Secrets][secrets]. When an array is set, actionlint will check `secrets` properties strictly against the
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
- `require-commit-hash`: Optional lint to require actions to be pinned to commit hashes instead of tags/branches. Defaults to `false`
  (disabled). When `true`, `uses:` of GitHub-hosted actions and reusable workflows must have a full-length 40-digit commit SHA ref,
  and Docker actions must be pinned by digest (`docker://image@sha256:...`). Local actions (`./`, `$/`) are exempt.
  See [the check document](checks.md#check-action-format) for more details.

## Configuration file location and priority

actionlint looks for a configuration file in the following order and uses the **first** one found. Configurations
are not merged:

1. The file passed via the `-config-file` command line option.
2. `actionlint.yaml` (or `actionlint.yml`) in the repository's `.github` directory. actionlint locates the project by
   searching upwards from the linted file's directory.
3. The user-global configuration at `$XDG_CONFIG_HOME/actionlint/actionlint.yaml` (or `actionlint.yml`). When
   `$XDG_CONFIG_HOME` is not set, `$HOME/.config/actionlint/actionlint.yaml` is used instead, following the
   [XDG Base Directory specification][xdg].

The user-global configuration is useful for personal or organization-wide defaults shared across many repositories
(for example via dotfiles), or for CI base images that need a baseline configuration without injecting a config file
into every checkout. A repository's own `.github/actionlint.yaml` always takes precedence over the global configuration,
so per-repository settings are never overridden by the global defaults.

`$XDG_CONFIG_HOME` must be an absolute path. A relative path is ignored as the specification requires. `$HOME/.config`
is used on all platforms including Windows and macOS (`%USERPROFILE%\.config` on Windows). The global
configuration is not used when `-config-file` is given.

## Generate the initial configuration

You don't need to write the first configuration file by your hand. `actionlint` command can generate a default configuration
with `-init-config` flag.

```sh
actionlint -init-config
vim .github/actionlint.yaml
```

---

[Checks](checks.md) | [Installation](install.md) | [Usage](usage.md) | [Go API](api.md) | [References](reference.md)

[xdg]: https://specifications.freedesktop.org/basedir-spec/latest/
[Super-Linter]: https://github.com/super-linter/super-linter
[pat]: https://pkg.go.dev/path#Match
[vars]: https://docs.github.com/en/actions/learn-github-actions/variables
[secrets]: https://docs.github.com/en/actions/security-guides/using-secrets-in-github-actions
[doublestar]: https://github.com/bmatcuk/doublestar
