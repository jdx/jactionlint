# fix-corpus

fix-corpus checks that `jactionlint --fix` is safe on real workflows. It applies the fixes to a copy of each repository's `.github` directory and reports whether the result is still valid YAML, stable on a second run and free of new findings. The repositories themselves are never modified.

## When to run it

Run it before you open a pull request that adds or changes a fixer, changes the order in which fixes claim the text (`fixPriority` in [fix.go](../../fix.go)), or changes a rule whose findings have a fix. Synthetic fixtures miss the layouts that real workflows have, and a fixer that breaks a file only on one of them shows up here.

No CI job and no hk step runs it, because the corpus is a set of repositories that this repository does not hold.

## Prerequisites

- Go, and a built jactionlint (see Usage). `mise run fix-corpus` builds it for you.
- Repositories to fix: directories with a `.github` directory. Without arguments the tool reads the maintainer's checkouts, `~/src/*-jactionlint` and `~/src/mise`. Pass your own directories as arguments.
- zizmor, unless you pass `-no-zizmor`. The default command, `mise x zizmor@<pinned version> -- zizmor`, downloads the pinned zizmor on first use, so it needs the network once. `-h` shows the pinned version.

## Usage

Run the tool from the repository root, because the default config path is relative to the working directory.

```sh
mise run fix-corpus
```

The task builds jactionlint and runs the tool without arguments. To choose the repositories and the options, run the build and the tool yourself:

```sh
go build -o jactionlint ./cmd/jactionlint
go run ./scripts/fix-corpus -jactionlint ./jactionlint -no-zizmor ~/src/my-repository ~/src/other-repository
```

| Flag | Meaning | Default |
| --- | --- | --- |
| `-jactionlint EXE` | The jactionlint executable. | `jactionlint` from `PATH` |
| `-config FILE` | The config file that jactionlint uses for every repository. | `scripts/fix-corpus/config.yaml` |
| `-zizmor CMD` | The command that runs zizmor. The tool splits it at spaces, so it cannot hold quotes. | `mise x zizmor@<pinned version> -- zizmor` |
| `-no-zizmor` | Skips check 4, the comparison with zizmor. | off |
| `-unsafe` | Applies the unsafe fixes too (`--fix=unsafe`). | off |
| `-keep DIR` | Keeps the fixed copies in DIR, as `DIR/NN-label/.github`, where NN is the position of the repository in the list. Compare them with `diff -ru`. | a temporary directory, which the tool removes unless a repository has a problem |

The positional arguments are the repository directories. The report is Markdown on stdout.

| Exit status | Meaning |
| --- | --- |
| 0 | No repository has a problem. A skipped repository is not a problem. |
| 1 | At least one repository has a problem. |
| 2 | The setup failed: the config file or the executable is missing, or the scratch directory cannot be created. |

## What it checks

For each repository the tool copies `.github/` to a scratch directory and follows symbolic links to files. It deletes the repository's own `.github/jactionlint.yaml` and `.github/jactionlint.yml`, because a repository config hides findings, and it creates an empty `.git` directory so that the copy counts as a repository. It counts the findings of jactionlint (`--format jsonl`, per rule) and of zizmor (`--offline --persona pedantic`, per audit), runs `--fix` and checks:

1. Every YAML file is still valid YAML.
2. A second `--fix` changes no file and applies no fix, and `--diff` prints nothing.
3. The number of findings does not go up for any jactionlint rule.
4. The number of findings does not go up for any zizmor audit.

It also records a problem when jactionlint refuses a fix or fails to converge (it prints `error:` lines for those), when `--fix` exits with status 3 or more, and when a step fails: the copy, a lint run or a zizmor run. Each of these is a problem in the report, and one problem in any repository makes the exit status 1. A problem is a failed check of this tool, not a finding of jactionlint.

The tool lints with [config.yaml](./config.yaml), which sets these keys:

```yaml
profile: pedantic
rules:
  missing-timeout:
    default-minutes: 30
  missing-permissions: error
  unused-ignore: error
```

The `pedantic` profile turns on as many rules with a fixer as possible. The fix of `missing-timeout` exists only when `default-minutes` is configured, and the number is for this tool only. The file also sets the level of `missing-permissions` and `unused-ignore` to `error`.

### The report

The tool prints a Markdown report with these parts:

- A table with a row per repository: YAML files, files changed, fixes applied, the findings of jactionlint and zizmor before and after, and the result `ok`, `N problem(s)` or `skipped: reason`.
- A table of the fixes by rule, with the findings before and after.
- A table of the zizmor findings by audit, when there are any.
- A section `Problems in LABEL (DIRECTORY)` for each repository with problems.

The label of a repository is its directory name without a `-jactionlint` suffix.

## Pitfalls

- The tool copies only `.github`. A fix in a file outside it, such as an `action.yml` at the repository root, is not exercised.
- A directory without `.github`, a path that does not exist and a repository without YAML files under `.github` are skipped, not failed. Check the `skipped` rows of the report, because a misspelled path makes the tool pass without testing anything.
- A zizmor run that fails counts as a problem of that repository. When the run before the fix fails, the tool drops the comparison with zizmor for that repository.
- A repository that has its own jactionlint config is tested with `config.yaml`, not with its own, so that as many fixers as possible run.
- After a run with problems, the temporary directory `fix-corpus-*` stays in the system temporary directory, and the report does not print its path. Use `-keep DIR` when you want to inspect the fixed copies.

## Tests

`go test ./scripts/fix-corpus` covers the parsing of the `Fixed N problem(s)` summary, the comparison of finding counts and the YAML check. It needs neither zizmor nor the network. The end-to-end run above is manual.

## Next steps

- [Contributing guide](../../CONTRIBUTING.md): when to run this tool while you write a fixer.
- [How fixing converges and what it checks](../../docs/fix.md#how-fixing-converges-and-what-it-checks): what `--fix` verifies about its own result.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
