generate-config-schema
======================

This is a script for generating the JSON Schemas of jactionlint into [`schema/`](../../schema):

- `jactionlint-config.json`: the config file `.github/jactionlint.yaml`. The rule IDs and the options of each rule come from
  the rule registry of the `jactionlint` package, and the other keys are written in `config_schema.go`.
- `jactionlint-output.json`: the findings of `--format json` and `--format jsonl`.

`jactionlint --print-config-schema` and `--print-output-schema` print the same documents. A test (`TestSchemasAreUpToDate`)
and `hk check` fail when a file is outdated.

## Usage

```
generate-config-schema [-check] [dstdir]
```

Regenerate `schema/` after adding or changing a rule, a rule option or a config key:

```sh
mise run schema
```

Check without writing:

```sh
mise run schema:check
```

## Publishing to SchemaStore

The schema is served from `main` by the URL in `jactionlint.ConfigSchemaURL`, which `--init-config` puts in a
`# yaml-language-server: $schema=` comment, so editors work without any registration. To also get the schema
for `.github/jactionlint.yaml` without the comment, add an entry to SchemaStore (this is not done by this repository):

1. Fork <https://github.com/SchemaStore/schemastore> and add an entry to `src/api/json/catalog.json` with
   `name: "jactionlint"`, `description`, `fileMatch: [".github/jactionlint.yaml", ".github/jactionlint.yml"]` and
   `url` set to the raw URL of `schema/jactionlint-config.json` (a tag, e.g. `.../v2.1.0/schema/...`, if a stable
   version is preferred over `main`).
2. Follow the contribution guide of SchemaStore for the checks it runs on a catalog entry (it changes over time).
3. Open the pull request and wait for the review. Update `url` when the schema moves.
