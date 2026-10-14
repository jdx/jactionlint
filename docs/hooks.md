# Git hooks and editors

Run jactionlint from Git hooks with hk or pre-commit, and connect an editor or another tool through the stdin and output contract of jactionlint. A hook checks the changed files before each commit.

## hk

[hk](https://hk.jdx.dev/) runs linters and fixers as Git hooks and from the command line. jactionlint prints a SARIF log, which carries the rule ID and the fixes of each finding. hk reads the findings from the log and can apply the fixes as a patch.

### Add the step

Add the step to the `hk.pkl` of your repository, below the `amends` line that `hk init` writes, and list it in the hooks that run it. The step needs an hk release that has the command `hk util sarif-diff`: `hk util sarif-diff --help` shows whether yours has it. This is the shape of the `hk.pkl` of the [jactionlint repository](https://github.com/jdx/jactionlint/blob/main/hk.pkl), with the `jactionlint` executable in place of `go run`:

```pkl
local linters = new Mapping<String, Step> {
    ["jactionlint"] {
        glob = List(".github/workflows/*.yaml", ".github/workflows/*.yml", ".github/dependabot.y*ml")
        batch = true
        diagnostic_format = "sarif"
        check = "jactionlint --format sarif {{files}}"
        check_diff = "hk util sarif-diff -- jactionlint --format sarif {{files}}"
        fix = "jactionlint --fix {{files}}"
    }
}

hooks {
    ["pre-commit"] {
        fix = true
        stash = "git"
        steps = linters
    }
    ["check"] {
        steps = linters
    }
    ["fix"] {
        fix = true
        steps = linters
    }
}
```

- `diagnostic_format = "sarif"` tells hk that the command prints a SARIF log. hk reads the rule ID and the fixes of each finding from it, and `hk check --sarif FILE` writes the findings of a run to a SARIF file.
- `check_diff` runs the command through `hk util sarif-diff`, which turns the fixes in the SARIF log into a patch.
- `fix` is the command that hk runs for a finding that has no fix in the log, see [What hk gets from jactionlint](#what-hk-gets-from-jactionlint).

`hk install` sets up the Git hooks. `hk check` and `hk fix` run the step from the command line, and `--all` selects every file of the repository:

```sh
hk install
hk check --all
hk fix --all
```

`hk check` exits with status 1 while findings remain. `hk fix` applies the safe fixes and reports the rest.

### Choose the files

The `glob` list decides which files hk passes to jactionlint. Each entry adds one kind of file:

| Glob entry | Files |
| --- | --- |
| `.github/workflows/*.yaml`, `.github/workflows/*.yml` | The workflow files. In hk, `*` also matches `/`, so a file in a subdirectory of `.github/workflows` is passed too. jactionlint checks every file that it gets as an argument, although GitHub loads only the files directly in `.github/workflows`. |
| `.github/dependabot.y*ml` | The Dependabot configuration, `dependabot.yml` or `dependabot.yaml`. |
| `**/action.y*ml` | The `action.yml` and `action.yaml` of composite actions, in the root and in any directory. The step above does not have this entry: add it to check them. |

### What hk gets from jactionlint

- The SARIF log holds the fixes that are safe, valid and do not conflict with each other. A finding with an unsafe or conflicting fix is in the log without a fix. hk prints no patch when a result has no fix, so it runs `jactionlint --fix`, which applies the safe fixes and reports the finding that remains.
- The paths in the log are relative to the directory where jactionlint ran, and the log counts columns in Unicode code points.
- stdout holds the SARIF log and nothing else. stderr stays empty unless you pass `--verbose` or `--debug`, because hk parses both streams together. The warnings and notes of the run go into the log instead, see [Streams and color](./output.md#streams-and-color).
- The exit status follows [Exit status](./cli.md#exit-status): 0 when no finding counts and 1 when one does. Statuses 2 and 3 are failures.

### Adopt the checks gradually

A repository that has findings fails the first `hk check`: the rule `missing-timeout` is in the default profile, so every job without `timeout-minutes` is a finding. Record the findings once as a [baseline](./baseline.md#baseline):

```sh
jactionlint --baseline-write
```

hk parses the SARIF log and does not know suppressions, so apply the baseline in the commands and leave the baselined findings out of the log:

```pkl
check = "jactionlint --baseline --sarif-hide-baselined --format sarif {{files}}"
check_diff = "hk util sarif-diff -- jactionlint --baseline --sarif-hide-baselined --format sarif {{files}}"
```

Alternatively, set `baseline: auto` in the config file and pass only `--sarif-hide-baselined`. Refresh the baseline with `jactionlint --baseline-write` outside hk when you pay findings down.

To follow another profile, set `profile: correctness` or `profile: pedantic` in `.github/jactionlint.yaml`, or pass `--profile NAME` in the hk commands. [Profiles](./config.md#profiles) defines all three.

### What hk fix changes

`hk fix` applies the fixes that are in the SARIF log, which are the safe ones. The fix of `missing-timeout` exists only when `rules.missing-timeout.default-minutes` is set, because the rule has no built-in number. With that option the fix is safe, so `hk fix` adds the timeouts. The fix of `missing-permissions` is in the log only when it is safe.

For a finding that has no fix in the log, hk runs the `fix` command of the step. `jactionlint --fix` leaves an unsafe fix alone and still reports the finding. To let hk apply the unsafe fixes too, set `fix = "jactionlint --fix=unsafe {{files}}"` in the step and read the changes before you commit them. [Safe and unsafe fixes](./fix.md#safe-and-unsafe-fixes) explains the difference.

### Add the online checks

The [online checks](./online.md#online-checks) need the network and, for many requests, a token. Run them as a second step in the `check` hook only, so that you run them on demand or in CI and not on every commit. Define the step next to `linters`:

```pkl
local online = new Mapping<String, Step> {
    ["jactionlint-online"] {
        glob = List(".github/workflows/*.yaml", ".github/workflows/*.yml", ".github/dependabot.y*ml")
        batch = true
        diagnostic_format = "sarif"
        check = "jactionlint --online --format sarif {{files}}"
        check_diff = "hk util sarif-diff -- jactionlint --online --format sarif {{files}}"
        fix = "jactionlint --online --fix {{files}}"
    }
}
```

Then list both mappings in the `check` hook, in place of `steps = linters`:

```pkl
    ["check"] {
        steps = new Mapping<String, Step> {
            ...linters
            ...online
        }
    }
```

The `pre-commit` and `fix` hooks still run `linters` only. hk runs the batches in parallel and they share the cache. When GitHub is not reachable, the step puts one warning in the SARIF log and reports what it could check. Keep this step out of the `pre-commit` hook unless a token is always set.

## pre-commit

[pre-commit](https://pre-commit.com) manages Git hooks for many languages. jactionlint provides hooks in its repository, so you add the repository and the hook to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/jdx/jactionlint
    rev: v2.1.0  # x-release-please-version
    hooks:
      - id: jactionlint
```

Run `pre-commit install` once in each clone. A commit then runs the hook for the staged files that jactionlint checks, and `pre-commit run --all-files` runs it for the whole repository. Set `rev` to a release tag: pre-commit takes the hook definitions and the source that it builds from that revision. A hook or a `files` pattern that is newer than your release is not available at that tag: use `rev: main` or a commit SHA to get the hooks and patterns of this page.

### Choose a hook

The repository provides these hooks. They differ in where the jactionlint executable comes from:

| Hook ID | What runs | What it needs |
| --- | --- | --- |
| `jactionlint` | jactionlint built from the source at `rev` with the Go toolchain, in an isolated `GOPATH` that pre-commit manages. | A `shellcheck` on your `PATH`, if you want shell scripts checked. |
| `jactionlint-shellcheck` | The same, with a shellcheck built from Go ([go-shellcheck](https://github.com/wasilibs/go-shellcheck)) installed next to it. | Nothing else. The scripts are checked without a system shellcheck, and the results match CI. |
| `jactionlint-docker` | The [Docker image](./install.md#docker-image) of the release that `rev` names. | Docker. The image has no Node.js, so the rule `github-script-syntax` is skipped. |
| `jactionlint-system` | The `jactionlint` on your `PATH`. | An [installation](./install.md) of jactionlint. |

A revision has only the hooks that existed when it was made. When pre-commit does not find a hook, read `.pre-commit-hooks.yaml` at your `rev`, and use `rev: main` or a commit SHA for a hook that your release does not have.

Without a shellcheck, the `jactionlint` hook does not check the shell scripts and prints a note, as [Companion programs](./install.md#companion-programs) describes.

### Pass options

pre-commit puts `args` before the file names, so a hook accepts every option of `jactionlint`:

```yaml
    hooks:
      - id: jactionlint
        args: [--profile=correctness]
```

With `args: [--fix]`, the hook applies the safe fixes. pre-commit reports the hook as failed with the message `files were modified by this hook`. Stage the changes and commit again. A finding that has no safe fix fails the hook until you fix it.

### Files that run the hooks

Every hook has the same `files` pattern and the type `yaml`. This is the pattern of `main`, and the pattern of an older `rev` can be narrower. A commit runs the hook for these files:

```
^(\.github/workflows/|\.github/dependabot\.ya?ml$|(.*/)?action\.ya?ml$)
```

- Any YAML file below `.github/workflows/`, in any depth. A run on a repository skips the files in subdirectories, because GitHub does not load them, but jactionlint checks every file that it gets as an argument, and pre-commit passes the files as arguments.
- `.github/dependabot.yml` and `.github/dependabot.yaml`.
- `action.yml` and `action.yaml` in any directory.

<a id="tools-integration"></a>
<a id="tools-integ"></a>
<a id="reviewdog"></a>
<a id="super-linter"></a>
<a id="megalinter"></a>
<a id="vs-code"></a>
<a id="emacs"></a>
<a id="vim-and-neovim"></a>
<a id="pulsar-edit"></a>
<a id="nova"></a>
<a id="trunk"></a>
## Editors and other tools

An editor plugin, a wrapper or any other tool can run jactionlint as a command, write the document to stdin and read the findings from stdout. This section lists what such a tool can rely on.

Run jactionlint with the document on stdin and the path of the file as its name. This `buffer.yaml` has an unknown event:

```yaml
on: pushh
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
```

```sh
jactionlint --profile correctness --stdin-filename .github/workflows/ci.yaml --format oneline --no-color - < buffer.yaml
```

The `correctness` profile keeps the output to one finding, and jactionlint exits with status 1:

```
.github/workflows/ci.yaml:1:5: unknown Webhook event "pushh". see https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#webhook-events for list of all Webhook event names [unknown-event]
```

### The contract

| Part | What you can rely on |
| --- | --- |
| Input | A lone `-` reads the document from stdin and checks it as one file. A document that is not valid YAML is a finding of the rule `yaml-syntax`, not a failure. |
| Name | `--stdin-filename=NAME` is the file name in the output. Without it the name is `<stdin>`. |
| File type | The name decides what the document is. `action.yml` or `action.yaml` outside `.github/workflows` is a composite action. `dependabot.yml` or `dependabot.yaml` without a directory or in a `.github` directory is the Dependabot configuration. `.github/workflows/actions.lock` is the Actions lock file. Any other name is a workflow file. |
| Configuration | When the file in `NAME` exists, jactionlint finds the repository and its config file from that path, searching upwards from its directory. For a name without a file behind it, and without a name, it does not read the config file of a repository, but the user-global config file still applies. `--config-file=FILE` names the config file yourself. |
| Formats | `oneline`, `gcc`, `json`, `jsonl` and `sarif` are made for tools, see [Output formats](#output-formats). |
| Exit status | 0 or 1 for a run that completed (1: a finding counts), 2 or 3 for a failure, such as an invalid command line or a file that cannot be read. [Exit status](./cli.md#exit-status) lists every cause. |
| Color | The output has no color when stdout is not a terminal. `--no-color` states it. |
| Options | Long options have two dashes, as in `--format`. jactionlint rejects the single-dash spellings of actionlint 1 with status 2. |
| Writing | `--fix` and `--baseline-write` exit with status 2 on stdin, because the document has no file to write. |

### Output formats

[Choose a format](./output.md#format-error-messages) says which consumer each format fits. A tool needs these facts:

| `--format` | A finding is | A clean run prints |
| --- | --- | --- |
| `oneline` | One line, `FILE:LINE:COLUMN: MESSAGE [RULE]`. The levels `warn` and `info` add a prefix, see [Text and oneline](./output.md#text-and-oneline). | Nothing |
| `gcc` | One line, `FILE:LINE:COLUMN: LEVEL: MESSAGE [RULE]` | Nothing |
| `json` | An object in a JSON array | `[]` |
| `jsonl` | An object on one line | Nothing |
| `sarif` | A result in a SARIF log | A log with no results |

[Fields of a finding](./output.md#fields-of-a-finding) lists the keys of the `json` and `jsonl` objects, and [Lines and columns](./output.md#lines-and-columns) explains the positions. `jactionlint --print-output-schema` prints the JSON Schema of both formats.

### Tools written for actionlint

Tools that were written for the original actionlint call the `actionlint` executable, and this project does not test them with jactionlint. Among them are the reviewdog action, super-linter, MegaLinter, trunk and the extensions for VS Code, Emacs, Vim and Neovim, Pulsar and Nova. A plugin that passes the single-dash options of actionlint 1 (nvim-lint, ALE, flymake-actionlint) fails with status 2 even when you name the jactionlint executable, because jactionlint rejects those options.

If a tool lets you set the executable and every argument, give it the command of this section. [Migrating to v2](./v2-migration.md#command-line-options) lists the options that changed. For annotations on GitHub, use the [action or a problem matcher](./ci.md#annotations-and-code-scanning).

## Next steps

- [Run jactionlint in CI](./ci.md#the-action) with the action, mise or Docker.
- [Record a baseline](./baseline.md#baseline) to adopt the checks on a repository that has findings.
- [Fix findings automatically](./fix.md#fix-errors-automatically) and learn which fixes are safe.
- [Read the output formats](./output.md#format-error-messages) that an editor or a tool can consume.
- [Choose a profile](./config.md#profiles) for the hooks.
