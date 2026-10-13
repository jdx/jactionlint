generate-popular-actions
========================

This is a script for generating [`popular_actions.go`](../../popular_actions.go).

It does:

- Fetches metadata of popular actions
  - from https://github.com
  - from JSONL file in local
- Generates the fetched data set of metadata
  - as Go source file
  - as JSONL file

## Usage

Generate Go source:

```sh
go run ./scripts/generate-popular-actions ./popular_actions.go
```

Detect new releases on GitHub:

```sh
go run ./scripts/generate-popular-actions -d
```

Detect new releases and register them in the data source file (adds the new tag to `tags` and advances `next`).
This is what the `generate` job of the [release-please workflow](../../.github/workflows/release-please.yaml) runs, so that detecting a new major
version does not fail the workflow. Without `-w` the exit status is 2 when a new release is detected:

```sh
go run ./scripts/generate-popular-actions -d -w -r scripts/generate-popular-actions/popular_actions.json
```

Please see output of `-help` flag for more details.

## What is generated

`popular_actions.go` has three maps: `PopularActions` (the inputs and outputs of each known version), `OutdatedPopularActionSpecs` (the
versions that run on a runtime older than `node20`, which are not kept in the first map) and `PopularActionRuntimes` (the `runs.using` of
every other version that runs JavaScript, such as `"actions/checkout@v4": "node20"`). The rule `deprecated-node-runtime` reads the last one.

## The data source file

The data source of the popular actions is defined in [`popular_actions.json`](./popular_actions.json). This file contains an array
of each action registry. Each registry is a JSON object containing the following keys:

| Key            | Description                                                     | Example                    | Required? |
|----------------|-----------------------------------------------------------------|----------------------------|-----------|
| `slug`         | GitHub repository slug                                          | `"actions/checkout"`       | Yes       |
| `tags`         | Known release tags                                              | `["v1", "v2", "v3", "v4"]` | Yes       |
| `next`         | The next release tag. Empty means new version won't be detected | `"v5"`                     | No        |
| `path`         | Absolute path to the action from the repository root            | `"/path/to/action"`        | No        |
| `skip_inputs`  | Skipping checking inputs of this action or not                  | `true`                     | No        |
| `skip_outputs` | Skipping checking outputs of this action or not                 | `true`                     | No        |
| `file_ext`     | File extension of action metadata file. The default is `"yml"`  | `"yaml"`                   | No        |

Alternative actions registry JSON file can be used via `-r` option.
