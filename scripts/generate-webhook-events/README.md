# generate-webhook-events

generate-webhook-events writes [all_webhooks.go](../../all_webhooks.go), the table of every webhook event that can trigger a workflow and the activity types of each. It reads the HTML page of the GitHub documentation about events that trigger workflows. The check [Webhook events validation](../../docs/checks.md#check-webhook-events) uses the table to report an unknown event or an activity type that the event does not have.

## When to run it

Run it when GitHub adds an event or an activity type, or when you change the script. You rarely start it by hand:

- `go generate` runs it through the directive in [rule_events.go](../../rule_events.go), and `mise run generate` runs `go generate` and then the generator of `docs/rules.md`.
- The `generate` job of [release-please.yaml](../../.github/workflows/release-please.yaml) runs `go generate` in every run of the workflow: on a push to `main`, on a weekly schedule and by hand. The job commits the result to the release pull request, and only while one is open.

## Prerequisites

- Go.
- Network access to `docs.github.com`, unless you pass a saved copy of the page.

## Usage

Run the tool from the repository root.

```
generate-webhook-events [[srcfile] dstfile]
```

| Arguments | Source | Output |
| --- | --- | --- |
| none | fetched from GitHub | stdout |
| `dstfile` | fetched from GitHub | the file |
| `srcfile dstfile` | the local HTML file | the file |

A `dstfile` of `-` writes to stdout. More than two arguments are a usage error, and every failure exits with status 1. The tool replaces `dstfile` atomically: it writes a temporary file next to it and renames it, and it touches nothing when the parsing fails, so a failed refresh never empties the existing file.

```sh
go run ./scripts/generate-webhook-events ./all_webhooks.go
```

Read a saved copy of the HTML page instead of fetching it:

```sh
go run ./scripts/generate-webhook-events ./input.html ./all_webhooks.go
```

Print the result to stdout to compare it with the file:

```sh
go run ./scripts/generate-webhook-events ./input.html -
```

## How it works

1. The tool fetches [the page](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows). It is the rendered HTML page, not the Markdown source of github/docs.
2. It parses the HTML, finds the article body and the heading `About events that trigger workflows`, and reads the table that follows each later `h2` heading. The heading names the event, and the table must say `Webhook event payload` and `Activity types` in its header cells.
3. It writes the map `AllWebhookTypes` from the name of an event to its activity types as Go source, sorted by name so that the file is stable.

How the tool reads the cell of the activity types:

- The `code` elements of the cell are the activity types.
- `Not applicable` becomes an empty slice, `[]string{}`: the event has no activity types.
- `Custom` becomes `nil`: the types are arbitrary, as for `repository_dispatch`.
- Any other text without `code` elements stops the tool with an error.

### Types that the documentation lacks

Some activity types are valid but missing from the page. They are listed in `extraTypes` in `main.go`, each with its source, and the tool adds them to the scraped types, so that regenerating does not drop them:

- `stacked` for `pull_request` and `pull_request_target`.
- `created`, `ready` and `deleted` for `image_version`, whose cell says `Not applicable`.

There is no entry for `merge_group`: the page and the workflow schemas list only `checks_requested`. Add an entry only with a source that shows that GitHub accepts the type, such as the workflow schema of the runner or of the language service. The tool fails when the event of an entry is missing from the page, which tells you that the entry is stale.

## Pitfalls

- The parser depends on the structure of the page: the `data-search="article-body"` and `markdown-body` containers, the `id` of the headings and the `aria-labelledby` and header cells of the tables. When GitHub restructures the page, the tool stops with a message that names the missing part, and you must adapt the parser.
- The header of the generated file names `./scripts/generate-webhook-events`, and the page `docs/api.md` links to this directory on GitHub, which renders this README. Keep the directory and this file.

## Tests

`go test ./scripts/generate-webhook-events` generates the table from `testdata/ok.html` and compares it with `testdata/ok.go`. It feeds the other HTML files of `testdata`, each of which breaks the page in one way, to the parser and expects the matching error. It also checks the `extraTypes` mechanism and that a failed run leaves the existing output untouched. One test, `TestFetchURL`, fetches `https://github.com`, so the test run needs network access. The other tests use local data.

## Next steps

- [Webhook events validation](../../docs/checks.md#check-webhook-events): what the check reports.
- [Contributing guide](../../CONTRIBUTING.md): the other generated files and the steps to refresh them.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
