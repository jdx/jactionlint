# Command line

This page lists every jactionlint option, environment variable, file, program, network host and exit status, for people who write commands, CI steps and hooks. [Installation](./install.md) says how to tell which of these your release supports.

For the tasks behind the options, start with [Run jactionlint](./usage.md). [Online checks](./online.md#online-checks) explains the online options, and [the configuration file](./config.md) the keys that mirror options.

## Synopsis and operands

```sh
jactionlint [OPTIONS] [FILES...]
jactionlint [OPTIONS] -
```

- **No operand:** jactionlint checks the repository that holds the working directory: its workflow files, Dependabot configuration, composite actions and Actions lock file. See [what a run checks](./usage.md#what-a-run-checks).
- **File operands:** jactionlint checks exactly these YAML files, wherever they are. A directory is not an operand: it exits with status 3 and the message `is a directory`.
- **`-`:** jactionlint reads one input from stdin. The `-` must be the only operand: `jactionlint ci.yaml -` exits with status 3 and the message `could not read "-"`. Use `--stdin-filename` to name the input and to say what it is.
- **`--`:** everything after it is an operand, so `jactionlint -- -odd.yaml` checks a file named `-odd.yaml`.

Standard output carries the result you asked for: the findings, the diff of `--diff`, the text of `--help`, `--version`, `--list-rules`, `--explain`, `--completion`, `--print-config-schema` and `--print-output-schema`, and the messages of `--baseline-write`, `--init-config`, `--migrate-config` and `--migrate-ignores`. Everything else goes to stderr: the summaries of `--fix` and `--diff`, hints, notes, warnings, the logs of `--verbose` and `--debug`, and error messages. [Streams and color](./output.md#streams-and-color) covers the cases of `--diff`, `--fix` and `--format sarif`.

## Option syntax

- **Long options** are kebab-case with two dashes and take their value as `--format=json` or `--format json`.
- **Short options** exist for `-h`, `-V`, `-v`, `-f`, `-p`, `-c` and `-i` only. They bundle, and a value follows attached or separate: `-vfjson`, `-cconfig.yaml` and `-c config.yaml` all work.
- **Optional values:** `--color`, `--fix`, `--online`, `--baseline` and `--baseline-write` take a value only with `=`, as in `--fix=unsafe` and `--online=cache`. `--fix unsafe` is `--fix` plus an operand `unsafe`, which jactionlint tries to read as a file. The tables below write such a value in brackets, as in `[=WHEN]`.
- **Switches:** an option without a value is a switch. `--oneline=false` is accepted as well.
- **`--no-` options:** `--no-online` and `--no-baseline` override what the config file or `JACTIONLINT_ONLINE` turned on. `--no-color` is `--color=never` and wins over `--color` in any order. `--no-hints` and `--no-ignores` are switches of their own.
- **Repeated options:** `--ignore`, `--select`, `--online-allow` and `--online-deny` can be given several times. `--select`, `--online-allow` and `--online-deny` also accept a comma-separated list. `--ignore` does not: `--ignore a,b` is one [pattern](./ignore.md#write-a-pattern), the regular expression `a,b`, so repeat `--ignore`. `--fix-rules` takes one comma-separated list.
- **No abbreviations:** `--form` is not `--format`. An unknown option, a missing value or a value that cannot be parsed prints one line, for example `jactionlint: unknown option --form (try --help)`, and exits with status 2.
- **Single-dash long options:** the old spellings that actionlint uses, with one dash, are rejected with a message that names the replacement, and exit with status 2. See [the migration table](./v2-migration.md#command-line-options).
- **Options that print and exit:** when you give several, the first one in this order runs and the others are ignored, whatever their order on the command line: `--help`, `--version`, `--list-rules`, `--print-config-schema` or `--print-output-schema` (giving both is one of the [option combinations that fail](#option-combinations-that-fail)), `--completion`, `--explain`. A command line that cannot be parsed exits with status 2 before any of them, `--help` included.

<a id="command-line-options"></a>

## Options

Each table lists the options of one group of `jactionlint --help`, in the order of `--help`. The default column shows what the option does when you do not give it. The environment variables and config keys that do the same work are in [the mapping table](#flag-environment-variable-and-configuration-key).

### General

| Option | Short | Value | Default | Effect |
| --- | --- | --- | --- | --- |
| `--help` | `-h` | | | Print the usage and the option list, then exit. |
| `--version` | `-V` | | | Print the version, how this binary was installed, and the Go version and platform it was built for, then exit. |
| `--verbose` | `-v` | | | Log to stderr what jactionlint does: the repository, the collected files and the time spent on each file. |
| `--debug` | | | | Log debug output to stderr. It is meant for developing jactionlint. |
| `--completion` | | `SHELL` | | Print the completion script of a shell and exit. SHELL is `bash`, `zsh`, `fish` or `powershell`; `pwsh` is accepted as `powershell`. Any other value exits with status 2. See [shell completion](./usage.md#shell-completion). |
| `--stdin-filename` | | `NAME` | `<stdin>` | The name that findings show for the input read from stdin. The name also decides what the input is: a workflow, a Dependabot configuration, a composite action or the Actions lock file. When the name is the path of an existing file in a repository, the config file of that repository applies to the input. Otherwise the user-global config file or the defaults apply. See [files and stdin](./usage.md#files-and-stdin). |

### Output

| Option | Short | Value | Default | Effect |
| --- | --- | --- | --- | --- |
| `--format` | `-f` | `FORMAT` | `text` | Output format: `text`, `oneline`, `json`, `jsonl`, `sarif`, `gcc`, `github`, `summary`, or a Go template that contains `{{ }}`. Any other value, and a template that does not parse, exits with status 2. See [the output formats](./output.md#format-error-messages). |
| `--oneline` | | | | Print one line per finding. Same as `--format oneline`. |
| `--rule-ids` | | | | Accepted for compatibility and without effect: the text formats always end a finding with its rule ID. |
| `--color` | | `[=WHEN]` | `auto` | Colorize the output. WHEN is `always`, `never` or `auto`, which colors only on a terminal. `yes` and `true` mean `always`, `no` and `false` mean `never`, and a bare `--color` means `always`. A non-empty `NO_COLOR` turns color off in every mode, `always` included. Only `text` and `oneline` use color. |
| `--no-color` | | | | Same as `--color=never`. |
| `--show-doc-urls` | | | | Print the documentation URL of the rule of each finding: as a `= docs:` line under the finding in the `text` format and inside the message in the `github` format. |
| `--finding-context` | | | | Add the events of the workflow and the permissions of the token of the job to each finding of the `json`, `jsonl` and `sarif` formats. See [the fields](./output.md#events-and-token-permissions-of-a-finding). |
| `--no-hints` | | | | Print no first-run hint and no note about a missing shellcheck, pyflakes or node, and leave the SARIF notification for it out. `JACTIONLINT_NO_HINTS` does the same. See [the first run](./usage.md#the-first-run). |
| `--min-severity` | | `LEVEL` | `info` | Hide the findings below LEVEL: `info`, `warn` or `error`. `warning` is accepted as `warn`. Hidden findings do not count for the exit status. |
| `--strict-exit` | | | | Exit with status 1 for a finding of any level, not only for `error`. |

### Rules and configuration

| Option | Short | Value | Default | Effect |
| --- | --- | --- | --- | --- |
| `--profile` | `-p` | `NAME` | `default` | Run the rules of a profile: `correctness`, `default` or `pedantic`. It overrides `profile` of the config file. The deprecated names `strict` and `all` work in the config file only: `--profile strict` exits with status 2. See [profiles](./config.md#profiles). |
| `--config-file` | `-c` | `FILE` | | Read this config file instead of the repository config file and the user-global config file. Its `extends` files still load. A file that is missing or invalid exits with status 3. |
| `--ignore` | `-i` | `PATTERN` | | Ignore the findings that match PATTERN: a rule ID, or a regular expression on the message. Repeat the option for more patterns. A comma belongs to the pattern. A pattern that is not a valid regular expression exits with status 2. See [ignore on the command line](./ignore.md#ignore-on-the-command-line). |
| `--init-config` | | | | Write a default config file `.github/jactionlint.yaml` at the root of the repository and exit. It works from any directory of the repository. The repository needs `.github/workflows`, `.github/actions` or a root `action.yml` (see [repository mode](./usage.md#repository-mode)), and the command writes into an existing `.github` directory, which a repository that has only a root `action.yml` may lack. It never overwrites a file. Every failure exits with status 3. See [the initial configuration](./config.md#generate-the-initial-configuration). |
| `--migrate-config` | | | | Rewrite the deprecated keys of the config file in place and exit. It reads the repository config file or `--config-file`, keeps the comments and changes nothing when no key is deprecated. See [deprecated keys](./config.md#deprecated-keys). |
| `--migrate-ignores` | | | | Rewrite `# zizmor: ignore[...]` comments into `# jactionlint ignore=...` comments and exit. It works on the files you name, or on the workflow files and the root `action.yml` or `action.yaml` of the repository. See [zizmor ignore comments](./ignore.md#zizmor-ignore-comments). |
| `--select` | | `RULES` | | Report only the rules you list, also those the profile does not enable. RULES is a comma-separated list of rule IDs; the option can be repeated. An unknown rule ID exits with status 2. See [run one rule](./usage.md#run-one-rule-or-run-without-the-ignores). |
| `--no-ignores` | | | | Apply no ignore comment, no `ignores` or `paths` ignore of the config file, no zizmor comment and no baseline, so that the run shows what they hide. `--ignore` still applies. See [option combinations](#option-combinations-that-fail). |
| `--list-rules` | | | | Print every rule with its group, profile, level and summary, then exit. With `--format json` or `--format jsonl` it prints JSON, and `--format oneline` prints the same table as `text`; any other format exits with status 2. See [list the rules](./usage.md#list-the-rules). |
| `--print-config-schema` | | | | Print the JSON Schema of the config file, with the rule IDs and rule options, then exit. See [the schema](./config.md#json-schema). |
| `--print-output-schema` | | | | Print the JSON Schema of the `json` and `jsonl` output, then exit. See [the output schema](./output.md#json-schema-of-the-output). |
| `--explain` | | `RULE` | | Print the documentation of a rule, then exit. It needs no network. An unknown rule ID exits with status 2. See [explain a rule](./usage.md#explain-a-rule). |

### Fixing

| Option | Short | Value | Default | Effect |
| --- | --- | --- | --- | --- |
| `--fix` | | `[=unsafe]` | | Apply the safe fixes in place. `--fix=unsafe` also applies the fixes that can change what a workflow does; `--fix=safe` is the same as a bare `--fix`. It refuses stdin (status 2). See [fixing](./fix.md#fix-errors-automatically). |
| `--diff` | | | | Print what `--fix` would change as a unified diff and write nothing. It implies `--fix`, so `--diff --fix=unsafe` previews the unsafe fixes. It exits with status 1 when the diff is not empty. |
| `--fix-rules` | | `RULES` | all fixes | Apply only the fixes of these rules, given as a comma-separated list of rule IDs. It needs `--fix` or `--diff`, and it overrides `fix.rules` of the config file. An unknown rule ID exits with status 2. |

### Baseline

| Option | Short | Value | Default | Effect |
| --- | --- | --- | --- | --- |
| `--baseline` | | `[=FILE]` | `.github/jactionlint-baseline.json` | Hide the findings that the baseline records. `--baseline=FILE` reads another file. A missing or invalid file exits with status 3. See [the baseline](./baseline.md#baseline). |
| `--no-baseline` | | | | Ignore the baseline, also when the `baseline` key of the config file enables it. |
| `--baseline-write` | | `[=FILE]` | `.github/jactionlint-baseline.json` | Record the current findings as the baseline and exit with status 0. `--baseline-write=FILE` writes another file. With file operands it records only those files and keeps the entries of the others: see [partly written baselines](./baseline.md#renamed-moved-and-partly-written-files). |
| `--baseline-check` | | | | Report the baseline entries that match no finding any more, as `unused-baseline-entry`. It implies `--baseline`. |
| `--sarif-hide-baselined` | | | | Leave the baselined findings out of `--format sarif` instead of marking them as suppressed. |

### GitHub API (online checks)

These options configure the checks that query the GitHub API. The checks run only when you turn them on, and nothing uses the network otherwise. [Online checks](./online.md#online-checks) explains the rules, the token lookup and the cache.

| Option | Short | Value | Default | Effect |
| --- | --- | --- | --- | --- |
| `--online` | | `[=MODE]` | off | Run the online checks. MODE is `cache` (answer from the cache and use no network), `strict` (exit with status 3 when a lookup is skipped) or `cache,strict`. A bare `--online` uses the network and the cache. `JACTIONLINT_ONLINE` sets the default. |
| `--no-online` | | | | Use no network, also when the `online` key or `JACTIONLINT_ONLINE` turns the online checks on. |
| `--online-api-url` | | `URL` | `$GITHUB_API_URL`, else derived from `$GITHUB_SERVER_URL` or `$GH_HOST`, else `https://api.github.com` | The REST API URL of a GitHub Enterprise Server, for example `https://ghe.example.com/api/v3`. It must be an http or https URL without credentials, or jactionlint exits with status 2. |
| `--online-token-env` | | `NAME` | | The name of the environment variable that holds the token. jactionlint reads it before `GITHUB_TOKEN` and `GH_TOKEN`. |
| `--online-token-file` | | `FILE` | | A file that holds the token. jactionlint reads it after `--online-token-env` and before the standard variables. |
| `--online-github-com-fallback` | | | off | On an Enterprise Server, look up on api.github.com the repositories that the Server does not have, which is where actions synced with GitHub Connect come from. The names of these repositories are sent to github.com. |
| `--online-github-com-token-env` | | `NAME` | | With the fallback: the name of the environment variable that holds the token for api.github.com. jactionlint reads this variable first, then the file of `--online-github-com-token-file`, then `GITHUB_COM_TOKEN`. |
| `--online-github-com-token-file` | | `FILE` | | With the fallback: a file that holds the token for api.github.com. |
| `--online-allow` | | `PATTERN` | | Look up only the repositories that match an `owner/repo` pattern. `*` matches within one part and case does not matter. Repeat the option or separate the patterns with commas. A pattern that has another shape exits with status 2. |
| `--online-deny` | | `PATTERN` | | Never look up the repositories that match an `owner/repo` pattern. It wins over `--online-allow`. Repeat the option or separate the patterns with commas. A pattern that has another shape exits with status 2. |
| `--online-cache-ttl` | | `DURATION` | `1h` | How long a cached answer is used without asking GitHub whether it changed. `0` revalidates every answer. A duration is a Go duration such as `30m` or `1h`. |
| `--online-report` | | `FILE` | | Write how complete the online checks were (checked and excluded repositories, skipped lookups, cache age) to FILE as JSON, whatever `--format` says. The file also says whether the online checks ran. |
| `--online-max-wait` | | `DURATION` | `30s` | The longest wait for a GitHub rate limit to reset. `0` never waits. A limit that resets later skips the lookups. |

### External tools

| Option | Short | Value | Default | Effect |
| --- | --- | --- | --- | --- |
| `--shellcheck` | | `CMD` | `shellcheck` | The command name or path of shellcheck. An empty value turns the integration off. A command you name must exist, or jactionlint exits with status 3. |
| `--pyflakes` | | `CMD` | `pyflakes` | The command name or path of pyflakes. An empty value turns the integration off. A command you name must exist, or jactionlint exits with status 3. |
| `--ruff` | | `CMD` | off | Check the Python scripts with ruff, using only its pyflakes rules, instead of pyflakes. `--pyflakes` is then ignored. The command must exist, or jactionlint exits with status 3. |
| `--shellcheck-rcfile` | | `FILE` | | Let shellcheck read this shellcheckrc instead of passing `--norc`. A relative path starts at the working directory. It overrides `shellcheck.rcfile` of the config file. A missing file exits with status 3. |

[External tools](./usage.md#external-tools) explains what each program checks and what a missing program costs.

## Option combinations that fail

These combinations exit with status 2 and one message. Where a message is quoted, it is the text that jactionlint prints.

| Combination | Message |
| --- | --- |
| `--baseline` with `--no-baseline` | `--baseline and --no-baseline cannot be combined` |
| `--baseline-write` with `--baseline`, `--no-baseline` or `--baseline-check` | `--baseline-write cannot be combined with --baseline or --baseline-check: it records every finding` |
| `--baseline-write` with `--fix` or `--diff` | `--baseline-write cannot be combined with --fix` |
| `--no-ignores` with `--baseline`, `--baseline-check` or `--baseline-write` | `--no-ignores cannot be combined with --baseline, --baseline-check or --baseline-write: it turns the baseline off` |
| `--online` with `--no-online` | `--online and --no-online cannot be combined` |
| `--print-config-schema` with `--print-output-schema` | `--print-config-schema and --print-output-schema cannot be combined` |
| `--fix-rules` without `--fix` or `--diff` | `--fix-rules can be used only with --fix or --diff` |
| `--fix` or `--diff` with the operand `-` | `--fix cannot be used with stdin because the fixed file would not be saved` |
| `--baseline-write` with the operand `-` | `--baseline-write cannot be used with stdin because the baseline would not know the file` |
| `--select` with no rule ID, as in `--select ''` | `--select lists no rule ID` |
| `--list-rules` with a format other than `text`, `oneline`, `json` or `jsonl` | `--list-rules supports --format text, json and jsonl, not "xml"` |

`--migrate-ignores -` is not on this list: it exits with status 3, because the option works on files and reads `-` as a file name.

## Environment variables

### Variables that jactionlint reads

| Variable | Effect | Accepted values |
| --- | --- | --- |
| `JACTIONLINT_ONLINE` | The default of `--online` and `--no-online`. The command line wins. | Empty: not set. `0`, `false`, `off` or `no`: no online checks. `1`, `true` or `on`: the plain online mode. `cache`, `strict` or `cache,strict`, in any order and separated by a comma, `+` or a space: that mode. Case does not matter. Anything else exits with status 2. |
| `JACTIONLINT_NO_HINTS` | Same as `--no-hints`. | Any value except empty, `0` and `false`. |
| `CI` | Lets the first-run hint appear when stderr is not a terminal. | Any value except empty, `false` and `0`. |
| `GITHUB_ACTIONS` | Same as `CI` for the first-run hint. | Any value except empty and `false`. |
| `XDG_CONFIG_HOME` | The directory of the user-global config file, in a `jactionlint` subdirectory. See [where the config file is read](./config.md#where-jactionlint-reads-the-config-file). | An absolute path. jactionlint ignores a relative path and uses `$HOME/.config`. |
| `XDG_CACHE_HOME` | The directory of the cache of the online checks, in a `jactionlint` subdirectory. | An absolute path. jactionlint ignores a relative path and uses `$HOME/.cache`. |
| `GITHUB_TOKEN`, `GH_TOKEN` | The token of the online checks, read after `--online-token-env` and `--online-token-file`. | A token without white space. |
| `GITHUB_ENTERPRISE_TOKEN`, `GH_ENTERPRISE_TOKEN` | On an Enterprise Server host, the token variables that jactionlint reads before `GITHUB_TOKEN` and `GH_TOKEN`. | A token without white space. |
| `GITHUB_COM_TOKEN` | The token for api.github.com with `--online-github-com-fallback`, read after the variable and the file that the options name. `GITHUB_TOKEN` and `GH_TOKEN` are never sent there. | A token without white space. |
| `GITHUB_API_URL`, `GITHUB_SERVER_URL`, `GH_HOST` | The API URL when no option or key gives one, tried in this order. GitHub Actions sets the first two. | A URL for the first two, a host name for `GH_HOST`. |
| `GITHUB_GRAPHQL_URL` | The GraphQL endpoint of the branch scan of `impostor-commit`. jactionlint ignores it, with a notice, unless it has the scheme and host of the REST API. | A URL. |
| `GITHUB_REPOSITORY` | The repository for the environment lookups of the online checks. Without it jactionlint reads the `origin` remote. | `owner/repo`. |

[Find the token](./online.md#find-the-token) gives the order of the token sources, and [the cache](./online.md#the-cache) explains the cache directory.

### Variables that libraries and other programs read

jactionlint does not read these itself, but they change what a run does.

| Variable | Effect |
| --- | --- |
| `NO_COLOR` | A non-empty value turns color off in every mode, also with `--color=always`. An empty value is ignored. |
| `TERM` | `TERM=dumb` turns color off in the `auto` mode. |
| `HOME` (`USERPROFILE` on Windows) | The base of `.config` and `.cache` when `XDG_CONFIG_HOME` or `XDG_CACHE_HOME` is not an absolute path. |
| `PATH` | Where jactionlint looks for shellcheck, pyflakes, ruff, node, git and gh. |
| `SHELLCHECK_OPTS` | Read by shellcheck itself. It still works although jactionlint passes `--norc`. |
| `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE` and the other `GIT_` variables that redirect git, `GIT_CONFIG*` included | jactionlint removes them from the environment of the `git` commands it runs, so that a Git hook cannot point those commands at another repository. |

<a id="files"></a>

## Files jactionlint reads and writes

### Files it reads

| File | When |
| --- | --- |
| The workflow files `.github/workflows/*.yml` and `*.yaml`, only those directly in the directory | A run without operands, or when you name them. |
| `.github/dependabot.yml` or `.github/dependabot.yaml` | A run without operands, or when you name it. |
| `action.yml` and `action.yaml` of composite actions | A run without operands finds them in the repository and follows local `uses:` references; or when you name them. |
| `.github/workflows/actions.lock` | A run without operands, when it exists, or when you name it. See [the lock file check](./checks.md#check-actions-lock). |
| The config file | Every run. `--config-file` names it. Without that option, jactionlint reads the repository config file or the user-global config file, and the files that `extends` lists. [Where the config file is read](./config.md#where-jactionlint-reads-the-config-file) has the names and the order. |
| The baseline file, `.github/jactionlint-baseline.json` or the file you name | With `--baseline`, `--baseline-check` and `baseline:` in the config file. |
| The shellcheckrc | With `--shellcheck-rcfile` or `shellcheck.rcfile`. |
| The token files | With `--online-token-file`, `--online-github-com-token-file` or their keys. |
| The cache directory, `$XDG_CACHE_HOME/jactionlint` or `~/.cache/jactionlint` | With the online checks. It holds answers of GitHub and has a size limit: see [the cache](./online.md#the-cache). |
| Files of the repository that some checks read, such as `.gitmodules`, `.git/config`, `package.json` and lock files | When a check needs them. The section of each check in [the checks page](./checks.md) names its files. |

### Files it writes

| File | Written by | How |
| --- | --- | --- |
| The files that it fixes | `--fix` | jactionlint writes a temporary file `.jactionlint-fix-*` in the same directory and renames it over the original. The directory of each file must be writable. `--diff` writes nothing. |
| The baseline file | `--baseline-write` | The same way. The directory must be writable. |
| `.github/jactionlint.yaml` | `--init-config` | It creates the file and never overwrites one. |
| The config file | `--migrate-config` | It rewrites the file in place. |
| The workflow files and the root `action.yml` or `action.yaml` | `--migrate-ignores` | It rewrites the files that hold zizmor comments, the same way as `--fix`. |
| The report file | `--online-report` | It writes JSON, also when the online checks did not run. |
| The cache directory | The online checks | It stores answers of GitHub. |

## Programs and network

### Programs

jactionlint starts these programs when it needs them. A missing program never stops a run unless you named the command yourself.

| Program | Used for | When it is missing |
| --- | --- | --- |
| shellcheck | The `shellcheck` rule, for `run:` scripts | A note on stderr and no check of the scripts. A command named with `--shellcheck` that does not exist exits with status 3. |
| pyflakes | The `pyflakes` rule, for `shell: python` scripts | The same as shellcheck, with `--pyflakes`. |
| ruff | The `pyflakes` rule instead of pyflakes, with `--ruff` | Exit status 3. |
| node 20 or later | The `github-script-syntax` rule, for the scripts of `actions/github-script` | A note on stderr and no check of the scripts. Turn the rule off to silence the note. |
| git | The `unmatched-paths-filter` rule, which lists the files of the repository with `git ls-files` | The rule walks the directory tree instead. |
| gh | Finding a token for the online checks: `gh auth token --hostname HOST` | jactionlint skips it. `gh-cli: false` in `online-options` turns it off. |

[External tools](./usage.md#external-tools) has the note texts and the setup of each tool.

### Network

Without the online checks, jactionlint contacts no host. With them it contacts only these hosts, which matters for a runner behind an egress allow list.

| Host | Used for | When |
| --- | --- | --- |
| `api.github.com`, or the host of `--online-api-url` | The REST and GraphQL requests of the online rules | `--online` and `--online=strict`. `--online=cache` uses no network. |
| `api.github.com` | The lookups of repositories that a Server does not have | `--online-github-com-fallback` on an Enterprise Server. |
| `auth.docker.io` and `registry-1.docker.io` | The digest of a Docker Hub image, for the unsafe fix of the findings of `unpinned-images` and of `unpinned-uses` on a `docker://` reference | `--online` and `--online=strict`, also without `--fix`. `--online=cache` uses no network. |
| `ghcr.io` | The digest of an image of ghcr.io, for the same fix | `--online` and `--online=strict`, also without `--fix`. `--online=cache` uses no network. |

[Hosts and data](./online.md#hosts-and-data) says what each request sends.

## Exit status

jactionlint exits with status 0, 1, 2 or 3. This table is the complete list of causes.

| Status | Meaning | Cause |
| --- | --- | --- |
| `0` | Success | The run finished and left no finding that counts: findings of level `warn` and `info` do not count, and neither do findings that an ignore, a baseline or `--min-severity` hides. |
| `0` | Success | `--help`, `--version`, `--list-rules`, `--explain`, `--completion`, `--print-config-schema` and `--print-output-schema` finished. |
| `0` | Success | `--init-config`, `--migrate-config`, `--migrate-ignores` and `--baseline-write` finished. |
| `0` | Success | `--fix` fixed every finding that counts. |
| `1` | Findings left | At least one finding of level `error` is left after ignores, baseline and `--min-severity`. |
| `1` | Findings left | `--strict-exit` is set and a finding of any level is left. |
| `1` | Findings left | `--diff` prints a diff that is not empty, also when only findings of level `warn` or `info` would change. |
| `1` | Findings left | `--fix` leaves a finding that counts. |
| `2` | Invalid command line | An unknown option, a missing value, or a value that cannot be parsed or is not allowed: `--profile`, `--format`, `--min-severity`, `--color`, `--fix`, `--baseline`, `--online`, `--online-api-url`, `--online-allow`, `--online-deny`, durations, `--completion` and `JACTIONLINT_ONLINE`. A `--format` template that does not parse is one of them. |
| `2` | Invalid command line | A rule ID that does not exist in `--select`, `--fix-rules` or `--explain`, or a `--list-rules` format that is not supported. |
| `2` | Invalid command line | An `--ignore` pattern that is not a valid regular expression. |
| `2` | Invalid command line | One of the [option combinations that fail](#option-combinations-that-fail), or `--fix`, `--diff` or `--baseline-write` with stdin. |
| `3` | Failure | No repository is found: no `.git` or `.jj` with `.github/workflows`, `.github/actions` or a root `action.yml` in any parent directory. |
| `3` | Failure | A file operand cannot be read, or is a directory. |
| `3` | Failure | A config file cannot be read or is invalid: an unknown key, a bad value, an unknown `platform`, a missing file given with `--config-file`, or a missing file that the file of `--config-file` lists in `extends`. |
| `3` | Failure | `--init-config` finds a config file already there or has no `.github` directory to write into, `--init-config` or `--migrate-config` finds no repository, or `--migrate-config` finds no config file. |
| `3` | Failure | The baseline file that `--baseline` or `baseline:` names does not exist, cannot be parsed or has a version that this jactionlint does not read. |
| `3` | Failure | A command named with `--shellcheck`, `--pyflakes` or `--ruff` does not exist, or the shellcheckrc of `--shellcheck-rcfile` or `shellcheck.rcfile` does not exist. |
| `3` | Failure | `--fix` or `--baseline-write` cannot write a file, or `--fix` refuses a fix that would damage the file or does not converge. |
| `3` | Failure | A `--format` template fails while it runs, for example because it names a field that does not exist. |
| `3` | Failure | `--online=strict` skipped a lookup. This wins over status 1. |

The level of a finding decides whether it counts. The rules of the `correctness` and `default` profiles have the level `error` by default, except `unused-baseline-entry`, which has the level `info`; many rules of `pedantic` and many online rules have the level `warn` or `info`. [The rules page](./rules.md) lists the default level of each rule, and `rules:` in [the config file](./config.md#rules) changes it. `--min-severity warn` hides the findings below `warn`, and `--strict-exit` makes every finding count.

```sh
jactionlint --strict-exit          # findings of every level fail
jactionlint --min-severity error   # show only findings of level error
```

## Flag, environment variable and configuration key

When the same setting has several sources, the first of these wins:

- **Profile:** `--profile`, then `profile` of the config file (applied after `extends`), then `default`.
- **Online checks:** `--online` or `--no-online`, then `JACTIONLINT_ONLINE`, then `online` and `online-options.mode`, then off.
- **Other online options:** the option, then the key of `online-options`, then the built-in default. For the API URL, the order is the option, the key, the variables `GITHUB_API_URL`, `GITHUB_SERVER_URL` and `GH_HOST`, then `https://api.github.com`. A list option replaces the list of the key; it does not extend it.
- **Baseline:** `--baseline`, `--baseline-write` or `--no-baseline`, then the `baseline` key.
- **Fix rules:** `--fix-rules`, then `fix.rules`.
- **shellcheckrc:** `--shellcheck-rcfile`, then `shellcheck.rcfile`.
- **Config file:** `--config-file`, then the repository config file, then the user-global config file. jactionlint never merges them, except through `extends`.

Most names agree. The names of the keys that differ are `online-options.max-rate-limit-wait` for `--online-max-wait` and `online-options.mode` for the mode of `--online`.

| Option | Config key | Environment variable |
| --- | --- | --- |
| `--profile` | `profile` | |
| `--config-file` | None: it selects the file | |
| `--fix-rules` | `fix.rules` | |
| `--baseline` | `baseline`: `auto` or a path | |
| `--no-baseline` | `baseline: false` | |
| `--online` | `online`, `online-options.mode` | `JACTIONLINT_ONLINE` |
| `--no-online` | `online: false` | `JACTIONLINT_ONLINE=0` |
| `--online-api-url` | `online-options.api-url` | `GITHUB_API_URL`, `GITHUB_SERVER_URL`, `GH_HOST` |
| `--online-token-env` | `online-options.token-env` | |
| `--online-token-file` | `online-options.token-file` | |
| `--online-github-com-fallback` | `online-options.github-com-fallback` | |
| `--online-github-com-token-env` | `online-options.github-com-token-env` | `GITHUB_COM_TOKEN`, read after the variable that the option names |
| `--online-github-com-token-file` | `online-options.github-com-token-file` | |
| `--online-allow` | `online-options.allow` | |
| `--online-deny` | `online-options.deny` | |
| `--online-cache-ttl` | `online-options.cache-ttl` | |
| `--online-max-wait` | `online-options.max-rate-limit-wait` | |
| `--shellcheck-rcfile` | `shellcheck.rcfile` | |
| `--no-hints` | | `JACTIONLINT_NO_HINTS` |
| `--color`, `--no-color` | | `NO_COLOR` |

The keys `online-options.retries`, `online-options.concurrency` and `online-options.gh-cli` have no option. The other options have no key: `--ignore` has no key that means the same, because `paths.<glob>.ignore` ignores patterns in the files that match a glob and `ignores` records [durable ignores](./config.md#durable-ignores). [The online keys](./config.md#online-options) are listed with their defaults in the configuration page.

## Next steps

- [Run jactionlint](./usage.md) for the first run, the profile choice and the external tools.
- [The configuration file](./config.md) for the keys that match these options.
- [Online checks](./online.md#online-checks) for the options of the GitHub API group.
- [Output formats](./output.md#format-error-messages) for `--format` and the fields of each format.
- [Ignore findings](./ignore.md#ignore-some-errors) for `--ignore`, ignore comments and `--no-ignores`.
