# generate-jactionlint-matcher

generate-jactionlint-matcher writes [.github/jactionlint-matcher.json](../../.github/jactionlint-matcher.json), the problem matcher that turns the text output of jactionlint into annotations on GitHub Actions. The regular expression in the file is generated and tested, so you never edit the JSON by hand. Run the script when you change the text output format.

## When to run it

Run it after a change to the first line of a finding in the text format, `path:line:column: message [rule-id]`. When the default format changes, refresh the test fixtures first, then the matcher:

```sh
mise run matcher:fixtures
mise run matcher
```

The workflow [matcher.yaml](../../.github/workflows/matcher.yaml) runs on a push to `main` and on a pull request that changes the script, the workflow itself, `testdata/examples/*.out` or `testdata/err/*.out`, and you can start it by hand. It refreshes the fixtures with `mise run matcher:fixtures`, runs the test, regenerates the file with `mise run matcher` and fails when `git diff` shows any change. hk does not run this script.

## Prerequisites

Node.js. `mise install` installs it. The script uses no npm package. The fixture task also needs a built jactionlint, which `mise run matcher:fixtures` builds.

## Usage

Run the script from the repository root, because the output path is relative to the working directory.

| Command | What it does |
| --- | --- |
| `mise run matcher` | Writes `.github/jactionlint-matcher.json`. |
| `node ./scripts/generate-jactionlint-matcher/main.mjs FILE` | Writes FILE and prints `Wrote to FILE`. |
| `node ./scripts/generate-jactionlint-matcher/main.mjs` | Prints the JSON to stdout. |

## How it works

The matcher has one pattern for the first line of a finding. The five groups of the regular expression are the file path, the line, the column, the message and the rule ID in brackets, which GitHub reads as `file`, `line`, `column`, `message` and `code`. The source lines that follow a finding in the text format do not match the pattern and are ignored.

The expression looks complicated because it matches colored output too. With colors on, jactionlint puts ANSI escape sequences such as `ESC[33m` and `ESC[0m` around the path, the separators and the message, and `ESC[90m` before the rule ID. The reset that ends the rule ID is at the start of the next line, so the first line ends at the closing bracket. The expression therefore allows one escape sequence before the path and any number of them between the other parts, so that one pattern matches both the colored and the plain line. The script builds the expression from named pieces in [object.mjs](./object.mjs), such as `ESCAPE`, `FILEPATH` and `LINE`, instead of one literal.

## Pitfalls

- The test compares the rule ID in brackets with the field `kind` of `want.json`, not with `id`. It passes only while the first finding of `testdata/err/one_error.yaml` has the same `kind` and `id`.
- A new golden file in `testdata/err` or `testdata/examples` is checked by this test too. A line that does not look like a finding line breaks it.
- The last step of the workflow runs `git diff` after it refreshed both the fixtures and the matcher, so a committed fixture that differs from the output of the current jactionlint fails it too. Its message names only the matcher. When it fails, run `mise run matcher:fixtures` and `mise run matcher`, and commit every file that changed.

## Tests

```sh
node ./scripts/generate-jactionlint-matcher/test.mjs
```

The test runs the expression over two kinds of text:

- The fixtures in `scripts/generate-jactionlint-matcher/test`. `escape.txt` and `no_escape.txt` hold the output of `testdata/err/one_error.yaml` with and without color, and `want.json` holds the same findings in JSON. The test matches the first line of each text file and compares the five groups with the first finding of `want.json`.
- Every line of `testdata/examples/*.out` and `testdata/err/*.out`, except empty lines and lines that start with `/`, which are patterns of the golden files. Each line must match and fill all five groups.

`mise run matcher:fixtures` rewrites `escape.txt`, `no_escape.txt` and `want.json` from the current jactionlint. It ignores the exit status 1 of jactionlint, which reports findings in that file. Run it after the default format changes, and commit the three files.

## Next steps

- [Problem matchers](../../docs/ci.md#problem-matchers): how users add the matcher to their workflows.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
