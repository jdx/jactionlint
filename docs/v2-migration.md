# Migrating to v2

jactionlint v2 keeps reading the same workflows, but it organizes its checks in profiles, gives every finding a stable rule ID and
severity, and checks much more by default. This page is for someone who ran jactionlint v1 (or [actionlint](https://github.com/rhysd/actionlint))
and wants to know what changes, what to do about it, and what is new. Everything here is in v2.

If you only want the old result, put this in `.github/jactionlint.yaml` and read no further:

```yaml
profile: correctness
```

The `correctness` profile is what actionlint checks plus jactionlint's own bug detectors. See [Coming from actionlint](actionlint.md).
If you want the new checks without a wall of findings on day one, record today's findings in a [baseline](#baseline).

## What changes at a glance

| Area | What it means for you |
| --- | --- |
| [Module path](#module-path) | Go programs import `github.com/jdx/jactionlint/v2`. |
| [Command line](#command-line-options) | POSIX/GNU options only. The single-dash long options of v1 are removed. |
| [Default profile](#behaviors-that-surprise) | The `default` profile is stricter than v1: 42 security and policy rules are on in addition to the 66 of `correctness`. A repository that was clean may fail. |
| [Rule IDs](#rule-ids-and-aliases) | Every finding has a stable ID such as `unpinned-uses`, usable in config, `--ignore` and ignore comments. |
| [Config](#config) | `profile:`, `rules:` and `extends:` replace the booleans. Unknown keys are errors. |
| [Exit codes](#exit-codes-and-severity) | Only `error` findings make the exit status 1. |
| [Output formats](#output-formats) | `sarif`, `gcc`, `github`, `jsonl`, `summary` and `--rule-ids` are new. The text format is unchanged. |
| [`--fix`](#fix) | Many rules have a fix. `--fix` converges, checks each pass and has `--diff` and `--fix-rules`. |
| [Baseline](#baseline) | `--baseline-write` and `--baseline` adopt the checks step by step. |
| [Durable ignores](#durable-ignores-and-ignore-comments) | `ignores:` in the config accepts findings by rule, file, job, step and `uses:`, with an expiry. |
| [zizmor](#zizmor-ignore-comments) | `# zizmor: ignore[...]` comments are honored. `--migrate-ignores` converts them. |
| [Online mode](#online-mode) | `--online` adds six checks that query GitHub and pins tags to commits with `--fix`. Off by default. |
| [Composite actions and Dependabot](#composite-actions-and-dependabot) | `action.yml` files and `dependabot.yml` are linted together with the workflows. |
| [hk](#hk) | A SARIF step gives hk diagnostics with rule IDs and fixes. |

## Module path

The Go module is `github.com/jdx/jactionlint/v2`. The package name is still `jactionlint`.

```sh
go install github.com/jdx/jactionlint/v2/cmd/jactionlint@latest
```

Programs that import the library change their import. See [Go API](#go-api). Installing with [mise](https://mise.jdx.dev)
(`mise use jactionlint`), Homebrew, the download script or Docker is unchanged. The Go version required is in `go.mod` (Go 1.26).

## Command line options

<!-- legacy-options:start -->

v2 uses POSIX/GNU command line options only, and **the single-dash long options of v1 and actionlint are gone**: `-format`, `-fix`,
`-no-color`, `-config-file`, `-ignore` and every other `-name` of more than one letter. There are no aliases and no deprecation
period. Rename them in scripts, CI steps, hk steps, Docker commands and `mise.toml` tasks:

| Before | After |
| --- | --- |
| `jactionlint -format sarif` | `jactionlint --format sarif` (or `-f sarif`) |
| `jactionlint -fix`, `-fix=unsafe` | `jactionlint --fix`, `--fix=unsafe` |
| `jactionlint -fix -rules missing-timeout` | `jactionlint --fix --fix-rules missing-timeout` |
| `jactionlint -profile correctness` | `jactionlint --profile correctness` (or `-p correctness`) |
| `jactionlint -no-color -config-file ci.yaml` | `jactionlint --no-color -c ci.yaml` |
| `jactionlint -online=false`, `-baseline=false` | `jactionlint --no-online`, `--no-baseline` |
| `jactionlint -version` | `jactionlint --version` (or `-V`) |

The rules of the syntax:

- A long option is `--long-name` in kebab-case. A value is given as `--format=json` or `--format json`.
- A short option is one letter and can be bundled and take its value attached or separate: `-vfjson`, `-cfile.yaml`, `-c file.yaml`.
  There are seven: `-h`, `-V`, `-v`, `-c`, `-f`, `-p` and `-i`. Everything else is long only.
- `--` ends the options, so `jactionlint -- -odd-name.yaml` checks a file whose name starts with a dash. A lone `-` is not an
  option: it means stdin.
- An option with an **optional** value takes it only as `--option=VALUE`: `--fix=unsafe`, `--online=cache`, `--baseline=FILE`,
  `--baseline-write=FILE`, `--color=never`. `--fix unsafe` is `--fix` and a file named `unsafe`.
- Abbreviations are not accepted (`--form` is an error). A boolean option takes no value, and a default that is on is turned off
  with `--no-X`.
- An unknown option, or a value that is not valid, exits with status 2 and one line on stderr that ends with `(try --help)`. An
  argument that is exactly an option of v1 names its replacement, before it can be read as a bundle (`-fix` is not `-f -i -x`):

```console
$ jactionlint -profile=default
jactionlint: unknown option -profile; did you mean --profile? (try --help)
```

The library is not affected: `Command.Main(args)` and `LinterOptions` keep their signatures; only the parsing and the help text changed.
`--help` now prints to stdout and exits with status 0.

All options, with what each replaces. Environment variables and config keys are unchanged.

| Option | Short | Value | Was | What it does |
| --- | --- | --- | --- | --- |
| `--help` | `-h` |  | `-h`, `-help` | Show this help and exit |
| `--version` | `-V` |  | `-version` | Show the version and how this binary was installed, then exit |
| `--verbose` | `-v` |  | `-verbose` | Print verbose logs to stderr |
| `--debug` |  |  | `-debug` | Print debug logs to stderr (for development) |
| `--stdin-filename` |  | `NAME` | `-stdin-filename` | File name used in the output when reading stdin (default `<stdin>`) |
| `--format` | `-f` | `FORMAT` | `-format` | Output format: text (default), oneline, json, jsonl, sarif, gcc, github, summary, or a Go template containing `{{ }}` |
| `--oneline` |  |  | `-oneline` | One line per finding; same as --format oneline |
| `--rule-ids` |  |  | `-rule-ids` | Show the rule ID instead of the kind at the end of each finding in text output |
| `--color` |  | `WHEN` (only as `--color=WHEN`) | `-color` | Colorize the output: always, never or auto (default). Bare --color means always |
| `--no-color` |  |  | `-no-color` | Same as --color=never |
| `--no-hints` |  |  | `-no-hints` | Do not print the hint line after a text run with many findings |
| `--min-severity` |  | `LEVEL` | `-min-severity` | Hide findings less severe than LEVEL: info (default), warn or error |
| `--strict-exit` |  |  | `-strict-exit` | Exit with status 1 also for findings of level warn and info |
| `--profile` | `-p` | `NAME` | `-profile` | Rule profile: correctness, default or pedantic. Overrides "profile" of the config file |
| `--config-file` | `-c` | `FILE` | `-config-file` | Use this config file instead of .github/jactionlint.yaml |
| `--ignore` | `-i` | `PATTERN` | `-ignore` | Ignore findings whose rule ID or message matches PATTERN (a rule ID or a regular expression). Repeatable |
| `--init-config` |  |  | `-init-config` | Write a default config file .github/jactionlint.yaml and exit |
| `--migrate-config` |  |  | `-migrate-config` | Rewrite the deprecated keys of the config file and exit |
| `--migrate-ignores` |  |  | `-migrate-ignores` | Rewrite "# zizmor: ignore[...]" comments into "# jactionlint ignore=..." comments and exit |
| `--fix` |  | `unsafe` (only as `--fix=unsafe`) | `-fix` | Apply the safe automatic fixes in place. --fix=unsafe also applies the fixes that may change behavior |
| `--diff` |  |  | `-diff` | Print what --fix would change as a unified diff and write nothing (implies --fix) |
| `--fix-rules` |  | `RULES` | `-rules` | With --fix or --diff: apply only the fixes of these comma separated rule IDs |
| `--baseline` |  | `FILE` (only as `--baseline=FILE`) | `-baseline` | Hide the findings recorded in the baseline (default .github/jactionlint-baseline.json). --baseline=FILE reads another file |
| `--no-baseline` |  |  | `-baseline=false` | Ignore the baseline even when the config file enables it |
| `--baseline-write` |  | `FILE` (only as `--baseline-write=FILE`) | `-baseline-write` | Record the current findings as the baseline and exit 0. --baseline-write=FILE writes another file |
| `--baseline-check` |  |  | `-baseline-check` | Report baseline entries which match no finding any more (implies --baseline) |
| `--sarif-hide-baselined` |  |  | `-sarif-hide-baselined` | Leave baselined findings out of --format sarif instead of marking them suppressed |
| `--online` |  | `MODE` (only as `--online=MODE`) | `-online` | Run the checks which query the GitHub API. MODE: cache (cache only, no network), strict (fail when a lookup is skipped) or cache,strict |
| `--no-online` |  |  | `-online=false` | Never use the network, even when the config file enables the online checks |
| `--online-api-url` |  | `URL` | `-online-api-url` | REST API URL of a GitHub Enterprise Server, e.g. https://ghe.example.com/api/v3 |
| `--online-token-env` |  | `NAME` | `-online-token-env` | Name of the environment variable that holds the GitHub token |
| `--online-token-file` |  | `FILE` | `-online-token-file` | File that holds the GitHub token |
| `--online-allow` |  | `PATTERN` | `-online-allow` | Look up only repositories matching owner/repo PATTERN ("*" is a wildcard). Repeatable |
| `--online-deny` |  | `PATTERN` | `-online-deny` | Never look up repositories matching owner/repo PATTERN. Repeatable |
| `--online-cache-ttl` |  | `DURATION` | `-online-cache-ttl` | How long a cached GitHub answer is used without revalidation (default 1h; 0 always revalidates) |
| `--online-max-wait` |  | `DURATION` | `-online-max-wait` | The longest to wait for a GitHub rate limit to reset (default 30s; 0 never waits) |
| `--shellcheck` |  | `CMD` | `-shellcheck` | Command or path of shellcheck (default shellcheck). Empty disables the integration |
| `--pyflakes` |  | `CMD` | `-pyflakes` | Command or path of pyflakes (default pyflakes). Empty disables the integration |

<!-- legacy-options:end -->

## Rule IDs and aliases

Every finding has a stable kebab-case ID. `--rule-ids` shows it at the end of the line instead of the legacy `kind`, and it is the
`id` of `--format json`, the `ruleId` of `--format sarif`, the `title` of `--format github` and the last word of `--format gcc`. The
list of the 131 rules, with group, level and profile, is [generated](rules.md). IDs are never renamed or reused.

`kind` is still on every finding, so a `--format` template, a problem matcher or a script written for v1 keeps working. A pattern of
`--ignore`, `paths.<glob>.ignore` or `# jactionlint ignore=` is a rule ID when it is exactly one and a regular expression on the
message otherwise, so existing regular expressions work as before.

During the development of v2 an audit was split into several IDs. They were merged into one ID per audit before the release.
The old IDs are still accepted where a pattern is expected, with a deprecation warning, and then match only the findings that the old
rule reported:

| Old ID | Now | How |
| --- | --- | --- |
| `template-injection-expansion` | `template-injection` | the option `pedantic` |
| `template-injection-trusted` | `template-injection` | the option `pedantic` |
| `misfeature-custom-shell` | `misfeature` | the option `pedantic` |
| `github-env-untrusted-input` | `github-env` | always reported |

They work in `--ignore`, in `paths.<glob>.ignore` and in ignore comments. They are refused, with a message that names the new place,
in `rules`, `ignores`, `fix.rules` and `--fix-rules`. To turn the pedantic findings of an audit on or off, use the option:

```yaml
rules:
  template-injection:
    pedantic: true   # unset: true under the pedantic profile, false otherwise
```

## Config

### Profiles

`profile:` in the config file, or `--profile NAME` on the command line (which wins), selects the rules that run. Each profile includes
the one before it.

| Profile | Rules | What it is |
| --- | --- | --- |
| `correctness` | 66 | What actionlint checks plus jactionlint's bug detectors (`unsound-ternary`, `workflow-run-names`, `local-action-checkout`, `action-syntax`, `dependabot-syntax`). No security posture or policy rule. |
| `default` | +42 | The security and policy rules worth failing a build on. Used when nothing is configured. |
| `pedantic` | +14 | Noisy and opinionated rules, and the pedantic findings of audits (option `pedantic`). |

Six [online rules](#online-mode) belong to no profile and run with `--online`. Three rules run only when configured:
`forbidden-uses`, `required-actions` and `timeout-too-long`.

`profile: strict` and `profile: all`, the names that preceded these, are read as `pedantic` with a deprecation warning, and
`--migrate-config` rewrites them. `pedantic` is a larger set than `strict` was, so `default` is usually what you meant.

### `rules`

`rules: {<id>: off|info|warn|error}` sets the level of one rule, whatever the profile, and turns on a rule the profile does not
have. A rule with options takes a mapping (`rules: {max-run-lines: {level: warn, max: 80}}`). See [Rules](config.md#rules).

### `extends`

`extends: [../shared/jactionlint.yaml]` inherits other config files. Later files win and the file itself wins over all of them;
`rules` and `paths` are merged by key. See [Extending config files](config.md#extending-config-files).

### Strict parsing

An unknown key, rule ID or rule option in the config file is an error with a suggestion, and the exit status is 3. In v1 unknown
keys were ignored silently, so a typo is now found:

```
unknown key "self-hosted-runnr" in the configuration at line:3,col:1. did you mean "self-hosted-runner"?
```

### Deprecated booleans and `--migrate-config`

The boolean options of v1 still work, are translated into rules, and print one deprecation warning per config file (in the SARIF
notifications with `--format sarif`). `jactionlint --migrate-config` rewrites the file in place, keeping comments and other keys.
Setting a key to `false` (or `max-run-lines: 0`) turns its rule off.

| Deprecated key | Replacement |
| --- | --- |
| `profile: strict` or `profile: all` | `profile: pedantic` |
| `require-commit-hash: true` | `rules: {unpinned-uses: error}` |
| `require-permissions: true` | `rules: {missing-permissions: error}` |
| `require-checkout-before-local-action: true` | `rules: {local-action-checkout: error}` (in `correctness` now) |
| `require-expression-wrapping: true` | `rules: {require-expression-wrapping: error}` |
| `check-falsy-ternary: true` | `rules: {unsound-ternary: error}` (in `correctness` now) |
| `check-workflow-run-names: true` | `rules: {workflow-run-names: error}` (in `correctness` now) |
| `require-shell: true` | `rules: {require-shell: error}` |
| `max-run-lines: N` | `rules: {max-run-lines: {level: error, max: N}}` |
| `timeout-minutes: {required: true}` | `rules: {missing-timeout: error}` |
| `timeout-minutes: {required: false}` | `rules: {missing-timeout: off}` |
| `timeout-minutes: {max: N}` | `rules: {timeout-too-long: {level: error, max: N}}` |

The `timeout-minutes` key maps in two independent parts. `max` becomes `timeout-too-long` and never touches `missing-timeout`, which
stays as the profile sets it (on in `default`). Only a written `required: true` or `required: false` turns `missing-timeout` on or off.
A rule written in `rules` wins over a deprecated key.

### A config file written for actionlint

`.github/actionlint.yaml` (or `.yml`) is still read when there is no `.github/jactionlint.yaml`. It has no `profile`, so the `default`
profile applies, and jactionlint says so once per run (a note on stderr, a notification in SARIF):

```
note: config file ".github/actionlint.yaml" was read as a jactionlint config. it sets no "profile", so the default profile applies, which has more rules than actionlint. add "profile: correctness" to the file or run with --profile correctness for the checks of actionlint. see https://jactionlint.jdx.dev/actionlint
```

## Exit codes and severity

| Status | Meaning |
| --- | --- |
| `0` | Nothing was found, or only `warn` and `info` findings. |
| `1` | At least one `error` finding (that is not in the [baseline](#baseline)). |
| `2` | Invalid command line option. |
| `3` | Fatal error: unreadable or invalid config, `--fix` refused a fix or did not converge, `--online=strict` skipped a lookup. |

Every rule of `correctness` and `default` reports `error` (the one exception, `unused-baseline-entry`, is `info`), so the exit
status is 1 whenever they find anything, as in v1. Pedantic rules keep their own levels: for example `self-hosted-runner`,
`undocumented-permissions`, `unused-needs` and `continue-on-error` are `info`, and `anonymous-definition` and `mutable-runner-label`
are `warn`. `warn` and `info` findings are printed with a `warning:` or `info:` prefix and do not fail the run unless
`--strict-exit` is given. `--min-severity warn|error` hides the lower levels. Set a level per rule with `rules`.

## Output formats

`--format` takes `text` (the default), `oneline`, `json`, `jsonl`, `sarif`, `gcc`, `github` or `summary`, or a Go template (any value
with `{{ }}`). The text output of an error is unchanged, so the [problem matcher](usage.md#problem-matchers) keeps working.

- `json` and `jsonl` are the same as `{{json .}}` templates and add `id`, `severity`, `doc_url`, `end_line`, `end_column` and `fix`.
- `sarif` is SARIF 2.1.0 with rule metadata, levels, regions and the fixes of the rules that have one. Findings accepted by a
  baseline are results with a suppression, unless `--sarif-hide-baselined` is given. Each result has a `partialFingerprints`
  `primaryLocationLineHash` (the baseline fingerprint of the finding without its message, and its number in the file), so code
  scanning keeps tracking a finding when lines are added above it, and the files are listed in `artifacts`.
- `gcc` prints `file:line:col: error|warning|note: message [id]`. `github` prints workflow commands (`::error`, `::warning`,
  `::notice`) with the rule ID as the `title`.
- `summary` prints counts per rule and per file instead of findings.
- In templates, `allKinds` still works and `allRules` lists every rule with its ID, group, level and profile. The error fields are
  the same as before plus `ID`, `Severity`, `DocURL`, `EndLine` and `EndColumn`.

With `--format sarif` stdout holds only the log and stderr is empty unless `--verbose` or `--debug` is given. See
[Format error messages](usage.md#format-error-messages).

## Fix

`--fix` applies the safe fixes and `--fix=unsafe` also those that may change what a workflow does. In v2:

- Many rules have a fix (and `--online --fix` pins tags); the list, with which fixes are safe, is in [Fix errors automatically](usage.md#fix-errors-automatically). A fix
  that needs a value you did not give is not offered: `missing-timeout` needs `rules: {missing-timeout: {default-minutes: N}}` and
  `dependabot-cooldown` needs `default-days`.
- `--fix` lints, fixes and lints again until no fix is left, so a second run changes nothing. Each file is written once and
  atomically. Every pass is checked: the result must be valid YAML that differs from the original only where the edits are, or the
  fix is refused and the exit status is 3 with the rule named.
- `jactionlint --diff` prints what `--fix` would do as a unified diff and writes nothing (exit 1 when there is a diff).
- `jactionlint --fix --fix-rules missing-timeout,artipacked` applies only the fixes of those rules, like `fix: {rules: [...]}` in the config.
- `--fix` cannot read stdin and exits with status 2 if asked to.
- `--fix=unsafe` of `self-repository` (in the `pedantic` profile) rewrites `uses: ./path` to `uses: $/path`. Every released
  actionlint, up to and including 1.7.12, rejects the `$/` syntax (`specifying action "$/..." in invalid format`), so a repository that
  also runs actionlint starts to fail there. That is why the fix is unsafe and never applied by `--fix` alone.
- Some fixes are not offered where they would be wrong: `missing-permissions` writes nothing into a reusable workflow
  (`workflow_call`), because the callers decide what the token can do and a callee asking for more than a caller grants is rejected
  at startup; and `template-injection` leaves the expression in a word list (`for f in ${{ ... }}`, `files=(...)`) alone, because quoting
  it would make one item of the list.

```console
$ jactionlint --diff --fix-rules artipacked
--- a/.github/workflows/ci.yaml
+++ b/.github/workflows/ci.yaml
@@ -8,6 +8,8 @@
     runs-on: ubuntu-latest
     steps:
       - uses: actions/checkout@v4
+        with:
+          persist-credentials: false
       - run: echo "${{ github.event.pull_request.title }}"
```

## Baseline

The stricter default can report hundreds of findings on a repository that never saw it. A baseline records them, hides them and
fails only on new ones:

```sh
jactionlint --format summary    # count the findings per rule and file
jactionlint --baseline-write    # write .github/jactionlint-baseline.json, exit 0
jactionlint --baseline          # hide the recorded findings; exit 1 only for new ones
```

`baseline: auto` in the config applies the default file without the flag. Entries are the file, the rule ID and a fingerprint,
not line numbers, so moving code does not resurrect a finding; editing the flagged line does. `--baseline-check` lists the entries that
match nothing any more (`unused-baseline-entry`, `info`), so the file can only shrink. Write the baseline in the environment CI runs in
(same config, `--online`, shellcheck). See [Adopt a stricter configuration with a baseline](usage.md#baseline).

## Durable ignores and ignore comments

An `# jactionlint ignore=...` comment is tied to a line, which Renovate or Dependabot rewrite when they bump `uses:`. The `ignores:`
list in the config file matches by structure instead and can expire:

```yaml
ignores:
  - rule: unpinned-uses
    uses: actions/checkout        # whatever the ref is
    file: .github/workflows/release.yaml
    reason: pinned by an organization ruleset
    expires: 2027-06-30
```

An entry past its date stops suppressing and is reported as `expired-ignore` (an error; `info` during the last 14 days). An entry that
matched nothing is `unused-ignore` (pedantic). See [Durable ignores](config.md#durable-ignores).

**A behavior change in ignore comments.** A `# jactionlint ignore=...` comment after YAML content (`- uses: a/b@v1 # jactionlint
ignore=unpinned-uses`) is now a directive. It covers that line and, when it is on the first line of a step, the whole step. In v1 and
in actionlint only a comment on its own line worked and this form was silently ignored, so an old comment of this shape that did
nothing now starts suppressing. Stacked comments on their own lines still cover the next line and what is nested under it.

## zizmor ignore comments

A `# zizmor: ignore[rule-a,rule-b]` comment is honored, with the scoping rules of zizmor: it applies to the findings whose region
contains the comment. A name stands for the jactionlint rule of the same ID. Two audits are reported under another ID:

| zizmor audit | jactionlint rule | Note |
| --- | --- | --- |
| `excessive-permissions` | `excessive-permissions` and `missing-permissions` | zizmor's report of a missing `permissions:` block is `missing-permissions` here |
| `unpinned-images` | `unpinned-uses` | only the Docker image findings of the rule |

A name with no rule of that ID (an audit jactionlint lacks) is skipped. `jactionlint --migrate-ignores [files]` rewrites the trailing
comments into `# jactionlint ignore=` comments on the line above, with the reason as a plain comment line. A name that would widen
the scope when migrated (`unpinned-images` covers only some findings of `unpinned-uses`) stays in a zizmor comment. Running it again
changes nothing. With `unused-ignore` enabled, a zizmor comment is reported as stale only when its audit maps to an enabled rule.
See [zizmor ignore comments](usage.md#zizmor-ignore-comments).

## Online mode

`--online` (or `online: true`) turns on six rules that ask GitHub about the actions a workflow uses: `impostor-commit`,
`known-vulnerable-actions`, `ref-confusion`, `stale-action-refs`, `archived-uses` and `ref-version-mismatch`. With `--fix` it also pins
a tag to the commit it points to and names the tag in a comment (`actions/checkout@<sha> # v4`). Without the flag jactionlint never
uses the network, and the playground has no network at all.

- A lookup that fails (404, 403, a server error, a timeout, DNS, a rate limit that resets too late) is skipped with one warning per
  kind of failure and does not change the exit status. `--online=strict` makes a skipped lookup exit with status 3; `--online=cache` answers
  from the disk cache only and never uses the network (`--online=cache,strict` combines them).
- The token comes from `--online-token-env`, `--online-token-file`, `GITHUB_TOKEN`, `GH_TOKEN` or `gh auth token`, and is sent only to the API
  host. A host named by a repository's own `.github/jactionlint.yaml` gets no token.
- `--online-api-url` (or `$GITHUB_API_URL`, `$GITHUB_SERVER_URL`, `$GH_HOST`) selects a GitHub Enterprise Server. `--online-allow` and
  `--online-deny` restrict which `owner/repo` are looked up. `--online-max-wait` bounds the wait for a rate limit and
  `--online-cache-ttl` how long an answer is used without asking again.
- The `online-options` key of the config file sets the same things (`mode`, `api-url`, `token-env`, `token-file`, `allow`, `deny`,
  `cache-ttl`, `max-rate-limit-wait`, `retries`, `concurrency`, `gh-cli`). Command line flags win.

See [Online checks](usage.md#online-checks) and [`online-options`](config.md#online-options).

## Composite actions and Dependabot

Running `jactionlint` without arguments now also checks:

- **`action.yml` files** (the root, `.github/actions/**` and the targets of local `uses: ./path`). Their steps get the same rules as
  workflow steps, `action-syntax` checks their metadata, and the rules that depend on the trigger use the local workflows that call
  the action. An action without a local caller is judged by its own steps only. See [composite actions](checks.md#check-composite-actions).
- **`.github/dependabot.yml`**: `dependabot-syntax` (`correctness`), `dependabot-cooldown` and `dependabot-execution` (`default`) and
  `dependabot-missing-actions-update` (`pedantic`). See [Dependabot](checks.md#check-dependabot-syntax).

A repository whose actions and Dependabot file were never linted can see new findings. Silence them by path
(`paths: {".github/actions/**": {ignore: [...]}}`) or per rule (`rules: {action-syntax: off}`).

## hk

[hk](https://hk.jdx.dev) can run jactionlint as a linter and a fixer. `--format sarif` gives hk the diagnostics with rule IDs and the
fixes, `hk util sarif-diff` turns them into a patch, and `jactionlint --fix` is the fallback for findings without a SARIF fix. Add this
step to `hk.pkl` (tested with hk 2.5.0):

```pkl
["jactionlint"] {
    glob = List(".github/workflows/*.yml", ".github/workflows/*.yaml")
    batch = true
    diagnostic_format = "sarif"
    check = "jactionlint --format sarif {{files}}"
    check_diff = "hk util sarif-diff -- jactionlint --format sarif {{files}}"
    fix = "jactionlint --fix {{files}}"
}
```

Add `--profile correctness` to the three commands to keep what actionlint gave you, or `--baseline --sarif-hide-baselined` to hide a
baseline from hk (hk does not read SARIF suppressions). Install jactionlint for the hook with mise (`mise use jactionlint`) and run
`hk install --mise`. See [hk](usage.md#hk).

## Go API

- Import `github.com/jdx/jactionlint/v2`.
- `Error` gains `ID`, `Severity`, `EndLine`, `EndColumn`, `DocURL` and `Fix` (byte-range `TextEdit`s). `Kind` stays.
- `RuleBase.ReportID`, `ReportIDf` and `ReportRange` report with a stable ID. `Error` and `Errorf` still work for custom rules, with the
  rule name as the ID.
- The boolean fields of `Config` and `TimeoutMinutesConfig` are removed. Use `Config.Rules` and `Config.RuleLevel`; `Config.Profile`,
  `Extends`, `Ignores`, `Baseline`, `Online` and `OnlineOptions` are new.
- The `ignore` list of `PathConfig` is an `IgnorePatterns` list of `IgnorePattern` values, each a rule ID or a regular expression.
- `LinterOptions` gains `Profile` (overrides the config, like `--profile`), `MinSeverity`, `Online`, `OnlineOptions`, `GitHubClient`
  and `Now`. A `Config` that sets no profile means `ProfileDefault`, so a program that used the library in v1 now runs the `default`
  profile; set `Config.Profile` (or `LinterOptions.Profile`) to `ProfileCorrectness` to keep v1 behavior.
- `Rules()`, `LookupRule()` and `RenamedRules()` expose the registry. `Linter.FixFiles`, `FixRepository`, `WriteBaseline`,
  `MigrateConfig` and `MigrateIgnores` are the library side of the new flags; `MigrateConfig(src)` and `MigrateConfigFile(path)` rewrite a config.

See [Go API](api.md).

## Behaviors that surprise

- **The default is stricter.** Without a profile you get `default`. A repository that passed v1 can now fail on `unpinned-uses`,
  `missing-permissions`, `missing-timeout`, `concurrency-limits`, `template-injection` sinks, `artipacked`, `dangerous-triggers`,
  `pipeline-without-pipefail`, `unverified-download` and others. Choose `profile: correctness`, or adopt with a [baseline](#baseline).
  Strict zizmor compatibility is not a goal; see [where jactionlint stands relative to zizmor](zizmor-parity.md).
- **`missing-timeout` is on.** A job without `timeout-minutes` is an error. There is no built-in number, so the finding has a fix only
  when `rules.missing-timeout.default-minutes` is set. `rules: {missing-timeout: off}` restores v1.
- **`pipeline-without-pipefail`.** The default shell (`bash -e {0}`) and `shell: sh` do not enable `pipefail`, so `curl ... | sh`
  hides a failing `curl`. Set `shell: bash` (which has `pipefail`) or `set -o pipefail`. The fix inserts `set -o pipefail` and is unsafe,
  because failures that used to pass now fail. It is the most intricate rule of the default profile, with a "Known limits" section in its check.
- **`concurrency-limits` has a fixer, and only for pull request workflows.** For a workflow whose triggers are all pull request events and
  that has no release or deploy job, `--fix` adds a group per pull request with `cancel-in-progress: true`
  (`${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}`). It never adds it to a workflow that releases or
  deploys. `concurrency-cancels-release` recommends `cancel-in-progress: false`, a group per ref or tag and `queue: max`.
- **No invented values.** A fixer never makes up a number: `missing-timeout` and `dependabot-cooldown` offer a fix only when
  `default-minutes` or `default-days` is configured.
- **`template-injection` covers more than scripts.** The `correctness` profile also reports untrusted input in the options of
  `container:` and `services:`, and in the prompt of AI agent actions (Claude Code Action, Gemini CLI, Codex and others).
  `agentic-actions` (`default`) reports an agent that outsiders can steer.
- **Pedantic findings share the rule's level.** Under `--profile pedantic` the expansions of `template-injection` are reported at `error`.
- **Unknown config keys are fatal** (exit 3), including a rule ID that does not exist.
- **Legacy `actionlint.yaml`** is read as a jactionlint config with the `default` profile and a one-line note.
- **Ignore comments at the end of a line now work** (see above).
- **shellcheck findings are on the line of the script.** For a literal `run: |` block the finding is reported at the line that has the
  problem, not at the `run:` key as actionlint does. An ignore comment on the step, above `run:` or at the end of the `run: |` line
  still covers all the lines of the script; only a tool that keeps `file:line` pairs has to be refreshed. See
  [where shellcheck findings are reported](actionlint.md#where-shellcheck-findings-are-reported).
- **Fixes of baselined findings.** `--fix` also fixes findings the baseline hides.

## Related

- [Coming from actionlint](actionlint.md): the `correctness` profile and the rule IDs of actionlint's checks.
- [zizmor](zizmor-parity.md): where jactionlint stands relative to zizmor, audit by audit.
- [Configuration](config.md), [Usage](usage.md), [Rules](rules.md).
