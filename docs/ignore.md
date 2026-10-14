<a id="ignore-some-errors"></a>
# Ignore findings

Silence a finding that you accept, with an ignore comment, `--ignore`, the config file or a zizmor comment, and find the ignores that no longer do anything.

## Choose a way

Pick the mechanism by what you want to silence. Prefer the narrowest scope that fits: the more it covers, the more it can hide.

| You want to silence | Use | It covers | Where it lives |
| --- | --- | --- | --- |
| A rule everywhere | `rules: {unpinned-uses: off}` | Every file | [Config file](#turn-a-rule-off) |
| A rule, or a message, in some files | `paths.<glob>.ignore` | The files that match the glob | [Config file](#ignore-patterns-in-some-files) |
| One finding or one block of a file | An [ignore comment](#ignore-comments) | A line and what is nested under it | The workflow file |
| A finding that you accept for now, with a reason and an end date | A [durable ignore](#durable-ignores): an entry of `ignores:` | A rule in a file, job, step or `uses:`, until an optional date | [Config file](#durable-ignores) |
| Every finding that exists today | A [baseline](./baseline.md#baseline) | The findings recorded in it | `.github/jactionlint-baseline.json` |
| One run | [`--ignore`](#ignore-on-the-command-line) | Every file of the run | The command line |
| What a repository already accepted with zizmor | A [zizmor comment](#zizmor-ignore-comments) | A step, a job or the triggers | The workflow file |

To look at one rule and hide all others, use `--select` ([Run one rule](./usage.md#run-one-rule-or-run-without-the-ignores)). To see what the ignores hide, use `--no-ignores` ([Keep your ignores honest](#keep-your-ignores-honest)).

## Write a pattern

`--ignore`, ignore comments and `paths.<glob>.ignore` take the same kind of pattern:

- A pattern that is exactly a [rule ID](./rules.md), such as `unpinned-uses`, ignores every finding of that rule.
- Any other pattern is a regular expression in [RE2 syntax](https://github.com/google/re2/wiki/Syntax). It is matched against the message of the finding, anywhere in it. It does not see the rule ID, the file or the line.

Every finding shows its rule ID at the end of the text line, in the `id` field of `--format json` and in the `ruleId` of `--format sarif`. This workflow file is the sample of this page. Save it as `.github/workflows/ci.yaml` in a Git repository. It has three findings in the default profile:

```yaml
name: ci
on: pull_request
permissions: {}
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
      - uses: actions/cache@v4
        with:
          path: ~/.cache
          key: cache
      - uses: actions/setup-node@v4
```

```sh
jactionlint --format oneline
```

```
.github/workflows/ci.yaml:12:15: actions/checkout leaves the GITHUB_TOKEN in the git config of the workspace, where a later step such as an artifact upload can publish it. set "persist-credentials: false" under "with:" unless a later step needs to push [artipacked]
.github/workflows/ci.yaml:13:15: action "actions/cache@v4" must be pinned to a full-length commit SHA like "{owner}/{repo}@{sha}" because the "unpinned-uses" rule is enabled [unpinned-uses]
.github/workflows/ci.yaml:17:15: action "actions/setup-node@v4" must be pinned to a full-length commit SHA like "{owner}/{repo}@{sha}" because the "unpinned-uses" rule is enabled [unpinned-uses]
```

The pattern `artipacked` ignores the first finding, and the pattern `actions/setup-node@v4` ignores the last one, because the message contains that text. Shellcheck reports all its findings under the rule ID `shellcheck`, so to ignore one shellcheck code, match the code in the message. The pattern `SC2086` ignores a finding with a message such as `shellcheck reported issue in this script: SC2086:info:2:6: Double quote to prevent globbing and word splitting`.

A list of patterns is written differently in each place:

| Place | Several patterns |
| --- | --- |
| `--ignore` | Repeat the option. A comma belongs to the pattern, so `--ignore a,b` is one pattern |
| Ignore comment | Separate them with commas. A comma inside `()`, `[]` or `{}`, or after a backslash, does not separate, so `x{1,2}` is one pattern |
| `paths.<glob>.ignore` | A YAML list |

Older spellings still work and print a warning once per run:

- A retired rule ID, listed under [Retired rule IDs](./rules.md#retired-rule-ids), ignores only the findings that the old rule reported. The ID of the rule that replaced it would ignore all findings of that rule.
- A legacy kind, such as `timeout-check`, is not a rule ID. It is read as a regular expression on the message, so it matches only a message that contains that text. When a finding of the run has that kind, the warning names the rule IDs to use instead.

## Ignore comments

An ignore comment sits next to the code it is about. Its form is `# jactionlint ignore=PATTERN`, with more patterns after commas. The form `# actionlint ignore=PATTERN` is read in the same way, so comments that you wrote for actionlint keep working. Ignore comments work in workflow files, composite actions and `.github/dependabot.yml`.

### Forms and scope

A comment has one of two forms:

- **On its own line**, it covers the next line that is not a comment and not blank, and the lines nested under that line. Comments and blank lines inside the range do not end it. You can stack several comments above the same line.
- **At the end of a line**, it covers that line and the lines nested under it. It can follow other comment text, so it can sit after the version comment of a pinned action.

What a comment covers depends on the line that it targets:

| The covered line | The comment covers |
| --- | --- |
| Starts a sequence item (`- uses: ...`, `- run: ...`) | The whole item: the step with its `with:` and `env:`, but not the next step |
| Is a key (`build:`, `jobs:`) | The key and everything nested under it: `build:` covers a whole job and `jobs:` covers every job |
| Is any other line | That line and the lines nested under it |

A comment applies to a finding when the line where the finding starts is inside the range. There is no comment for a whole file: to ignore a rule in all of a file, use [`paths.<glob>.ignore`](#ignore-patterns-in-some-files), or put the comment on a top-level key such as `jobs:` to cover everything under it.

This version of the sample has two comments. The comment at the end of the `checkout` line follows the version comment and covers that step. The comment above `actions/cache` covers the step with its `with:` block. Nothing covers `actions/setup-node`, so its finding remains.

```yaml
name: ci
on: pull_request
permissions: {}
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0 # jactionlint ignore=artipacked
      # jactionlint ignore=unpinned-uses
      - uses: actions/cache@v4
        with:
          path: ~/.cache
          key: cache
      - uses: actions/setup-node@v4
```

```sh
jactionlint
```

```
.github/workflows/ci.yaml:18:15: action "actions/setup-node@v4" must be pinned to a full-length commit SHA like "{owner}/{repo}@{sha}" because the "unpinned-uses" rule is enabled [unpinned-uses]
   |
18 |       - uses: actions/setup-node@v4
   |               ^~~~~~~~~~~~~~~~~~~~~
```

A tool that rewrites a `uses:` line, such as Renovate or Dependabot updating a pin, replaces the comment at the end of that line. Put the ignore comment on its own line above the step instead, or write a [durable ignore](#durable-ignores) when you want a reason and an expiry date.

### A comment inside a script

The comment at the end of the `run: |` line is an ordinary YAML comment and covers the whole script:

```yaml
      - run: | # jactionlint ignore=template-injection
          echo '${{ github.event.pull_request.title }}'
```

Inside the script, a `#` that follows other text on a line, or sits in a quoted YAML string, is not a comment and is not read. A line of the script that starts with `# jactionlint ignore=` is read as an own-line comment. It covers the next line of the script and the lines nested under it:

```yaml
      - run: |
          echo start
          # jactionlint ignore=template-injection
          echo '${{ github.event.pull_request.title }}'
```

### A comment that is broken

A pattern in a comment that is not a valid regular expression is a finding of its own, `invalid-ignore-comment` ([correctness profile](./config.md#profiles), level `error`), and it ignores nothing. The finding points at the comment. This is the finding for the comment `# jactionlint ignore=(` on line 11 of a workflow file:

```
.github/workflows/ci.yaml:11:9: invalid regular expression "(" in inline ignore comment: error parsing regexp: missing closing ): `(` [invalid-ignore-comment]
   |
11 |       # jactionlint ignore=(
   |         ^~~~~~~~~~~
```

## Ignore on the command line

`--ignore PATTERN` (or `-i PATTERN`) ignores the findings that match the pattern in every file of the run. Repeat the option for more patterns. It adds to the comments and to the config file. On the sample of [Forms and scope](#forms-and-scope), where one finding is left, it removes that finding:

```sh
jactionlint --ignore 'actions/setup-node@v4'
```

The command prints nothing and exits with status 0. A pattern that is not a valid regular expression exits with status 2. Use `--ignore` for a single run or a script. An exception that should stay belongs in a comment or in the config file, where a reviewer sees it.

## Ignore in the config file

The [config file](./config.md#configuration-file) can turn a rule off, ignore patterns in some files and hold durable ignores.

### Turn a rule off

`rules` sets the level of a rule, and `off` stops the rule everywhere. [Rules](./config.md#rules) lists the values and the options of the rules.

```yaml
rules:
  unpinned-uses: off
```

### Ignore patterns in some files

`paths` takes a glob that is relative to the root of the repository, with `/` as the separator, and an `ignore` list of [patterns](#write-a-pattern) for the files that match it:

```yaml
paths:
  .github/workflows/release.yaml:
    ignore:
      - unpinned-uses
      - SC2086
```

### Durable ignores

An entry of `ignores:` accepts the findings of a rule by what the workflow says, not by a line number: the file, the job, the step or the `uses:` value. It survives a tool that rewrites the line, it carries a `reason` for the reader of the file, and an optional `expires` date ends it. [Durable ignores](./config.md#durable-ignores) describes every key and how a finding is matched.

```yaml
ignores:
  - rule: unpinned-uses
    uses: actions/setup-node
    reason: pinned by an organization ruleset
    expires: "2030-12-31"
```

With this config file, the sample of [Forms and scope](#forms-and-scope) has no finding left. Replace the date with `"2020-01-01"`, and the finding comes back together with a finding about the entry:

```sh
jactionlint --format oneline
```

```
.github/workflows/ci.yaml:18:15: action "actions/setup-node@v4" must be pinned to a full-length commit SHA like "{owner}/{repo}@{sha}" because the "unpinned-uses" rule is enabled [unpinned-uses]
.github/jactionlint.yaml:2:5: the ignore for "unpinned-uses" (uses "actions/setup-node") expired on 2020-01-01 and no longer suppresses anything. fix the findings, or renew it with a new date in "expires" (reason: pinned by an organization ruleset) [expired-ignore]
```

## Keep your ignores honest

An ignore that nobody checks turns into a blind spot. These rules and options keep the exceptions visible.

- **`unused-ignore`** ([pedantic profile](./config.md#profiles), level `error`) reports a pattern of an ignore comment that suppressed nothing, a comment with no line after it to apply to, and an entry of `ignores:` that matched nothing.
  - It skips a pattern for a rule that did not run: a rule that is off, an online rule without `--online`, or shellcheck without its command.
  - Turn it on without the rest of the pedantic profile with `rules: {unused-ignore: error}`.
  - When you combine it with `--select`, select the rules of the patterns as well, as in `--select unused-ignore,unpinned-uses`: a pattern for a rule that does not run is never reported.
- **`expired-ignore`** ([correctness profile](./config.md#profiles), level `error`) reports an entry of `ignores:` after its `expires` date and warns shortly before. [Expiry and unused entries](./config.md#expiry-and-unused-entries) has the details.
- **`invalid-ignore-comment`** reports a comment whose pattern is not a valid regular expression, as shown above.
- **`--fix`** removes the unused patterns of an ignore comment, and the whole comment when no pattern is left. The fix is safe.
- **`--no-ignores`** shows what the ignores hide.

This file pins the `checkout` step, so the pattern `unpinned-uses` is unused, and the second comment never suppresses a finding:

```yaml
name: ci
on: pull_request
permissions: {}
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      # jactionlint ignore=unpinned-uses,artipacked
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
      - run: echo hi # jactionlint ignore=template-injection
```

Turn the rule on in the config file:

```yaml
# .github/jactionlint.yaml
rules:
  unused-ignore: error
```

A plain run reports both:

```sh
jactionlint
```

```
.github/workflows/ci.yaml:12:28: ignore pattern "unpinned-uses" did not suppress any error. remove it [unused-ignore]
   |
12 |       # jactionlint ignore=unpinned-uses,artipacked
   |                            ^~~~~~~~~~~~~~~~~~~~~~~~
.github/workflows/ci.yaml:14:43: ignore pattern "template-injection" did not suppress any error. remove it [unused-ignore]
   |
14 |       - run: echo hi # jactionlint ignore=template-injection
   |                                           ^~~~~~~~~~~~~~~~~~
```

`--diff` prints what `--fix` would change as a diff on stdout and writes nothing ([Preview and apply fixes](./fix.md#preview-and-apply-fixes)). The count of the fixes goes to stderr and is not shown here:

```sh
jactionlint --diff --fix-rules unused-ignore
```

```
--- a/.github/workflows/ci.yaml
+++ b/.github/workflows/ci.yaml
@@ -9,6 +9,6 @@
     runs-on: ubuntu-latest
     timeout-minutes: 10
     steps:
-      # jactionlint ignore=unpinned-uses,artipacked
+      # jactionlint ignore=artipacked
       - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
-      - run: echo hi # jactionlint ignore=template-injection
+      - run: echo hi
```

`--no-ignores` applies none of the ways to accept a finding that the repository carries: the ignore comments (`# jactionlint ignore=` and `# zizmor: ignore[...]`), the `ignores` and the `paths.<glob>.ignore` of the config file, and the [baseline](./baseline.md#baseline). The `--ignore` options of the same command stay, because you wrote them for this run. Nothing is applied, so `unused-ignore`, `expired-ignore` and `invalid-ignore-comment` report nothing. `--no-ignores` exits with status 2 when you combine it with `--baseline`, `--baseline-check` or `--baseline-write`.

```sh
jactionlint --no-ignores --format summary
```

On the sample of [Forms and scope](#forms-and-scope) with its two comments, the summary shows all three findings: the two that the comments hide and the one that remains.

```
3 findings in 1 of 1 files

by rule        findings
unpinned-uses         2
artipacked            1

by file                    findings
.github/workflows/ci.yaml         3
```

## zizmor ignore comments

A repository that used [zizmor](https://docs.zizmor.sh/usage/#ignoring-results) keeps the findings that it already triaged. jactionlint reads the ignore comments of zizmor, so you can keep them while you switch, and move them to jactionlint comments later.

### Syntax and names

The comment is `# zizmor: ignore[name,name]`:

- It must be a YAML comment spelled exactly like this, with one space after `#` and one after `:`. A reason may follow the closing bracket after white space. The comment may follow another comment, as in `# v4.1.0 # zizmor: ignore[unpinned-uses]`.
- Names are separated by commas. A name is a rule ID of jactionlint, or one of the audit names of [the alias table](#audit-names-that-differ). A name that has no rule is skipped and does not fail the run.
- `# zizmor: ignore` without a list in brackets does nothing.
- Text inside a block scalar or a quoted string is never a comment.

### Scope of a zizmor comment

A zizmor finding has more than the line it points at: it also belongs to the step, the job or the trigger it is about. jactionlint follows that. A comment applies to a finding in these cases:

| The comment is on | It applies to |
| --- | --- |
| The line of the finding, or any line of a finding that spans several lines | That finding |
| The line that opens a value, such as `permissions:` or a `run:` block scalar, with the comment after the opening | Every finding inside that value |
| Any line of a step: its `name:`, a line between two keys, `env:` | The findings of `template-injection`, `artipacked`, `unpinned-uses`, `ref-version-mismatch`, `superfluous-actions` and `adhoc-packages` in that step |
| Any line of a job | The `secrets-outside-env` finding of the job. For a job that calls a reusable workflow, also its `unpinned-uses` and `ref-version-mismatch` findings |
| The `uses:` or the `secrets:` of a job that calls a reusable workflow | The `secrets-inherit` finding of the job |
| Any line of `on:` | The `dangerous-triggers` finding |

A comment on a line of its own above the first line of a step belongs to the step above, not to the step below it.

### Audit names that differ

A name stands for the rule of the same ID. These audit names of zizmor are reported under other IDs:

| zizmor audit | Applies to | Note |
| --- | --- | --- |
| `excessive-permissions` | The rules `excessive-permissions` and `missing-permissions` | zizmor reports a missing `permissions:` block as `excessive-permissions`, and jactionlint reports it as `missing-permissions` |
| `unpinned-images` | The rule `unpinned-images` and the Docker image findings of `unpinned-uses` | A comment on a `uses: docker://...` line applies to the Docker finding. It does not apply to the other findings of `unpinned-uses` |

### Migrate the comments

`jactionlint --migrate-ignores [files]` rewrites the zizmor comments at the end of a line into `# jactionlint ignore=` comments on the line above, and moves the reason into a plain comment line. Without files, it rewrites the workflow files and the `action.yml` or `action.yaml` at the root of the repository. This file has three zizmor comments:

```yaml
on: pull_request
permissions: {}
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4 # zizmor: ignore[artipacked,unpinned-uses] pinned by the mirror
      - run: echo '${{ github.event.pull_request.title }}' # zizmor: ignore[template-injection]
      - name: Node # zizmor: ignore[unpinned-uses]
        uses: actions/setup-node@v4
```

```sh
jactionlint --migrate-ignores .github/workflows/ci.yaml
```

```
.github/workflows/ci.yaml: migrated zizmor ignore comments: artipacked, unpinned-uses, template-injection
.github/workflows/ci.yaml:13: not migrated: the comment is on the first key of a step with more keys, and a comment above would cover the whole step. Move it to a later key of the step (for instance "name:") or keep it
```

The first two comments move above their steps. The third stays: a comment above `- name:` would cover the whole step, and the command moves a comment only when the new comment covers the same lines as the old one. Move such a comment to a later key of the step, or keep it. After the command, the file holds:

```yaml
    steps:
      # pinned by the mirror
      # jactionlint ignore=artipacked,unpinned-uses
      - uses: actions/checkout@v4
      # jactionlint ignore=template-injection
      - run: echo '${{ github.event.pull_request.title }}'
      - name: Node # zizmor: ignore[unpinned-uses]
        uses: actions/setup-node@v4
```

More details:

- A version after the comment on a `uses:` line, as in `# zizmor: ignore[artipacked] v4.2.2`, stays on that line as `# v4.2.2`. It is the version comment of Renovate and Dependabot, not a reason.
- A name stays in the zizmor comment when jactionlint has no rule of that name, and when moving it would widen the scope (`unpinned-images` covers only some findings of `unpinned-uses`).
- A comment on a line of its own is not migrated.
- Running the command again changes nothing. It prints `No zizmor ignore comment could be migrated. Nothing was changed` and lists the comments that stay.

### Find leftover comments

`unused-ignore` does not report zizmor comments, because zizmor may still run on the repository and decides what its comments suppress. When zizmor is gone, turn on the `zizmor` option of the rule to find the comments that are left over. The rule then reports a comment only when its audit maps to a rule that is enabled:

```yaml
rules:
  unused-ignore:
    zizmor: true
```

## Next steps

- [Adopt new checks with a baseline](./baseline.md#baseline): accept every finding that exists today and fail only on new ones.
- [Durable ignores](./config.md#durable-ignores): every key of an `ignores:` entry and how a finding is matched.
- [Run one rule](./usage.md#run-one-rule-or-run-without-the-ignores): `--select` reports only the rules that you name.
- [Fix findings automatically](./fix.md#fix-errors-automatically): fix a finding instead of ignoring it.
- [How jactionlint relates to zizmor](./zizmor-parity.md): where the two tools overlap.
