# Coming from actionlint

Switch a repository from actionlint to jactionlint: keep the checks of actionlint with the `correctness` profile, rename the options and read the differences.

jactionlint is a fork of [actionlint](https://github.com/rhysd/actionlint). It reads the same workflow files and the same configuration file. Its `correctness` profile covers the checks of actionlint and adds rules for more mistakes, and its `default` profile adds security and policy rules that you can adopt after the switch.

## Switch in five steps

1. Install jactionlint. A package named `actionlint` installs the original project, so [choose an installation method](./install.md#choose-a-method) of this fork.
2. Select the `correctness` profile in `.github/jactionlint.yaml` or with `--profile correctness`, as [Get the behavior of actionlint](#get-the-behavior-of-actionlint) shows.
3. [Rename the options](#rename-the-options) in your scripts, CI steps and hooks. actionlint accepts the two-dash spellings too, so you can rename before you replace the binary.
4. Keep the [config file written for actionlint](#a-config-file-written-for-actionlint) or move it to `.github/jactionlint.yaml`.
5. Check what depends on the output: the [ignore patterns](#turn-rules-off-on-top-of-it), the templates and the label at the end of a finding ([the rule behind a finding](#find-the-rule-behind-a-finding)).

When the switch works, read [where jactionlint differs](#where-jactionlint-differs) and [adopt more checks](#adopt-more-checks).

## Get the behavior of actionlint

Select the `correctness` [profile](./config.md#profiles) to get the checks of actionlint without the security and policy rules that `default` adds. Write it in `.github/jactionlint.yaml`:

```yaml
profile: correctness
```

Or pass it on the command line, which wins over the file:

```sh
jactionlint --profile correctness
```

Without a profile jactionlint applies `default`, which also has rules such as `unpinned-uses`, `missing-permissions`, `missing-timeout` and `concurrency-limits`. A repository that never asked for them sees new findings until it selects a profile.

The `correctness` profile is not an exact copy of actionlint. It has rules for mistakes that actionlint does not check, and it accepts some constructs that actionlint rejects. [Where jactionlint differs](#where-jactionlint-differs) lists both.

### Turn rules off on top of it

Turn a rule off, or lower its level, under `rules` with its [rule ID](./rules.md):

```yaml
profile: correctness
rules:
  unsound-ternary: off
  workflow-run-names: off
  shellcheck: warn # lower the level: warnings do not fail the run
```

Every rule of the `correctness` profile that actionlint has reports at the level `error`, so the exit status is 1 when one of them finds something, as with actionlint. A rule that you set to `warn` or `info` does not fail the run unless you pass `--strict-exit`.

Ignore single findings in one of three ways:

- `--ignore PATTERN`, and `ignore` under `paths` in the config file, take a regular expression that matches the message, as in actionlint, or a rule ID.
- A `# jactionlint ignore=RULE` comment ignores the findings of one line or step. actionlint has no ignore comments.
- An entry of `ignores` in the config file ignores findings by file, job, step or `uses:`, and it can expire. actionlint has no such entries.

[Ignore findings](./ignore.md#ignore-some-errors) has the syntax and the scopes.

jactionlint rewords some messages ([what stays the same](#what-stays-the-same-and-what-is-tested) shows an example). A regular expression written for the old wording can stop matching, so run jactionlint once after the switch and replace the pattern of a finding that reappears by its rule ID.

## Rename the options

Every option of actionlint has a two-dash spelling in jactionlint. The table names each option by what it does. The old single-dash spellings are in the [migration table](./v2-migration.md#command-line-options).

| What it does | Option | Notes |
| --- | --- | --- |
| Print the help | `--help` (`-h`) | Prints to stdout. actionlint prints to stderr. |
| Print the version | `--version` (`-V`) | |
| Print verbose or debug logs | `--verbose` (`-v`), `--debug` | |
| Choose the output format | `--format FORMAT` (`-f`) | Also takes the names `text`, `oneline`, `json`, `jsonl`, `sarif`, `gcc`, `github` and `summary`. A value that contains `{{ }}` is a Go template. |
| Print one line per finding | `--oneline` | Same as `--format oneline`. |
| Force or disable color | `--color`, `--no-color` | `--color` takes a value only as `--color=never`, `--color=always` or `--color=auto`. A bare `--color` means always. |
| Ignore findings | `--ignore PATTERN` (`-i`) | A regular expression on the message, or a rule ID. Repeatable. |
| Use another config file | `--config-file FILE` (`-c`) | |
| Write a config file | `--init-config` | Writes `.github/jactionlint.yaml`, where actionlint writes `.github/actionlint.yaml`. Exits with status 3 when the repository has a config file already, also one named for actionlint. |
| Name the file read from stdin | `--stdin-filename NAME` | |
| Choose shellcheck | `--shellcheck CMD` | An empty value turns the integration off. A command that does not exist exits with status 3, where actionlint ignores it. |
| Choose pyflakes | `--pyflakes CMD` | Same as `--shellcheck`. |

The short options `-V`, `-v`, `-c`, `-f`, `-p` and `-i` exist in jactionlint only. A script that must run with both tools uses the long spellings. [Command line](./cli.md#command-line-options) lists every option of jactionlint, including those that actionlint does not have.

## A config file written for actionlint

jactionlint reads the config file of actionlint. It looks for the name `jactionlint.yaml` first and for `actionlint.yaml` after it, in the repository and in your user config directory. [Where jactionlint reads the config file](./config.md#where-jactionlint-reads-the-config-file) lists the names and their order.

The keys `self-hosted-runner`, `config-variables` and `paths` carry over with the same meaning. `config-secrets`, `profile`, `rules` and the other keys of jactionlint do not exist in actionlint, which ignores keys that it does not know. A file that both tools read can therefore hold `profile: correctness`.

A file named for actionlint has no `profile`, so the `default` profile applies. jactionlint says so once per run, on stderr and as a notification in the SARIF log:

```
note: config file ".github/actionlint.yaml" was read as a jactionlint config. it sets no "profile", so the default profile applies, which has more rules than actionlint. add "profile: correctness" to the file or run with --profile correctness for the checks of actionlint. see https://jactionlint.jdx.dev/actionlint
```

To get the checks of actionlint, add `profile: correctness` to the file or pass `--profile correctness`, which also silences the note. To use the name of jactionlint, rename the file to `.github/jactionlint.yaml`. `--migrate-config` does not add a profile: it reports that the file has no deprecated key.

jactionlint parses the file strictly. A key that it does not know, such as the typo `self-hosted-runnr`, ends the run with exit status 3 and the suggestion `did you mean "self-hosted-runner"?`, where actionlint ignores the key.

<a id="from-the-checks-of-actionlint-to-rule-ids"></a>
## Find the rule behind a finding

A finding of jactionlint ends with the ID of its rule, where actionlint prints its kind. The kind is still on every finding: it is the `kind` field of `--format json` and `{{.Kind}}` in a template. The table maps each kind of actionlint to the rules that report it, and [Rules](./rules.md) has the group, level and profile of each.

| Label in actionlint | What it covers | Rules of jactionlint |
| --- | --- | --- |
| `syntax-check` | Structure of the workflow file: unexpected, missing and duplicate keys, empty mappings, YAML anchors and merge keys | [`workflow-syntax`](./rules.md#workflow-syntax), [`duplicate-key`](./rules.md#duplicate-key), [`yaml-syntax`](./rules.md#yaml-syntax), [`merge-key`](./rules.md#merge-key), [`recursive-alias`](./rules.md#recursive-alias), [`unused-anchor`](./rules.md#unused-anchor) |
| `expression` | Syntax, types, contexts and functions of `${{ }}`, script injection and the types of reusable workflow inputs | [`expression-syntax`](./rules.md#expression-syntax), [`expression-type`](./rules.md#expression-type), [`undefined-property`](./rules.md#undefined-property), [`undefined-function`](./rules.md#undefined-function), [`invalid-function-call`](./rules.md#invalid-function-call), [`context-availability`](./rules.md#context-availability), [`template-injection`](./rules.md#template-injection), [`workflow-input-type`](./rules.md#workflow-input-type) |
| `if-cond` | Constant conditions and `if:` that is always true | [`constant-condition`](./rules.md#constant-condition), [`if-always-true`](./rules.md#if-always-true) |
| `action` | The format of `uses:`, the inputs of actions, local actions and outdated runtimes | [`invalid-uses`](./rules.md#invalid-uses), [`missing-action-input`](./rules.md#missing-action-input), [`unknown-action-input`](./rules.md#unknown-action-input), [`deprecated-action-input`](./rules.md#deprecated-action-input), [`outdated-action-runner`](./rules.md#outdated-action-runner), [`invalid-local-action`](./rules.md#invalid-local-action) |
| `events` | Webhook events, activity types, filters, inputs of `workflow_dispatch` and `workflow_call`, cron and time zones | [`unknown-event`](./rules.md#unknown-event), [`invalid-activity-type`](./rules.md#invalid-activity-type), [`invalid-event-filter`](./rules.md#invalid-event-filter), [`invalid-event-config`](./rules.md#invalid-event-config), [`invalid-workflow-dispatch-input`](./rules.md#invalid-workflow-dispatch-input), [`invalid-workflow-call-input`](./rules.md#invalid-workflow-call-input), [`invalid-cron`](./rules.md#invalid-cron), [`cron-too-frequent`](./rules.md#cron-too-frequent), [`invalid-timezone`](./rules.md#invalid-timezone) |
| `workflow-call` | Calls of reusable workflows | [`invalid-workflow-call`](./rules.md#invalid-workflow-call), [`missing-workflow-input`](./rules.md#missing-workflow-input), [`unknown-workflow-input`](./rules.md#unknown-workflow-input), [`missing-workflow-secret`](./rules.md#missing-workflow-secret), [`unknown-workflow-secret`](./rules.md#unknown-workflow-secret), [`invalid-local-workflow`](./rules.md#invalid-local-workflow) |
| `job-needs` | Dependencies and IDs of jobs | [`undefined-job-needs`](./rules.md#undefined-job-needs), [`cyclic-job-needs`](./rules.md#cyclic-job-needs), [`duplicate-job-needs`](./rules.md#duplicate-job-needs), [`duplicate-job-id`](./rules.md#duplicate-job-id) |
| `matrix` | Values and `exclude` of a matrix | [`matrix-duplicate-value`](./rules.md#matrix-duplicate-value), [`matrix-invalid-exclude`](./rules.md#matrix-invalid-exclude) |
| `runner-label` | Labels of runners | [`unknown-runner-label`](./rules.md#unknown-runner-label), [`conflicting-runner-labels`](./rules.md#conflicting-runner-labels), [`invalid-label-pattern`](./rules.md#invalid-label-pattern) |
| `id` | Job and step IDs | [`invalid-id`](./rules.md#invalid-id), [`duplicate-step-id`](./rules.md#duplicate-step-id) |
| `permissions` | Scopes and values of `permissions:` | [`invalid-permissions`](./rules.md#invalid-permissions) |
| `glob` | Glob patterns of filters | [`invalid-glob`](./rules.md#invalid-glob) |
| `credentials` | Hardcoded credentials of containers | [`hardcoded-container-credentials`](./rules.md#hardcoded-container-credentials) |
| `env-var` | Names of environment variables | [`invalid-env-var-name`](./rules.md#invalid-env-var-name) |
| `shell-name` | Names of shells | [`invalid-shell-name`](./rules.md#invalid-shell-name) |
| `deprecated-commands` | Deprecated workflow commands | [`deprecated-commands`](./rules.md#deprecated-commands) |
| `shellcheck`, `pyflakes` | shellcheck and pyflakes on `run:` scripts | [`shellcheck`](./rules.md#shellcheck), [`pyflakes`](./rules.md#pyflakes) |

Rules that actionlint does not have carry a kind too, and a few share one of the kinds above: `unsound-ternary` has the kind `expression`, `action-syntax` has `syntax-check` and `workflow-call-permissions` has `workflow-call`. A script that filters on a kind can therefore see findings that actionlint never reported.

Two commands map a finding to its documentation. The first prints what a rule does, its options and its section of the [checks](./checks.md) page. The second lists every rule with its group, profile, level and summary.

```sh
jactionlint --explain duplicate-key
jactionlint --list-rules
```

## What stays the same and what is tested

The table lists what you can rely on besides the findings, checked by running both tools.

| What | Same in jactionlint | Different |
| --- | --- | --- |
| Text format | `file:line:col: message [label]`, followed by an excerpt of the source | The label is the rule ID, where actionlint prints the kind. |
| Exit status | 0 for no findings, 1 for findings, 2 for an invalid option and 3 for a fatal error | A finding of level `warn` or `info` does not fail the run ([exit status](./cli.md#exit-status)). |
| Templates | `--format` takes a Go template with the fields `Message`, `Filepath`, `Line`, `Column` and `Kind`, and `{{json .}}` keeps its keys | A finding has more fields, such as the rule ID, the severity and the documentation URL ([output formats](./output.md#format-error-messages)). |
| Ignore patterns | A regular expression on the message in `--ignore` and in `paths` | Some messages are reworded, and a pattern can be a rule ID. |
| Config keys | `self-hosted-runner`, `config-variables` and `paths` | jactionlint has more keys, and it rejects keys that it does not know ([config file](#a-config-file-written-for-actionlint)). |

A duplicate job ID shows the label and the wording. For this workflow file, saved as `.github/workflows/x.yaml`:

```yaml
on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo a
  A:
    runs-on: ubuntu-latest
    steps:
      - run: echo a
```

jactionlint prints this finding with the `correctness` profile:

```sh
jactionlint --profile correctness
```

```
.github/workflows/x.yaml:7:3: key "A" is duplicated in "jobs" section. previously defined at line 3, column 3. note that this key is case insensitive [duplicate-key]
  |
7 |   A:
  |   ^~
```

actionlint prints the same finding with another label and another wording:

```
.github/workflows/x.yaml:7:3: key "A" is duplicated in "jobs" section. previously defined at line:3,col:3. note that this key is case insensitive [syntax-check]
  |
7 |   A:
  |   ^~
```

`go test` runs the `correctness` profile on the workflow files and expected outputs that come from the tests of actionlint, with the config file of a workflow file when it has one. For each line of an expected output, jactionlint must report a finding with the same message, line, column and kind, and findings that the output does not list are not checked, because the profile may report more. The test has two limits. Where jactionlint words a message differently or accepts a construct that actionlint rejects, the expected output of the file was edited, so the test shows that the profile covers the checks of actionlint, not that every message is identical. It calls the library only, so the command line, exit statuses, output formats, config discovery and the online checks are not part of it.

<a id="what-is-stricter-than-actionlint"></a>
## Where jactionlint differs

The tables compare the `correctness` profile with actionlint on the same workflow file. A later release of actionlint can close a gap. The cases of the second and third table were checked by running both tools.

### Findings that only jactionlint reports

actionlint has no check for these rules of the `correctness` profile, so a repository that passed actionlint can have findings from them. Turn one off with `rules: {RULE: off}`. For the step keys that `background-step-not-waited` and `invalid-parallel-step` need, actionlint reports an unexpected key instead ([constructs that only jactionlint accepts](#constructs-that-only-jactionlint-accepts)). A run without operands also checks composite actions and `.github/dependabot.yml`, which actionlint does not check ([what a run checks](./usage.md#what-a-run-checks)). The Actions lock file is checked by a rule of the `default` profile.

| Rule | What it finds |
| --- | --- |
| [`action-syntax`](./rules.md#action-syntax) | The metadata of an action does not follow the syntax of `action.yml`. |
| [`dependabot-syntax`](./rules.md#dependabot-syntax) | `.github/dependabot.yml` does not follow the syntax of the Dependabot configuration. |
| [`background-step-not-waited`](./rules.md#background-step-not-waited) | A step reads the outputs of a background step before a `wait` step covers it. |
| [`invalid-parallel-step`](./rules.md#invalid-parallel-step) | A step inside a `parallel` group is not allowed there or refers to a wrong step. |
| [`concurrency-deadlock`](./rules.md#concurrency-deadlock) | A workflow and the local reusable workflow that it calls use the same concurrency group, so GitHub cancels the run. |
| [`conflicting-action-inputs`](./rules.md#conflicting-action-inputs) | Two inputs of a well-known action exclude each other. |
| [`environment-file-redirect`](./rules.md#environment-file-redirect) | A script writes to `GITHUB_OUTPUT` or another environment file without the `$`, or empties the file with `>` after writing to it. |
| [`gh-without-token`](./rules.md#gh-without-token) | A script runs `gh` but neither `GH_TOKEN` nor `GITHUB_TOKEN` reaches the step. |
| [`github-script-syntax`](./rules.md#github-script-syntax) | The script of `actions/github-script` has a JavaScript syntax error. |
| [`local-action-checkout`](./rules.md#local-action-checkout) | A local action is used before a step checks out the repository. |
| [`malformed-expression-delimiters`](./rules.md#malformed-expression-delimiters) | Text such as `${ github.ref }` or `$ {{ x }}` is meant as an expression but is not one. |
| [`multiline-plain-run`](./rules.md#multiline-plain-run) | A `run:` is written over several lines without `\|`, so YAML folds the lines into one command. |
| [`unsound-ternary`](./rules.md#unsound-ternary) | `a && b \|\| c` where `b` can be falsy, so the expression always evaluates to `c`. |
| [`workflow-call-permissions`](./rules.md#workflow-call-permissions) | A caller job grants fewer permissions than the reusable workflow that it calls requires. |
| [`workflow-run-names`](./rules.md#workflow-run-names) | A `workflow_run` trigger names a workflow that does not exist in the repository. |
| [`invalid-ignore-comment`](./rules.md#invalid-ignore-comment), [`expired-ignore`](./rules.md#expired-ignore), [`unused-baseline-entry`](./rules.md#unused-baseline-entry) | A malformed ignore comment, an expired entry of `ignores` and a baseline entry that matches nothing. They report only when you use ignore comments, `ignores` or a baseline. |

### Findings that differ

| Case | actionlint | jactionlint | Rule |
| --- | --- | --- | --- |
| Untrusted input in a place other than a script | Reports scripts only. | Also reports the `options` of a container, the prompt of an AI agent action and, in the `default` profile, free text from outside the workflow such as string inputs and `ref_name` ([the check](./checks.md#check-template-injection-sinks)). | [`template-injection`](./rules.md#template-injection) |
| A property of `github.event` that no webhook event has, such as `github.event.worflow_run` | Reports nothing, because it does not type this object ([actionlint#39](https://github.com/rhysd/actionlint/issues/39)). | Reports the property with a suggestion, except in a workflow with a trigger whose payload is not known ([the check](./checks.md#check-contextual-github-event)). | [`undefined-property`](./rules.md#undefined-property) |
| A quoted value passed to an input of a reusable workflow | Reports `"true"` for an input of type `string` and accepts `'1'` for an input of type `number`. | Treats a quoted value as a string: it accepts `"true"` for a string input and reports `'1'` for a number input. | [`workflow-input-type`](./rules.md#workflow-input-type) |
| A call of an overloaded function such as `contains` that fits no signature | Reports one finding per signature. | Reports one finding about the signature that fits best. | [`invalid-function-call`](./rules.md#invalid-function-call) |
| A context that is not available at a place, such as `runner` in the `env` of a job | Reports the context, and also a property that does not exist, such as `runner.fooooooo`. | Reports the context once. | [`context-availability`](./rules.md#context-availability) |
| The wording of a message | `previously defined at line:3,col:3` | `previously defined at line 3, column 3` | [`duplicate-key`](./rules.md#duplicate-key) |
| The position of a shellcheck finding | The `run:` key of the step. | The token of the script that has the problem ([where shellcheck findings are reported](#where-shellcheck-findings-are-reported)). | [`shellcheck`](./rules.md#shellcheck) |
| shellcheck or pyflakes is not installed | Says nothing. | Prints a `note:` on stderr and does not change the exit status. An explicit `--shellcheck=CMD` or `--pyflakes=CMD` with a command that does not exist exits with 3. | [`shellcheck`](./rules.md#shellcheck), [`pyflakes`](./rules.md#pyflakes) |
| Workflow files in a subdirectory of `.github/workflows` | Checks every `.yml` and `.yaml` file below the directory. | Checks the files directly in the directory, because GitHub does not load the others. A file that you pass as an argument is always checked. | none |

### Constructs that only jactionlint accepts

jactionlint follows the workflow schema of GitHub where it is newer than actionlint, and its built-in data, such as runner labels and the inputs of popular actions, is newer too. jactionlint accepts each construct below. actionlint reports it as the second column says:

| Construct | What actionlint reports | Rule |
| --- | --- | --- |
| `needs: []` and a filter without a value such as `tags:` | The section or the string should not be empty. | [`workflow-syntax`](./rules.md#workflow-syntax) |
| `if: \|` followed by a line with `${{ }}` alone | The condition is always true because of the newline at the end of the block scalar, which GitHub trims. | [`if-always-true`](./rules.md#if-always-true) |
| A top-level `description:`, `cancel-timeout-minutes` on a job, `types` of the `image_version` event and an empty string in the `options` of a `choice` input | An unexpected key, or a string that should not be empty. | [`workflow-syntax`](./rules.md#workflow-syntax) |
| The `stacked` type of `pull_request` and `pull_request_target` | An invalid activity type. | [`invalid-activity-type`](./rules.md#invalid-activity-type) |
| The `drives` permission | An unknown permission scope. | [`invalid-permissions`](./rules.md#invalid-permissions) |
| `github.artifacts` and `github.artifacts_list` | A property that is not defined. | [`undefined-property`](./rules.md#undefined-property) |
| The `steps`, `job`, `runner` and `env` contexts, the status functions and `hashFiles` in `snapshot.if` | A context or function that is not allowed there. | [`context-availability`](./rules.md#context-availability) |
| The step keys `background`, `wait`, `wait-all`, `cancel` and `parallel`, and the top-level key `cache-mode` | An unexpected key, or a step that has neither `run` nor `uses`. | [`workflow-syntax`](./rules.md#workflow-syntax) |
| A runner label or an input of a popular action that is newer than the data of actionlint, such as `ubuntu-26.04` or the input `allow-unsafe-pr-checkout` of `actions/checkout` | An unknown label or an input that is not defined. | [`unknown-runner-label`](./rules.md#unknown-runner-label), [`unknown-action-input`](./rules.md#unknown-action-input) |

### Where shellcheck findings are reported

actionlint reports a shellcheck finding at the `run:` key of the step. jactionlint reports it at the token of the script that has the problem, the `$FOO` of `echo $FOO`, with a region that covers the token. A pull request annotation then sits on the right line and column.

This holds for every way to write the script: a literal block (`|`, `|-`, `|+`), a folded block (`>`), and a plain, single-quoted or double-quoted scalar, with escapes, folded lines and a `${{ }}` that spans several lines. The position is the position in the workflow file. The line and column that shellcheck computed for the script stay in the message, as in `SC2086:info:2:6:` for the second line of a script. When jactionlint cannot map the position, for example for a finding at the end of the script, it reports the finding in the column of the `run:` key.

A tool that keys on the line of a finding sees the difference:

- **An ignore comment keeps working.** A `# jactionlint ignore=shellcheck` comment on the step still covers every line of the script. [Ignore comments](./ignore.md#ignore-comments) has the scopes, including a comment inside the script.
- **An `--ignore` pattern or an `ignore` under `paths` is not tied to a line.** It matches the message or the rule ID, so the difference does not affect it.
- **A tool that keeps `file:line` pairs written for actionlint needs the new lines.** Refresh the list once. A [baseline](./baseline.md#baseline) written by jactionlint does not use line numbers.

## Adopt more checks

After the switch you can raise the profile in steps. `default` adds the security and policy rules, such as `unpinned-uses`, `missing-permissions`, `missing-timeout`, `excessive-permissions`, `concurrency-limits`, `artipacked`, `cache-poisoning`, `dangerous-triggers` and `use-trusted-publishing`. `pedantic` adds the noisy and opinionated rules. Select `default`, then record today's findings in a [baseline](./baseline.md#baseline) so that runs fail only on new findings.

A pin policy shrinks the first wave of `unpinned-uses` findings: it accepts tags for the owners that you trust and keeps the full commit SHA for the rest. The Actions lock file of the repository counts as a pin by default (the option `actions-lock` of `unpinned-uses` is `true`).

```yaml
rules:
  unpinned-uses:
    policies:
      "actions/*": ref-pin
```

The online checks, which ask GitHub about the actions that a workflow uses, are off until you turn them on. [Turn on the online checks](./online.md#turn-on-the-online-checks) shows how, and [Online checks](./online.md#online-checks) describes them.

<a id="hk"></a>
## CI, hooks and editors

The names of the action, the image, the hook and the problem matcher differ from the ones of actionlint. Each task has a page that owns it:

- GitHub Actions: the action `jdx/jactionlint` ([the action](./ci.md#the-action)), mise ([mise](./ci.md#mise)), the Docker image `ghcr.io/jdx/jactionlint` ([Docker](./ci.md#docker)) and the problem matcher file `.github/jactionlint-matcher.json` ([problem matchers](./ci.md#problem-matchers)).
- hk: add `--profile correctness` to the `check`, `check_diff` and `fix` commands of the step to keep the findings of actionlint ([hk](./hooks.md#hk)).
- pre-commit: the hook ID is `jactionlint` ([pre-commit](./hooks.md#pre-commit)).
- Editors and other tools: [editors and other tools](./hooks.md#editors-and-other-tools). Tools written for actionlint do not run jactionlint, and some pass the single-dash options of actionlint, which jactionlint rejects ([tools written for actionlint](./hooks.md#tools-written-for-actionlint)).

## Go programs that import actionlint

A Go program that imports `github.com/rhysd/actionlint` imports `github.com/jdx/jactionlint/v2` instead, and the package is named `jactionlint`. A program that sets no profile runs the `default` profile. Set `Config.Profile` or `LinterOptions.Profile` to `ProfileCorrectness` to keep the checks of actionlint. [Go API](./api.md#go-api) describes the library.

## Next steps

- [Adopt more checks with a baseline](./baseline.md#baseline) without a wall of findings.
- [Command line reference](./cli.md#command-line-options) for every option of jactionlint.
- [Configuration](./config.md) for profiles, rule levels, ignores and the online options.
- [Checks](./checks.md) for what each rule finds, with examples.
- [jactionlint and zizmor](./zizmor-parity.md) for the security and policy rules compared with zizmor.
