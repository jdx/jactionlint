Usage
=====

This document describes how to use [jactionlint](https://github.com/jdx/jactionlint).

## `jactionlint` command

With no argument, jactionlint finds all workflow files in the current repository and checks them. It checks the Dependabot
configuration `.github/dependabot.yml` (or `.github/dependabot.yaml`) of the repository as well.

```sh
jactionlint
```

When paths to YAML workflow files are given as arguments, jactionlint checks them.

```sh
jactionlint path/to/workflow1.yaml path/to/workflow2.yaml
```

When `-` argument is given, jactionlint reads inputs from stdin and checks it as workflow source.

```sh
cat path/to/workflow.yaml | jactionlint -
```

The Dependabot configuration is recognized by its path: a file named `dependabot.yml` or `dependabot.yaml` in a `.github`
directory. Give it as an argument or use `-stdin-filename` to check it. Workflow rules are not applied to it. See
[the check of its syntax](checks.md#check-dependabot-syntax).

```sh
jactionlint .github/dependabot.yml
cat dependabot.yml | jactionlint -stdin-filename .github/dependabot.yml -
```

To know all flags and options, see an output of `jactionlint -h` or [the online command manual][cmd-manual].

### Ignore some errors

Every error has a stable [rule ID](rules.md) such as `unpinned-uses`. `-rule-ids` shows it at the end of each error instead of the
kind, and the `id` field of `-format json` and the `ruleId` of `-format sarif` have it.

```
.github/workflows/ci.yaml:12:15: action "actions/checkout@v4" must be pinned to a full-length commit SHA ... [unpinned-uses]
```

To ignore some errors, `-ignore` option filters errors by rule IDs or by messages using regular expression. A pattern which is
exactly a rule ID ignores all the errors of the rule. Any other pattern is a regular expression matched to the error messages. The
regular expression syntax is the same as [RE2][re2]. The option is repeatable.

```sh
jactionlint -ignore template-injection -ignore 'label ".+" is unknown'
```

The same patterns are available in [the configuration file](config.md) (`paths.<glob>.ignore`) and in ignore comments.
An ignore comment is a comment on its own line. It applies to the next line which is not a comment nor blank, and to the lines
nested under it. Several patterns are separated with commas.

```yaml
steps:
  # jactionlint ignore=template-injection,label ".+" is unknown
  - run: echo '${{ github.event.pull_request.title }}'
```

A comment which suppresses nothing is reported by the [`unused-ignore`](rules.md#unused-ignore) rule of the `strict` profile.

`-shellcheck` and `-pyflakes` specifies file paths of executables. Setting empty string to them disables `shellcheck` and
`pyflakes` rules. As a bonus, disabling them makes jactionlint much faster Since these external linter integrations spawn many
processes.

```sh
jactionlint -shellcheck= -pyflakes=
```

<a id="format"></a>
### Format error messages

`-format` option selects the output format. The available formats are as follows.

| Format    | Description                                                                                                                                                              |
|-----------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `text`    | The default. File path, position, message and kind followed by the source snippet and an indicator. `-oneline` or `oneline` omits the snippet.                           |
| `oneline` | The same as `text` with one line per error.                                                                                                                              |
| `json`    | A JSON array of error objects. It is the same as `-format '{{json .}}'`.                                                                                                 |
| `jsonl`   | One error object per line ([JSON Lines][jsonl]).                                                                                                                         |
| `sarif`   | A [SARIF][sarif] 2.1.0 log with the rule metadata, levels, regions and fixes. Useful for code scanning and for tools like [hk](#hk).                                     |
| `gcc`     | `file:line:col: severity: message [id]` like GCC. `severity` is `error`, `warning` or `note`.                                                                            |
| `github`  | [Workflow commands][ga-annotate-error] (`::error file=...,line=...,col=...,title=<id>::message`) which GitHub shows as annotations. Use `warning` and `notice` for lower levels. |

Any other value which has `{{ }}` is a custom template in [Go template syntax][go-template] as explained below. The output of
the structured formats goes to stdout and the logs (including the deprecation warnings of the configuration) go to stderr, so the
output can be piped to other tools safely.

The text format of errors is the same as in the former versions. An error whose level is lowered to `warn` or `info` in
[the configuration](config.md#rules) has `warning: ` or `info: ` before the message. `-rule-ids` shows the rule ID instead of
the kind at the end of the line.

An error object of `json` and `jsonl` has these fields.

| Field          | Description                                                                                                       |
|----------------|-------------------------------------------------------------------------------------------------------------------|
| `message`      | Body of the error message                                                                                         |
| `filepath`     | Canonical relative file path. It may be omitted when the input is stdin                                           |
| `line`         | Line number of the start of the error (1-based)                                                                   |
| `column`       | Column number of the start of the error (1-based, counted in Unicode code points)                                 |
| `end_line`     | Line number of the end of the region of the error                                                                 |
| `end_column`   | Column of the last character of the indicator (legacy). SARIF has the exclusive end column of the region          |
| `kind`         | The legacy group of the error such as `expression`                                                                |
| `id`           | The stable [rule ID](rules.md) such as `template-injection`                                                       |
| `severity`     | `error`, `warn` or `info`                                                                                         |
| `doc_url`      | The URL of the documentation of the rule. It is omitted for custom rules                                          |
| `snippet`      | Code snippet to indicate the position of the error                                                                |
| `fix`          | The automatic fix of the error: its `description` and the byte-range `edits`. It is omitted when there is no fix  |

Before explaining the template details, let's see some examples.

#### Example: Serialized into JSON

```sh
jactionlint -format '{{json .}}'
```

This is the same as `jactionlint -format json`.

Output:

```
[{"message":"unexpected key \"branch\" for ...
```

#### Example: Markdown

````sh
jactionlint -format '{{range $err := .}}### Error at line {{$err.Line}}, col {{$err.Column}} of `{{$err.Filepath}}`\n\n{{$err.Message}}\n\n```\n{{$err.Snippet}}\n```\n\n{{end}}'
````

Output:

````markdown
### Error at line 21, col 20 of `test.yaml`

property "platform" is not defined in object type {os: string}

```
          key: ${{ matrix.platform }}-node-${{ hashFiles('**/package-lock.json') }}
                   ^~~~~~~~~~~~~~~
```
````

#### Example: Serialized in [JSON Lines][jsonl]

```sh
jactionlint -format '{{range $err := .}}{{json $err}}{{end}}'
```

Output:

```
{"message":"unexpected key \"branch\" for ...
{"message":"character '\\' is invalid for branch ...
{"message":"label \"linux-latest\" is unknown. ...
```

#### Example: [Error annotation][ga-annotate-error] on GitHub Actions

````sh
jactionlint -format '{{range $err := .}}::error file={{$err.Filepath}},line={{$err.Line}},col={{$err.Column}}::{{$err.Message}}%0A```%0A{{replace $err.Snippet "\\n" "%0A"}}%0A```\n{{end}}' -ignore 'SC2016:'
````

Output:

<img src="https://github.com/rhysd/ss/blob/master/actionlint/ga-annotate.png?raw=true" alt="annotations on GitHub Actions" width="731" height="522"/>

To include newlines in the annotation body, it prints `%0A`. (ref [actions/toolkit#193](https://github.com/actions/toolkit/issues/193)).
And it suppresses `SC2016` shellcheck rule error since it complains about the template argument.

Basically it is more recommended to use [Problem Matchers](#problem-matchers) or reviewdog as explained in
['Tools integration' section](#tools-integ) below.

#### Example: [SARIF format][sarif]

[The Static Analysis Results Interchange Format (SARIF)][sarif] is a standardized format for the results of static analysis tools.
`jactionlint -format sarif` prints a SARIF 2.1.0 log. Each result has the rule ID (`ruleId`), the level (`error`, `warning` or
`note`), the region of the error (`startLine`, `startColumn`, `endLine` and `endColumn`; `columnKind` is
`unicodeCodePoints`) and, for the rules which have an automatic fix, `fixes` with `artifactChanges[].replacements[]`
(`deletedRegion` and `insertedContent.text`). The `rules` of the driver describe the rules which appear in the results with
their summary, group and a link to the documentation.

A custom template still works. This is [the template file in test data](https://github.com/jdx/jactionlint/blob/main/testdata/format/sarif_template.txt)
which was used before `-format sarif`. [The output example in test data](https://github.com/jdx/jactionlint/blob/main/testdata/format/test.sarif)
is its output.

#### Formatting syntax

In [Go template syntax][go-template], `.` within `{{ }}` means the target object. Here, the target object is a sequence of error
objects.

The sequence can be traversed with `range` action, which is like `for ... = range ... {}` in Go.

```
{{range $err := .}} this part iterates error objects with the iteration variable $err {{end}}
```

The error object has the following fields.

| Field                | Description                                           | Example                                                          |
|----------------------|-------------------------------------------------------|------------------------------------------------------------------|
| `{{$err.Message}}`   | Body of error message                                 | `property "platform" is not defined in object type {os: string}` |
| `{{$err.Snippet}}`   | Code snippet to indicate error position               | `          node_version: 16.x\n          ^~~~~~~~~~~~~`          |
| `{{$err.Kind}}`      | Name of rule the error belongs to (legacy)            | `expression`                                                     |
| `{{$err.ID}}`        | Stable ID of the rule such as `unpinned-uses`         | `template-injection`                                             |
| `{{$err.Severity}}`  | Level of the error: `error`, `warn` or `info`         | `error`                                                          |
| `{{$err.DocURL}}`    | URL of the documentation of the rule                  | `https://jactionlint.jdx.dev/rules#template-injection`           |
| `{{$err.Filepath}}`  | Canonical relative file path of the error position    | `.github/workflows/ci.yaml`                                      |
| `{{$err.Line}}`      | Line number of the error position (1-based)           | `9`                                                              |
| `{{$err.Column}}`    | Column number of the error's start position (1-based) | `11`                                                             |
| `{{$err.EndLine}}`   | Line number of the error's end position (1-based)     | `9`                                                              |
| `{{$err.EndColumn}}` | Column number of the error's end position (1-based)   | `23`                                                             |

Functions called in `{{ }}` placeholder are template actions. There are many actions defined by Go standard library. In addition,
there are a few custom actions defined by jactionlint. Most useful action would be `json` as we already used it in the above JSON
example. List of all custom actions are as follows:

| Action           | Description                                                                      | Example usage                             |
|------------------|----------------------------------------------------------------------------------|-------------------------------------------|
| `json x`         | Serialize `x` as JSON string followed by newline character                       | `{{json $err}}`                           |
| `replace x y z`  | Replace string `y` with `z` in `x`                                               | `{{replace $err.Filepath "\\" "/"}}`      |
| `toPascalCase x` | Convert `x` into PascalCase (e.g. 'foo-bar' to 'FooBar')                         | `{{toPascalCase $err.Kind}}`              |
| `allRules`       | Return an array of rule objects (all rules with their IDs). Explained below      | `{{range $ = allRules}}{{$.ID}}{{end}}`   |
| `allKinds`       | Return an array of kind objects. The kind object is explained in the below table | `{{range $ = allKinds}}{{$.Name}}{{end}}` |
| `getVersion`     | Return the version of jactionlint as string                                       | `{{getVersion}}`                          |

The kind object returned from `allKinds` action has the following fields.

| Field                   | Description                   | Example                                     |
|-------------------------|-------------------------------|---------------------------------------------|
| `{{$kind.Name}}`        | Name of the kind              | `syntax-check`                              |
| `{{$kind.Description}}` | Short description of the kind | `Checks for GitHub Actions workflow syntax` |

The rule object returned from `allRules` action has the following fields: `ID`, `Name` (the ID in PascalCase),
`Description`, `Group` (`correctness`, `security`, `policy` or `style`), `DefaultLevel`, `Profile` and `URL`.

For example, the following simple iteration body

```
line is {{$err.Line}}, col is {{$err.Column}}, message is {{$err.Message | printf "%q"}}
```

will produce output like below.

```
line is 21, col is 20, message is "property \"platform\" is not defined in object type {os: string}"
```

In `{{ }}` placeholder, input can be piped and action can be used to transform texts. In above example, the message is piped with
`|` and transformed with `printf "%q"`.

Note that special characters escaped with backslash like `\n` in the format string are automatically unescaped.

### Exit status

`jactionlint` command exits with one of the following exit statuses.

| Status | Description                                                                                  |
|--------|----------------------------------------------------------------------------------------------|
| `0`    | The command ran successfully and no problem was found                                        |
| `1`    | The command ran successfully and some problem was found                                      |
| `2`    | The command failed due to invalid command line option                                        |
| `3`    | The command failed due to some fatal error                                                   |

Only the errors whose level is `error` count as problems. Every rule is an `error` unless [the configuration](config.md#rules)
lowers it, so the exit status is `1` whenever something is reported by default. The findings of `warn` and `info` level are
printed but the exit status stays `0` unless `-strict-exit` is given. `-min-severity warn` (or `error`) hides the findings
below the level.

```sh
jactionlint -strict-exit                # warnings and infos fail, too
jactionlint -min-severity error         # show only errors
```

### Fix errors automatically

`-fix` applies the automatic fixes of the errors which have one, rewrites the files and reports the errors which remain.
It repeats until nothing changes so running it twice makes no further change, and it exits with `1` when an error remains.
Only the fixes which do not change the behavior of the workflow are applied; `-fix=unsafe` applies all of them.

```sh
jactionlint -fix
jactionlint -fix .github/workflows/ci.yaml
jactionlint -fix -rules missing-timeout,artipacked   # only the fixes of these rules
jactionlint -diff                                    # show the changes as a unified diff, write nothing
```

When it is done `-fix` says what it changed, per rule, on stderr:

```
Fixed 12 problem(s) in 3 file(s)
  missing-timeout: 8
  artipacked: 4
```

- **`-rules <id>[,<id>...]`** restricts `-fix` (and `-diff`) to the fixes of the listed rules; the other findings are still
  reported. The same list can be set with [`fix.rules`](config.md#configuration-file) in the configuration file. An unknown ID is
  an error.
- **`-diff`** computes the fixes like `-fix` (safe ones, or `-diff -fix=unsafe`) but writes nothing. The unified diff is on
  stdout, so `jactionlint -diff | patch -p1` applies it, and the remaining errors are on stderr. The exit status is `1` when there is a
  diff or an error remains.
- **Several fixes in one run.** Fixing happens in memory and the file is written once, atomically (a temporary file in the same
  directory is renamed over it), with its permission bits, its line breaks (CRLF stays CRLF) and a symbolic link kept. A file that
  changed on disk while it was being fixed is not overwritten.

#### How fixing converges and what it checks

Each pass lints the text, chooses fixes that do not overlap, applies them and lints the result again, until no fix is left.

- **Overlapping fixes.** When the edits of two fixes overlap, exactly one is applied in the pass and the other is dropped for the
  pass; its finding is reported again in the next pass against the new text, or is gone. The winner does not depend on the order of
  the findings: safe fixes come before unsafe ones, then rules in this order (`template-injection`,
  `insecure-commands`, `artipacked`, `bot-conditions`, `unpinned-uses`, `self-repository`, `obfuscation`, `missing-permissions`,
  `missing-timeout`, `anonymous-definition`, any other rule, `unused-ignore`), then by position. Fixes that remove a security
  problem from the code come first, and the removal of an unused ignore comment last.
- **Every pass is checked.** The result of a pass must be valid YAML, and the parsed document must equal the document before it
  except where the edits are: a key or item that appears, disappears or changes away from every edit, or a value that changes its type
  (`'true'` becoming `true`), makes the pass fail. Comments, the order of keys and the quoting style do not count. The fixes are then
  tried one by one: the one that breaks the file is **refused**, the rest are applied, and the run exits with status `3` after printing
  which rule it was (`error: .github/workflows/ci.yaml: the fix at line 12 would damage the file: ... (rules: template-injection)`).
  That rule is not fixed again in that file.
  This is a guard against bugs in fixers, not a proof: it cannot tell that an edit put a wrong value in the right place, a difference next
  to an edit is accepted, anchors are compared only by the aliases that use them, and line breaks other than LF, CRLF and CR make the
  positions approximate.
- **No endless loops.** If the text of a pass comes back (two rules undo each other) or fixes remain after 10 passes, the file is not
  written with them: it stays as it was before the fixes that kept repeating, and the run exits with status `3` naming the rules.
- **Text is escaped for where it goes.** A fix that inserts text into a scalar escapes it for how the scalar is written: a backslash or
  quote in a double quoted scalar, a quote in a single quoted one, indentation in a block scalar, and an environment variable
  value that is not a plain-safe string is quoted. A fix that cannot represent its text in the place offers no fix.

Fixes arrive with the rules that can fix their findings mechanically. The errors of the rules without a fix are only reported.
`-fix` cannot be used with stdin. These rules have a fix today:

| Rule                  | What `-fix` does                                                                                                             | Safe                                                              |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| `missing-timeout`     | Adds `timeout-minutes: N` to the job after `runs-on:`, only when [`default-minutes`](config.md#rules) sets `N`.            | Yes                                                               |
| `missing-permissions` | Adds `permissions:` with `contents: read` to the workflow after the `on:` block.                                             | Only when nothing shows that a job needs the token, else `unsafe` |
| `unused-ignore`       | Removes the ignore comment, or only its patterns which did nothing when the comment has others.                              | Yes                                                               |

A finding in a shape the fix does not understand (a job written as `job: {runs-on: ...}`, a job with a YAML anchor, a file with
a bare carriage return) is reported without a fix. See [the checks document](checks.md#check-timeout-minutes) for the details.

<a id="online-checks"></a>
### Online checks

Six checks need to ask GitHub about the actions a workflow uses, so they are off unless you give `-online` (or set `online: true` in
[the configuration](config.md#configuration-file)). **Without it jactionlint never uses the network.**

| Rule | What it finds |
| --- | --- |
| [`impostor-commit`](checks.md#check-impostor-commit) | A hash-pinned action whose commit is on no branch or tag of the repository: it comes from a fork |
| [`known-vulnerable-actions`](checks.md#check-known-vulnerable-actions) | An action version covered by a GitHub security advisory |
| [`ref-confusion`](checks.md#check-ref-confusion) | A ref which is both a branch and a tag |
| [`stale-action-refs`](checks.md#check-stale-action-refs) | A pinned commit that no tag points to |
| [`archived-uses`](checks.md#check-archived-uses) | An action in an archived repository |
| [`ref-version-mismatch`](checks.md#check-ref-version-mismatch) | A `# v1.2.3` comment that does not match the pinned commit |

```sh
export GITHUB_TOKEN=$(gh auth token)   # or GH_TOKEN
jactionlint -online
```

- **Token.** `GITHUB_TOKEN` or `GH_TOKEN` is used when set. Without one the requests are unauthenticated, which works but GitHub
  allows only 60 an hour; jactionlint says so once. A token that GitHub rejects is dropped with a warning and the run goes on
  without it. `GITHUB_API_URL` selects a GitHub Enterprise Server.
- **Cache.** Answers are kept in `$XDG_CACHE_HOME/jactionlint` (`~/.cache/jactionlint`), at most 32 MiB, shared safely by parallel
  processes. An answer is used for an hour (`-online-cache-ttl`), then asked for again with its ETag, which costs no rate limit
  when it did not change. `-online-cache-ttl=0` checks every answer.
- **Few requests.** Every repository, tag and commit is asked for once per run however often it is used, and the checks share what
  they learn. The number of requests grows with the number of different actions, not steps.
- **Rate limit and failures.** When GitHub refuses (rate limit), is unreachable, or fails repeatedly, the online checks stop for the
  rest of the run with one warning on stderr (in the SARIF log with `-format sarif`) and the findings that needed
  GitHub are missing. Nothing is retried and the exit status does not change. Interrupting with Ctrl-C stops the lookups.
- **Levels.** The online rules do not belong to a profile; `-online` turns them on at their own level (`impostor-commit` and
  `known-vulnerable-actions` are errors, `ref-confusion`, `archived-uses` and `ref-version-mismatch` warnings, `stale-action-refs` is
  informational). Set a level or `off` in `rules` as for any rule.
- **Not in the playground.** The WebAssembly build has no network access, so `-online` is an error there.

#### Pin tags to commits with `-online -fix`

With the online checks, `-fix` can repair `unpinned-uses` findings (a rule of the `strict` profile) by replacing a tag with the commit
it points to and naming the tag in a comment, the format Dependabot and Renovate keep up to date:

```yaml
- uses: actions/checkout@v4
# becomes
- uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4
```

The fix is applied by plain `-fix` because it keeps the code the same: it is offered only when the ref is the name of a tag
(annotated tags are followed to the commit) and no branch has the same name. A branch, an unknown tag, an abbreviated SHA, a Docker
image or a line with a comment that does not name the ref is left to you.

```sh
jactionlint -online -fix
```

<a id="hk"></a>
### hk

[hk][] runs linters and fixers as Git hooks and from the command line. `jactionlint -format sarif` gives hk the diagnostics
with the rule IDs. `hk util sarif-diff` turns the fixes in the SARIF log into a patch, and `jactionlint -fix` is the fixer hk
runs when a finding has no fix. Add this step to `hk.pkl`:

```pkl
["jactionlint"] {
    glob = List(".github/workflows/*.yml", ".github/workflows/*.yaml")
    batch = true
    diagnostic_format = "sarif"
    check = "jactionlint -format sarif {{files}}"
    check_diff = "hk util sarif-diff -- jactionlint -format sarif {{files}}"
    fix = "jactionlint -fix {{files}}"
}
```

- Only the fixes which are safe, valid and do not conflict with each other are in the SARIF log. A finding which cannot be fixed
  stays without a fix so hk runs `jactionlint -fix` and still reports it.
- The log uses `columnKind: unicodeCodePoints` and file URIs relative to the directory where jactionlint ran.
- The exit status is `0` for a clean run and `1` when errors were found. Other statuses are failures. Nothing but the SARIF log is
  on stdout, and nothing is on stderr unless `-verbose` or `-debug` is given, because hk parses both together. The
  deprecation warnings of the configuration are put in `invocations[].toolConfigurationNotifications` of the log instead of
  stderr in this format.
- To use the `strict` profile, set `profile: strict` in `.github/jactionlint.yaml`.
- `missing-timeout` is in the default profile, so the first `hk check` on a repository whose jobs have no `timeout-minutes` fails
  on every job. Its fix exists only when `rules.missing-timeout.default-minutes` is set (there is no built-in
  number). The fix is safe and is in the SARIF log, so `hk fix` adds the timeout without running `jactionlint -fix`. `missing-permissions` (`strict`) is only in the log when the fix is safe; otherwise hk runs
  `jactionlint -fix`, which leaves the unsafe fix alone and still reports the finding, and you decide whether to run
  `jactionlint -fix=unsafe`.
- To add the [online checks](#online-checks) put the flag in the commands. A second step keeps them apart from the offline checks, so
  you can run it on demand or in CI, where a token is at hand:

  ```pkl
  ["jactionlint-online"] {
      glob = List(".github/workflows/*.yml", ".github/workflows/*.yaml")
      batch = true
      diagnostic_format = "sarif"
      check = "jactionlint -online -format sarif {{files}}"
      check_diff = "hk util sarif-diff -- jactionlint -online -format sarif {{files}}"
      fix = "jactionlint -online -fix {{files}}"
  }
  ```

  The batches run in parallel and share the cache. Without a reachable GitHub the step prints one warning in the SARIF log and
  reports what it could check. Keep the online step out of the `pre-commit` hook unless a token is always set.

<a id="on-github-actions"></a>
## Use jactionlint on GitHub Actions

Preparing `jactionlint` executable with the download script is recommended. See [the instruction](install.md#download-script) for
more details. It sets an absolute file path of downloaded executable to `executable` output in order to use the executable in the
following steps easily.

Here is an example of simple workflow to run jactionlint on GitHub Actions. Please ensure `shell: bash` since the default
shell for Windows runners is `pwsh`.

```yaml
name: Lint GitHub Actions workflows
on: [push, pull_request]

jobs:
  jactionlint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - name: Download jactionlint
        id: get_jactionlint
        run: bash <(curl https://raw.githubusercontent.com/jdx/jactionlint/main/scripts/download-jactionlint.bash)
        shell: bash
      - name: Check workflow files
        run: ${{ steps.get_jactionlint.outputs.executable }} -color
        shell: bash
```

Or simply download the executable and run it in one step:

```yaml
- name: Check workflow files
  run: |
    bash <(curl https://raw.githubusercontent.com/jdx/jactionlint/main/scripts/download-jactionlint.bash)
    ./jactionlint -color
  shell: bash
```

The download script allows to specify the version of jactionlint and the download directory. Try to give `--help` argument
to the script for more usage details.

If you want to enable [shellcheck integration](checks.md#check-shellcheck-integ), install `shellcheck` command. Note that
shellcheck is [pre-installed on Ubuntu worker][preinstall-ubuntu].

If you want to [annotate errors][ga-annotate-error] from jactionlint on GitHub, consider using
[Problem Matchers](#problem-matchers).

If you prefer Docker image to running a downloaded executable, using [jactionlint Docker image](#docker) is another option.

```yaml
name: Lint GitHub Actions workflows
on: [push, pull_request]

jobs:
  jactionlint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - name: Check workflow files
        uses: docker://ghcr.io/jdx/jactionlint:latest
        with:
          args: -color
```

## Online playground

Thanks to WebAssembly, jactionlint playground is available on your browser. It never sends any data to outside your browser.

https://jactionlint.jdx.dev/

Paste your workflow content to the code editor at left pane. It automatically shows the results at right pane. When editing
the workflow content in the code editor, the results will be updated on the fly. Clicking an error message in the results
table moves a cursor to position of the error in the code editor.

<a id="docker"></a>
## [Docker][docker] image

[Official Docker image][docker-image] is available. The image contains `jactionlint` executable and all dependencies (shellcheck
and pyflakes).

Available tags are:

- `jactionlint:latest`: Latest stable version of jactionlint. This image is recommended.
- `jactionlint:{version}`: Specific version of jactionlint. (e.g. `jactionlint:1.8.2`) <!-- x-release-please-version -->

Just run the image with `docker run`:

```sh
docker run --rm ghcr.io/jdx/jactionlint:latest -version
```

To check all workflows in your repository, mount your repository's root directory as a volume and run jactionlint in the mounted
directory. When you are at a root directory of your repository:

```sh
docker run --rm -v $(pwd):/repo --workdir /repo ghcr.io/jdx/jactionlint:latest -color
```

To check a file with jactionlint in a Docker container, pass the file content via stdin and use `-` argument:

```sh
cat /path/to/workflow.yml | docker run --rm -i ghcr.io/jdx/jactionlint:latest -color -
```

Or mount the workflows directory and pass the paths as arguments:

```sh
docker run --rm -v /path/to/workflows:/workflows ghcr.io/jdx/jactionlint:latest -color /workflows/ci.yml
```

## Using jactionlint from Go program

Go APIs are available. See [the Go API document](api.md) for more details.


<a id="tools-integ"></a>
## Tools integration

### reviewdog

[reviewdog][] is an automated review tool for various code hosting services. It officially [supports actionlint][reviewdog-actionlint].
You can check errors from jactionlint easily with inline review comments at pull request review.

The usage is easy. Run `reviewdog/action-actionlint` action in your workflow as follows.

```yaml
name: reviewdog
on: [pull_request]
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: reviewdog/action-actionlint@v1
```

<a id="problem-matchers"></a>
### Problem Matchers

[Problem Matchers][problem-matchers] is a feature to extract GitHub Actions annotations from terminal outputs of linters.

Copy [jactionlint-matcher.json][jactionlint-matcher] to `.github/jactionlint-matcher.json` in your repository.

Then enable the matcher using `add-matcher` command before running `jactionlint` in the step of your workflow.

```yaml
- name: Check workflow files
  run: |
    echo "::add-matcher::.github/jactionlint-matcher.json"
    bash <(curl https://raw.githubusercontent.com/jdx/jactionlint/main/scripts/download-jactionlint.bash)
    ./jactionlint -color
  shell: bash
```

When you change your workflow and the changed line causes a new error, CI will annotate the diff with the extracted error message.

<img src="https://github.com/rhysd/ss/blob/master/actionlint/problem-matcher.png?raw=true" alt="annotation by Problem Matchers" width="715" height="221"/>

### super-linter

[super-linter][] is a Bash script for a simple combination of various linters, provided by GitHub. It has support for jactionlint.
Running super-linter in your repository automatically runs jactionlint.

To ignore some errors, please add `-ignore` option by using [`GITHUB_ACTIONS_COMMAND_ARGS` environment variable][super-linter-env-var].
Please see [super-linter/super-linter#1852](https://github.com/super-linter/super-linter/issues/1852) for the discussion.

### MegaLinter

[MegaLinter][] is a linters aggregator for CI, embedding linters for many languages and formats. It has support for jactionlint
out of the box. Running MegaLinter in your repository automatically runs jactionlint on your workflow files. Please see
[the jactionlint page of MegaLinter documentation][megalinter-actionlint] for more details.

### pre-commit

[pre-commit][] is a framework for managing and maintaining multi-language Git pre-commit hooks. jactionlint is available as a
pre-commit hook to check workflow files in `.github/workflows/` directory.

Add this to your `.pre-commit-config.yaml` in your repository:

```yaml
---
repos:
  - repo: https://github.com/jdx/jactionlint
    rev: v1.8.2  # x-release-please-version
    hooks:
      - id: jactionlint
```

As alternatives to `jactionlint` hook, `jactionlint-docker` or `jactionlint-system` hooks are available.

| Hook ID | Explanation |
|-|-|
| `jactionlint` | Automatically installs `jactionlint` command in isolated `$GOPATH` directory using [Go toolchain][go-install]. |
| `jactionlint-docker` | Automatically pulls [the jactionlint Docker image](#docker). |
| `jactionlint-system` | Uses system-installed `jactionlint` command. The command is necessary to be [installed manually](install.md). |

### VS Code

[Linter extension][vsc-extension] for [VS Code][vscode] is available. The extension automatically detects `.github/workflows`
directory, runs `jactionlint` command, and reports errors in the code editor while editing workflow files.

### Emacs

Plugins for both [Flycheck][emacs-flycheck] and [Flymake][emacs-flymake] are available via [MELPA][emacs-melpa].

Their respective repositories are [flycheck-actionlint][emacs-flycheck-extension] and [flymake-actionlint][emacs-flymake-extension].

### Vim and Neovim

[nvim-lint][] supports jactionlint on Neovim. The plugin automatically and asynchronously runs jactionlint and notifies errors
on the fly when you edit GitHub Actions CI workflows. Please read the plugin's documentation for more details.

[ALE][vim-ale] supports jactionlint on Vim and Neovim. Similar to nvim-lint, The plugin automatically and asynchronously runs
jactionlint and notifies errors on the fly when you edit GitHub Actions CI workflows. Please read the plugin's documentation for
more details.

### Pulsar Edit

A [Linter package][pulsar-linter] for [Pulsar Edit][pulsar] is available. The package automatically detects a `workflows`
directory, executes the `jactionlint` command on any detected GitHub Actions files within the directory, and reports returned
information in the code editor display tab while editing workflow files.

### Nova

[Nova.app][nova] is a MacOS only editor and IDE. The [Actionlint for Nova][nova-extension] allows you to get inline feedback
while editing actions.

### trunk

[trunk][trunk-io] is an extendable superlinter with a builtin language server and preexisting issue detection. Actionlint is
integrated [here](https://github.com/trunk-io/plugins).

Once you have [initialized trunk in your repo](https://docs.trunk.io/docs/check-get-started), to enable at the latest actionlint
version, just run:

```bash
trunk check enable actionlint
```

or if you'd like a specific version:

```bash
trunk check enable actionlint@1.7.12
```

or modify `.trunk/trunk.yaml` in your repository to contain:

```yaml
lint:
  enabled:
    - jactionlint@1.7.12
```

Then just run:

```bash
trunk check
```

and it will check your modified files via actionlint, if applicable, and show you the results. Trunk also will detect preexisting
issues and highlight only the newly added actionlint issues. For more information, check the [trunk docs][trunk-docs].

You can also see actionlint issues inline in VS Code via the [Trunk VS Code extension][trunk-vscode].

---

[Checks](checks.md) | [Installation](install.md) | [Configuration](config.md) | [Go API](api.md) | [References](reference.md)

[reviewdog-actionlint]: https://github.com/reviewdog/action-actionlint
[reviewdog]: https://github.com/reviewdog/reviewdog
[cmd-manual]: https://github.com/jdx/jactionlint/blob/main/docs/usage.md
[re2]: https://golang.org/s/re2syntax
[go-template]: https://pkg.go.dev/text/template
[jsonl]: https://jsonlines.org/
[ga-annotate-error]: https://docs.github.com/en/actions/learn-github-actions/workflow-commands-for-github-actions#setting-an-error-message
[hk]: https://hk.jdx.dev/
[sarif]: https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html
[problem-matchers]: https://github.com/actions/toolkit/blob/master/docs/problem-matchers.md
[super-linter]: https://github.com/github/super-linter
[super-linter-env-var]: https://github.com/super-linter/super-linter#environment-variables
[megalinter]: https://megalinter.io/
[megalinter-actionlint]: https://megalinter.io/latest/descriptors/action_actionlint/
[jactionlint-matcher]: https://raw.githubusercontent.com/jdx/jactionlint/main/.github/jactionlint-matcher.json
[preinstall-ubuntu]: https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md
[pre-commit]: https://pre-commit.com
[go-install]: https://go.dev/doc/install
[docker]: https://www.docker.com/
[docker-image]: https://github.com/jdx/jactionlint/pkgs/container/jactionlint
[vsc-extension]: https://marketplace.visualstudio.com/items?itemName=arahata.linter-actionlint
[vscode]: https://code.visualstudio.com/
[emacs-melpa]: https://melpa.org/
[emacs-flymake]: https://www.gnu.org/software/emacs/manual/html_node/flymake/
[emacs-flymake-extension]: https://github.com/ROCKTAKEY/flymake-actionlint
[emacs-flycheck]: https://www.flycheck.org/
[emacs-flycheck-extension]: https://github.com/tirimia/flycheck-actionlint
[nvim-lint]: https://github.com/mfussenegger/nvim-lint
[vim-ale]: https://github.com/dense-analysis/ale
[pulsar]: https://pulsar-edit.dev/
[pulsar-linter]: https://web.pulsar-edit.dev/packages/linter-github-actions
[nova-extension]: https://extensions.panic.com/extensions/org.netwrk/org.netwrk.actionlint/
[nova]: https://nova.app
[trunk-io]: https://docs.trunk.io/docs
[trunk-docs]: https://docs.trunk.io/docs/check
[trunk-vscode]: https://marketplace.visualstudio.com/items?itemName=trunk.io
