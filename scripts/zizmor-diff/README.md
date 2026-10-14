# zizmor-diff

zizmor-diff measures how many of the findings of [zizmor](https://docs.zizmor.sh/) jactionlint also reports. It runs both tools over a corpus of repositories and prints a report of the findings that match, the findings that only one tool has, and the zizmor audits that jactionlint has no rule for. Run it by hand when you compare a rule with its zizmor audit.

## When to run it

- Before you move a rule into the `default` profile, or claim that a rule matches a zizmor audit. The [contributing guide](../../CONTRIBUTING.md#accepting-a-new-check) asks for a review of the false positives on a corpus of real repositories, and this tool produces the numbers of the comparison with zizmor.
- After you add a rule that covers a zizmor audit, to check the match and to add the audit to [mapping.json](./mapping.json).
- After you pin another zizmor version, to see which audits changed.

No CI job and no hk step runs it, because the corpus is a set of local checkouts. [The measurements ledger](../../docs/zizmor-measurements.md) records the runs that the project keeps.

## Prerequisites

- Go.
- [mise](https://mise.jdx.dev/), which runs the pinned zizmor with `mise x zizmor@<pinned version> -- zizmor`. mise downloads zizmor on first use, so the first run needs the network. `--help` shows the pinned version.
- The repositories of the corpus as local directories. zizmor runs with `--offline`, so nothing else is fetched, unless you pass `--online`.
- With `--online`: a token in `GITHUB_TOKEN` or `GH_TOKEN`.

## Usage

Run the tool from the repository root.

```sh
mise run zizmor-diff
```

The task builds jactionlint and runs the tool over [corpus.txt](./corpus.txt) with the config [pedantic.yaml](./pedantic.yaml). That corpus lists the checkouts of the maintainer under `~/src`, so on another machine every repository is skipped. Pass your own repositories with `--repos`, which replaces the corpus file:

```sh
go build -o jactionlint ./cmd/jactionlint
go run ./scripts/zizmor-diff \
    --jactionlint ./jactionlint \
    --jactionlint-config scripts/zizmor-diff/pedantic.yaml \
    --repos /path/to/one --repos mine=/path/to/two \
    --markdown report.md --json report.json
```

| Flag | Meaning | Default |
| --- | --- | --- |
| `--corpus-file FILE` | The repositories, one per line. | `scripts/zizmor-diff/corpus.txt` |
| `--repos DIR` | One repository, as `DIR` or `label=DIR`. Repeat it for more. It replaces the corpus file. | none |
| `--jactionlint EXE` | The jactionlint executable. A path becomes absolute, because the tool runs it inside each repository. | `jactionlint` from `PATH` |
| `--jactionlint-config FILE` | A config file for every repository, instead of its own. The tool passes it as `--config-file`. | each repository's own config |
| `--jactionlint-arg ARG` | An extra jactionlint argument, for example `--profile=correctness`. Repeat it for more. | none |
| `--zizmor CMD` | The command that runs zizmor. The tool splits it at spaces. | `mise x zizmor@<pinned version> -- zizmor` |
| `--online` | Compares the online audits (`impostor-commit`, `known-vulnerable-actions` and others): jactionlint runs with `--online` and zizmor without `--offline`. It makes GitHub API requests. | off |
| `--mapping FILE` | An audit mapping instead of the embedded `mapping.json`. | the embedded file |
| `--line-tolerance N` | The distance in lines that two matching findings may have. `mise run zizmor-diff` passes none, so its figures are not those of [the measurements ledger](../../docs/zizmor-measurements.md), which gives the value of its runs. | 0 |
| `--jobs N` | The number of repositories analyzed in parallel. | 4 |
| `--markdown FILE` | Where to write the Markdown report. | stdout |
| `--json FILE` | Where to write the JSON report. | none |

The tool prints `analyzed NAME` to stderr for each repository. It exits with status 0 once it wrote the report, also when a tool failed on a repository, because the report shows that failure. It exits with status 1 when it cannot read the corpus, the corpus is empty, the mapping is invalid or it cannot write a report, and with status 2 for a bad flag.

### The corpus file

A line is a directory or `label=directory`. A leading `~` expands to your home directory. Blank lines and lines that start with `#` are ignored, and so is a trailing ` # comment`. The label defaults to the directory name without a `-jactionlint` suffix. A directory that does not exist is skipped and listed in the report as `skipped: directory not found`.

### The config of the repositories

jactionlint reads the `.github/jactionlint.yaml` of a repository unless you pass `--jactionlint-config`. Most corpus repositories already pass the rules that their config chose, so with their own config the report would show almost nothing in common. `mise run zizmor-diff` therefore replaces the config with `pedantic.yaml`, which selects the `pedantic` profile to match `zizmor --persona pedantic`. That also drops settings that belong to one repository, such as the labels of self-hosted runners, so unrelated findings of `unknown-runner-label` and `mutable-runner-label` show up under "jactionlint only".

## How findings are compared

For each repository the tool runs jactionlint (`--format` with a JSON template, plus the arguments you gave) and zizmor (`--persona pedantic --format sarif`, with `--offline` unless you pass `--online`) in the directory of the repository. A run of jactionlint with exit status 0 or 1 and a run of zizmor with exit status 0 or 10 to 14 are successful. Any other status is a failure of that tool.

Both outputs are reduced to the repository, the file, the line and the rule or audit. Then:

- Workflow files (`.github/workflows/*.yml` and `*.yaml`) are compared. zizmor findings in `.github/dependabot.yml` are compared too, for the audits that `mapping.json` knows. The other zizmor findings outside workflows, such as those in an `action.yml`, count as "zizmor out of scope" in the table of the repository.
- A zizmor finding is also reported when jactionlint reports a rule that `mapping.json` maps to the audit in the same file and on the same line. `--line-tolerance` widens the line.
- A jactionlint finding is "only jactionlint" when no zizmor finding matched it. This includes the findings of jactionlint in files other than workflows.
- When either tool fails on a repository, the table of repositories shows the failure and the tool leaves that repository out of every count. The run completes.
- A rule without an entry in `mapping.json` is always "only jactionlint".

The report has these parts:

| Part | Content |
| --- | --- |
| Mapped audits | For each audit of the mapping: the coverage, the jactionlint rules, the findings of zizmor, how many jactionlint also reports, how many it misses and in how many repositories. |
| jactionlint only | For each rule: the findings that no zizmor finding matches, with up to five examples. A correctness rule that zizmor lacks belongs here, and so does a disagreement. |
| Unmapped zizmor audits | The audits that zizmor reports and `mapping.json` has no rule for. |
| Repositories | For each repository: the findings of both tools, the shared ones, the findings out of scope and notes about skips and failures. |

## Maintaining mapping.json

`mapping.json` says which jactionlint rules cover which zizmor audit. The comparison above is only as good as this file, so update it whenever a rule starts to cover an audit. This is the entry of `unpinned-uses`:

```json
"unpinned-uses": {
  "coverage": "partial",
  "notes": "The blanket hash-pin policy and the per-pattern policies (hash-pin, ref-pin, any) for actions, reusable workflows and docker:// images. Supported patterns are *, owner/*, owner/repo and owner/repo/path.",
  "jactionlint": [{ "rule": "unpinned-uses" }]
}
```

| Key | Meaning |
| --- | --- |
| `zizmor` (top level) | The zizmor version that the mapping was written for. The report prints it next to the version that ran. |
| `coverage` | `full` or `partial`, and the tool rejects any other value. [The parity page](../../docs/zizmor-parity.md) grades each audit with its own words (`same`, `differs`, `not comparable`). Those say whether the two tools agree on the cases that were compared, and `coverage` says how much of the audit a rule covers, so the two scales need not match. |
| `notes` | Free text about what differs. The JSON report carries it. |
| `jactionlint` | A list of at least one match. `rule` is a rule ID, and the optional `message_contains` narrows the match to findings whose message holds that text. |

An audit without an entry shows up under "Unmapped zizmor audits" and is never matched.

- Take the rule IDs from `jactionlint --list-rules`. An entry with an ID that is not a rule matches nothing, and nothing warns you.
- Several audits can map to one rule, and one audit can map to several rules. An audit and its rule can have different names: `unsound-condition` maps to `if-always-true`, and `unsound-ternary` (the falsy ternary idiom `a && b || c`, where `b` can be falsy) to the rule of the same name.
- Write in `notes` what differs, and update it when a rule changes. The note of `unsound-condition` says that `if-always-true` ignores the trailing newlines of a block scalar, which GitHub trims and zizmor reports.
- The pinned zizmor version appears in the default of `--zizmor` here and in [fix-corpus](../fix-corpus/README.md), in `mise.toml`, in `mapping.json` and in the sentences of [the parity page](../../docs/zizmor-parity.md) and [the measurements ledger](../../docs/zizmor-measurements.md) that name the compared version. Change all of them together when you pin another version.

The output shapes of both tools are read in [adapter.go](./adapter.go) and nowhere else. When the JSON output of jactionlint changes, edit `jactionlintArgs` and `parseJactionlint` there. When the SARIF of zizmor changes, edit `parseZizmor`.

## Tests

```sh
go test ./scripts/zizmor-diff
```

The tests use the fixtures in `testdata/` and a fake command runner, so they need neither zizmor nor the network. They cover the parsing of both outputs, the corpus file, the mapping, the comparison with and without line tolerance, the reports and the handling of a failed tool.

## Next steps

- [Measurements of the zizmor comparison](../../docs/zizmor-measurements.md): the runs of this tool that the project keeps.
- [Parity with zizmor](../../docs/zizmor-parity.md): how each rule compares with its audit.
- [Contributing guide](../../CONTRIBUTING.md#accepting-a-new-check): the policy for new checks that this tool supports.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
