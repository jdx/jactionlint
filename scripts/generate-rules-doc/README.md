generate-rules-doc
==================

This is a script for generating [`docs/rules.md`](../../docs/rules.md), the reference of all rule IDs.

It reads the rule registry of the `jactionlint` package (`Rules()`) and writes one section per rule ID with its group,
default level, profile and options. The registry is the single source of truth: add or change rules there, then
regenerate the document. A test fails when `docs/rules.md` is outdated.

## Usage

```
generate-rules-doc [dstfile|-]
```

Generate `docs/rules.md`:

```sh
go run ./scripts/generate-rules-doc
```
