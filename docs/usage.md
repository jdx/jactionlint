<a id="usage"></a>
# Run jactionlint

Run jactionlint on a repository, on files or on stdin, choose a profile, look at one rule and set up the programs that jactionlint calls. Output formats, ignores, baselines, fixes, online checks and CI each have their own page.

<a id="jactionlint-command"></a>
## Quick start

A first run needs a repository that has a `.github/workflows` directory.

1. Install jactionlint with one of [the install methods](./install.md#choose-a-method).
2. Change into a Git or Jujutsu repository that has workflow files.
3. Run `jactionlint`. With no operand it checks the workflow files, the composite actions, the Dependabot configuration and the Actions lock file of the repository.
4. Read the findings and the exit status: `0` means no finding that counts and `1` means at least one. [Exit status](./cli.md#exit-status) lists the other values.

This site describes the development version (the `main` branch). `jactionlint --help` lists what your installed version supports, and [Installation](./install.md#build-from-source) shows how to build `main`.

Save this workflow as `.github/workflows/ci.yaml` in a Git repository. It passes the default profile except for one typo: the job `deploy` needs a job `biuld`, which does not exist.

```yaml
name: CI
on:
  push:
    branches: [main]
permissions:
  contents: read
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          persist-credentials: false
      - run: echo building
  deploy:
    needs: [biuld]
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - run: echo deploy
```

Run jactionlint in that repository. With the default profile it prints this finding and the exit status:

```sh
jactionlint
echo $?
```

```
.github/workflows/ci.yaml:19:3: job "deploy" needs job "biuld" which does not exist in this workflow [undefined-job-needs]
   |
19 |   deploy:
   |   ^~~~~~~
1
```

## What a run checks

With no operand jactionlint checks the repository. With operands it checks exactly the files you name.

### Repository mode

The repository is the nearest directory, from the working directory upward, that has a `.git` (a directory, or a file in a worktree or a submodule) or a `.jj` (a [Jujutsu](https://jj-vcs.github.io/jj/) workspace) and also has a `.github/workflows` directory, a `.github/actions` directory, or an `action.yml` or `action.yaml` at its root.

| What | Where | Notes |
| --- | --- | --- |
| Workflow files | The `.yml` and `.yaml` files directly in `.github/workflows` | GitHub does not load files in subdirectories (test data, prompts, tool configuration), so a run without operands does not check them. |
| Dependabot configuration | `.github/dependabot.yml` or `.github/dependabot.yaml` | The Dependabot rules apply, not the workflow rules. See [the check](./checks.md#check-dependabot-syntax). |
| Composite actions | `action.yml` or `action.yaml` in any directory of the repository, and in the directories that a local `uses: ./path` refers to | The search skips directories named `.git`, `node_modules`, `vendor`, `testdata`, `fixtures`, `.venv`, `venv`, `target` and `dist`, but a `uses:` that refers to an action there still brings it in. An `action.yml` directly in `.github/workflows` is a workflow, and one in a subdirectory is an action. See [composite actions](./checks.md#check-composite-actions). |
| The Actions lock file | `.github/workflows/actions.lock`, when it exists | See [the check](./checks.md#check-actions-lock). |

Outside a repository, `jactionlint` without operands exits with status 3 and the message `no project was found in any parent directories of`, followed by the directory.

### Files and stdin

Name the files to check them wherever they are. A file below `.github/workflows/` that you name is checked, even in a subdirectory. Each file uses the config file of the repository that holds it. A file outside any repository uses the [user-global config file](./config.md#configuration-file-location-and-priority) or the defaults.

```sh
jactionlint path/to/workflow1.yaml path/to/workflow2.yaml
```

A directory is not an operand: `jactionlint .github/workflows` exits with status 3. Omit it to check the repository, or name the files.

A `-` reads one input from stdin. It must be the only operand: `jactionlint a.yaml -` exits with status 3, and `--fix` and `--baseline-write` refuse stdin.

```sh
cat path/to/workflow.yaml | jactionlint -
```

An input from stdin gets the config file of a repository only when its name, which `--stdin-filename` sets, is the path of an existing file inside that repository. Otherwise it gets the user-global config file or the defaults.

The name of the input decides how jactionlint checks it. A file operand follows the same rules, and `--stdin-filename` gives a name to stdin:

| Name | Checked as |
| --- | --- |
| `dependabot.yml` or `dependabot.yaml`, in a directory named `.github`, or as a bare name that you give to stdin | A Dependabot configuration |
| `action.yml` or `action.yaml`, anywhere except directly in `.github/workflows` | A composite action |
| `.github/workflows/actions.lock`, or a path that ends in `/.github/workflows/actions.lock` | The Actions lock file |
| Anything else, including the default `<stdin>` | A workflow |

```sh
jactionlint .github/dependabot.yml
cat dependabot.yml | jactionlint --stdin-filename .github/dependabot.yml -
cat action.yml | jactionlint --stdin-filename action.yml -
```

The steps of a composite action are checked with the rules for workflow steps, and the rules that depend on how the action runs use the local workflows that call it. Ignore comments, `paths:`, `--format sarif` and `--fix` work as they do for workflows.

## Read a finding

In the `text` format a finding shows where it is, what is wrong, the rule ID in brackets and the source line. The finding of the quick start has level `error`, which has no prefix. A finding of level `warn` or `info` starts its message with `warning:` or `info:`. In a repository whose only workflow file starts with `on: push` and has no `name:`, `jactionlint --select anonymous-definition` prints a finding of level `warn`:

```
.github/workflows/ci.yaml:1:1: warning: workflow has no "name:", so GitHub shows its file path in the Actions UI. add a top-level "name:" to make the workflow recognizable [anonymous-definition]
  |
1 | on: push
  | ^~~
```

The location is `file:line:column`. The rule ID is the name that `--ignore`, the config file and ignore comments accept. `--show-doc-urls` adds the documentation URL of the rule under each finding, `--show-fix-status` adds what the fix of the finding is or why it has none (see [Why a finding was not fixed](./fix.md#why-a-finding-was-not-fixed)), and `--oneline` and `--format` change the layout: see [the output formats](./output.md#format-error-messages).

Every rule has a default level, listed on [the rules page](./rules.md). Only findings of level `error` make the exit status 1, unless you pass `--strict-exit`: see [Exit status](#exit-status). [The command line reference](./cli.md#command-line-options) lists every option.

<a id="profile"></a>
## Choose a profile

A profile is a set of rules, and each profile includes the one before it. The default profile applies when nothing is configured.

| Profile | Rules |
| --- | --- |
| `correctness` | What actionlint checks, plus the bug detectors of jactionlint. |
| `default` | Adds the security posture and policy rules. |
| `pedantic` | Adds the noisier and more opinionated rules. |

No profile turns on the online rules, which need `--online`, or the opt-in rules `forbidden-uses`, `required-actions` and `timeout-too-long`, which run only when the config file configures them. [Profiles](./config.md#profiles) is the full definition.

Choose a profile on the command line, which wins, or in the config file:

```sh
jactionlint --profile correctness   # what actionlint checks, plus the bug detectors of jactionlint
jactionlint --profile pedantic      # the rules of every profile
```

```yaml
# .github/jactionlint.yaml
profile: correctness
rules:
  unpinned-uses: error # a rule of a higher profile on top of it
```

[Coming from actionlint](./actionlint.md#coming-from-actionlint) shows how `correctness` relates to actionlint.

### The first run

The default profile includes the security and policy rules, so a repository that never ran it often reports many findings. When a run of the `text` or `oneline` format reports 20 or more findings, jactionlint prints one line to stderr that says what to do next. The numbers are those of your run. This note comes from five small workflow files:

```
note: 30 findings in 5 files, which is a lot for a first run of the default profile. to adopt the checks without fixing everything first, record the findings with --baseline-write: later runs then fail only on new findings. see --format summary for the counts per rule; the name at the end of a finding is its rule ID, which --ignore, the config and ignore comments accept; for the checks of actionlint only use --profile correctness. silence this note with --no-hints or JACTIONLINT_NO_HINTS=1
```

The note appears only when all of these hold:

- Stderr is a terminal, or `CI` is set to a value other than empty, `false` or `0`, or `GITHUB_ACTIONS` is set to a value other than empty or `false`.
- At least one linted file uses a profile above `correctness`. The flag `--profile correctness` and `profile: correctness` in the config file both silence it, because those findings are mistakes and not something to adopt.
- You gave neither `--no-hints` nor `JACTIONLINT_NO_HINTS` with a value other than empty, `0` or `false`.

Once a baseline hides findings, the note no longer advises writing one. [Write and apply the baseline](./baseline.md#write-and-apply-the-baseline) shows what `--baseline-write` prints.

Ways forward:

- Run with `--profile correctness` to start with the checks of actionlint.
- Record today's findings and fail only on new ones: see [Baseline](./baseline.md#baseline).
- Fix what has a fix: see [Fix findings automatically](./fix.md#fix-errors-automatically).

## Look at one rule

Three commands inspect or narrow the rule set: `--explain` for one rule, `--select` to run some, `--list-rules` for all.

### Explain a rule

`jactionlint --explain RULE` prints what the installed version knows about a rule: its summary, group, profile, default level, whether it has a fix and what that fix needs (an option or `--online`), its options, the configuration and ignore comment to use, and its section of [the checks page](./checks.md) with the examples. It needs no network.

```sh
jactionlint --explain missing-timeout
```

An unknown rule ID exits with status 2 and names the closest ID when there is one, as in `unknown rule ID "missing-timeot" in --explain. did you mean "missing-timeout"?`. A rule ID that was merged into another rule gets a message that names the new ID. [The rules page](./rules.md) lists the IDs, and [shell completion](#shell-completion) offers them.

### Run one rule, or run without the ignores

`--select` reports only the rules you name, whatever the profile or the configuration says. Use it to review all findings of one rule, to run one check in a hook, or to preview a rule that your profile does not enable.

```sh
jactionlint --select unpinned-uses,missing-timeout
jactionlint --select anonymous-definition   # a pedantic rule, on the default profile
```

- The option takes rule IDs separated by commas and can be repeated. An unknown ID exits with status 2 and names the closest ID.
- A selected rule that the config file turns off runs at its default level. A level that the config file sets, such as `unpinned-uses: warn`, stays, and so do the options of the rule. Every other rule reports nothing, and `shellcheck` and `pyflakes` run only when you select them.
- Rules that need `--online` still need it, and the opt-in rules still need their configuration: `--select forbidden-uses` reports nothing until `forbidden-uses` has `allow`, `deny`, `images-allow` or `images-deny` in the config file.

`--no-ignores` is the opposite: it applies no ignore and no baseline, so the run shows what they hide. [Keep your ignores honest](./ignore.md#keep-your-ignores-honest) lists what it switches off.

```sh
jactionlint --no-ignores --format summary   # what do the ignores of the repository hide?
```

### List the rules

`jactionlint --list-rules` prints every rule with its group, the profile that enables it and its default level. The profile column shows `online` for the rules that need `--online` and `opt-in` for the rules that need configuration.

```sh
jactionlint --list-rules | grep -E '^(RULE|missing-timeout|forbidden-uses|impostor-commit|anonymous-definition) '
```

```
RULE                               GROUP        PROFILE      LEVEL  SUMMARY
anonymous-definition               policy       pedantic     warn   A workflow has no top-level name:.
forbidden-uses                     policy       opt-in       error  An action or reusable workflow is not allowed or is denied by the configuration.
impostor-commit                    security     online       error  A hash-pinned action uses a commit which is not part of the repository's own history (it exists only in a fork).
missing-timeout                    policy       default      error  A job does not set timeout-minutes.
```

`--format json` and `--format jsonl` print the same list as JSON, with the fields `id`, `group`, `profile`, `default_level`, `online`, `fixable`, `summary` and `doc_url`. `--format oneline` prints the same table as `text`, and any other format exits with status 2.

## Shell completion

`jactionlint --completion=SHELL` prints a completion script for `bash`, `zsh`, `fish` or `powershell`; `pwsh` works as a synonym of `powershell`. The script completes the options, the values of `--format`, `--profile`, `--min-severity`, `--color`, `--fix`, `--online` and `--completion`, and the [rule IDs](./rules.md) for `--ignore`, `--explain`, `--fix-rules` and `--select`. `--fix-rules` and `--select` take a comma-separated list. The binary generates the script from its own options and rules, so generate it again after an upgrade.

```sh
# bash: in ~/.bashrc
source <(jactionlint --completion=bash)

# zsh: in ~/.zshrc (after compinit), or save the output as _jactionlint in a directory of $fpath
source <(jactionlint --completion=zsh)

# fish
jactionlint --completion=fish > ~/.config/fish/completions/jactionlint.fish

# PowerShell: in $PROFILE
jactionlint --completion=powershell | Out-String | Invoke-Expression
```

## External tools

jactionlint runs other programs on the scripts of a workflow when it finds them. Without a program, jactionlint does not check those scripts and says so in a note.

| Program | Checks | Option |
| --- | --- | --- |
| shellcheck | The `run:` scripts, with the `shellcheck` rule. See [the check](./checks.md#check-shellcheck-integ). | `--shellcheck` |
| pyflakes | The scripts with `shell: python`, with the `pyflakes` rule | `--pyflakes` |
| ruff | The same scripts, instead of pyflakes | `--ruff` |
| node 20 or later | The scripts of `actions/github-script`, with the `github-script-syntax` rule. See [the check](./checks.md#check-github-script-syntax). | None |

jactionlint looks the programs up on `PATH`. [The Docker image](./install.md#docker-image) has shellcheck and pyflakes only, so it skips the `github-script-syntax` check. [Companion programs](./install.md#companion-programs) says how to install them.

### shellcheck and pyflakes

`--shellcheck` and `--pyflakes` take a command name that jactionlint looks up on `PATH`, or a path. An empty value turns the integration off, and a tool that you turned off is never noted. Turning them off also saves the time of the processes that jactionlint starts for every script.

```sh
jactionlint --shellcheck= --pyflakes=
```

A command that you name must exist: `--shellcheck=/bad/path` exits with status 3 and says that the command was not found or is not executable, instead of running without the tool. The same holds for `--pyflakes` and `--ruff`. The default names are not required, and `--shellcheck=shellcheck` counts as named too.

jactionlint runs shellcheck with `--norc`, so a `.shellcheckrc` in the working directory cannot change the results. It also turns off a fixed set of shellcheck codes that give false findings on workflow scripts, which [the check](./checks.md#check-shellcheck-integ) lists with the reason for each. To use a shellcheckrc, to pass arguments or to turn on optional checks, set [the `shellcheck` key](./config.md#options-for-shellcheck) of the config file, or give a file with `--shellcheck-rcfile`:

```sh
jactionlint --shellcheck-rcfile=.shellcheckrc
```

The file must exist, or jactionlint exits with status 3. The option resolves a relative path from the working directory and wins over the key `shellcheck.rcfile`, which resolves it from the repository root. Use the key where `SHELLCHECK_OPTS` cannot be set, for example in a [pre-commit](./hooks.md#pre-commit) hook on pre-commit.ci:

```yaml
# .github/jactionlint.yaml
shellcheck:
  rcfile: .shellcheckrc
```

`SHELLCHECK_OPTS` still works, because shellcheck reads it itself.

<a id="external-tools-ruff"></a>
### ruff

`--ruff=ruff` checks the Python scripts (`shell: python`) with [ruff](https://docs.astral.sh/ruff/) instead of pyflakes. jactionlint runs `ruff check --isolated --no-cache --select F --output-format json -`: only the `F` rules of ruff, which are the rules of pyflakes, and none of the configuration of the directory, so a `pyproject.toml` in the working directory does not change the result. The findings keep the rule ID `pyflakes`, with a message such as ``ruff reported issue in this script: 2:7: F821 Undefined name `undefined_name` ``, and `--pyflakes` is ignored. The command must exist, or jactionlint exits with status 3.

### node

The `github-script-syntax` rule belongs to the `correctness` profile, so it runs by default. It needs `node` 20 or later on `PATH` to parse the scripts of `actions/github-script`. Without it, jactionlint skips the check and prints a note. There is no option for node: turn the rule off with `rules: {github-script-syntax: off}` in the config file to silence the note.

### When shellcheck or pyflakes is missing

A run that finds no tool on `PATH` for a script that needs it is not silent about it. It prints one line per tool to stderr after the findings, with the number of scripts that it did not check. The note does not change the exit status. This is the output of a run without the tools, in a repository whose workflow has one shell step, one Python step and one `actions/github-script` step:

```
note: shellcheck was not found, so it did not check 1 shell script. install it, or pass --shellcheck= to skip it on purpose
note: pyflakes was not found, so it did not check 1 Python script. install it, or pass --pyflakes= to skip it on purpose
note: node 20 or later was not found, so it did not check 1 JavaScript script. install it, or turn the check off with "rules: {github-script-syntax: off}" in the configuration
```

- A tool gets a note once per run, and only when a script needed it: a repository whose steps are all `uses:` gets none.
- Without shellcheck, the rule [`shell-syntax`](./checks.md#check-shell-syntax) still reads the `bash` and `sh` scripts with a parser that is built into jactionlint and reports the syntax errors that stop a script, such as a missing `fi`. It reports nothing else, so it does not replace shellcheck.
- Silence a note with `--shellcheck=` or `--pyflakes=`, which say that you do not want the tool, or with `--no-hints` or `JACTIONLINT_NO_HINTS`, which silence the other notes too.
- `--format sarif` prints no line. The log has a notification in `invocations[].toolConfigurationNotifications` instead, which `--no-hints` leaves out, and the property `externalTools` in `runs[].properties`. A tool that ran has `status` `ran`. A tool that was missing has `status` `not found`, and `skippedScripts` counts the scripts that it would have checked. A tool that no script needed, and a tool that you turned off with an empty command, are not listed.

```sh
jactionlint --format sarif | jq '.runs[0].properties'
```

```json
{
  "externalTools": {
    "node": {
      "status": "not found",
      "command": "node",
      "scripts": 0,
      "skippedScripts": 1
    },
    "pyflakes": {
      "status": "not found",
      "command": "pyflakes",
      "scripts": 0,
      "skippedScripts": 1
    },
    "shellcheck": {
      "status": "not found",
      "command": "shellcheck",
      "scripts": 0,
      "skippedScripts": 1
    }
  }
}
```

## Exit status

The exit status is `0` when no finding counts, `1` when a finding counts, `2` for an invalid command line and `3` for a failure such as a missing repository or an unreadable file. A finding counts when its level is `error`, or at any level with `--strict-exit`. An ignore, a [baseline](./baseline.md#baseline) and `--min-severity` hide findings, and hidden findings do not count. [The exit status table](./cli.md#exit-status) lists every cause.

## When something goes wrong

| What you see | Cause | Where to go |
| --- | --- | --- |
| `jactionlint: unknown option --form (try --help)` | The option does not exist in your release, or it is an abbreviation, which jactionlint does not accept. | [Option syntax](./cli.md#option-syntax). `jactionlint --help` lists the options of your release. |
| `no project was found in any parent directories of` | No `.git` or `.jj` next to a `.github/workflows` directory, a `.github/actions` directory or an `action.yml` above the working directory. | [Repository mode](#repository-mode). Change the directory, or name the files. |
| `could not read ".github/workflows": read .github/workflows: is a directory` | A directory is not an operand. | Run `jactionlint` without operands, or name the files. |
| `could not read "-"` | `-` is not the only operand. | [Files and stdin](#files-and-stdin). |
| `note: shellcheck was not found` | The tool is not on `PATH`. | [When shellcheck or pyflakes is missing](#when-shellcheck-or-pyflakes-is-missing). |
| `--shellcheck=/bad/path: the command was not found or is not executable` | A command that you named does not exist. | [shellcheck and pyflakes](#shellcheck-and-pyflakes). |
| `unknown key "self-hosted-runnr" in the configuration at line 2, column 1` | The config file has a key that does not exist. The status is 3. | [The configuration file](./config.md#configuration-file). |
| `--online and --no-online cannot be combined` | Two options contradict each other. | [Option combinations that fail](./cli.md#option-combinations-that-fail). |
| `could not write` a workflow file, with `no temporary file can be made in its directory` | `--fix` and `--baseline-write` write a temporary file next to the target, and the directory is not writable, as in a Docker container that runs as another user. | [Files it writes](./cli.md#files-it-writes). For Docker, see [the Docker image](./ci.md#docker). |
| `--fix` prints no `Fixed` line and the findings stay | The rules of those findings have no safe fix. | [Fix findings automatically](./fix.md#fix-errors-automatically). |
| A first run reports dozens of findings | The default profile includes the security and policy rules. | [The first run](#the-first-run). |

## Where to go after the first run

<a id="fix-errors-automatically"></a>
### Fix findings automatically

`jactionlint --fix` applies the safe fixes in place, `--fix=unsafe` also applies the fixes that can change behavior, and `--diff` previews them. [Fix findings automatically](./fix.md#fix-errors-automatically) explains the safe and the unsafe fixes, how to find the rules that have a fix, why a finding has none (`--show-fix-status` prints it under the finding), and how fixing converges.

<a id="ignore-some-errors"></a>
### Ignore a finding

Silence a finding with an ignore comment `# jactionlint ignore=RULE` above the line, with an entry in the config file, or with `--ignore`. The rule ID at the end of the finding is the name to use. [Ignore findings](./ignore.md#ignore-some-errors) explains the syntax and the scope of each form.

<a id="baseline"></a>
### Adopt with a baseline

`jactionlint --baseline-write` records today's findings, so that later runs report only new ones. [Adopt new checks with a baseline](./baseline.md#baseline) gives the steps.

<a id="online-checks"></a>
### Turn on the online checks

Some checks need the GitHub API, for example whether a pinned commit belongs to the repository. They are off by default, and jactionlint does not use the network unless you turn them on with `--online`, `JACTIONLINT_ONLINE` or the `online` key. [Online checks](./online.md#online-checks) covers tokens, the cache and GitHub Enterprise Server.

### Check only what a pull request changed

`jactionlint --changed-since origin/main` shows only the findings on the lines that changed since the merge base with `origin/main`, so a pull request check reports what the pull request adds. [Show only what a pull request changed](./ci.md#show-only-what-a-pull-request-changed) gives the workflow and the limits.

<a id="hk"></a>
### Run it in CI and Git hooks

[Run jactionlint in CI](./ci.md#the-action) shows the GitHub Action, mise, Docker and the download script. [Git hooks and editors](./hooks.md#hk) shows hk, pre-commit and the editors.

## Next steps

- [Command line reference](./cli.md#command-line-options) for every option, variable and exit status.
- [The configuration file](./config.md) for profiles, rule levels and shared settings.
- [Output formats](./output.md#format-error-messages) for `json`, `sarif` and templates.
- [Checks](./checks.md) for what each check does, with examples.
- [Rules](./rules.md) for the ID, group, level and options of every rule.
