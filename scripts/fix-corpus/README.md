fix-corpus
==========

This is the corpus test of `jactionlint -fix`. It applies the fixes to a copy of real workflows and checks that
fixing is safe. The repositories are never modified.

For each repository it copies `.github/`, runs `jactionlint -fix` there (with [config.yaml](./config.yaml), not the
repository's own config, so that as many fixers as possible run) and checks:

1. every YAML file is still valid YAML,
2. a second `-fix` changes nothing (and `-diff` prints nothing),
3. the number of findings does not go up for any jactionlint rule,
4. the number of findings of zizmor (`--persona pedantic --offline`) does not go up for any audit,

and that no fix was refused or failed to converge (jactionlint prints `error:` lines for those). It prints a Markdown
report with a row per repository, the fixes applied and the findings before and after per rule, and exits with status 1
when any check failed.

## Usage

```sh
mise run fix-corpus        # builds jactionlint and uses ~/src/*-jactionlint and ~/src/mise
# or
go build -o jactionlint ./cmd/jactionlint
go run ./scripts/fix-corpus -jactionlint ./jactionlint [-unsafe] [-no-zizmor] [-keep DIR] [dir ...]
```

| Flag | Meaning |
|---|---|
| `-jactionlint EXE` | the jactionlint executable. Default `jactionlint` from `PATH` |
| `-config FILE` | jactionlint config used for every repository. Default `scripts/fix-corpus/config.yaml` |
| `-zizmor CMD` | command that runs zizmor. Default `mise x zizmor@1.30.1 -- zizmor` |
| `-no-zizmor` | skip check 4 |
| `-unsafe` | apply the unsafe fixes too (`-fix=unsafe`) |
| `-keep DIR` | keep the fixed copies in DIR, to read the changes with `diff -ru` |

Directories are repositories (a directory with `.github/`). Without arguments the script uses `~/src/*-jactionlint` and
`~/src/mise`. The config turns on `profile: all` and gives `missing-timeout` a `default-minutes`, because without a
configured number that fixer offers nothing.
