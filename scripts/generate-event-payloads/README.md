generate-event-payloads
=======================

This is a script for generating [`event_payloads.go`](../../event_payloads.go), which holds the shape of the payload of
each webhook event: the properties of `github.event` which a workflow can read. [`event_payload.go`](../../event_payload.go)
uses it to report a property which does not exist (see [the check](../../docs/checks.md#check-contextual-github-event)).

It does:

1. Fetch the JSON schemas of the webhook payloads of [octokit/webhooks](https://github.com/octokit/webhooks) (the
   directory `payload-schemas/api.github.com` of a tarball of the `main` branch) and
   [the OpenAPI description of the REST API of GitHub](https://github.com/github/rest-api-description), whose
   `x-webhooks` are the webhooks. Both are under the MIT licence
2. Convert the schema of every action of an event (`pull_request` `opened`, `closed`, ...) of each source into a shape
   and merge them: a property is known when any of them has it
3. Reduce the shapes: objects deeper than `maxLevels` and the properties of `looseNames` and `accountNames` (see
   `schema.go`) are not looked into, and `extraProps` (see `main.go`) adds the properties which GitHub sends but the
   sources lack
4. Write the shapes of the events in the table `events` of `main.go`, and the shape of all the events together (the key `*`),
   as text. Objects which many shapes have are written once and referred to by name

The shapes only say which properties an object has. The types of the values are not looked at: a value is a string, a number,
`null` or anything, and nothing is known of it.

## Usage

```
generate-event-payloads [[octokit-schema-dir openapi.json] dstfile]
```

Generate `event_payloads.go` file:

```sh
go run ./scripts/generate-event-payloads ./event_payloads.go
```

When the sources are local, `octokit-schema-dir` is `payload-schemas/api.github.com` of a checkout of octokit/webhooks and
`openapi.json` is `descriptions/api.github.com/api.github.com.json` of a checkout of rest-api-description:

```sh
go run ./scripts/generate-event-payloads ./webhooks/payload-schemas/api.github.com ./api.github.com.json ./event_payloads.go
```

For debugging, specifying `-` to `dstfile` outputs the generated source to stdout:

```sh
go run ./scripts/generate-event-payloads ./webhooks/payload-schemas/api.github.com ./api.github.com.json -
```

## Notes

- The output is sorted, and it has the commit of octokit/webhooks and the version of the OpenAPI description, so that a
  change of it in a pull request shows what changed upstream. The file is small (about 40 KB of source), because the
  shapes are limited to `maxLevels` levels and shared objects are written once
- A new event type needs an entry in `events`. An event which has no entry is not typed: nothing of `github.event` is
  checked for a workflow that has it
- `maxLevels`, `accountNames`, `looseNames` and `extraProps` decide how many false reports the check makes. Lower the
  levels or add to the lists when a real workflow is reported for a property which GitHub sends
