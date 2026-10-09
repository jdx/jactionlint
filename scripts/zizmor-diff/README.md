zizmor-diff
===========

This is a script to measure how much of [zizmor](https://docs.zizmor.sh/)'s findings jactionlint also
reports. It runs both tools over a corpus of repositories and prints a report with:

- for each zizmor audit that `mapping.json` knows: how many findings zizmor has, how many of them jactionlint
  also reports, and how many it misses
- the findings only jactionlint reports (correctness checks zizmor does not have, or disagreements)
- the zizmor audits that have no jactionlint rule yet (unmapped)
- a per repository summary, including repositories that were skipped or where a tool failed

It is the differential corpus test of the v2 plan. Each rule that is enabled by default needs a documented
false positive rate of about zero on the corpus, and this script produces the numbers for that review.

## Prerequisites

- Go
- [mise](https://mise.jdx.dev/), which runs the pinned zizmor (`mise x zizmor@1.30.1 -- zizmor`)
- the repositories of the corpus checked out locally. Nothing is fetched from the network (`zizmor --offline`)

## Usage

```sh
mise run zizmor-diff
```

builds jactionlint and runs the script over [corpus.txt](./corpus.txt) with
[strict-v1.yaml](./strict-v1.yaml). Run the program directly for more control.

```sh
go build -o jactionlint ./cmd/jactionlint
go run ./scripts/zizmor-diff \
    --jactionlint ./jactionlint \
    --repos ~/src/mise --repos mine=/path/to/checkout \
    --markdown report.md --json report.json
```

| Flag | Meaning |
|---|---|
| `--corpus-file FILE` | repositories, one per line. Default `scripts/zizmor-diff/corpus.txt` |
| `--repos DIR` | a repository directory or `label=DIR`. Repeatable. Replaces the corpus file |
| `--jactionlint EXE` | the jactionlint executable. Default `jactionlint` from `PATH` |
| `--jactionlint-config FILE` | config used instead of each repository's own (`-config-file`) |
| `--jactionlint-arg ARG` | extra jactionlint argument, for example `-profile` and `strict` later. Repeatable |
| `--zizmor CMD` | command that runs zizmor. Default `mise x zizmor@1.30.1 -- zizmor` |
| `--mapping FILE` | audit mapping instead of the embedded `mapping.json` |
| `--line-tolerance N` | lines of distance allowed between matching findings. Default 0 |
| `--jobs N` | repositories analyzed in parallel. Default 4 |
| `--markdown FILE` / `--json FILE` | where to write the reports. Markdown goes to stdout by default |

### Corpus file

One repository per line: a directory or `label=directory`. `~` expands to the home directory, blank lines and
`#` comments are ignored. The label defaults to the directory name without a `-jactionlint` suffix. A
directory that does not exist is reported as skipped.

### Config of the repositories

jactionlint reads the `.github/jactionlint.yaml` of each repository. Most corpus repositories already pass the
default checks, so with their own config the report shows almost nothing in common. `mise run zizmor-diff`
therefore replaces the config with `strict-v1.yaml`, which turns on the opt-in checks that overlap with zizmor
(`require-permissions`, `require-commit-hash`, `check-falsy-ternary`). That also drops repository specific
settings such as self-hosted runner labels, so unrelated `runner-label` findings show up in "jactionlint only".

## How findings are compared

Both outputs are normalized to `(repo, file, line, rule)`:

- Only workflow files (`.github/workflows/*.yml` and `*.yaml`) are compared. zizmor also audits `action.yml`
  and `dependabot.yml`; those findings are counted as "zizmor out of scope" per repository.
- A zizmor finding is *also reported* when jactionlint reports a rule that `mapping.json` maps to the audit in
  the same file and on the same line (`--line-tolerance` widens that).
- A jactionlint finding is *only jactionlint* when no zizmor finding matches it that way.
- If either tool fails on a repository (zizmor exits with an unexpected code, or the output cannot be parsed),
  the failure is shown in the repositories table and that repository is left out of all counts. The run still
  completes.

### mapping.json

`mapping.json` lists the zizmor audits that have a jactionlint counterpart:

```json
"unpinned-uses": {
  "coverage": "partial",
  "notes": "Only the opt-in require-commit-hash check for actions.",
  "jactionlint": [{ "rule": "action", "message_contains": "must be pinned to a full-length commit SHA" }]
}
```

`coverage` is `full` or `partial`. jactionlint v1 only has coarse rule names (`expression`, `action`), so an entry
can narrow a rule with `message_contains`. Once findings carry stable rule IDs, replace the entry with the
ID and drop `message_contains`. Add an entry whenever a new jactionlint rule covers a zizmor audit. Audits
without an entry show up under "Unmapped zizmor audits".

`unsound-ternary` stays in the file for the falsy ternary check, but zizmor 1.30.1 has no audit of that name, so
its row is always zero.

## Retargeting for jactionlint v2

The output shapes of both tools are read in [adapter.go](./adapter.go) and nowhere else. When the v2 `-format
json` or `sarif` shapes change, edit `jactionlintArgs` and `parseJactionlint` there. `parseJactionlint` already
prefers an `id` field over `kind` when it exists.

## Running it weekly

zizmor and jactionlint both change often, so drift is easiest to see on a schedule. The corpus lives on a local
machine today, so there is no workflow in this repository. A workflow which checks out the corpus repositories
could look like this. It needs a token with read access to every corpus repository, so add it deliberately.

```yaml
name: zizmor-diff
on:
  schedule:
    - cron: "0 6 * * 1"
  workflow_dispatch:
permissions: {}
jobs:
  diff:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@<sha>
        with:
          persist-credentials: false
      - uses: jdx/mise-action@<sha>
      # Check out every repository of the corpus into ~/src, for example with a loop over `gh repo clone`
      # using a read-only token stored as a secret.
      - run: mise run zizmor-diff | tee "$GITHUB_STEP_SUMMARY"
```

## Test

```sh
go test ./scripts/zizmor-diff
```

The tests use the fixtures in `testdata/` and a fake command runner, so they need neither zizmor nor network.
