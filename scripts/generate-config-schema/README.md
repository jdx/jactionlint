# generate-config-schema

generate-config-schema writes the three JSON Schemas of jactionlint into [schema/](../../schema): one for the config file, one for the output of `--format json` and one for the document of `--export-facts`. Run it after you add or change a rule, a rule option or a config key, and commit the result.

| File | Describes | Built from |
| --- | --- | --- |
| `jactionlint-config.json` | The config file `.github/jactionlint.yaml`. | The rule IDs and the options of each rule come from the rule registry of the `jactionlint` package. The other keys are written in [config_schema.go](../../config_schema.go). |
| `jactionlint-output.json` | The findings of `--format json` and `--format jsonl`. | `OutputSchema` in [config_schema.go](../../config_schema.go). |
| `jactionlint-facts.json` | The document of `--export-facts`. | `FactsSchema` of the `jactionlint` package. |

`jactionlint --print-config-schema`, `jactionlint --print-output-schema` and `jactionlint --print-facts-schema` print the same three documents.

## When to run it

Run it whenever the registry or the config keys change, because the rule IDs and rule options are part of the schema. The schema is not part of `mise run generate`, so `mise run generate` does not refresh it.

| Command | Use |
| --- | --- |
| `mise run schema` | Writes the three files. |
| `mise run schema:check` | Fails when a file is outdated and writes nothing. |

An outdated file fails in each of these places:

- The hk step `config_schema` of `hk.pkl`. `hk check --all` (`mise run lint`, the `lint` job of CI and the pre-push hook) runs the check, and `hk fix --all` (`mise run lint-fix`) runs the generator.
- The test `TestSchemasAreUpToDate` in [config_schema_test.go](../../config_schema_test.go), which `go test .` runs.
- The task `mise run schema:check`.

## Prerequisites

Go. The tool uses no network and no other program.

## Usage

Run the tool from the repository root, because the default output directory `schema` is relative to the working directory.

```
generate-config-schema [-check] [dstdir]
```

| Argument | Meaning | Default |
| --- | --- | --- |
| `-check` | Compares the files with what the registry produces, writes nothing and exits with status 1 for an outdated file. It names the file and the command that fixes it. | off |
| `dstdir` | The directory that holds the three files. | `schema` |

The flag has one dash only: `--check` is a usage error. A Windows checkout with CRLF line endings is compared as if it had LF.

```sh
go run ./scripts/generate-config-schema
go run ./scripts/generate-config-schema -check
```

## How it works

The tool calls `ConfigSchema`, `OutputSchema` and `FactsSchema` of the `jactionlint` package in the working tree and writes the three documents as indented JSON in the JSON Schema draft-07 format. The config schema has an entry for every rule ID with the options of that rule, so editors complete and check them.

The published location of the config schema is the constant `ConfigSchemaURL`, the raw URL of the file on the `main` branch. `jactionlint --init-config` writes a `# yaml-language-server: $schema=` comment with that URL at the top of the config file, so editors that read the comment use the schema without any registration. A schema change reaches those editors when it reaches `main`.

## Publishing to SchemaStore

This repository does not register the schema in SchemaStore. An entry there gives `.github/jactionlint.yaml` the schema in editors that do not read the comment:

1. Fork [SchemaStore/schemastore](https://github.com/SchemaStore/schemastore) and add an entry to `src/api/json/catalog.json` with `name: "jactionlint"`, a `description`, `fileMatch: [".github/jactionlint.yaml", ".github/jactionlint.yml"]` and `url` set to the raw URL of `schema/jactionlint-config.json`. Use a tag in the URL, such as `.../vX.Y.Z/schema/...`, if you prefer a stable version to `main`.
2. Follow the contribution guide of SchemaStore for the checks that it runs on a catalog entry. They change over time.
3. Open the pull request and wait for the review. Update `url` when the schema moves.

## Pitfalls

- Edit the text of the schemas in Go, not in the JSON. The description of a rule comes from its `RuleInfo` (summary, group, profile, default level and documentation URL), the description of a rule option from its `RuleOption`, and the descriptions of the other keys are strings in `config_schema.go`.
- A schema change reaches editors when it reaches `main`, because the published URL is the raw file of that branch. Review the diff of `schema/` like code.

## Tests

The generator has no tests of its own. `TestSchemasAreUpToDate` compares the files in `schema/` with the generated documents, and the other tests in `config_schema_test.go` check the content of the schemas, for example that they know every rule, option and key of the parser:

```sh
go test -run 'TestSchemasAreUpToDate|TestConfigSchema' .
```

## Next steps

- [Configuration reference](../../docs/config.md): the keys and rule options that the config schema describes.
- [Output formats](../../docs/output.md): the fields that the output schema describes.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
