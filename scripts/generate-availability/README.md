# generate-availability

generate-availability writes [availability.go](../../availability.go), the table of the contexts and special functions that each workflow key allows. It reads the "Context availability" table of the GitHub documentation.

GitHub Actions limits contexts and functions to certain places. For example, only some workflow keys can read the `secrets` context, and `always()` is available in places such as `jobs.<job_id>.if` and `jobs.<job_id>.steps.if` but not everywhere. The check [Contexts and built-in functions](../../docs/checks.md#check-contexts-and-builtin-func) uses the generated table to report an expression in a place where its context or function is not available.

## When to run it

Run it when GitHub changes the table or when you change the script. You rarely start it by hand:

- `go generate` runs it through the directive in [rule_expression.go](../../rule_expression.go), and `mise run generate` runs `go generate` and then the generator of `docs/rules.md`.
- The `generate` job of [release-please.yaml](../../.github/workflows/release-please.yaml) runs `go generate` in every run of the workflow: on a push to `main`, on a weekly schedule and by hand. The job commits the result to the release pull request, and only while one is open.

## Prerequisites

- Go.
- Network access to `raw.githubusercontent.com`, unless you pass a local copy of the source.

## Usage

Run the tool from the repository root.

```
generate-availability [[srcfile] dstfile]
```

| Arguments | Source | Output |
| --- | --- | --- |
| none | fetched from GitHub | stdout |
| `dstfile` | fetched from GitHub | the file |
| `srcfile dstfile` | the local file | the file |

A `dstfile` of `-` writes to stdout. More than two arguments print the usage and exit with status 1, and so does any failure.

```sh
go run ./scripts/generate-availability ./availability.go
```

Read a local copy of the source instead of fetching it:

```sh
go run ./scripts/generate-availability /path/to/contexts.md ./availability.go
```

Print the result to stdout to compare it with the file:

```sh
go run ./scripts/generate-availability -
```

## How it works

1. The tool fetches `content/actions/reference/workflows-and-actions/contexts.md` from the `main` branch of [github/docs](https://github.com/github/docs). It is Markdown with Liquid directives.
2. It finds the third-level heading `Context availability` and the first table below it. A missing heading or table stops the tool with `no "Context availability" table was found`.
3. It reads each row as a workflow key, its contexts and its special functions. It removes Liquid directives such as `{% ... %}`, unescapes HTML entities, skips a row with an empty key and reads `None` as no special function. A row whose first cell is the directive `{% else %}` ends the table, and a directive `{% else %}` in any other cell is an error.
4. It writes these identifiers into the package `jactionlint` and formats them with `gofmt`:
   - `WorkflowKeyAvailability` maps a workflow key to its contexts and special functions. An empty list of contexts means that every context is available, and an empty list of functions means that no special function is.
   - `SpecialFunctionNames` maps a special function to the workflow keys that allow it.
   - `AllContexts` maps a context to the workflow keys that allow it.
   - `allWorkflowKeys` lists every key, for tests.

The documentation table lacks the key `jobs.<job_id>.snapshot.if` ([github/docs#41255](https://github.com/github/docs/issues/41255)), so the tool adds that row itself unless the table has it: the contexts `github`, `vars`, `inputs`, `needs`, `strategy`, `matrix`, `steps`, `job`, `runner` and `env`, and the functions `always`, `cancelled`, `failure`, `hashfiles` and `success`. The comment in `main.go` names the runner schema that this row follows.

## Pitfalls

- The script depends on the shape of the Markdown of github/docs. When GitHub restructures the page, the tool fails with a message like the one above, and you must adapt the parser.
- The generated file and `testdata/ok.go` link to `https://github.com/jdx/jactionlint/blob/main/scripts/generate-availability/`, which GitHub renders with this README. If you rename the directory, change the URL in `main.go` and regenerate both.

## Tests

`go test ./scripts/generate-availability` generates the table from `testdata/ok.md` and compares it with `testdata/ok.go`. It also feeds `no_heading.md`, `no_table.md`, `broken_table.md` and `else_directive.md` to the tool and expects the matching error. The tests use local files, except `TestFetchError`, which requests a file that does not exist from `raw.githubusercontent.com`, so the test run needs network access.

## Next steps

- [Contexts and built-in functions](../../docs/checks.md#check-contexts-and-builtin-func): the check that reads the table.
- [Contributing guide](../../CONTRIBUTING.md): the other generated files and the steps to refresh them.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
