# Scripts and generated files

The tools in this directory generate files of the repository, check that those files are current, and measure jactionlint against other tools. Find your tool in the table, or look up the change you made to see what to run.

Run every command from the root of the repository. `mise install` installs the tools that the commands need.

## The tools

Each tool has its own directory and README, which has the flags, the prerequisites and the details. The last column says who runs the tool besides you.

| Tool | Writes or checks | Command | Run by |
| --- | --- | --- | --- |
| [check-checks](./check-checks/README.md) | The `Output:` blocks and the playground links of the examples in `docs/checks.md`, and that they are current | `go run ./scripts/check-checks -fix ./docs/checks.md` writes. `go run ./scripts/check-checks -quiet ./docs/checks.md` checks. | hk step `checks_doc` |
| [fix-corpus](./fix-corpus/README.md) | Applies `--fix` to copies of real repositories and checks that the result is valid YAML, that a second run changes nothing and that no rule of jactionlint or zizmor reports more findings | `mise run fix-corpus` | You |
| [generate-availability](./generate-availability/README.md) | `availability.go`: the contexts and special functions that each workflow key can use. It fetches the contexts reference of github/docs. | `go generate -run generate-availability` | `generate` job of `release-please.yaml` |
| [generate-config-schema](./generate-config-schema/README.md) | `schema/jactionlint-config.json` (the config file, with the rule IDs and options of the registry) and `schema/jactionlint-output.json` (`--format json` and `jsonl`) | `mise run schema` writes. `mise run schema:check` checks. | hk step `config_schema`, and `TestSchemasAreUpToDate` |
| [generate-event-payloads](./generate-event-payloads/README.md) | `event_payloads.go`: the properties of `github.event` of each webhook event. It fetches the payload schemas of octokit/webhooks and the OpenAPI description of the GitHub REST API. | `go generate -run generate-event-payloads` | `generate` job of `release-please.yaml` |
| [generate-jactionlint-matcher](./generate-jactionlint-matcher/README.md) | `.github/jactionlint-matcher.json`, the problem matcher for the text output, and the test data of its generator | `mise run matcher` writes the matcher. `mise run matcher:fixtures` updates the test data. `node ./scripts/generate-jactionlint-matcher/test.mjs` runs the test. | `matcher.yaml` |
| [generate-popular-actions](./generate-popular-actions/README.md) | `popular_actions.go`: the inputs, outputs and runtimes of popular actions, from `scripts/generate-popular-actions/popular_actions.json` and GitHub. With the detection flags it finds new major releases and registers them in the JSON file. | `go generate -run generate-popular-actions` generates. `go run ./scripts/generate-popular-actions -d -w -r scripts/generate-popular-actions/popular_actions.json` detects. | `generate` job of `release-please.yaml` |
| [generate-rules-doc](./generate-rules-doc/README.md) | `docs/rules.md`, from the rule registry. The prose of the page is in the generator. | `mise run rules-doc` writes. `mise run rules-doc:check` checks. | hk step `rules_doc`, and `TestRulesDocIsUpToDate` |
| [generate-webhook-events](./generate-webhook-events/README.md) | `all_webhooks.go`: the webhook events and their activity types. It fetches the page about the events that trigger workflows on docs.github.com. | `go generate -run generate-webhook-events` | `generate` job of `release-please.yaml` |
| [zizmor-diff](./zizmor-diff/README.md) | A report of the zizmor findings that jactionlint also reports, over the repositories of `scripts/zizmor-diff/corpus.txt` or the directories that you pass with `--repos` | `mise run zizmor-diff` | You |

`go generate` runs the tools that write Go files (`generate-popular-actions`, `generate-event-payloads`, `generate-webhook-events` and `generate-availability`, in this order) and fetches their data from GitHub, so it needs the network. `go generate -n` prints the commands without running them. `mise run generate` runs `go generate` and then `generate-rules-doc`. It does not run `mise run schema`.

### Who else runs the tools

- **hk.** `hk check --all` (`mise run lint`) runs the steps `checks_doc`, `config_schema` and `rules_doc` among others, and the CI job `lint` runs it. `hk fix --all` (`mise run lint-fix`) rewrites the files that these steps find outdated. [Lint](../CONTRIBUTING.md#lint) lists every step and the git hooks.
- **`release-please.yaml`.** Its `generate` job runs on every push to `main` and every Saturday. While a release pull request is open, it runs the detection of `generate-popular-actions` and `go generate` on the branch of that pull request and pushes the changed files there. Without an open release pull request it does nothing, so nothing else refreshes the generated Go files between releases. [How a release runs](../CONTRIBUTING.md#how-a-release-runs) has the steps of a release.
- **`go test`.** The CI job `unit-tests` runs `go test -race ./...`, which includes the Go tests of the tools in this directory that have them and the tests that fail on an outdated generated file.

## What to run after a change

Find the row of your change, run its commands and commit the files that they rewrite. [Open a pull request](../CONTRIBUTING.md#open-a-pull-request) has the steps of a whole pull request.

| You changed | Run | It rewrites or checks |
| --- | --- | --- |
| Any Go code | `mise run test`, then `hk fix --all` and `hk check --all` | The tests that CI runs, formatting, vet, staticcheck, the workflows of this repository, and the generated files of the next rows |
| A rule: its ID, summary, group, level, profile or options | `hk fix --all`, or `mise run rules-doc` and `mise run schema` | `docs/rules.md` and `schema/*.json` |
| The group, level or profile of a rule | `go test -run TestRuleProfilesSnapshot -update-profiles .` | `testdata/profiles.txt` |
| A new rule | `mise run rule-ids NAME`, and the other steps of [Add a rule](../CONTRIBUTING.md#add-a-rule) | `testdata/rule_ids.d/NAME.txt` |
| A section or an example of `docs/checks.md` | `go run ./scripts/check-checks -fix ./docs/checks.md`, then `mise run docs:test` | The `Output:` blocks and the playground links, then the playground tests that read them |
| The prose of `docs/rules.md` | Edit `scripts/generate-rules-doc/main.go`, then `mise run rules-doc` | `docs/rules.md`. Edits made to the page itself are lost. |
| Another page of `docs/` | `mise run docs:build` | The build of the site. It fails on a dead link, a lost anchor and a dead `#fragment`. |
| A key of the config file, a rule option or a field of the JSON output | `mise run schema`, then `go test -run TestConfigSchema .` | `schema/*.json`, and the check that the example of `docs/config.md` validates |
| An option of the command line (`cliOptions` in `command_options.go`) | Edit `docs/cli.md` and `man/jactionlint.1.ronn`, then `go test -run TestCLIOptionsAreDocumented .` | Nothing is generated: both files are written by hand and the test checks that they name the option |
| The text output format | `mise run matcher:fixtures`, then `mise run matcher` | The test data of the generator and `.github/jactionlint-matcher.json` |
| The fix of a rule | `mise run fix-corpus` | A report on copies of the repositories in `~/src/*-jactionlint` and `~/src/mise`, or of the directories that you pass (see the README of the tool) |
| Code that parses or scans a whole file | `mise run bench`, `go test -run TestScalingBudget .` | The benchmarks, and the test that fails when the time grows faster than the size of the file |
| An action of `popular_actions.json` | Edit `scripts/generate-popular-actions/popular_actions.json`, then `go generate -run generate-popular-actions` | `popular_actions.go` |
| A webhook event, a context or the shape of `github.event` | `go generate` | `all_webhooks.go`, `availability.go` and `event_payloads.go`, from the current GitHub documents |
| GitHub changed the workflow syntax (`TestSchemaConformance` fails) | `mise run schema-pin [COMMIT]` | `testdata/schema/runner/`. The workflow `schema-pin.yaml` does it every week and opens a pull request. |
| `man/jactionlint.1.ronn` | `mise run man` | `man/jactionlint.1`, which git ignores: see [Build the manual](../CONTRIBUTING.md#build-the-manual) |
| The Go code of the playground | `mise run docs:wasm`, `mise run docs:test` and `mise run lint:wasm` | `docs/public/playground/main.wasm` and `wasm_exec.js`, which git ignore, the playground tests and staticcheck of `./playground` |
| `scripts/download-jactionlint.bash` | `./scripts/test-download-jactionlint.bash` | Nothing: the test downloads real releases. The workflow `download.yaml` runs it on three operating systems. |
| A workflow or `.github/dependabot.yml` of this repository | `hk check --all -S jactionlint` | The check that the files pass the default profile |

## Scripts without a README

These files have no README of their own.

| Script | What it does |
| --- | --- |
| `action.bash` | The step of the action `jdx/jactionlint` (`action.yml`) that builds the command line from the `JACTIONLINT_*` environment variables and runs jactionlint. The workflow `action.yaml` tests it. |
| `build-man.bash` | Builds `man/jactionlint.1` from `man/jactionlint.1.ronn`: `mise run man`. [Build the manual](../CONTRIBUTING.md#build-the-manual) has the details, including why it stops in a git worktree. |
| `build-wasm.bash` | Builds the playground: `main.wasm` from `./playground` and the `wasm_exec.js` of the Go toolchain, into `docs/public/playground/`. `mise run docs:wasm` runs it, and the tasks `docs:dev`, `docs:build` and `docs:test` depend on it. |
| `download-jactionlint.bash` | The download script for users: `bash scripts/download-jactionlint.bash [[VERSION] DIR]` puts the `jactionlint` executable of a release into `DIR` (default: the current directory). The environment variables `JACTIONLINT_REPO` and `JACTIONLINT_NAME` choose another repository and executable name. The release workflow updates its default version after each release and runs it as a check. |
| `test-download-jactionlint.bash` | The test of the download script, which downloads real releases. The workflow `download.yaml` runs it. |
| `touch.ps1` | A PowerShell version of `touch`. Nothing in the repository calls it. |
| `update-schema-pin.bash` | Pins the workflow schemas of actions/runner that `TestSchemaConformance` compares the parser with: `mise run schema-pin [COMMIT]`. It needs `gh` and `jq`. The workflow `schema-pin.yaml` runs it every week. |
| `yaml-to-playground-url.js` | Reads a workflow from stdin and prints its playground link, without the comment lines: `node ./scripts/yaml-to-playground-url.js < workflow.yaml`. |

## Next steps

- [CONTRIBUTING.md](../CONTRIBUTING.md): the whole pull request, from the setup to the release.
- [The README of check-checks](./check-checks/README.md): the format of a section of the checks page.
- [The README of fix-corpus](./fix-corpus/README.md): the flags of the tool that checks fixes on a corpus.
- [Fuzz the parsers](../fuzz/README.md): how to fuzz the parsers.
