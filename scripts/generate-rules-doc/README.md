# generate-rules-doc

generate-rules-doc writes [docs/rules.md](../../docs/rules.md), the reference of every rule ID, from the rule registry of the `jactionlint` package. The text of that page lives in the function `generate` of [main.go](./main.go), not in the page: edit the generator, then regenerate, because a hand edit of `docs/rules.md` is lost and fails the check.

## When to run it

Run it after you add or change a rule (its ID, group, summary, default level, profile, `Online` or `Fixable` flag, options or `DocsAnchor`), after you retire a rule ID, and after you edit the text in `main.go`.

| Command | Use |
| --- | --- |
| `mise run rules-doc` | Writes `docs/rules.md`. |
| `mise run rules-doc:check` | Fails when `docs/rules.md` is outdated and writes nothing. |
| `mise run generate` | Runs `go generate` (which fetches data from GitHub) and then this generator. |

An outdated page fails in each of these places:

- The hk step `rules_doc` of `hk.pkl`. `hk check --all` (`mise run lint`, the `lint` job of CI and the pre-push hook) runs the check, and `hk fix --all` (`mise run lint-fix`) runs the generator.
- The test `TestRulesDocIsUpToDate`, which `go test ./scripts/generate-rules-doc` and `go test ./...` run.
- The task `mise run rules-doc:check`.

## Prerequisites

Go. The tool uses no network and no other program.

## Usage

Run the tool from the repository root, because the default output path `docs/rules.md` is relative to the working directory.

```
generate-rules-doc [-check] [dstfile|-]
```

| Argument | Meaning | Default |
| --- | --- | --- |
| `-check` | Compares `dstfile` with what the registry produces and writes nothing. It must come first. | off |
| `dstfile` | The file to write or to check. `-` writes to stdout. | `docs/rules.md` |

The tool exits with status 1 for an outdated file, a bad argument or a failed write. The message of an outdated file names the command that fixes it. The check compares with LF line endings, so a Windows checkout with CRLF passes.

```sh
go run ./scripts/generate-rules-doc
go run ./scripts/generate-rules-doc -check
```

## What it produces

The page has these parts, in order:

1. A comment that says the page is generated.
2. An introduction and a legend. They define the group, the default level, the profile, the fix and the option `pedantic`, say which rules skip a job or step whose `if:` is `false`, and show a sample config file. This text is fixed in `main.go`, together with its links to other pages of the documentation.
3. A table with one row per rule and the columns ID, Summary, Group, Default level, Profile and Fixable. The ID links to the section of the rule, and Fixable says `yes` or `no`. The Profile column says `only when configured` for a rule that no profile enables and ``only with `--online` `` for an online rule.
4. A section per rule. The heading is the rule ID, which makes the ID the public anchor `https://jactionlint.jdx.dev/rules#<rule-id>` that released binaries print. The section starts with the summary and lists the group, the default level and the profile. An online rule adds `Needs network access: yes`, and a fixable rule adds `Fixable: yes`. Each rule option is a bullet with its name, kind, default and summary. When the rule has a `DocsAnchor`, the last bullet links to that section of [the checks page](../../docs/checks.md).
5. The table of retired rule IDs, with the columns Retired ID, Rule and Findings are on with. It comes from `RenamedRules()`: the old ID, the rule that took over its findings, and the option that turns those findings on, or `always` when there is none.

The rows and sections come from `RuleInfo` in the registry, and the retired IDs from `RenamedRules()`. Everything else is text in `main.go`. To change the row or the section of a rule, edit its `RuleInfo` in the file of the rule. To change the introduction, the legend or the explanation of the retired IDs, edit `generate` in `main.go`.

## Pitfalls

- The links in the text of `main.go` point at other pages of the documentation. VitePress checks that a page exists, and `mise run docs:build` then runs `docs/scripts/verify-docs.mjs`, which fails on a link to a fragment that no page defines. Run it when a page or a heading moves.
- A released rule ID is never renamed or removed, because it is an anchor of the page, a config key and part of the ignore syntax. The table of retired IDs comes from `RenamedRules()` in the registry.
- The page lists the rules sorted by ID, the order of `Rules()`, so a new rule appears in its alphabetical place.

## Tests

`go test ./scripts/generate-rules-doc` runs these tests:

- `TestRulesDocIsUpToDate` compares `docs/rules.md` with the output of the generator.
- `TestRulesDocMentionsEveryRule` checks that every rule has a section and that extra arguments are an error.
- `TestRunWritesFile` and `TestRunCheck` check writing to a file, writing to a missing directory and the `-check` mode.

## Next steps

- [The rules reference](../../docs/rules.md): the generated page.
- [Contributing guide](../../CONTRIBUTING.md): how to add a rule and which tests and generated files it touches.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
