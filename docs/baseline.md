<a id="baseline"></a>
<a id="adopt-a-stricter-configuration-with-a-baseline"></a>
# Adopt new checks with a baseline

A baseline records today's findings so that jactionlint fails only on new ones: use it to adopt a stricter profile before everything is fixed. This page covers writing, applying and shrinking a baseline, its file format and how a finding matches an entry.

## Before you start

- [Install jactionlint](./install.md#choose-a-method) and run the commands in the repository, the directory that holds `.git` or `.jj`.
- Decide where the baseline will be read: your CI, your hooks and your machine. A finding that the writing run cannot produce is not recorded, so CI would report it as new. Write the baseline with the config file, the profile and the options of that run: the same `--online` setting, and `shellcheck` and `pyflakes` installed if CI has them.

## When to use a baseline

A baseline suits debt that you intend to pay down over time while new findings still fail the build.

| Situation | Use |
| --- | --- |
| `--fix` can repair the findings | [`--fix`](./fix.md#fix-errors-automatically) |
| A finding is wrong, or you accept it for good | An [ignore](./ignore.md#ignore-some-errors) with a reason |
| You want only the checks of actionlint | `--profile correctness` (see [profiles](./config.md#profiles)) |
| Many findings that you will fix over time, and no new ones | A baseline |

When a text run of the default profile reports many findings, jactionlint prints a hint that names `--baseline-write`: see [the first run](./usage.md#the-first-run).

## Write and apply the baseline

Four steps take a repository from a failing run to a build that fails only on new findings. The examples use the default profile and a repository with two workflow files. Save them as `.github/workflows/ci.yaml` and `.github/workflows/release.yaml` in a Git repository to follow along.

```yaml
name: CI
on:
  push:
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo "${{ github.event.pull_request.title }}"
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make lint
```

```yaml
name: Release
on:
  push:
    tags: ["v*"]
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make publish
```

1. **See what you face.** `--format summary` prints the number of findings per rule and per file instead of the findings. The run exits with status 1, like a plain run.

   ```sh
   jactionlint --format summary
   ```

   ```
   16 findings in 2 of 2 files

   by rule              findings
   artipacked                  3
   missing-permissions         3
   missing-timeout             3
   unpinned-uses               3
   concurrency-limits          2
   duplicate-triggers          1
   template-injection          1

   by file                         findings
   .github/workflows/ci.yaml             11
   .github/workflows/release.yaml         5
   ```

2. **Write the baseline.** The command lints the whole repository like a run without arguments, writes `.github/jactionlint-baseline.json` and exits with status 0, although it recorded findings.

   ```sh
   jactionlint --baseline-write
   ```

   ```
   Wrote 16 entries for 2 files in .github/jactionlint-baseline.json
   A plain run does not use the baseline yet. Pass --baseline, or put this line in .github/jactionlint.yaml so that every run and hook does:

     baseline: auto
   ```

   `--baseline-write=FILE` writes another path. Write the `=`: an argument after `--baseline-write` is a workflow file to check, not the path of the baseline. When the file already has the content, the first line reads `Baseline is up to date: ...` instead of `Wrote ...`.

3. **Apply it.** A baseline is used only when you ask for it. Put the line that step 2 printed in the config file so that every run and hook uses the baseline, or pass `--baseline` to a single run.

   ```yaml
   # .github/jactionlint.yaml
   baseline: auto
   ```

   With the baseline applied, a plain run hides the recorded findings, so it exits with status 0 while no new finding exists. The text formats print a note on stderr with the number of hidden findings.

   ```sh
   jactionlint
   ```

   ```
   note: 16 finding(s) are hidden by the baseline
   ```

4. **Commit the file and the config file, and run the same command in CI.** The build fails only on findings that are not in the baseline. To see this, add a job without `timeout-minutes` to `release.yaml`:

   ```yaml
     notify:
       runs-on: ubuntu-latest
       permissions: {}
       steps:
         - run: make notify
   ```

   The run reports the new job, notes the hidden findings and exits with status 1:

   ```
   .github/workflows/release.yaml:11:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set "default-minutes" in the "missing-timeout" rule options of the configuration to turn on the --fix fix [missing-timeout]
      |
   11 |   notify:
      |   ^~~~~~~
   note: 16 finding(s) are hidden by the baseline
   ```

The switches that apply or skip a baseline:

| Where | Setting | Effect |
| --- | --- | --- |
| Config file | `baseline: auto` (or `true`) | Applies `.github/jactionlint-baseline.json` when the file exists. |
| Config file | `baseline: path/to/file.json` | Applies that file, with a path relative to the repository root. The file must exist. |
| Config file | `baseline: false`, or no key | Applies no baseline. |
| Command line | `--baseline` | Applies the default file, which must exist. It overrides the config file. |
| Command line | `--baseline=FILE` | Applies `FILE`, which must exist. |
| Command line | `--no-baseline` | Ignores a baseline that the config file enables. |

[Baseline and fixes](./config.md#baseline-and-fixes) lists the `baseline` key with the other keys of the config file.

## Shrink the baseline

The baseline is debt: fix findings, then remove the entries of the fixed ones. The examples continue with the sample of the previous section, after you remove the `notify` job again.

1. **Fix findings.** `--fix` also fixes the findings that the baseline hides (see [Fix findings automatically](./fix.md#fix-errors-automatically)). This command fixes the `artipacked` finding of `release.yaml`:

   ```sh
   jactionlint --fix --fix-rules artipacked .github/workflows/release.yaml
   ```

   ```
   Fixed 1 problem(s) in 1 file(s)
     artipacked: 1
   note: 4 finding(s) are hidden by the baseline; 1 baseline entr(ies) match nothing any more (--baseline-check lists them)
   ```

   The entry of the fixed finding matches nothing any more, and the note of a run counts such entries.

2. **List the entries that are unused.** `--baseline-check` reports each of them as an [`unused-baseline-entry`](./rules.md#unused-baseline-entry) finding, located in the baseline file at the line of the entry. `--baseline-check` implies `--baseline`.

   ```sh
   jactionlint --baseline-check
   ```

   ```
   .github/jactionlint-baseline.json:124:1: info: baseline entry for rule "artipacked" in ".github/workflows/release.yaml" matches no finding any more: it was fixed, changed or moved. run jactionlint --baseline-write to remove it [unused-baseline-entry]
       |
   124 |     {
       | ^
   note: 15 finding(s) are hidden by the baseline
   ```

   The level of the rule is `info`, so the run exits with status 0. To fail the build until the file is shrunk, set the level to `error` in the config file (the `info: ` prefix is then gone and the exit status is 1), or pass `--strict-exit`, which makes every `warn` and `info` finding fail.

   ```yaml
   rules:
     unused-baseline-entry: error
   ```

3. **Remove the entries.** `--baseline-write` writes the file again without them.

   ```sh
   jactionlint --baseline-write
   ```

   ```
   Wrote 15 entries for 2 files in .github/jactionlint-baseline.json
   ```

`--baseline-write` lints with the baseline switched off, records every finding it sees and exits with status 0. A new finding that is present when you write is therefore accepted without a message. Run `jactionlint --baseline` first, fix what it reports, and write the baseline only when that run reports nothing.

To make CI enforce a baseline that matches the code, run `jactionlint --baseline-check` with `unused-baseline-entry: error`. It fails on new findings and on stale entries, so a pull request that fixes a finding has to update the file, and the update shows in review. Review an added entry as an accepted finding.

## The baseline file

`jactionlint --baseline-write` writes `.github/jactionlint-baseline.json`. Commit the file, and write it again instead of editing it by hand. It is JSON:

```json
{
  "version": 1,
  "entries": [
    {
      "file": ".github/workflows/ci.yaml",
      "rule": "missing-timeout",
      "fingerprint": "7f915da24540610a",
      "context": "2fcd66cb1402c448",
      "occurrence": 0,
      "message": "\"timeout-minutes\" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set \"default-minutes\" in the \"missing-timeout\" rule options of the configuration to turn on the --fix fix"
    }
  ]
}
```

| Key | Meaning |
| --- | --- |
| `version` | The format version, `1`. A file with another version is refused with a message that tells you to write it again with `--baseline-write`. |
| `file` | The path of the workflow file relative to the repository root, with forward slashes. |
| `rule` | The ID of the rule that reported the finding. |
| `fingerprint` | A hash of the rule, the job (or top-level key) that holds the finding, the source line of the finding and its message. Whitespace and the line or column numbers that a message mentions do not count. |
| `context` | The same hash without the message. |
| `occurrence` | The number of the finding among identical ones in the file, from 0. |
| `message` | The message when the baseline was written. It is for people who read the file: matching ignores it. |

The entries are sorted by file, and those of one file follow the order of the findings in it. They hold no line numbers, and writing the baseline again without a change in the findings gives the same bytes, so the file changes in a pull request only when findings change.

## How a finding matches

jactionlint compares the findings of a file with the entries of the same file in two steps. An entry first matches a finding of the same rule with the same fingerprint and the same occurrence. Each finding that is left then matches a left-over entry of the same rule and the same context. An entry accepts one finding.

For some rules, such as `missing-action-input` and `known-vulnerable-actions`, the message says which input or advisory the finding is about, while the context holds only the line that calls the action. These rules match by fingerprint only: a new missing input or a new advisory for the same action is a new finding, not a reworded one.

### What keeps a finding baselined and what makes it new

| Change | Effect |
| --- | --- |
| Add, remove or move lines elsewhere, move a job, reindent | None: an entry holds no line number, and line or column numbers that a message mentions do not count. |
| Check out the repository with CRLF instead of LF line breaks | None, and `--baseline-write` writes the same bytes. |
| Edit the source line of a finding, such as its quoting | The finding is new again and its old entry is unused, so the changed code is reviewed. |
| Rename the job or top-level key that holds a finding | Its findings are new again. |
| A release rewords the message of a rule | None, except for the rules named above: the context still matches. Writing the baseline again refreshes the stored messages. |
| Add a second identical finding (same rule, same line text, same message in one job) | The extra finding is new: an entry accepts as many findings as it recorded. Fixing one of them leaves one entry unused. |
| Rename or move a workflow file | All its findings are new again and its entries are unused: see below. |

### Renamed, moved and partly written files

Entries are keyed by the path relative to the repository root, so a renamed or moved file loses them: its findings are reported as new and the old entries are unused. Write the entries of the new path only:

```sh
jactionlint --baseline-write .github/workflows/build.yaml
```

With file arguments, `--baseline-write` lints only those files, replaces their entries and keeps the entries of the other files that still exist. The entries of the old path go, because that file does not exist any more. When the baseline file does not exist yet, it holds only the files that you named. The command still records every finding of the named files, new ones included, so run `jactionlint --baseline` first.

### Checkouts at another path

A baseline is portable: the same file works in a checkout at another path, such as a CI runner or the machine of a colleague. Messages about a local action show its path relative to the repository (`./.github/actions/setup`), and the path of the checkout is replaced in the fingerprint of any message that still contains it. A finding about a local action, such as a broken `action.yml` or a file that it names and that is missing, is reported once, at its first use in the order of the files, however many workflows use the action.

### What the baseline records

`--baseline-write` records the findings as a run reports them, after everything that hides a finding: `--ignore`, an entry of `ignores:` or `paths:` in the config file, an ignore comment (see [Ignore findings](./ignore.md#ignore-some-errors)), a rule that is `off` or that the profile does not enable, and `--min-severity`. Change these first and write the baseline last. An entry whose finding a later ignore hides is unused, like a fixed one. The baseline never records `unused-baseline-entry`.

An entry is unused when its file was linted, or no longer exists, and no finding matched it. The entries of rules that the run cannot reproduce are not unused, because nothing says that they were fixed:

- a rule that is `off` or not in the profile of the run,
- an online rule when the online checks are off or a lookup was skipped,
- `shellcheck` or `pyflakes` findings when the tool is not installed.

The entries of files that the run did not lint, such as a run on one file, are not unused either.

### What each output shows

| Format | Baselined findings |
| --- | --- |
| `text`, `oneline` | Hidden. A note on stderr gives the number of hidden findings and of unused entries (`--baseline-check` lists them). |
| `summary` | Counted: the tables have `new`, `baselined` and `total` columns, and a line gives the number of entries that match nothing. |
| `json`, `jsonl`, `gcc`, `github` | Hidden, without a note. |
| `sarif` | Results with a `suppressions` entry of kind `external` and the justification `accepted by the jactionlint baseline`. |

`--sarif-hide-baselined` leaves the baselined findings out of the SARIF log, for tools that do not read suppressions, such as [hk](./hooks.md#hk). [Format error messages](./output.md#format-error-messages) describes the formats and their fields.

## Exit status and failure modes

Only findings that are not in the baseline decide the exit status. The levels of the rules, `--strict-exit` and `--min-severity` apply to them as to any run, and [the exit status table](./cli.md#exit-status) lists every cause.

| Situation | Result |
| --- | --- |
| `--baseline-write` | Exit 0, whatever it records. |
| `--baseline=FILE`, or `baseline: path` in the config file, and the file does not exist | Exit 3. The message ends with `does not exist. create it with jactionlint --baseline-write`. |
| `--baseline` or `--baseline-check` and the default file does not exist | Exit 3, with the same message. |
| `baseline: auto` and the default file does not exist | The run goes on without a baseline. |
| The file is not JSON or has another version | Exit 3. For another version the message says to regenerate the file with `--baseline-write`. |
| An option combination that cannot work, such as `--baseline` with `--no-baseline`, `--baseline-write` with `--fix`, `--diff` or the operand `-`, or `--no-ignores` with a baseline option | Exit 2: see [option combinations that fail](./cli.md#option-combinations-that-fail). Alone, `--no-ignores` also turns the baseline off. |

## Next steps

- [Fix findings automatically](./fix.md#fix-errors-automatically): `--fix` also repairs the findings that the baseline hides.
- [Ignore findings](./ignore.md#ignore-some-errors): hide a finding for good, with a reason.
- [Run jactionlint in CI](./ci.md#the-action): use the same command on pull requests.
- [Run jactionlint as a hook](./hooks.md#hk): lint the changed files before a commit.
- [The config file](./config.md#baseline-and-fixes): the `baseline` key and the levels of rules.
