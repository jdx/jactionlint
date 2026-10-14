# generate-popular-actions

generate-popular-actions writes [popular_actions.go](../../popular_actions.go), the data set of the inputs, outputs and runtimes of popular actions, from the registry [popular_actions.json](./popular_actions.json) and the `action.yml` files of the actions on GitHub. It also detects new major releases of the registered actions. Run it when you add a popular action or a release of one.

jactionlint reads the data set to check the inputs of popular actions ([Popular action inputs validation](../../docs/checks.md#check-popular-action-inputs)), to report outdated versions ([Outdated popular actions detection](../../docs/checks.md#detect-outdated-popular-actions)) and to find removed Node.js runtimes ([the rule `deprecated-node-runtime`](../../docs/checks.md#check-deprecated-node-runtime)).

## When to run it

- When you add an action or a tag to the registry: edit `popular_actions.json`, then regenerate the Go file.
- `go generate` runs it through the directive in [action_metadata.go](../../action_metadata.go), and `mise run generate` runs `go generate` and then the generator of `docs/rules.md`.
- The `generate` job of [release-please.yaml](../../.github/workflows/release-please.yaml) runs it in every run of the workflow: on a push to `main`, on a weekly schedule and by hand. It first detects and registers new releases, then runs `go generate`, and it commits the result to the release pull request, and only while one is open. A new release does not fail the job, because `-w` registers it instead of exiting with status 2, although a failed request still does. The output of the detection goes into the commit message and a comment of the pull request.

## Prerequisites

- Go.
- Network access to `raw.githubusercontent.com`, unless you read the metadata from a JSONL file with `-s`.

## Usage

Run the tool from the repository root. The flags have one dash, as the flag package of Go writes them.

```
go run ./scripts/generate-popular-actions [FLAGS] [FILE]
```

FILE is the output file. Without it the tool writes to stdout.

| Flag | Meaning | Default |
| --- | --- | --- |
| `-s FILE` | Reads the metadata from a JSONL file instead of fetching it from GitHub. The name must end in `.jsonl`. | fetch |
| `-f FORMAT` | The format of the output: `go` or `jsonl`. | `go` |
| `-r FILE` | Uses this registry file instead of the embedded `popular_actions.json`. | the embedded file |
| `-d` | Detects new releases instead of generating. It prints the URLs of the new releases. | off |
| `-w` | With `-d`, registers the detected releases in the registry file of `-r`. It needs `-d` and `-r`. | off |
| `-q` | Silences the log on stderr. | off |

| Exit status | Meaning |
| --- | --- |
| 0 | Success, including `-d` that found nothing and `-d -w` that registered releases. |
| 1 | An error: bad flags, an unreadable file, a failed request or a file that does not parse. |
| 2 | `-d` without `-w` found a new release. |

Generate the Go source:

```sh
go run ./scripts/generate-popular-actions ./popular_actions.go
```

Detect new releases on GitHub:

```sh
go run ./scripts/generate-popular-actions -d
```

Detect new releases and register them in the registry file, which adds the new tag to `tags` and advances `next`. This is the command that the `generate` job runs:

```sh
go run ./scripts/generate-popular-actions -d -w -r scripts/generate-popular-actions/popular_actions.json
```

Save the metadata once and regenerate the Go file from it without the network. This helps when you work on the generator itself:

```sh
go run ./scripts/generate-popular-actions -f jsonl /tmp/popular_actions.jsonl
go run ./scripts/generate-popular-actions -s /tmp/popular_actions.jsonl ./popular_actions.go
```

`-h` prints the full usage.

## What is generated

The tool fetches `https://raw.githubusercontent.com/SLUG/TAG/PATH/action.EXT` for every tag of every registry entry and parses it. The first request or file that fails stops the run, so a bad tag never produces a partial output. The Go file has these maps, sorted by spec (`owner/repo@tag`):

| Map | Content |
| --- | --- |
| `PopularActions` | The name, inputs and outputs of each known version, except the outdated ones. |
| `OutdatedPopularActionSpecs` | The versions that are outdated. |
| `PopularActionRuntimes` | The `runs.using` of every other version that runs JavaScript, such as `"actions/checkout@v4": "node20"`. The rule `deprecated-node-runtime` reads it. |

A version is outdated when its `runs.using` is a Node.js runtime older than `node20` (the constant `minNodeRunnerVersion`), or when it is one of the versions that `action.yml` cannot reveal as outdated: `actions/labeler@v1`, `actions/checkout@v1`, `actions/upload-artifact@v1` and `actions/download-artifact@v1`. The list `outdatedActions` in `main.go` holds them.

### Detect new releases

For every registry entry with a non-empty `next`, the tool sends a HEAD request for the `action.yml` of that tag. A 404 means that the release does not exist yet, and any other answer outside 2xx is an error. With `-w` the tool rewrites the registry file in place and keeps its formatting. It appends the detected tag to `tags` and sets `next` to the following major version: a detected `v5` joins `tags` and `next` becomes `v6`. It checks that the result is valid JSON before it writes the file.

A tag that is not a plain major version such as `v7` cannot be advanced automatically. The tool lists it as `Not added automatically, update FILE by hand: SPEC`, and you edit the entry.

## The registry file

[popular_actions.json](./popular_actions.json) is an array of entries. Each entry is a JSON object with these keys:

| Key | Description | Example | Required |
| --- | --- | --- | --- |
| `slug` | The GitHub repository of the action. | `"actions/checkout"` | yes |
| `tags` | The known release tags. | `["v1", "v2", "v3", "v4"]` | yes |
| `next` | The next release tag. When it is empty, the tool does not look for a new release. | `"v5"` | no |
| `path` | The path of the action inside the repository, starting with `/`. | `"/save"` | no |
| `skip_inputs` | Whether jactionlint skips the check of the inputs of this action. | `true` | no |
| `skip_outputs` | Whether jactionlint skips the check of the outputs of this action. | `true` | no |
| `file_ext` | The extension of the metadata file. | `"yaml"` | no, the default is `"yml"` |

Use `skip_inputs` for an action that accepts inputs that its `action.yml` does not list, and `skip_outputs` for an action that sets outputs dynamically. `TestDefaultPopularActions` checks the embedded file: every entry has a slug, and no slug and path pair appears twice. Every entry has a tag, and a tag is neither empty, duplicated nor equal to `next`. The extension is `yml`, `yaml` or empty.

To add a popular action, add an entry with its `slug`, its `tags` and its `next`, then run the first command of Usage.

## Pitfalls

- The registry is embedded in the executable. `go run` builds from the file on disk, so an edit applies at once. A built binary keeps the file it was built with, and `-r` replaces it.
- A tag such as `v4` can move to a newer commit of its action, so the output can change without a change of the registry. Read the diff of `popular_actions.go` before you commit it.
- `main.go` patches the metadata of a few actions in place: an input or an output that works but is missing from `action.yml`, and a parse error that the `action.yml` of one action causes by a typo. Each patch has a comment with its issue.

## Tests

`go test ./scripts/generate-popular-actions` checks the registry, the generated Go and JSONL output, reading JSONL back, and the detection and registration of releases. The tests of the registration of releases replace the HTTP transport with a fake one, but `TestFetchRemoteYAML`, `TestWriteOutdatedActionAsJSONL`, `TestDetectNewRelease`, `TestDetectNoRelease` and `TestCouldNotFetch` request `raw.githubusercontent.com`, so the test run needs network access.

## Next steps

- [Popular action inputs validation](../../docs/checks.md#check-popular-action-inputs): what jactionlint does with the data.
- [Contributing guide](../../CONTRIBUTING.md): the other generated files and the steps to refresh them.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
