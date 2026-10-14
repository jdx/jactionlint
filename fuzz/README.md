# Fuzz the parsers

This directory holds the go-fuzz targets that feed random input to the parsers and checkers of jactionlint to find panics and broken invariants. Run them before you change a parser, a glob validator, the expression lexer or the shell script analyzer. Native Go fuzz tests in the test files of the repository do the same job and are described further down.

CI does not fuzz. It runs the seeds of the native fuzz tests as ordinary tests, and it never builds or runs the go-fuzz targets.

## The two kinds of fuzz test

| | go-fuzz targets | native fuzz tests |
|---|---|---|
| Where | the files of this directory, which carry the build tag `gofuzz` | `FuzzXxx(f *testing.F)` functions in `*_test.go` files |
| Runner | [go-fuzz](https://github.com/dvyukov/go-fuzz), started by `mise run fuzz` | `go test` with the fuzz flag |
| Leaves behind | `corpus/` and `crashers/` in the directory you run it in | a failing input in `testdata/fuzz/` of the package |
| CI | not run | the seeds run as part of `go test ./...` |

## Run a go-fuzz target

Before you start, you need Go, [mise](https://mise.jdx.dev/) (or the two `go run` commands below) and a network connection: the first run downloads go-fuzz.

`mise run fuzz` fails on a clean checkout. `go.mod` does not require the package that go-fuzz instruments the code with, so `go-fuzz-build` stops:

```
-: no required module provides package github.com/dvyukov/go-fuzz/go-fuzz-dep; to add it:
	go get github.com/dvyukov/go-fuzz/go-fuzz-dep
typechecking of ./fuzz failed
```

The task does not add the package itself, so you add it by hand and undo that afterwards:

1. Add the package to the module. The command adds one line to `go.mod` and two to `go.sum`, which you must not commit (step 4).

   ```sh
   go get github.com/dvyukov/go-fuzz/go-fuzz-dep
   ```

2. Build the targets and run one of them. The name is the function of the target (see [the targets](#the-targets)), and `FuzzParse` is the default.

   ```sh
   mise run fuzz FuzzParse
   ```

   The task is the same as these two commands, which you can run without mise:

   ```sh
   go run github.com/dvyukov/go-fuzz/go-fuzz-build@latest ./fuzz
   go run github.com/dvyukov/go-fuzz/go-fuzz@latest -bin ./jactionlint_fuzz-fuzz.zip -func FuzzParse
   ```

3. Watch the status line that go-fuzz prints every few seconds, and stop the run with Ctrl-C. The line shows the number of `corpus` inputs, the `crashers`, the executions (`execs`) and the coverage (`cover`). A `crashers` value above 0 means go-fuzz found an input that makes the target panic.

4. Undo the change of step 1.

   ```sh
   git restore go.mod go.sum
   ```

## What a run leaves behind

go-fuzz writes into the current directory, so run it at the root of the repository.

| Path | Content | Git and `mise run clean` |
|---|---|---|
| `jactionlint_fuzz-fuzz.zip` | the instrumented build of the targets, made by `go-fuzz-build` | ignored by git, removed |
| `corpus/` | the inputs that go-fuzz keeps and mutates | ignored by git, removed |
| `crashers/` | one entry per crash: the input as `HASH`, the same input as a Go string in `HASH.quoted` and the panic with its stack trace in `HASH.output` | ignored by git, removed |
| `suppressions/` | one file per crash, holding the panic message and the functions of its stack trace | neither ignored nor removed: delete it yourself |

## Handle a crasher

1. Read `crashers/HASH.output` to see where the target panicked, and `crashers/HASH.quoted` for the input.
2. Add the input to a test next to the code that crashed, in the root package or in `internal/runscript`, so that the crash stays fixed. Call the function that the target calls.
3. Fix the code and run `go test ./...`.
4. Delete `crashers/` and `suppressions/`, then run the target again.

## The targets

Every target is a function `FuzzXxx(data []byte) int` of the package `jactionlint_fuzz`. It panics when the code under test breaks an invariant. Most targets return 1 when the code accepts the input and 0 when it rejects it.

| Target | What it does with the input |
|---|---|
| `FuzzParse` | parses it as a workflow with `Parse` when go-yaml can parse it |
| `FuzzCheck` | parses it as a workflow and runs a fixed set of the rules over the result (see `check.go` for the list) |
| `FuzzExprParse` | lexes and parses it as an expression when it is valid UTF-8, and runs the semantic check on the result |
| `FuzzGlobGitRef` | validates it as a glob pattern for Git ref names with `ValidateRefGlob` |
| `FuzzGlobFilePath` | validates it as a glob pattern for file paths with `ValidatePathGlob` |
| `FuzzParseDependabot` | parses it as a Dependabot configuration with `ParseDependabot` |
| `FuzzCheckDependabot` | parses it as a Dependabot configuration and visits every `updates` entry, and panics when one is missed |
| `FuzzCommentIndex` | builds the index of the comments of a source with `NewCommentIndex`, and panics when the comments are not in increasing line order or one has a column below 1 |
| `FuzzParseUses` | parses it as the value of `uses:` with `ParseUses`, and panics when the result lacks the raw value or an invalid reference lacks its problem |
| `FuzzUsesPattern` | takes a `forbidden-uses` pattern and a `uses:` value separated by a NUL byte, and panics when the pattern matches a value that is not an action or a reusable workflow |
| `FuzzRunScript` | analyzes it as a `run:` script with `runscript.Analyze` and checks the invariants of the result |

To add a target, write a function with that signature in a file of this directory (every file starts with `//go:build gofuzz`), add a row to the table, and run it with its name.

## Run a native fuzz test

The native fuzz tests are Go test functions that take a `*testing.F`. They live in `*_test.go` files of the root package and of `internal/runscript`. List them with:

```sh
grep -rn "^func Fuzz.*testing.F" --include='*_test.go' --exclude-dir=node_modules .
```

Fuzz one of them with `go test`. The fuzz flag takes a regular expression that must match exactly one test, and `go test` fuzzes one package at a time, so anchor the name and name the package. The run goes on until it finds a failure or you press Ctrl-C, unless you limit it with the fuzz time flag.

```sh
go test -run '^$' -fuzz '^FuzzCommandLine$' -fuzztime 30s .
```

The last argument is the package. `.` is the root package, and `./internal/runscript` holds `FuzzAnalyze`.

A test has two sets of seeds: the `f.Add` calls in the test itself and the files in `testdata/fuzz/FuzzName/` of its package. A plain `go test` runs the seeds as ordinary test cases, which is how CI runs them. When a fuzz run fails, `go test` writes the failing input to `testdata/fuzz/FuzzName/` and prints the command that runs it again (add the package path when the test is not in the root package):

```sh
go test -run=FuzzName/HASH
```

Fix the code, then commit the input file so that the seeds keep covering it. `go clean -fuzzcache` deletes the inputs that `go test` keeps in the build cache between runs.

## Open failures

Two native fuzz tests fail within seconds, and neither failure comes from a change of yours.

| Test | Command | Failure |
|---|---|---|
| `FuzzUsesPattern` of the root package | `go test -run '^$' -fuzz '^FuzzUsesPattern$' -fuzztime 30s .` | A `uses:` value with a control character, such as a carriage return or a form feed, does not match the pattern made of its own name and ref. A run reports `"\r/0@0" does not match its own pattern`. |
| `FuzzAnalyze` of `internal/runscript` | `go test -run '^$' -fuzz '^FuzzAnalyze$' -fuzztime 30s ./internal/runscript` | A script with a NUL byte breaks an invariant of the analysis. A run reports `"local\x00": command word: raw "local" is not the source "local\x00"`. |

The inputs differ from run to run. Each run writes the failing input to `testdata/fuzz/FuzzName/` of the package, and `go test` then fails on that file until the code is fixed. Delete the file when you do not fix the failure, so that it does not reach your pull request. The go-fuzz target `FuzzUsesPattern` of this directory checks a different invariant, and it is not the test of this row.

## Next steps

- [Fuzz in CONTRIBUTING.md](../CONTRIBUTING.md#fuzz): where fuzzing fits in the tests that a pull request needs.
- [What to run after a change](../scripts/README.md#what-to-run-after-a-change): the commands for each kind of change.
- [go-fuzz](https://github.com/dvyukov/go-fuzz) and [Go fuzzing](https://go.dev/doc/security/fuzz/): the details of the two runners.
