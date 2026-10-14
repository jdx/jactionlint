<a id="fix-errors-automatically"></a>
# Fix findings automatically

`jactionlint --fix` rewrites the files that it checks to repair the findings that have a mechanical fix, and `--diff` shows the changes first. This page covers previewing, applying and limiting fixes, safe and unsafe fixes, the fixes that need the online checks, the reason a finding is not fixed and how to print it, and what a fix run guarantees.

## Preview and apply fixes

Preview the changes with `--diff`, apply them with `--fix` and read what remains: a finding without a fix stays in the output. The examples use this workflow file, `.github/workflows/ci.yaml`, and the default profile:

```yaml
name: CI
on:
  pull_request:
jobs:
  build:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
      - run: echo "${{ github.event.pull_request.title }}"
```

1. **Preview the fixes.** `--diff` computes the fixes of `--fix` and writes nothing.

   ```sh
   jactionlint --diff
   ```

   ```
   --- a/.github/workflows/ci.yaml
   +++ b/.github/workflows/ci.yaml
   @@ -1,9 +1,20 @@
    name: CI
    on:
      pull_request:
   +
   +concurrency:
   +  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
   +  cancel-in-progress: true
   +
   +permissions:
   +  contents: read
    jobs:
      build:
        runs-on: ubuntu-24.04
        steps:
          - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
   -      - run: echo "${{ github.event.pull_request.title }}"
   +        with:
   +          persist-credentials: false
   +      - run: echo "${PULL_REQUEST_TITLE}"
   +        env:
   +          PULL_REQUEST_TITLE: ${{ github.event.pull_request.title }}
   .github/workflows/ci.yaml:12:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set "default-minutes" in the "missing-timeout" rule options of the configuration to turn on the --fix fix [missing-timeout]
      |
   12 |   build:
      |   ^~~~~~
   Would fix 4 problem(s) in 1 file(s)
     artipacked: 1
     concurrency-limits: 1
     missing-permissions: 1
     template-injection: 1
   ```

2. **Apply the fixes.** `--fix` rewrites the files and reports what remains.

   ```sh
   jactionlint --fix
   ```

   ```
   .github/workflows/ci.yaml:12:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set "default-minutes" in the "missing-timeout" rule options of the configuration to turn on the --fix fix [missing-timeout]
      |
   12 |   build:
      |   ^~~~~~
   Fixed 4 problem(s) in 1 file(s)
     artipacked: 1
     concurrency-limits: 1
     missing-permissions: 1
     template-injection: 1
   ```

One finding remains, because its fix needs a setting that jactionlint does not choose: see [choose which fixes to apply](#choose-which-fixes-to-apply). A second `jactionlint --fix` changes nothing. The line numbers of the remaining findings refer to the file after the fixes, also in the preview: `build:` is at line 5 of the original file and at line 12 in the output.

`--diff` and `--fix` split their output between the streams, and print the remaining findings in the format that `--format` selects. The `--diff` block above shows both streams in the order that a terminal prints them.

| | `--diff` | `--fix` |
| --- | --- | --- |
| stdout | The unified diff. | The remaining findings. |
| stderr | The remaining findings, then `Would fix N problem(s) in M file(s)` and one line per rule. | `Fixed N problem(s) in M file(s)` and one line per rule. Nothing when no fix was applied. |
| Exit status | 1 when there is a diff or a finding of level `error` remains, otherwise 0. | 1 when a finding of level `error` remains, otherwise 0. |

`--strict-exit` counts the remaining findings of level `warn` and `info` too. A fix run also exits with status 3 when a fix is refused or does not converge, or when a file cannot be written (see [what fixing guarantees](#what-fixing-guarantees)), and with status 2 for a usage error such as `--fix-rules` without `--fix` or `--diff`. [The exit status table](./cli.md#exit-status) lists every cause.

- **Which files.** Without arguments, jactionlint fixes the files that a plain run checks (see [what a run checks](./usage.md#what-a-run-checks)). With paths, as in `jactionlint --fix .github/workflows/ci.yaml`, it fixes only those files. It cannot fix stdin: `jactionlint --fix -` and `jactionlint --diff -` exit with status 2, because the fixed text would not be saved.
- **Which findings.** A finding that an [ignore](./ignore.md#ignore-some-errors) hides is not fixed. A finding that [the baseline](./baseline.md#baseline) hides is fixed, which pays the baseline down.
- **Apply a diff later.** The diff names the files relative to the directory where you ran the command. In that directory, `git apply` (or `patch -p1`) makes the changes:

  ```sh
  jactionlint --diff > fixes.patch
  git apply fixes.patch
  ```

## Safe and unsafe fixes

`--fix` applies only the fixes that keep the behavior of the workflow, and `--fix=unsafe` also applies the fixes that may change it. Write the `=`: `--fix unsafe` is `--fix` and a file named `unsafe`.

This release workflow has two findings with an unsafe fix. `concurrency-cancels-release` stops a new run from cancelling a release that is still running, and `pipeline-without-pipefail` makes a failure of `curl` fail the step instead of being hidden by the pipeline:

```yaml
name: Release
on:
  push:
    tags: ["v*"]
permissions: {}
concurrency:
  group: release
  cancel-in-progress: true
jobs:
  notes:
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          persist-credentials: false
      - run: |
          curl -fsSL https://api.github.com/repos/jdx/jactionlint/releases/latest | jq -r .tag_name
```

`jactionlint --diff` prints no diff for this file: both findings stay in the output. Preview the unsafe fixes, read the diff, and apply them with `jactionlint --fix=unsafe`:

```sh
jactionlint --diff --fix=unsafe
```

```
--- a/.github/workflows/release.yaml
+++ b/.github/workflows/release.yaml
@@ -5,7 +5,7 @@
 permissions: {}
 concurrency:
   group: release
-  cancel-in-progress: true
+  cancel-in-progress: false
 jobs:
   notes:
     runs-on: ubuntu-24.04
@@ -15,4 +15,5 @@
         with:
           persist-credentials: false
       - run: |
+          set -o pipefail
           curl -fsSL https://api.github.com/repos/jdx/jactionlint/releases/latest | jq -r .tag_name
Would fix 2 problem(s) in 1 file(s)
  concurrency-cancels-release: 1
  pipeline-without-pipefail: 1
```

Whether a fix is safe depends on the rule and on the shape of the workflow. The fixes of `template-injection` show the range:

- A quoted reference such as `echo "${{ github.event.pull_request.body }}"` has a safe fix that moves the expression into `env:`.
- An unquoted value of an assignment such as `TITLE=${{ github.event.pull_request.title }}` has an unsafe fix.
- An unquoted argument such as `echo ${{ github.event.pull_request.head.ref }}` has no fix, because quoting it would change word splitting.

To list the unsafe fixes of a repository, read the `unsafe` field of the `fix` object in the JSON output:

```sh
jactionlint --format json | jq -r '.[] | select(.fix.unsafe) | "\(.filepath):\(.line) \(.id)"'
```

```
.github/workflows/release.yaml:8 concurrency-cancels-release
.github/workflows/release.yaml:18 pipeline-without-pipefail
```

The section of each rule on [the checks page](./checks.md) says when its fix applies and why an unsafe fix is unsafe, and `jactionlint --explain RULE` prints that section.

## Choose which fixes to apply

Limit a fix run to some rules, and set the options that some fixes need.

`--fix-rules` takes a comma separated list of rule IDs and applies only the fixes of those rules, with `--fix` or `--diff`. The other findings are still reported.

```sh
jactionlint --fix --fix-rules missing-timeout,artipacked
```

The `fix.rules` key of [the config file](./config.md#baseline-and-fixes) sets the same list for every run, and `--fix-rules` overrides it. A rule ID that does not exist exits with status 2 and suggests the closest ID.

Some rules have a fix that writes a value which jactionlint does not choose, so they have no fix until you set the option:

| Rule | Option | The fix writes |
| --- | --- | --- |
| `missing-timeout` | `default-minutes` | `timeout-minutes` with that number of minutes. |
| `dependabot-cooldown` | `default-days` | `cooldown.default-days` with that number of days. |
| `mutable-runner-label` | `pin` | The label that `pin` maps the moving label to, for example `ubuntu-24.04` for `ubuntu-latest`. Without an entry for the label, the finding has the reason `option-required`, which names `mutable-runner-label.pin`. |

```yaml
# .github/jactionlint.yaml
rules:
  missing-timeout:
    default-minutes: 15
  dependabot-cooldown:
    default-days: 7
```

Continue the first example. After you set `default-minutes`, run `jactionlint --fix` again. It fixes the last finding and exits with status 0, and a plain `jactionlint` run then reports nothing:

```sh
jactionlint --fix
```

```
Fixed 1 problem(s) in 1 file(s)
  missing-timeout: 1
```

See [rule options](./config.md#rules) for the options and their limits.

## Fixes that need the online checks

Some fixes need a lookup, so they work only with `--online`. Without it, the findings of `unpinned-uses`, `unpinned-images` and `missing-version-comment` carry `online-required`, and `ref-version-mismatch`, an online rule, does not run.

- `unpinned-uses` with `--online --fix` replaces a tag with its commit and names the tag in a comment. The fix is safe. See [pin tags to commits](./online.md#pin-tags-to-commits-with-online-fix).
- `missing-version-comment` (a rule of the `pedantic` profile) with `--online --fix` adds the version comment to a pin that has none: the most specific version tag of the commit, such as `# v4.2.2`. The fix is safe, because it only adds a comment. See [pin tags to commits](./online.md#pin-tags-to-commits-with-online-fix).
- `unpinned-images` with `--online --fix=unsafe` pins an image tag to a digest. The fix is unsafe, because the image behind a tag moves. See [pin container images to digests](./online.md#pin-container-images-to-digests-with-online-fix-unsafe).
- `ref-version-mismatch` has an unsafe fix that rewrites the version in the comment to the tag of the pinned commit.

## Which rules have a fix

Look a rule up instead of reading a list: the rules page and the command line say whether a rule has a fix.

- The `Fixable` column of the table on [the rules page](./rules.md) says whether a rule has a fix.
- `jactionlint --explain RULE` prints a `Fix:` line, and a `Fix needs:` line for a rule whose fix needs an option or `--online`:

  ```sh
  jactionlint --explain missing-timeout | grep '^Fix'
  ```

  ```
  Fix:            yes. --fix applies the safe fixes; a fix which may change the behavior of the workflow needs --fix=unsafe
  Fix needs:      the option default-minutes (jactionlint does not choose a timeout). Without it a finding says why it has no fix (no_fix of json, noFixCode of sarif, --show-fix-status)
  ```

- `jactionlint --list-rules --format json` has a `fixable` field for each rule. The text form of `--list-rules` has no such column.

  ```sh
  jactionlint --list-rules --format json | jq -r '.[] | select(.fixable) | .id'
  ```

A rule is fixable when it can fix some of its findings. Whether one finding has a fix depends on its shape, on the options of the rule and on `--online`. Setting an option does not make every finding of the rule fixable: the reason of a finding says what its own fix needs.

## Why a finding was not fixed

A finding of a rule that can fix some of its findings is in one of four states, and the output formats tell them apart:

| State | `json`, `jsonl` | SARIF | `--show-fix-status` |
| --- | --- | --- | --- |
| A safe fix, which `--fix` applies | `fix` | `fixes` | `= fix: DESCRIPTION (apply it with --fix)` |
| A fix that may change behavior, which `--fix=unsafe` applies | `fix` with `unsafe: true` | `noFixCode: unsafe-edit`, because the log never carries an unsafe fix | `= fix: DESCRIPTION (may change behavior: apply it with --fix=unsafe)` |
| No fix, with a reason | `no_fix` with a `code` and a `reason` | `noFixCode` and `noFixReason` | `= no fix (CODE): REASON` |
| Advice only: the rule never fixes this kind of finding | neither | neither | no line |

<a id="fix-status"></a>
### Print the status of each finding

The text format is one line per finding that other tools parse, so it shows no status by default. `--show-fix-status` adds a line under each finding of the `text` format: the fix, or the reason there is none. These findings are those of the workflow file of [the first example](#preview-and-apply-fixes) before `--fix` changed it, with two rules selected:

```sh
jactionlint --show-fix-status --select artipacked,missing-timeout
```

```
.github/workflows/ci.yaml:5:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set "default-minutes" in the "missing-timeout" rule options of the configuration to turn on the --fix fix [missing-timeout]
  |
5 |   build:
  |   ^~~~~~
  = no fix (option-required): set the missing-timeout option default-minutes to enable the fix, jactionlint does not choose a timeout
.github/workflows/ci.yaml:8:15: actions/checkout leaves the GITHUB_TOKEN in the git config of the workspace, where a later step such as an artifact upload can publish it. set "persist-credentials: false" under "with:" unless a later step needs to push [artipacked]
  |
8 |       - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
  = fix: Set persist-credentials: false (apply it with --fix)
```

`missing-timeout` has no fix because its fix needs `default-minutes` (see [choose which fixes to apply](#choose-which-fixes-to-apply)), and `artipacked` has a safe fix. With `--format oneline` the option prints nothing more, `--format github` appends the status to the message as a second line, and `json`, `jsonl`, `sarif`, `gcc` and `summary` print the same with and without it, because the data is in them already.

### Read the reason as data

A finding of a fixable rule that has no fix usually says why in its `no_fix` object, which the JSON, JSON Lines and SARIF formats carry:

```sh
jactionlint --format json | jq -r '.[] | select(.no_fix) | "\(.id) \(.no_fix.code): \(.no_fix.reason)"'
```

```
missing-timeout option-required: set the missing-timeout option default-minutes to enable the fix, jactionlint does not choose a timeout
```

In SARIF, the result has the properties `noFixCode` and `noFixReason`. A finding with neither `fix` nor `no_fix` is advice only: its rule has no fix, or it never fixes this kind of finding (`concurrency-limits` fixes only a workflow that pull requests alone start).

| Code | Meaning | What to do |
| --- | --- | --- |
| `option-required` | The fix needs a value that jactionlint does not choose. `no_fix.option` names it as `rule.option`. | Set the option in the config file (see [choose which fixes to apply](#choose-which-fixes-to-apply)). |
| `option-invalid` | The option has a value that the fix cannot use, such as `default-minutes: 0`. | Correct the value: the reason gives the limit. |
| `online-required` | The fix needs GitHub to resolve a tag or an image. | Add `--online` (see [fixes that need the online checks](#fixes-that-need-the-online-checks)). |
| `lookup-failed` | The online checks are on but the lookup failed, for example for a repository that does not exist or because of a rate limit. | Fix the cause that the reason names, such as the name or a token, and run again. |
| `unsupported-shape` | The YAML is written in a shape that the fix does not edit exactly, such as a job in flow style (`build: {runs-on: ubuntu-24.04, steps: [...]}`), a job with a YAML anchor, a scalar written with an escape, a script that is not a literal block or a file with a bare carriage return. | Make the change by hand: the reason says what to add. |
| `needs-judgment` | The right change depends on a decision, or an automatic change would not be safe, such as an unquoted argument that holds an expression. | Edit the workflow by hand: the reason says how. |

SARIF adds codes of its own, because its log carries the safe fixes alone. A finding with an unsafe fix has `unsafe-edit`: apply it with `--fix=unsafe`. `edit-conflict` means that the fix overlaps the fix of another finding in the same log: apply that one and run jactionlint again. `invalid-edit` means that a fix attached by a custom rule does not fit the source.

## Fix in a container, in CI or in a hook

A fix run writes next to each file that it changes, so it needs a user that can write there.

- **Docker.** The Docker image runs as the user `guest`, which cannot write to the files of a mounted repository that you own. Run the container as your user: [Run the image](./ci.md#run-the-image) shows the command, and you add `--fix` to the arguments.
- **Review in CI.** `jactionlint --diff` prints the patch that a developer can apply, and exits with status 1 when fixes are available or a finding of level `error` remains.
- **Editors and hooks.** `--format sarif` carries the safe fixes for the tools that apply them. [hk](./hooks.md#hk) runs `jactionlint --fix` for a finding that has no fix in the log, and [pre-commit](./hooks.md#pre-commit) passes `--fix` as an argument of the hook.

<a id="how-fixing-converges-and-what-it-checks"></a>
## What fixing guarantees

A fix run changes a file only in ways that it has checked, and it leaves the file alone when it cannot. [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#rules-with-a-fix) describes the order of the fixes and the check for contributors.

- **It converges.** Each pass lints the text, applies the fixes that do not overlap and lints the result again, until no fix is left, so a second run changes nothing. A safe fix goes before an unsafe one, and the rules go in a fixed order, so the result does not depend on the order of the findings.
- **It refuses a fix that breaks the file.** The result of a pass must be valid YAML that differs from the text before it only where the edits are. jactionlint then tries the fixes one by one, refuses the one that breaks the file and applies the others. A refused fix prints `error: <file>: <reason> (rules: <ids>)` on stderr, the rule is not fixed again in that file during the run, and the run exits with status 3.
- **It cannot prove an edit right.** The check guards against bugs in a fix, not against a wrong value in the right place, so read the diff of an unsafe fix.
- **It does not loop.** After 10 passes with fixes still left, none of the fixes is written to the file, and the run exits with status 3 and names the rules. When the text of a pass comes back, because two rules undo each other, the file keeps the passes before the cycle, and the run also exits with status 3.
- **It escapes text for its place.** A fix that inserts text into a scalar escapes it for how the scalar is written, and an environment variable value that is not a plain safe string is quoted. A fix that cannot represent its text in the place offers no fix.
- **It writes once and atomically.** Fixing happens in memory. jactionlint writes a temporary file in the directory of the file and renames it over the file, so the file holds the old or the new text and never half of it. The directory must be writable: otherwise the run exits with status 3 with `could not write "<file>": no temporary file can be made in its directory, so the file was left as it was`.
- **It keeps the file's properties.** The permission bits, the line breaks (CRLF stays CRLF) and a symbolic link (the link stays a link, and the target is rewritten) are kept. A file that changed on disk while it was being fixed is not overwritten: the run exits with status 3 with `the file changed while it was being fixed`.

## Next steps

- [Adopt new checks with a baseline](./baseline.md#baseline): fix findings and shrink the baseline.
- [Online checks](./online.md#online-checks): the flags and the lookups behind the online fixes.
- [The config file](./config.md#rules): the options that fixes need and `fix.rules`.
- [Rules](./rules.md): the level, the profile and whether a fix exists for each rule.
- [Run jactionlint as a hook](./hooks.md#hk): fix files before a commit.
