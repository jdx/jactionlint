generate-jactionlint-matcher
===========================

This script generates [`jactionlint-matcher.json`](../../.github/jactionlint-matcher.json).

## Usage

```sh
mise run matcher
```

or directly run the script

```sh
node ./scripts/generate-jactionlint-matcher/main.mjs .github/jactionlint-matcher.json
```

## Test

```sh
node ./scripts/generate-jactionlint-matcher/test.mjs
```

The test uses test data at `./scripts/generate-jactionlint-matcher/test/*.txt`. They should be updated when jactionlint changes
the default error message format. To update them:

```sh
mise run matcher:fixtures
```
