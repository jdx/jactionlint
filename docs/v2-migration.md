# Migrating to v2

Upgrade a repository from jactionlint 1.x to v2: what breaks, the steps that fix it, the new option spellings and the features that v2 adds.

jactionlint v2 reads the same workflow files. It selects rules by profile, gives every finding a rule ID and a level, and applies a stricter profile when you configure nothing, so a repository that was clean can fail after the upgrade. If you come from actionlint and not from jactionlint 1.x, read [Coming from actionlint](./actionlint.md) instead.

<a id="what-changes-at-a-glance"></a>
<a id="behaviors-that-surprise"></a>
## What breaks

Each row is a change that can turn a passing run red or change what a script reads. The last column links to the section or the page that owns the topic.

| Change | What you see | What to do | Details |
| --- | --- | --- | --- |
| Long options with a single dash | Exit status 2 and a one-line message that names the replacement, as in "did you mean --format?". | Rename the options in scripts, CI steps, hk steps, Docker commands and mise tasks. | [Command line options](#command-line-options) |
| The default profile | Findings of security and policy rules, such as `unpinned-uses`, `missing-permissions`, `missing-timeout` and `concurrency-limits`, and exit status 1. | Set `profile: correctness`, or record the findings in a baseline and fail only on new ones. | [Profiles](#profiles), [baseline](./baseline.md#baseline) |
| A hint on stderr | After a text run with many findings in a terminal or in CI, one line names `--baseline-write` and `--profile correctness`. | Follow it, or silence it with `--no-hints` or `JACTIONLINT_NO_HINTS=1`. | [The first run](./usage.md#the-first-run) |
| Unknown keys in the config file | Exit status 3 and a message that names the key and suggests a fix. | Fix the typo or remove the key. | [Strict parsing](#strict-parsing) |
| Deprecated keys of the config file | One deprecation warning per key on stderr. The keys still work. | Run `jactionlint --migrate-config`. | [Deprecated keys](#deprecated-keys-and-migrate-config) |
| Levels | A finding of level `warn` or `info` does not make the exit status 1. | Pass `--strict-exit` to fail on them. | [Exit codes and severity](#exit-codes-and-severity) |
| Ignore comments at the end of a line | A comment such as `- uses: a/b@v1 # jactionlint ignore=unpinned-uses` suppresses findings. In 1.x it did nothing. | Review the comments of this shape. | [Ignore comments](#durable-ignores-and-ignore-comments) |
| `action.yml` files, `.github/dependabot.yml` and the Actions lock file | A run without operands also checks them, so new findings can appear. | Ignore a rule for a path with `paths`, or turn it off with `rules: {action-syntax: off}`. | [What a run checks](./usage.md#what-a-run-checks) |
| Workflow files in a subdirectory of `.github/workflows` | A run without arguments does not check them, because GitHub does not load them. | Pass the file as an argument when you want it checked. | [Repository mode](./usage.md#repository-mode) |
| shellcheck or pyflakes that is not installed | A `note:` on stderr says that the tool was not found. A `--shellcheck=CMD` or `--pyflakes=CMD` with a command that does not exist exits with status 3. 1.x said nothing. | Install the tool, or pass an empty value to turn the integration off. | [External tools](./usage.md#when-shellcheck-or-pyflakes-is-missing) |
| The label at the end of a finding | The text format prints the rule ID where 1.x printed the kind. | Update scripts that read the label. The `kind` stays in `--format json` and in templates. | [Rule IDs](#rule-ids-and-aliases) |
| The position of a shellcheck finding | The finding points at the token of the script, not at the column of `run:`. | Refresh tools that keep a list of positions, such as `file:line:column`. | [Where shellcheck findings are reported](./actionlint.md#where-shellcheck-findings-are-reported) |
| Go programs | The import path ends in `/v2`, the fields of `Config` that held the deprecated keys are gone, and a program without a profile runs `default`. | Change the import and set a profile. | [Go programs](#go-programs) |

## Upgrade in five steps

1. Rename the options. Use the two-dash spellings in scripts, CI steps, hk steps, Docker commands and mise tasks, as [Command line options](#command-line-options) lists them. 1.x accepts the two-dash spellings too, so you can rename before you upgrade.
2. Choose a profile. `profile: correctness` runs the checks of actionlint and the bug detectors of jactionlint. To adopt the `default` profile instead, record today's findings with `jactionlint --baseline-write` and run `jactionlint --baseline` from then on.
3. Migrate the config file. `jactionlint --migrate-config` rewrites the deprecated keys in place and keeps comments and other keys.

   ```sh
   jactionlint --migrate-config
   ```

4. Run jactionlint and read the exit status. Status 2 means an option that you did not rename. Status 3 means a config problem, such as a key that 1.x ignored. Status 1 means findings: review the ignore comments, the files that are newly checked and the positions of shellcheck findings.

   ```sh
   jactionlint --profile correctness
   ```

5. Update the Go programs that import the library ([Go programs](#go-programs)).

## Command line options

v2 accepts POSIX/GNU options only. A long option has two dashes, such as `--format`. The single-dash long options have no alias and no deprecation period: a command that uses one exits with status 2 and a message that names the replacement. Besides the spelling:

- A value is given as `--format=json` or `--format json`. A short option is one letter, bundles with others and takes its value attached or separate: `-vfjson`, `-cfile.yaml`, `-c file.yaml`. The short options are `-h`, `-V`, `-v`, `-c`, `-f`, `-p` and `-i`.
- An option with an optional value takes it only as `--option=VALUE`: `--fix=unsafe`, `--online=cache`, `--baseline=FILE`, `--baseline-write=FILE` and `--color=never`. `--fix unsafe` is `--fix` and a file named `unsafe`.
- A boolean option takes no separate value (`--oneline=false` is accepted, `--oneline false` reads `false` as a file), and a default that is on is turned off with `--no-X`.
- An abbreviation such as `--form` is an unknown option.
- `--` ends the options, and a lone `-` means stdin.
- `--help` prints to stdout, where 1.x printed to stderr. The exit status is 0.

[Command line](./cli.md#command-line-options) describes every option of v2, the environment variables and the exit status. The next tables map the old spellings to the new ones.

<!-- legacy-options:start -->

jactionlint recognizes an argument that is exactly a single-dash long option of 1.x before it can read the argument as a bundle (`-fix` is not `-f -i -x`). The message names the replacement:

```sh
jactionlint -profile=default
```

```
jactionlint: unknown option -profile; did you mean --profile? (try --help)
```

### Before and after

The common renames, as command lines. The two tables that follow say which spelling belongs to a release of 1.x and which to a development build.

| Before | After |
| --- | --- |
| `jactionlint -format sarif` | `jactionlint --format sarif` (or `-f sarif`) |
| `jactionlint -no-color -config-file ci.yaml` | `jactionlint --no-color -c ci.yaml` |
| `jactionlint -profile correctness` | `jactionlint --profile correctness` (or `-p correctness`) |
| `jactionlint -fix -rules missing-timeout` | `jactionlint --fix --fix-rules missing-timeout` |
| `jactionlint -online=false`, `-baseline=false` | `jactionlint --no-online`, `--no-baseline` |
| `jactionlint -version` | `jactionlint --version` (or `-V`) |

### Spellings that existed in 1.x

These options exist in the releases of 1.x and in actionlint. Both accept the two-dash spelling too.

| Before | After | Change besides the spelling |
| --- | --- | --- |
| `-h`, `-help` | `--help` (`-h`) | Prints to stdout. |
| `-version` | `--version` (`-V`) | None. |
| `-verbose` | `--verbose` (`-v`) | None. |
| `-debug` | `--debug` | None. |
| `-stdin-filename NAME` | `--stdin-filename NAME` | None. |
| `-format FORMAT` | `--format FORMAT` (`-f`) | Also takes the names `text`, `oneline`, `json`, `jsonl`, `sarif`, `gcc`, `github` and `summary`. A value with `{{ }}` is a Go template. |
| `-oneline` | `--oneline` | None. |
| `-color` | `--color` | Takes a value only as `--color=WHEN`. |
| `-no-color` | `--no-color` | None. |
| `-ignore PATTERN` | `--ignore PATTERN` (`-i`) | The pattern can be a rule ID. |
| `-init-config` | `--init-config` | The generated file lists the keys of v2 as comments. |
| `-config-file FILE` | `--config-file FILE` (`-c`) | None. |
| `-shellcheck CMD` | `--shellcheck CMD` | A command that does not exist exits with status 3. |
| `-pyflakes CMD` | `--pyflakes CMD` | A command that does not exist exits with status 3. |

### Spellings from development builds

These spellings appear in no release of 1.x. They existed only in builds of the main branch before the first v2 release. jactionlint recognizes them, so that a command written against such a build fails with a hint instead of a vague message.

| Before | After |
| --- | --- |
| `-rule-ids` | `--rule-ids` (accepted and ignored: the text output always shows the rule ID) |
| `-no-hints` | `--no-hints` |
| `-min-severity LEVEL` | `--min-severity LEVEL` |
| `-strict-exit` | `--strict-exit` |
| `-profile NAME` | `--profile NAME` (`-p`) |
| `-migrate-config` | `--migrate-config` |
| `-migrate-ignores` | `--migrate-ignores` |
| `-fix`, `-fix=unsafe` | `--fix`, `--fix=unsafe` |
| `-diff` | `--diff` |
| `-rules RULES` | `--fix-rules RULES` |
| `-baseline`, `-baseline=FILE` | `--baseline`, `--baseline=FILE` |
| `-baseline=false` | `--no-baseline` |
| `-baseline-write`, `-baseline-write=FILE` | `--baseline-write`, `--baseline-write=FILE` |
| `-baseline-check` | `--baseline-check` |
| `-sarif-hide-baselined` | `--sarif-hide-baselined` |
| `-online`, `-online=MODE` | `--online`, `--online=MODE` |
| `-online=false` | `--no-online` |
| `-online-api-url URL` | `--online-api-url URL` |
| `-online-token-env NAME` | `--online-token-env NAME` |
| `-online-token-file FILE` | `--online-token-file FILE` |
| `-online-allow PATTERN` | `--online-allow PATTERN` |
| `-online-deny PATTERN` | `--online-deny PATTERN` |
| `-online-cache-ttl DURATION` | `--online-cache-ttl DURATION` |
| `-online-max-wait DURATION` | `--online-max-wait DURATION` |

<!-- legacy-options:end -->

<a id="config"></a>
## Config keys

The keys of the config file that changed between 1.x and v2.

### Profiles

`profile:` in the config file, or `--profile NAME` on the command line, selects the rules that run, and the option wins. The three profiles are `correctness`, `default` and `pedantic`, and each includes the one before it ([Profiles](./config.md#profiles) defines them). 1.x had no `profile` key, so a config file of 1.x gets `default`, which has more rules than 1.x ran. The closest profile to 1.x is `correctness`.

Builds of the main branch before the first v2 release knew `profile: strict` and `profile: all`. jactionlint reads them as `pedantic` with a deprecation warning, and `--migrate-config` rewrites them.

<a id="rules"></a>
<a id="extends"></a>
### Rule levels and shared config files

`rules: {RULE: off|info|warn|error}` sets the level of one rule whatever the profile is, and it turns on a rule that the profile does not have. A rule with options takes a mapping, such as `rules: {max-run-lines: {level: warn, max: 80}}`. [Rules](./config.md#rules) describes the key.

`extends: [../shared/jactionlint.yaml]` inherits other config files. Later files win and the file itself wins over all of them, and `rules` and `paths` are merged by key. [Extending config files](./config.md#extending-config-files) describes the key.

### Strict parsing

1.x ignored a key that it did not know, so a typo was harmless. jactionlint reports an unknown key, rule ID or rule option in the config file and exits with status 3. The message names the unknown name and suggests or lists the valid ones. The message of a misspelled key ends with:

```
unknown key "self-hosted-runnr" in the configuration at line 1, column 1. did you mean "self-hosted-runner"?
```

<a id="deprecated-booleans-and-migrate-config"></a>
### Deprecated keys and `--migrate-config`

The deprecated keys of 1.x, such as `require-commit-hash`, `max-run-lines` and `timeout-minutes`, still work. jactionlint translates them into rules and prints one deprecation warning per key on stderr, and as a notification in the SARIF log with `--format sarif`. `jactionlint --migrate-config` rewrites the file in place and keeps comments and other keys. A file of 1.x like this:

```yaml
self-hosted-runner:
  labels:
    - my-runner
require-commit-hash: true
max-run-lines: 40
timeout-minutes:
  required: true
```

becomes:

```yaml
self-hosted-runner:
  labels:
    - my-runner
rules:
  unpinned-uses: error
  max-run-lines: {level: error, max: 40}
  missing-timeout: error
```

Setting a boolean key to `false` turns its rule off. A rule written under `rules` wins over a deprecated key. [Deprecated keys](./config.md#deprecated-keys) has the table of every key and its replacement.

### A config file written for actionlint

A config file named `.github/actionlint.yaml` is read as a jactionlint config when no `.github/jactionlint.yaml` exists. It has no `profile`, so the `default` profile applies, and jactionlint prints a note on stderr. [A config file written for actionlint](./actionlint.md#a-config-file-written-for-actionlint) shows the note and how to silence it.

## Rule IDs and aliases

Every finding has a stable rule ID, such as `unpinned-uses`. The text format prints it at the end of the line, where 1.x printed the kind. It is the `id` of `--format json`, the `ruleId` of `--format sarif`, the `title` of `--format github` and the last word of `--format gcc` ([output formats](./output.md#format-error-messages)).

`kind` is on every finding too, so a `--format` template or a script written for 1.x keeps working. A rule ID is never renamed or reused after a release, so you can rely on it in `rules`, `--ignore`, `paths` and ignore comments. A pattern is a rule ID when it equals one and a regular expression on the message otherwise ([Ignore findings](./ignore.md#ignore-some-errors)).

A few IDs were merged into other rules before the first v2 release, such as `template-injection-expansion`. They never shipped in a release of 1.x. jactionlint still accepts them in `--ignore`, in the `ignore` lists of `paths` and in ignore comments, with a deprecation warning, but not in `rules` or `ignores`. [Rules](./rules.md#retired-rule-ids) lists them.

## Ignore findings

### Durable ignores and ignore comments

A comment at the end of a line is a directive. `- uses: a/b@v1 # jactionlint ignore=unpinned-uses` covers that line and, on the first line of a step, the whole step. 1.x read only comments on their own line and ignored this form without a message, so an old comment of this shape that did nothing starts to suppress findings. List the comments and review each one:

```sh
grep -rn 'actionlint ignore=' .github
```

Comments on their own lines are unchanged: stacked comments cover the next line and what is nested under it. `# actionlint ignore=RULE` is accepted as an alias of `# jactionlint ignore=RULE`.

A comment is tied to a line, and Renovate or Dependabot rewrite lines when they bump a `uses:`. The `ignores` list of the config file matches by file, job, step and `uses:` instead, and an entry can carry a reason and an expiry date. [Durable ignores](./config.md#durable-ignores) describes the keys.

### zizmor ignore comments

jactionlint honors `# zizmor: ignore[rule-a,rule-b]` comments with the scoping of zizmor, and `jactionlint --migrate-ignores` rewrites them into `# jactionlint ignore=` comments. [zizmor ignore comments](./ignore.md#zizmor-ignore-comments) has the scoping rules, and [audit names that differ](./ignore.md#audit-names-that-differ) the names that map to another rule.

<a id="module-path"></a>
<a id="go-api"></a>
## Go programs

The Go module is `github.com/jdx/jactionlint/v2`, and the package is still named `jactionlint`. Install the command with `go install` under the new path ([build from source](./install.md#build-from-source) names the Go version it needs). mise, Homebrew, the download script and Docker are unchanged ([installation](./install.md#choose-a-method)).

```sh
go install github.com/jdx/jactionlint/v2/cmd/jactionlint@latest
```

A program that imports the library changes its import to `github.com/jdx/jactionlint/v2`. These changes can break the build or the result:

- The fields of `Config` that held the deprecated keys (`RequireCommitHash`, `TimeoutMinutes`, `MaxRunLines` and the other boolean ones) and the type `TimeoutMinutesConfig` are removed. Use `Config.Rules` and `Config.RuleLevel`. `Config.Profile`, `Extends`, `Ignores`, `Baseline`, `Online` and `OnlineOptions` are new.
- The type of `PathConfig.Ignore` changes to `IgnorePatterns`, a list of `IgnorePattern` values that each hold a rule ID or a regular expression.
- A `Config` that sets no profile means `ProfileDefault`, so a program that used the library in 1.x runs the `default` profile. Set `Config.Profile` or `LinterOptions.Profile` to `ProfileCorrectness` to keep the checks of 1.x.

`Error` gains fields such as `ID`, `Severity`, `EndLine`, `EndColumn`, `DocURL` and `Fix`, and `Kind` stays. `LinterOptions` gains `Profile`, `MinSeverity`, `Online`, `OnlineOptions`, `GitHubClient` and `Now`, and a custom rule reports with a stable ID through `RuleBase.ReportID`. The library side of the new features, such as `Rules()`, `LookupRule()`, `Linter.FixFiles` and `Linter.WriteBaseline`, is in [Go API](./api.md#go-api).

## Exit codes and severity

1.x exited with 1 for any finding. v2 exits with 1 for a finding of level `error` that the ignores and the baseline do not hide, and with 0 when it finds nothing or only findings of level `warn` and `info`. Two exceptions are `--strict-exit` and `--diff`, which exits with 1 when it prints a diff. The [exit status table](./cli.md#exit-status) lists every cause of each status.

Every rule of `correctness` and `default` reports at `error`, except `unused-baseline-entry`, which reports at `info`. The exit status is 1 whenever they find anything, as in 1.x. Rules of the `pedantic` profile and the online rules report at `error`, `warn` or `info`, and the findings of level `warn` and `info` do not fail the run unless you pass `--strict-exit`. `--min-severity warn` and `--min-severity error` hide the lower levels.

<a id="output-formats"></a>
<a id="fix"></a>
<a id="baseline"></a>
<a id="online-mode"></a>
<a id="composite-actions-and-dependabot"></a>
<a id="hk"></a>
## What is new since 1.x

Nothing in this table changes a run until you use it, except where [What breaks](#what-breaks) says so. Each row links to the page that owns the feature.

| Feature | What you get | Details |
| --- | --- | --- |
| Rule IDs and profiles | A stable ID, a group and a level for every finding, and profiles that select the rules. | [Rules](./rules.md), [profiles](./config.md#profiles) |
| Output formats | `--format` takes `text`, `oneline`, `json`, `jsonl`, `sarif`, `gcc`, `github` and `summary`, or a Go template. | [Output formats](./output.md#format-error-messages) |
| Fixes | `--fix` applies the safe fixes, `--fix=unsafe` the others, `--diff` previews them and `--fix-rules` restricts them. | [Fix findings automatically](./fix.md#fix-errors-automatically) |
| Baseline | `--baseline-write` records today's findings, and runs with `--baseline` or `baseline: auto` fail only on new ones. `--fix` also fixes the findings that the baseline hides: preview with `--diff`. | [Baseline](./baseline.md#baseline) |
| Durable ignores | `ignores` in the config file matches findings by structure and can expire. | [Durable ignores](./config.md#durable-ignores) |
| zizmor ignore comments | `# zizmor: ignore[...]` comments are honored, and `--migrate-ignores` converts them. | [zizmor ignore comments](./ignore.md#zizmor-ignore-comments) |
| Online checks | `--online` adds rules that ask GitHub about the actions in use, and `--online --fix` pins tags to commits. The rule `action-release-cooldown` (the version of an action is younger than a number of days) is online too, but runs only when `rules` has an entry for it. They are off by default. | [Online checks](./online.md#online-checks) |
| Composite actions and Dependabot | `action.yml` files and `.github/dependabot.yml` are checked with the workflow files. | [Composite actions](./checks.md#check-composite-actions), [Dependabot](./checks.md#check-dependabot-syntax) |
| hk | A SARIF step gives hk findings with rule IDs and fixes. | [hk](./hooks.md#hk) |
| Explaining rules | `--explain RULE` and `--list-rules` describe the rules, and `--select RULES` runs only some of them. | [Explain a rule](./usage.md#explain-a-rule) |

<a id="related"></a>
## Next steps

- [Coming from actionlint](./actionlint.md) for the `correctness` profile and the differences from actionlint.
- [Command line reference](./cli.md#command-line-options) for every option, environment variable and exit status.
- [Configuration](./config.md) for profiles, rule levels, ignores and deprecated keys.
- [Adopt with a baseline](./baseline.md#baseline) to see new checks without a wall of findings.
- [jactionlint and zizmor](./zizmor-parity.md) for how the security rules compare with zizmor.
