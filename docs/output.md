# Output formats

Choose the output format for people, annotations, code scanning or scripts, and look up the fields of a finding, the SARIF log and the template syntax. It also covers `--export-facts`, which prints a JSON document about the repository instead of findings.

<a id="format"></a>
<a id="format-error-messages"></a>
## Choose a format

`--format` (or `-f`) selects the format. Pick the one that the consumer of the findings reads.

| The consumer | Format | Command |
| --- | --- | --- |
| A person in a terminal or a CI log | `text` (the default) | `jactionlint` |
| A person or `grep`, one line per finding | `oneline` | `jactionlint --format oneline` or `jactionlint --oneline` |
| An editor or a tool that reads compiler messages | `gcc` | `jactionlint --format gcc` |
| Pull request annotations on GitHub Actions | `github` | `jactionlint --format github` |
| Code scanning, hk or another SARIF consumer | `sarif` | `jactionlint --format sarif > jactionlint.sarif` |
| A script that parses one document | `json` | `jactionlint --format json` |
| A script that reads one finding per line | `jsonl` | `jactionlint --format jsonl` |
| A count per rule and per file in a CI log | `summary` | `jactionlint --format summary` |
| Your own text | A Go template: any value that contains `{{ }}` | [Custom templates](#custom-templates) |

A value that is neither a format nor a template exits with status 2. The exit status does not depend on the format, and `--min-severity` hides the findings below a level from every format. [Exit status](./cli.md#exit-status) lists every cause.

A run without findings prints this:

| Format | Output of a run without findings |
| --- | --- |
| `text`, `oneline`, `gcc`, `github`, `jsonl` | Nothing |
| `json` | `[]` |
| `summary` | `0 findings in 0 of N files`, where N is the number of files checked |
| `sarif` | A log with an empty `results` list |

## Streams and color

Every format writes the findings to stdout, except in a `--diff` run, which the paragraph after the lists describes. Everything else that a lint run says goes to stderr, so `jactionlint --format json | jq` is safe. [Synopsis and operands](./cli.md#synopsis-and-operands) covers the options that print and exit.

Stdout carries:

- the findings in the chosen format;
- the result line of `--baseline-write`;
- the unified diff of `--diff`;
- the document of [`--export-facts`](#export-the-facts-of-the-repository) and the report of `--suggest-timeouts`, which replace the findings.

Stderr carries:

- the logs of `--verbose` and `--debug`;
- warnings: a deprecated key of the config file, a retired rule ID in an ignore, a skipped online lookup;
- the note about a tool that was not found;
- the note about findings that the [baseline](./baseline.md#baseline) or [`--changed-since`](./ci.md#show-only-what-a-pull-request-changed) hides, and the [first-run hint](./usage.md#the-first-run), for `text` and `oneline` only;
- the summary of `--fix` (`Fixed N problem(s)`) and of `--diff` (`Would fix N problem(s)`).

With `--diff`, the findings that remain go to stderr in the chosen format, because stdout holds the diff. With `--fix`, they stay on stdout. With `--format sarif`, the warnings and the notes about the run are in the log (`invocations[].toolConfigurationNotifications` and `properties.externalTools`) and stderr stays empty unless you pass `--verbose` or `--debug`, or a `--fix` or `--diff` run prints its summary there.

Only `text` and `oneline` use color. The default, `--color=auto`, colors when stdout is a terminal, so a CI log has no color unless you pass `--color`. [The output options](./cli.md#output) describe `--color` and `--no-color`, and [the variables that other programs read](./cli.md#variables-that-libraries-and-other-programs-read) describe `NO_COLOR` and `TERM`.

## One input, every format

The samples in this section come from one workflow file with three findings. Save it as `.github/workflows/ci.yaml` in a Git repository:

```yaml
on: push
permissions:
  contents: write
jobs:
  test:
    runs-on: ubuntu-latest
    needs: build
    env:
      REF: ${ github.ref }
    steps:
      - run: echo ${{ matrix.msg }}
```

Every command passes `--profile correctness`, so the findings of the policy rules, such as a missing `timeout-minutes`, stay out of the samples. In each of them, jactionlint exits with status 1 because the run has findings.

Within a file, the findings are ordered by line, then column, then message. Files appear in the order of the arguments, or in path order when you run on the repository.

The level of a rule comes from the profile and from [the `rules` key](./config.md#rules). Each format spells the level of a finding in its own way:

| Level | `text`, `oneline` | `gcc` | `github` | `sarif` | `severity` |
| --- | --- | --- | --- | --- | --- |
| `error` | no prefix | `error` | `::error` | `error` | `error` |
| `warn` | `warning: ` | `warning` | `::warning` | `warning` | `warn` |
| `info` | `info: ` | `note` | `::notice` | `note` | `info` |

### Text and oneline

`text` prints the file, the line and the column, the message and the rule ID in brackets, followed by the source line and an indicator under the region of the finding. `oneline` prints only the first line of each finding.

```sh
jactionlint --profile correctness
```

```
.github/workflows/ci.yaml:5:3: job "test" needs job "build" which does not exist in this workflow [undefined-job-needs]
  |
5 |   test:
  |   ^~~~~
.github/workflows/ci.yaml:9:12: "${ github.ref }" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{ github.ref }}` [malformed-expression-delimiters]
  |
9 |       REF: ${ github.ref }
  |            ^~~~~~~~~~~~~~~
.github/workflows/ci.yaml:11:23: property "msg" is not defined in object type {} [undefined-property]
   |
11 |       - run: echo ${{ matrix.msg }}
   |                       ^~~~~~~~~~
```

```sh
jactionlint --profile correctness --format oneline
```

```
.github/workflows/ci.yaml:5:3: job "test" needs job "build" which does not exist in this workflow [undefined-job-needs]
.github/workflows/ci.yaml:9:12: "${ github.ref }" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{ github.ref }}` [malformed-expression-delimiters]
.github/workflows/ci.yaml:11:23: property "msg" is not defined in object type {} [undefined-property]
```

`--show-doc-urls` adds the URL of the documentation of the rule under the snippet of a `text` finding. `oneline` stays at one line per finding, so it does not show the URL.

```sh
jactionlint --profile correctness --select undefined-property --show-doc-urls
```

```
.github/workflows/ci.yaml:11:23: property "msg" is not defined in object type {} [undefined-property]
   |
11 |       - run: echo ${{ matrix.msg }}
   |                       ^~~~~~~~~~
  = docs: https://jactionlint.jdx.dev/rules#undefined-property
```

`--show-fix-status` adds a line in the same way: the fix of the finding, or the reason it has none. [Print the status of each finding](./fix.md#print-the-status-of-each-finding) shows it.

### gcc

`gcc` prints `file:line:col: level: message [id]` like a compiler.

```sh
jactionlint --profile correctness --format gcc
```

```
.github/workflows/ci.yaml:5:3: error: job "test" needs job "build" which does not exist in this workflow [undefined-job-needs]
.github/workflows/ci.yaml:9:12: error: "${ github.ref }" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{ github.ref }}` [malformed-expression-delimiters]
.github/workflows/ci.yaml:11:23: error: property "msg" is not defined in object type {} [undefined-property]
```

<a id="example-error-annotation-on-github-actions"></a>
### github

`github` prints one [workflow command](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands) per finding, in the shape `::LEVEL file=FILE,line=N,endLine=N,col=N,endColumn=N,title=ID::MESSAGE`. GitHub shows each command as an annotation on the pull request. The rule ID is the `title`, and `endColumn` is inclusive.

```sh
jactionlint --profile correctness --format github
```

```
::error file=.github/workflows/ci.yaml,line=5,endLine=5,col=3,endColumn=7,title=undefined-job-needs::job "test" needs job "build" which does not exist in this workflow
::error file=.github/workflows/ci.yaml,line=9,endLine=9,col=12,endColumn=26,title=malformed-expression-delimiters::"${ github.ref }" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{ github.ref }}`
::error file=.github/workflows/ci.yaml,line=11,endLine=11,col=23,endColumn=32,title=undefined-property::property "msg" is not defined in object type {}
```

The format escapes `%` as `%25`, a carriage return as `%0D` and a line feed as `%0A` in the message, and also `:` as `%3A` and `,` as `%2C` in the properties, as the command syntax requires. `--show-doc-urls` ends the message with a second line, `docs: URL`, and `--show-fix-status` adds a line with `fix: ...` or `no fix (CODE): ...` in the same way. A region over several lines has no `endColumn`. [Annotations and code scanning](./ci.md#annotations-and-code-scanning) shows the workflow step.

### summary

`summary` prints counts instead of findings: how many per rule and per file. When a [baseline](./baseline.md#baseline) hides findings, it counts the new and the baselined ones apart.

```sh
jactionlint --profile correctness --format summary
```

```
3 findings in 1 of 1 files

by rule                          findings
malformed-expression-delimiters         1
undefined-job-needs                     1
undefined-property                      1

by file                    findings
.github/workflows/ci.yaml         3
```

<a id="example-serialized-into-json"></a>
### json

`json` prints one JSON array of finding objects, in one line. `--select` limits the run to one rule here so that the line stays short.

```sh
jactionlint --profile correctness --select undefined-property --format json
```

```
[{"message":"property \"msg\" is not defined in object type {}","filepath":".github/workflows/ci.yaml","line":11,"column":23,"kind":"expression","snippet":"      - run: echo ${{ matrix.msg }}\n                      ^~~~~~~~~~","end_column":32,"id":"undefined-property","severity":"error","doc_url":"https://jactionlint.jdx.dev/rules#undefined-property","end_line":11}]
```

The encoder escapes `<`, `>` and `&` as `\u003c`, `\u003e` and `\u0026`, so the path of stdin is written `"\u003cstdin\u003e"`. Every JSON parser reads these back to the original characters, but a `grep` for `&&` in the output finds nothing. [Fields of a finding](#fields-of-a-finding) describes each key. `jactionlint --format '{{json .}}'` prints the same bytes.

A finding that has a fix carries it. This is the complete output for the rule that has one in the sample, formatted with `jq`:

```sh
jactionlint --profile correctness --select malformed-expression-delimiters --format json | jq .
```

```
[
  {
    "message": "\"${ github.ref }\" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{ github.ref }}`",
    "filepath": ".github/workflows/ci.yaml",
    "line": 9,
    "column": 12,
    "kind": "malformed-expression-delimiters",
    "snippet": "      REF: ${ github.ref }\n           ^~~~~~~~~~~~~~~",
    "end_column": 26,
    "id": "malformed-expression-delimiters",
    "severity": "error",
    "doc_url": "https://jactionlint.jdx.dev/rules#malformed-expression-delimiters",
    "end_line": 9,
    "fix": {
      "description": "Write the expression with two braces",
      "edits": [
        {
          "start": 120,
          "end": 120,
          "new_text": "{"
        },
        {
          "start": 132,
          "end": 132,
          "new_text": "}"
        }
      ]
    }
  }
]
```

<a id="example-serialized-in-json-lines"></a>
### jsonl

`jsonl` prints one finding object per line ([JSON Lines](https://jsonlines.org/)), in the same order as `json`. Each line is valid against the same schema, so a script can process the findings as a stream. `jactionlint --format '{{range $err := .}}{{json $err}}{{end}}'` prints the same bytes.

```sh
jactionlint --profile correctness --select undefined-job-needs,undefined-property --format jsonl
```

```
{"message":"job \"test\" needs job \"build\" which does not exist in this workflow","filepath":".github/workflows/ci.yaml","line":5,"column":3,"kind":"job-needs","snippet":"  test:\n  ^~~~~","end_column":7,"id":"undefined-job-needs","severity":"error","doc_url":"https://jactionlint.jdx.dev/rules#undefined-job-needs","end_line":5}
{"message":"property \"msg\" is not defined in object type {}","filepath":".github/workflows/ci.yaml","line":11,"column":23,"kind":"expression","snippet":"      - run: echo ${{ matrix.msg }}\n                      ^~~~~~~~~~","end_column":32,"id":"undefined-property","severity":"error","doc_url":"https://jactionlint.jdx.dev/rules#undefined-property","end_line":11}
```

`sarif` is the longest format. The [SARIF](#sarif) section shows one result of it.

## Fields of a finding

One table covers the three places that expose a finding: the keys of `json` and `jsonl`, the fields that a [template](#custom-templates) reads, and the properties of a SARIF result. [Lines and columns](#lines-and-columns) explains the position fields.

| JSON key | Template field | SARIF | Meaning | Present |
| --- | --- | --- | --- | --- |
| `message` | `{{$err.Message}}` | `message.text` | The message of the finding | Always |
| `filepath` | `{{$err.Filepath}}` | `artifactLocation.uri` | The path of the file, relative to the working directory when possible. For stdin it is `<stdin>`, or the value of `--stdin-filename` | Always |
| `line` | `{{$err.Line}}` | `region.startLine` | The line where the finding starts (1-based) | Always |
| `column` | `{{$err.Column}}` | `region.startColumn` | The column where the finding starts (1-based, in Unicode code points) | Always |
| `end_line` | `{{$err.EndLine}}` | `region.endLine` | The line where the region ends. It equals `line` unless the region spans several lines | Always |
| `end_column` | `{{$err.EndColumn}}` | `region.endColumn` | The end of the region. The JSON key and the template field are inclusive, the SARIF value is exclusive. For a region over several lines, the JSON key and the template field end the indicator on the first line, and the SARIF value is a column of `end_line` | Always |
| `kind` | `{{$err.Kind}}` | `properties.kind` | The legacy checker name, such as `expression` or `job-needs`, which several rules can share. A rule that has no legacy checker uses its own ID. Use `id` instead | Always (SARIF: only when it differs from the ID) |
| `id` | `{{$err.ID}}` | `ruleId` | The [rule ID](./rules.md), such as `template-injection` | Always |
| `severity` | `{{$err.Severity}}` | `level` | The level: `error`, `warn` or `info` (SARIF: `error`, `warning` or `note`) | Always |
| `doc_url` | `{{$err.DocURL}}` | `helpUri` of the rule | The URL of the documentation of the rule | Built-in rules |
| `snippet` | `{{$err.Snippet}}` | none | The source line with the `^~~~` indicator under the region, separated by a line feed | When there is a line to show |
| `fix` | `{{$err.Fix}}` | `fixes` | The automatic fix, described below | When the finding has a fix (SARIF: only a safe one) |
| `no_fix` | `{{$err.NoFix}}` | `properties.noFixCode`, `noFixReason`, `noFixOption` | Why a finding that the rule could fix has no fix, described below | Only for such a finding |
| `events` | `{{$err.Events}}` | `properties.events` | The events of the workflow, with `--finding-context` | See [Events and token permissions](#events-and-token-permissions-of-a-finding) |
| `permissions` | `{{$err.Permissions}}` | `properties.permissions` | The permissions of the token of the job, with `--finding-context` | See [Events and token permissions](#events-and-token-permissions-of-a-finding) |

The `fix` object has three keys:

- `description` says what the fix does.
- `unsafe` is present only when it is `true`: the fix may change the behavior of the workflow.
- `edits` is a list of edits. An edit replaces the bytes from `start` up to, but not including, `end` with `new_text`. The offsets are byte offsets from the start of the file, not lines and columns. `start == end` inserts the text, and an empty `new_text` deletes the range.

The `no_fix` object has three keys:

- `code` is one of `option-required`, `option-invalid`, `online-required`, `lookup-failed`, `unsupported-shape` and `needs-judgment`. SARIF adds `unsafe-edit`, `edit-conflict` and `invalid-edit`.
- `reason` is a sentence for people.
- `option` names the option to set, such as `missing-timeout.default-minutes`. It is present only for `option-required` and `option-invalid`.

A finding never has both `fix` and `no_fix`, and an advice-only finding has neither. [Why a finding was not fixed](./fix.md#why-a-finding-was-not-fixed) says what to do for each code, and `--show-fix-status` prints the status in the `text` and `github` formats. This is the object of a finding that needs an option:

```sh
jactionlint --select missing-timeout --format json | jq '.[0].no_fix'
```

```
{
  "code": "option-required",
  "reason": "set the missing-timeout option default-minutes to enable the fix, jactionlint does not choose a timeout",
  "option": "missing-timeout.default-minutes"
}
```

### Events and token permissions of a finding

`--finding-context` adds two fields to each finding of `json`, `jsonl` and `sarif` (to `properties` of the result). With them you can filter the findings of many repositories by what can trigger the workflow and what its token can do.

| Field | Meaning |
| --- | --- |
| `events` | The sorted names of the events of `on:`, such as `["pull_request_target", "push"]`. It is absent for a file that is not a workflow (an action, `dependabot.yml`). |
| `permissions` | The permissions of the token of the job that holds the finding: its `permissions:`, or else the ones of the workflow. It maps each scope to `read`, `write` or `none`. |

The cases of `permissions`:

- `read-all` and `write-all` are `{"*": "read"}` and `{"*": "write"}`, and an empty `permissions: {}` is `{"*": "none"}`.
- A value that an expression decides is `"unknown"`, never `"none"`: `{"*": "unknown"}` for `permissions: ${{ inputs.p }}` and `{"contents": "unknown"}` for `contents: ${{ inputs.c }}`.
- The field is absent when neither the job nor the workflow sets permissions, because the default token then depends on a setting of the repository that jactionlint does not know.
- A finding outside of the jobs has the permissions of the workflow.

A filter on `.permissions["contents"]` alone misses a job with `permissions: write-all`, so fall back to the `*` key. This filter selects the findings of workflows that run on `push` with a write token for `contents`:

```sh
jactionlint --profile correctness --finding-context --format json | jq -c '.[] | select(.events | index("push")) | select((.permissions["contents"] // .permissions["*"]) == "write") | {id, line, events, permissions}'
```

```
{"id":"undefined-job-needs","line":5,"events":["push"],"permissions":{"contents":"write"}}
{"id":"malformed-expression-delimiters","line":9,"events":["push"],"permissions":{"contents":"write"}}
{"id":"undefined-property","line":11,"events":["push"],"permissions":{"contents":"write"}}
```

### JSON Schema of the output

`jactionlint --print-output-schema` prints the JSON Schema (draft-07) of the output of `--format json`: an array of finding objects. Each line of `--format jsonl` is one finding object, valid against `#/definitions/finding` of the same schema. The committed copy is [`schema/jactionlint-output.json`](https://github.com/jdx/jactionlint/blob/main/schema/jactionlint-output.json). It is the contract for programs that read the findings:

- A field is not removed and does not change its meaning within a major version.
- New fields can be added, so a reader has to ignore the fields it does not know. The schema does not set `additionalProperties: false`.
- The schema requires `message`, `line`, `column`, `end_line`, `end_column`, `kind`, `id` and `severity`. The command line always prints `filepath` as well, and the table above says when the other fields are present.

The [config file](./config.md#json-schema) has a schema of its own.

### Lines and columns

Lines and columns are 1-based in every format. Columns count Unicode code points: a tab, an accented letter and an emoji are one column each, and a CR before a LF is not counted. The SARIF log says `columnKind: unicodeCodePoints`.

The end of the region is where the formats differ:

| Where | Name | The end column is |
| --- | --- | --- |
| `json`, `jsonl` | `end_column` | The column of the last character of the region (inclusive). For a region over several lines, the end of the indicator on `line`, not a column of `end_line` |
| `github` | `endColumn` | Inclusive, the same number as `end_column`. It is absent for a region over several lines |
| `sarif` | `region.endColumn` | The column after the last character on `endLine` (exclusive) |
| Template | `$err.EndColumn` | Inclusive, the same number as `end_column` |
| Go API | `Error.EndColumn` | Exclusive, the same number as SARIF |
| `text`, `oneline`, `gcc` | none | No end column is printed. `text` underlines the region under the snippet |

The indicator under the snippet is aligned by display width, so a wide character takes two cells there but one column in the numbers. To try it, save this file as `.github/workflows/ci.yaml`, replacing the sample. It has a key of five Japanese characters:

```yaml
on:
  push:
    日本語日本: main
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
```

```sh
jactionlint --profile correctness --format json | jq -c '.[0] | {column, end_column, snippet}'
jactionlint --profile correctness --format sarif | jq -c '.runs[0].results[0].locations[0].physicalLocation.region'
```

```
{"column":5,"end_column":10,"snippet":"    日本語日本: main\n    ^~~~~~~~~~~"}
{"startLine":3,"startColumn":5,"endLine":3,"endColumn":11}
```

The region is the key and its colon: six code points from column 5. The JSON `end_column` is 10, the SARIF `endColumn` is 11, and the indicator is 11 cells wide. Do not read `end_column` off the indicator.

A finding inside a string (an expression, a line of a `run:` script, a secret) is on the line and at the column of the text in the file. This holds in every style of YAML scalar: plain, single quoted (`''`), double quoted (escapes such as `\n`, `\"`, `\x41`, `\u00e9` and `\<newline>`), literal blocks (`|`) and folded blocks (`>`), also over several lines.

<a id="example-sarif-format"></a>
## SARIF

`--format sarif` prints a [SARIF](https://sarifweb.azurewebsites.net/) 2.1.0 log, the format that code scanning and hk read. [Annotations and code scanning](./ci.md#annotations-and-code-scanning) shows how to upload it, and [hk](./hooks.md#hk) shows the hook. This is the result of the one rule of the sample that has a fix. `--select` limits the run to that rule:

```sh
jactionlint --profile correctness --select malformed-expression-delimiters --format sarif | jq '.runs[0].results[0]'
```

```
{
  "ruleId": "malformed-expression-delimiters",
  "ruleIndex": 0,
  "level": "error",
  "message": {
    "text": "\"${ github.ref }\" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{ github.ref }}`"
  },
  "locations": [
    {
      "physicalLocation": {
        "artifactLocation": {
          "uri": ".github/workflows/ci.yaml",
          "index": 0
        },
        "region": {
          "startLine": 9,
          "startColumn": 12,
          "endLine": 9,
          "endColumn": 27
        }
      }
    }
  ],
  "fixes": [
    {
      "description": {
        "text": "Write the expression with two braces"
      },
      "artifactChanges": [
        {
          "artifactLocation": {
            "uri": ".github/workflows/ci.yaml"
          },
          "replacements": [
            {
              "deletedRegion": {
                "startLine": 9,
                "startColumn": 14,
                "endLine": 9,
                "endColumn": 14
              },
              "insertedContent": {
                "text": "{"
              }
            },
            {
              "deletedRegion": {
                "startLine": 9,
                "startColumn": 26,
                "endLine": 9,
                "endColumn": 26
              },
              "insertedContent": {
                "text": "}"
              }
            }
          ]
        }
      ]
    }
  ],
  "partialFingerprints": {
    "primaryLocationLineHash": "ada39da3f318b102:1"
  }
}
```

The log has these parts:

| Part | What it holds |
| --- | --- |
| `runs[0].tool.driver` | `name`, `version` and `informationUri`, and `semanticVersion` when the version has the form `major.minor.patch` |
| `rules` of the driver | The rules that appear in the results, sorted by ID. Each has `id`, `name` (the ID in Pascal case), `shortDescription`, `helpUri`, `defaultConfiguration.level`, and `properties.tags` (the group). `properties.profile` is the profile that enables the rule, or `online` for an online rule, and is absent for an opt-in rule |
| `artifacts` | One entry per file with findings, with `sourceLanguage: yaml`. The paths stay relative, so a consumer resolves them from the directory where jactionlint ran. An absolute path becomes a `file://` URI |
| `columnKind` | `unicodeCodePoints` |
| `results` | One result per finding, with the files sorted by path. A result has `ruleId`, `ruleIndex` (its position in `rules`), `level` (`error`, `warning` or `note`), `message.text`, the location with the region and `partialFingerprints` |
| `partialFingerprints` | `primaryLocationLineHash` is the context hash of the [baseline](./baseline.md#how-a-finding-matches) (the rule, the enclosing job or top-level key and the normalized text of the line, without the message) and `:n` for the n-th finding with the same hash in the file. Code scanning keeps tracking a finding when lines move above it, the indentation changes or the message is reworded |
| `fixes` | The safe fix as `artifactChanges[].replacements[]`, each with a `deletedRegion` in lines and columns and the `insertedContent.text` |
| `properties` of a result | `kind` when it differs from the ID, `events` and `permissions` with `--finding-context`, and `noFixCode`, `noFixReason` and `noFixOption` for a finding without a fix |
| `suppressions` | A finding that the baseline hides has one `external` suppression. `--sarif-hide-baselined` leaves such findings out of the log instead |
| `invocations[0].toolConfigurationNotifications` | The warnings about the run, such as a deprecated key of the config file |
| `properties` of the run | `externalTools` says whether shellcheck, pyflakes and node ran (see [external tools](./usage.md#external-tools)), and `onlineCoverage` says how complete the online checks were (see [Prove that the checks ran](./online.md#prove-that-the-checks-ran)) |

A result never has both `fixes` and `noFixCode`. The log holds no unsafe fix, so code scanning never offers a change that may alter the behavior of the workflow. A fix that exists but is not in the log has the code `unsafe-edit`, `edit-conflict` or `invalid-edit`: see [Why a finding was not fixed](./fix.md#why-a-finding-was-not-fixed).

## Custom templates

A `--format` value that contains `{{ }}` is a template in [Go template syntax](https://pkg.go.dev/text/template). The template runs once, on the list of all findings of the run, so a template that prints one line per finding starts with `range`. `--format '{{json .}}'` prints the same as `--format json`, and `--format '{{range $err := .}}{{json $err}}{{end}}'` the same as `--format jsonl`.

### Formatting syntax

In `{{ }}`, a dot (`.`) is the list of findings. The `range` action walks the list, like `for ... = range ... {}` in Go:

```
{{range $err := .}} this part runs once for each finding, with the finding in $err {{end}}
```

The fields of `$err` are the Template column of the [table of fields](#fields-of-a-finding). Without `--finding-context`, `$err.Events` and `$err.Permissions` are empty. Two fields have parts of their own:

| Field | Parts |
| --- | --- |
| `$err.Fix` | `.Description`, `.Unsafe` and `.Edits`, a list of edits with `.Start`, `.End` and `.NewText`. It is empty when there is no fix, so `{{if $err.Fix}}` tests for one |
| `$err.NoFix` | `.Code`, `.Reason` and `.Option` |

Besides the actions of the Go library (`if`, `with`, `printf`, `len` and others), jactionlint defines these:

| Action | Result | Example |
| --- | --- | --- |
| `json x` | `x` as JSON, followed by a line feed | `{{json $err}}` |
| `replace x y z` | `x` with each `y` replaced by `z`. More pairs of `y z` can follow | `{{replace $err.Filepath "/" "\\\\"}}` |
| `toPascalCase x` | `x` in PascalCase, for example `foo-bar` becomes `FooBar` | `{{toPascalCase $err.Kind}}` |
| `allRules` | The list of rule objects | `{{range $r := allRules}}{{$r.ID}} {{end}}` |
| `allKinds` | The list of legacy kind objects | `{{range $k := allKinds}}{{$k.Name}} {{end}}` |
| `getVersion` | The version of jactionlint | `{{getVersion}}` |

A rule object from `allRules` has `ID`, `Name` (the ID in PascalCase), `Description`, `Group` (`correctness`, `security`, `policy` or `style`), `DefaultLevel`, `Profile` and `URL`. `Profile` is the first profile that enables the rule, and it is empty when no profile does. A kind object from `allKinds` has `{{$kind.Name}}` and `{{$kind.Description}}`, for example `syntax-check` and `Checks for GitHub Actions workflow syntax`. Kinds are the legacy checker names of the `kind` field, not rules.

A pipe sends a value to an action. This template prints the line and the message of the first finding as a quoted string:

```sh
jactionlint --profile correctness --format 'line is {{(index . 0).Line}}, message is {{(index . 0).Message | printf "%q"}}\n'
```

```
line is 5, message is "job \"test\" needs job \"build\" which does not exist in this workflow"
```

Before it parses the template, jactionlint turns the escapes `\a`, `\b`, `\f`, `\n`, `\r`, `\t`, `\v` and `\\` in the format string into the characters they stand for. That is why `\n` ends a line in the examples. It also means that a backslash inside a string of the template needs four backslashes on the command line: the unescaping turns `\\\\` into `\\`, and the template reads `\\` as one backslash. This command prints the path with backslashes:

```sh
jactionlint --profile correctness --format '{{range $err := .}}{{replace $err.Filepath "/" "\\\\"}}|{{end}}\n'
```

```
.github\workflows\ci.yaml|.github\workflows\ci.yaml|.github\workflows\ci.yaml|
```

A template that writes one backslash as `"\\"` does not parse, because the unescaping leaves a single backslash that escapes the closing quote. jactionlint then exits with status 2. A template that parses but fails while it runs, for example by naming a field that does not exist, exits with status 3.

### Example: Markdown

This template prints a Markdown report with the snippet of each finding in a code block:

````sh
jactionlint --profile correctness --format '{{range $err := .}}### Finding at line {{$err.Line}}, column {{$err.Column}} of `{{$err.Filepath}}`\n\n{{$err.Message}}\n\n```\n{{$err.Snippet}}\n```\n\n{{end}}'
````

````markdown
### Finding at line 5, column 3 of `.github/workflows/ci.yaml`

job "test" needs job "build" which does not exist in this workflow

```
  test:
  ^~~~~
```

### Finding at line 9, column 12 of `.github/workflows/ci.yaml`

"${ github.ref }" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{ github.ref }}`

```
      REF: ${ github.ref }
           ^~~~~~~~~~~~~~~
```

### Finding at line 11, column 23 of `.github/workflows/ci.yaml`

property "msg" is not defined in object type {}

```
      - run: echo ${{ matrix.msg }}
                      ^~~~~~~~~~
```

````

### Example: one line per finding with its fix

This template prints the file, the line and the rule ID, and marks the findings that `--fix` can repair. It uses `if` and the `Fix` field:

```sh
jactionlint --profile correctness --format '{{range $err := .}}{{$err.Filepath}}:{{$err.Line}} {{$err.ID}}{{if $err.Fix}} (fixable){{end}}\n{{end}}'
```

```
.github/workflows/ci.yaml:5 undefined-job-needs
.github/workflows/ci.yaml:9 malformed-expression-delimiters (fixable)
.github/workflows/ci.yaml:11 undefined-property
```

## Export the facts of the repository

`jactionlint --export-facts` prints what the workflows and the composite actions of the repository declare, and what the call graph of the repository knows, as one JSON document instead of checking them. It is for tools that apply a policy of their own to the same model as jactionlint, such as a [Conftest](https://www.conftest.dev/) policy or a script, without parsing the YAML and the rules of GitHub Actions again.

Without operands it exports the workflows and the actions of the repository. With files it exports those, and the callers are still those of the whole repository. It only reads: it exits with status 2 with stdin, `--fix`, `--diff`, `--baseline-write`, `--migrate-ignores`, `--suggest-timeouts` or `--changed-since`, as [Option combinations that fail](./cli.md#option-combinations-that-fail) lists.

This example has a workflow that calls a reusable workflow from a job with write access. Save them as `.github/workflows/ci.yaml` and `.github/workflows/release.yaml` in a Git repository:

```yaml
name: CI
on:
  pull_request:
    branches: [main]
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
      - run: echo testing
  release:
    needs: [test]
    permissions:
      contents: write
    uses: ./.github/workflows/release.yaml
    with:
      channel: stable
```

```yaml
name: Release
on:
  workflow_call:
    inputs:
      channel:
        type: string
        required: true
permissions:
  contents: read
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: echo publishing
```

The document is large, so read it with a tool such as `jq`. This command lists the jobs whose token is the default token of the repository:

```sh
jactionlint --export-facts | jq -r '.workflows[].jobs[] | select(.effective_permissions.source == "repository-setting") | .id'
```

```
test
```

This one lists the reusable workflows that a job with write access calls, and on which events:

```sh
jactionlint --export-facts | jq -c '.calls[] | select(.permissions.scopes.contents == "write") | {from, job, to, events}'
```

```
{"from":".github/workflows/ci.yaml","job":"release","to":".github/workflows/release.yaml","events":["pull_request"]}
```

The top level of the document has `version`, `tool`, `tool_version`, `workflows`, `actions` and `calls`:

| Part | Facts |
| --- | --- |
| `workflows[]` | `path` (relative to the root of the repository), `parsed`, `name`, `events[]` (`name`, `types`, `filters`, `schedules`, `inputs`), `callable`, `called_by`, `permissions`, `concurrency`, `cache_mode`, `jobs[]` |
| `jobs[]` | `id`, `name`, `needs`, `runs_on`, `permissions` (as declared), `effective_permissions`, `concurrency`, `environment`, `timeout_minutes`, `cache_mode`, `if` (as written, not evaluated), `uses` (a call of a reusable workflow, with `secrets_inherit`, `secrets` and `inputs`), `steps[]` |
| `steps[]` | `index`, `id`, `name`, `kind`, `if`, `shell`, `uses` (with `kind` and, for an action, `repository`, `ref` and `ref_kind` such as `full-sha`), `inputs` of `with:` |
| `actions[]` | `path`, `parsed`, `name`, `using`, `inputs`, `steps[]`, and `called_by`: the workflows that run the action, with the actions between them (`via`), their `events` and the `inputs` they pass |
| `calls[]` | Each local call, either a job that calls a reusable workflow or a step that runs a local action: `kind`, `from`, `job`, `step`, `to` and `location`, with the `permissions` of the calling job and the `events` of the calling workflow |

Every position is a `location` with a 1-based `line` and `column`, and the column counts Unicode code points, as in the findings. The document says what it does not know instead of leaving it out or guessing:

- A value with a `${{ }}` expression has `dynamic: true` and the text as written, for example `concurrency.group`, `environment`, a `with:` input or a name. `concurrency.cancel_in_progress` is `true`, `false`, `unset` (not written, so nothing is cancelled) or `unknown` (an expression), and a `concurrency` written as one expression has `form: expression`. `timeout_minutes` has a `dynamic` flag and a job has `runs_on_dynamic`.
- `effective_permissions` has a `source`: `job`, `workflow` or `repository-setting`. The last one means that neither the job nor the workflow declares permissions, so the default token of the repository applies, which the file does not say: `scopes` is absent, and that means unknown, not `none`. A scope that an expression decides is `"unknown"`.
- `called_by.known` is `false` when nothing in the repository calls a reusable workflow: another repository may call it on any event, so `events` is not all of them. For an action it is `false` when no workflow of the repository runs it.
- A file that is not valid YAML has `parsed: false` and no facts of its content.

The facts are not findings: jactionlint judges none of them, and it never evaluates an expression. `jactionlint --print-facts-schema` prints the JSON Schema (draft-07) of the document, and [`schema/jactionlint-facts.json`](https://github.com/jdx/jactionlint/blob/main/schema/jactionlint-facts.json) is the committed copy. Like the schema of the findings, it does not set `additionalProperties: false`: fields are added within a `version`, so a reader ignores the fields that it does not know.

## Next steps

- [Annotations and code scanning in CI](./ci.md#annotations-and-code-scanning): the workflow steps for `github` and `sarif`.
- [Fix findings automatically](./fix.md#fix-errors-automatically): preview and apply the fixes that the `fix` field describes.
- [Adopt new checks with a baseline](./baseline.md#baseline): hide the findings that exist today.
- [Ignore findings](./ignore.md#ignore-some-errors): silence a finding that you accept.
- [Command line options](./cli.md#command-line-options): the options that change the output.
