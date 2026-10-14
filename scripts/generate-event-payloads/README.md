# generate-event-payloads

generate-event-payloads writes [event_payloads.go](../../event_payloads.go), which holds the shape of the payload of each webhook event: the properties of `github.event` that a workflow can read. The check [Contextual typing for `github.event` object](../../docs/checks.md#check-contextual-github-event) uses [event_payload.go](../../event_payload.go) to report a property that does not exist.

The shapes say which properties an object has. They do not say anything about the values: a value is a string, a number, `null` or anything, and nothing more is known about it.

## When to run it

Run it when GitHub adds an event or a property, or when the check reports a property that GitHub does send. You rarely start it by hand:

- `go generate` runs it through the directive in [event_payload.go](../../event_payload.go).
- The `generate` job of [release-please.yaml](../../.github/workflows/release-please.yaml) runs `go generate` in every run of the workflow: on a push to `main`, on a weekly schedule and by hand. The job commits the result to the release pull request, and only while one is open.

The output is sorted and names the commit of octokit/webhooks and the version of the OpenAPI description, so a pull request that changes the file shows what changed upstream.

## Prerequisites

- Go.
- Network access to `codeload.github.com` and `raw.githubusercontent.com`, unless you pass local copies of both sources.

## Usage

Run the tool from the repository root.

```
generate-event-payloads [[octokit-schema-dir openapi.json] dstfile]
```

| Arguments | Sources | Output |
| --- | --- | --- |
| none | fetched from GitHub | stdout |
| `dstfile` | fetched from GitHub | the file |
| `octokit-schema-dir openapi.json dstfile` | local copies | the file |

The tool accepts exactly 0, 1 or 3 arguments. A `dstfile` of `-` writes to stdout. The output file is replaced only after the whole generation succeeded, so a failed run never empties it.

```sh
go run ./scripts/generate-event-payloads ./event_payloads.go
```

With local sources, `octokit-schema-dir` is `payload-schemas/api.github.com` of a checkout of octokit/webhooks, and `openapi.json` is `descriptions/api.github.com/api.github.com.json` of a checkout of github/rest-api-description:

```sh
go run ./scripts/generate-event-payloads ./webhooks/payload-schemas/api.github.com ./api.github.com.json ./event_payloads.go
```

Print the result to stdout to compare it with the file:

```sh
go run ./scripts/generate-event-payloads ./webhooks/payload-schemas/api.github.com ./api.github.com.json -
```

A run with local sources writes `(local)` where the header of the file names the commit of octokit/webhooks, so it differs from a run with fetched sources in that line.

## How it works

1. The tool reads two sources, both under the MIT licence: the JSON schemas of the webhook payloads of [octokit/webhooks](https://github.com/octokit/webhooks) (the directory `payload-schemas/api.github.com` of a tarball of the `main` branch) and [the OpenAPI description of the GitHub REST API](https://github.com/github/rest-api-description), whose `x-webhooks` are the webhooks.
2. It converts the schema of every action of an event, such as `opened` and `closed` of `pull_request`, into a shape and merges them. A property is known when any action of any source has it.
3. It reduces the shapes:
   - An object deeper than `maxLevels` is not looked into. The payload is level 1, the objects in it level 2, and so on.
   - The properties of `looseNames` (such as `changes` and `inputs`) are untyped, and those of `accountNames` (such as `sender` and `user`) are known but not looked into. Both lists are in [schema.go](./schema.go).
   - `extraProps` in `main.go` adds properties that GitHub sends but the sources lack.
4. It writes the shapes as text into two variables of the generated file. `eventPayloads` has the shape of each event under the name of the event in `on:`. Events with the same payload share one entry: `pull_request_target` is `=pull_request`. `eventPayloadObjects` has the objects that many shapes contain, written once and referred to by name.

The table `events` in `main.go` is an input of the tool, not part of its output. It maps the name of an event in `on:` to its name in the sources. An event that is not in the table is untyped: nothing of `github.event` is checked for a workflow that has it. `workflow_call`, `workflow_dispatch`, `repository_dispatch` and `schedule` are not in it on purpose, because their payload is the caller's, arbitrary or without a schema.

The entry `*` of `eventPayloads` is the union of the payloads of all the events. The check uses it when the option `strict-event-payloads` of `undefined-property` is off, which is the default except under the `pedantic` profile. Then a workflow for `push` that reads `github.event.pull_request.number` is not reported, and only a property that no event has is. `universeExtra` in `main.go` adds `inputs`, `client_payload` and `schedule` to it.

## Pitfalls

- `maxLevels`, `accountNames`, `looseNames` and `extraProps` decide how many false findings the check reports. Lower the levels or extend the lists when a real workflow is reported for a property that GitHub sends.
- A new event type needs an entry in `events`. Add it, regenerate and check that the new payload is reasonable.
- The shapes lag behind GitHub. A property that the sources lack is reported until you add it to `extraProps` or the sources catch up.

## Tests

`go test ./scripts/generate-event-payloads` generates the output from small sources written in the tests and compares it with the golden file `testdata/event_payloads.go`. After you change the generator on purpose, `go test ./scripts/generate-event-payloads -update` rewrites the golden file. The tests also check that the output is deterministic, that the shapes are reduced as described, that bad input fails, and that a failed run leaves an existing output untouched. The tests need no network.

## Next steps

- [Contextual typing for `github.event` object](../../docs/checks.md#check-contextual-github-event): what the check reports and the option `strict-event-payloads`.
- [Contributing guide](../../CONTRIBUTING.md): the other generated files and the steps to refresh them.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
