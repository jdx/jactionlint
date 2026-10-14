# Contributing to jactionlint

This guide shows how to report a bug, propose a check and send a pull request to jactionlint. It also holds the procedures of the maintainers: adding a rule, writing the checks page, editing the site, releasing and refreshing generated files.

To change something, start with [Set up](#set-up) and [Find the command for your change](#find-the-command-for-your-change). To propose a new check, read the [policy](#policy-for-jactionlints-features) first.

**Contents**

- [Report a bug or propose a check](#report-a-bug-or-propose-a-check)
- [Set up](#set-up)
- [Find the command for your change](#find-the-command-for-your-change)
- [Build, test and lint](#build-test-and-lint)
- [Open a pull request](#open-a-pull-request)
- [Policy for jactionlint's features](#policy-for-jactionlints-features)
- [Add a rule](#add-a-rule)
- [Write the checks page](#write-the-checks-page)
- [Edit the site and the playground](#edit-the-site-and-the-playground)
- [Cut a release](#cut-a-release)
- [Generated files](#generated-files)

<a id="reporting-an-issue"></a>
## Report a bug or propose a check

Open an [issue](https://github.com/jdx/jactionlint/issues/new) for a bug, a false positive or an idea for a check. Search the existing issues first, closed ones included.

A report that can be acted on holds:

- the smallest workflow file (or `dependabot.yml` or `action.yml`) that shows the problem, pasted as text
- the command line you ran, the config file if one applies, and the profile
- the output you got and the output you expected
- the output of `jactionlint --version`
- for an online rule, the mode of `--online` that you used (never paste a token)

A report without input that reproduces the problem gets a lower priority, because it takes time to investigate.

To propose a check, describe the construct, show a workflow that the check reports and one that it leaves alone, and name the group and the profile you expect. The [policy](#policy-for-jactionlints-features) decides whether a check is accepted.

<a id="development"></a>
## Set up

You need git and [mise](https://mise.jdx.dev/). mise installs the tools that `mise.toml` pins and runs the tasks of the repository.

```sh
git clone https://github.com/jdx/jactionlint.git
cd jactionlint
mise install
mise run build
./jactionlint --version
```

`mise install` provides Go, node (the LTS release), hk, pkl, shellcheck, uv, pyflakes, staticcheck and govulncheck. The `man` task installs the Ruby that it needs when you run it.

- **Go.** Builds, tests and govulncheck use the toolchain that `go.mod` selects. The Go entry of `mise.toml` is pinned for staticcheck, which cannot read the export data of a newer toolchain: read the comment above it before you change it.
- **Missing tools.** A test that needs shellcheck, pyflakes or a `node` that supports the github-script check skips itself when the tool is missing, so a green run without them proves less. `mise install` provides all three.
- **Windows.** CI runs the unit tests, the tests of the action and the tests of the download script on Windows. `mise run test` leaves out `-race` there, because `-race` needs cgo. The tool that maintains `docs/checks.md` runs on Linux and macOS only.

`mise tasks` lists every task. These are the ones you use while you develop:

| Task | What it does |
| --- | --- |
| `mise run build` | Builds `./jactionlint` without cgo. |
| `mise run test` | Runs `go test -race ./...`. |
| `mise run cov` | Writes `coverage.html` and prints the coverage per function. |
| `mise run bench` | Runs the benchmarks (`go test -bench Lint -benchmem`). |
| `mise run lint` | Runs `hk check --all`: the checks that CI runs. |
| `mise run lint-fix` | Runs `hk fix --all`: the same checks, fixing what can be fixed. |
| `mise run lint:wasm` | Runs staticcheck on `./playground` with `GOOS=js`. |
| `mise run fuzz [TARGET]` | Runs go-fuzz (see [Fuzz](#fuzz)). |
| `mise run docs:wasm`, `docs:dev`, `docs:build`, `docs:preview`, `docs:test` | Build the playground and run, build, preview and test the site. |
| `mise run clean` | Removes build outputs. |

The tasks that write generated files or check a corpus (`generate`, `rules-doc`, `schema`, `rule-ids`, `schema-pin`, `matcher`, `man`, `fix-corpus` and `zizmor-diff`) are in [the tool index](scripts/README.md#the-tools) and in [what to run after a change](scripts/README.md#what-to-run-after-a-change).

## Find the command for your change

[What to run after a change](scripts/README.md#what-to-run-after-a-change) lists the commands for each kind of change, from a Go file to the man page. Find the row of your change, run its commands and commit the files that they rewrite.

Whatever you change, run `mise run test` and `mise run lint` before you open a pull request, because CI runs both. `hk fix --all` rewrites `docs/rules.md`, the JSON Schemas and the outputs of `docs/checks.md` in one command. It leaves the changes unstaged unless you add `--stage`.

## Build, test and lint

These three steps are the development loop: build the binary, run the tests and lint the repository.

<a id="building"></a>
### Build

```sh
mise run build
```

The task builds `./jactionlint` with `CGO_ENABLED=0`, because jactionlint uses no cgo features and the build then avoids linking libc. `go build ./cmd/jactionlint` does the same when you set the variable yourself. The build does not regenerate the generated Go files: see [Generated files](#generated-files).

<a id="testing"></a>
### Test

```sh
mise run test
```

The task runs `go test -race ./...`. Run one test with `go test -run TestName .`, and the coverage report with `mise run cov`. CI runs `go test -v -race ./...` on Linux, macOS and Windows.

| Where | What it tests |
| --- | --- |
| `*_test.go` next to the code | The API, with data in `testdata/`. |
| `linter_test.go` | The fixtures: it lints every file of `testdata/ok/`, `err/`, `examples/` and `projects/` and compares the findings with the `.out` files. |
| `testdata/err/` | A workflow (`NAME.yaml`) and the findings it must produce (`NAME.out`, one per line). |
| `testdata/ok/` | Workflow files that must cause no finding. |
| `testdata/examples/` | Inputs and findings of many examples of the checks page. The page itself is verified by `check-checks` and `mise run docs:test`, not by these files. |
| `testdata/projects/` | One directory per repository, for cases where several files relate: reusable workflows, local actions, config files. An empty `.out` file means no finding. |
| `testdata/policy/`, `fix/`, `nofix/`, `actions/`, `online/` | The goldens of one area each: see [Test the rule](#test-the-rule). |
| `testdata/positions/` | A fixture with non-ASCII text for `positions_test.go`. It has no golden file. |
| `schema_conformance_test.go` | The tables of the parser and the rules against `workflow-v1.0.json` and `action_yaml.json` of actions/runner. |
| `positions_test.go` | That every region of every fixture is valid in the SARIF log and the JSON output, and that a finding in a multi-line scalar sits on the line and column of its text. |
| `perf_test.go` | The time budgets and the benchmarks. |
| `robustness_test.go` | Pathological inputs: huge or deeply nested files, odd encodings and line breaks, aliases that expand. It fails on a panic or a hang. |

The conformance test runs offline: the schemas are vendored in `testdata/schema/runner` at the commit that `COMMIT` names. A difference fails the test unless `schemaConformanceAllowed` lists it with a reason, and an entry that is no longer needed fails the test too. The `schema-pin.yaml` workflow moves the pin every week and opens a pull request, whose test run shows the syntax that GitHub added. `mise run schema-pin` does the same by hand. Follow the new syntax in the parser, or add the entry with the reason.

<a id="linting"></a>
### Lint

```sh
mise run lint       # hk check --all
mise run lint-fix   # hk fix --all
```

`hk check --all` runs these steps, which are defined in `hk.pkl`:

| Step | What it checks |
| --- | --- |
| `go_fmt`, `go_vet` | Formatting and `go vet`. |
| `shellcheck` | The scripts `scripts/*.bash`. |
| `jactionlint` | The workflows and `dependabot.yml` of this repository, with the default profile. |
| `staticcheck`, `go_vuln_check` | staticcheck on `./`, `./scripts/...`, `./cmd/...` and the playground, and govulncheck. |
| `rules_doc`, `config_schema`, `checks_doc` | That `docs/rules.md`, the JSON Schemas and the outputs of `docs/checks.md` are up to date. |

`hk fix --all` fixes what can be fixed: formatting, the fixable findings of the workflows, and the generated files of the last row. Run one step with `hk check --all -S rules_doc`.

Run staticcheck through hk. The raw commands are `GOTOOLCHAIN=local staticcheck ./ ./scripts/... ./cmd/...` and `mise run lint:wasm`. They run on the Go that mise installs, and they avoid `./...` because `docs/node_modules` can hold Go packages. Plain `staticcheck ./...` reports an internal error about the export data version.

To run the fast steps before every commit, and every step plus the tests before every push, install the git hooks once:

```sh
hk install
```

The pre-commit hook stashes your unstaged changes and runs `go_fmt`, `go_vet`, `shellcheck` and `jactionlint` with fixes on, so it can rewrite files. The pre-push hook runs every step and `go test ./...` without the race detector.

<a id="fuzzing"></a>
### Fuzz

Fuzz tests feed random input to a parser, a scanner or a fixer to find panics and broken invariants. Native Go fuzz tests are the `Fuzz...` functions of `*_test.go` files, and their seeds in `testdata/fuzz/` run as part of `go test`, so CI runs the seeds and does not fuzz. Fuzz one target for a while with:

```sh
go test -run '^$' -fuzz '^FuzzCommandLine$' -fuzztime 30s .
```

Go fuzzes one target per run. A failing input is written to `testdata/fuzz/<target>/`: commit it with the fix as a regression test.

The `fuzz/` package holds the go-fuzz targets, which `mise run fuzz [TARGET]` runs. That task fails on a clean checkout, and two native tests, `FuzzUsesPattern` and `FuzzAnalyze`, fail within seconds. [Fuzz the parsers](fuzz/README.md) has the workaround, the open failures, the list of targets and what to do with a crasher.

<a id="sending-a-patch"></a>
## Open a pull request

Open a [pull request](https://github.com/jdx/jactionlint/pulls) against `main`. The title follows [Conventional Commits](https://www.conventionalcommits.org/), because squash-merged titles become the release notes:

```text
<type>[(scope)][!]: <description>
```

- The types are `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `security`, `style` and `test`.
- The scope is optional and holds letters, digits, `.`, `/`, `_` and `-`. A `!` marks a breaking change.
- The description starts with a lowercase letter or with an acronym of two or more capitals, such as `CLI`. A title such as `feat: Add a check` is rejected.
- `feat`, `fix`, `perf`, `security` and `revert` get a section in `CHANGELOG.md`. The other types stay out of it.

An example of a title: `feat(actions-lock): lint GitHub's Actions lock file against the workflows`. The `conventional-commits` check runs again when you edit the title.

Before you open the pull request:

1. Run `mise run test` and `mise run lint`. They pass on your branch.
2. Add tests for new behavior: a fixture under `testdata/` for a rule or an output, a unit test for an API. A new public Go API also gets a doc comment, and a line on [the Go API page](docs/api.md) when users of the package need it.
3. Describe a user-visible change on the page of `docs/` that owns the behavior. [`docs/README.md`](docs/README.md) lists the pages.
4. For a new or changed rule, put the table of the changes of `testdata/profiles.txt` in the description: the rule, its group, its profile and its level before and after. Reviewers check the profile and the level there.
5. If a new rule is on in the example configuration of the checks page, it can change the outputs of other examples. `mise run lint` then fails with the diff: see [Write the checks page](#write-the-checks-page).

CI runs these workflows on a pull request:

| Workflow | Runs for | What it checks |
| --- | --- | --- |
| `ci.yaml` | Every pull request | Unit tests on Linux, macOS and Windows, builds without cgo and with `-race`, and a lint of this repository with the built binary and the problem matcher. The jobs are listed after the table. |
| `docs-build.yaml` | Changes under `docs/**`, `playground/**`, `man/**`, `scripts/**/README.md`, the root Go files, `README.md`, `CONTRIBUTING.md`, `NOTICE.md`, `go.mod`, `go.sum`, `mise.toml`, `scripts/build-wasm.bash` and the workflow itself | `mise run docs:build` and `mise run docs:test`. |
| `action.yaml` | Changes to `action.yml` and `scripts/action.bash` | The action on three operating systems, with a build of the commit. |
| `download.yaml` | Changes to the download scripts | `scripts/test-download-jactionlint.bash` on three operating systems. |
| `matcher.yaml` | Changes to the problem matcher generator and to `testdata/err/*.out` and `testdata/examples/*.out` | The matcher test, and that `.github/jactionlint-matcher.json` is up to date. |
| `conventional-commits.yaml` | Every pull request | The format of the title. |
| `codeql.yaml` | Pull requests to `main` | The CodeQL analysis of the Go code and of the TypeScript of the site. |

The jobs of `ci.yaml` besides the unit tests:

- `Wasm` runs `mise run docs:test`.
- `Lint` runs `hk check --all`.
- `Dockerfile` builds the image, runs it on the repository and lints the Dockerfile with hadolint.
- `final` gathers the results of the other jobs for the branch protection.

## Policy for jactionlint's features

Every rule belongs to one group, and its profile decides whether it runs for someone who configured nothing. The two are separate decisions. The group says what the rule is about. [Profiles](docs/config.md#profiles) defines the profiles, and this section says where a new check goes and when it is accepted.

| Group | What its rules find |
| --- | --- |
| `correctness` | Workflows that are broken or do not do what they say: syntax, types, bad inputs. |
| `security` | Workflows that are exploitable or weaken the supply chain: injection, unsafe triggers, unverified downloads. |
| `policy` | Practices that are not mistakes by themselves: explicit `permissions:`, timeouts, pinned actions. |
| `style` | Code style only. |

The group follows what the check detects, not how noisy it is. A noisy security check is still a security rule: it lives in the `pedantic` profile.

### Profiles: what runs without configuration

A repository that configures nothing gets the `default` profile. Each profile includes the one before it: `correctness`, `default`, `pedantic`. Put a check where it belongs:

| It goes in | When the check |
| --- | --- |
| `correctness` | Finds something that is wrong whatever the taste of the project: the workflow fails or misbehaves, or actionlint reports it. It has essentially no false positives. |
| `default` | Finds an exploitable or weakening construct, or breaks a convention that is worth failing a build on, and the [review of false positives](#accepting-a-new-check) shows none. Everything that the regular persona of zizmor reports belongs here unless the review says otherwise. |
| `pedantic` | Is noisy, opinionated or informational: it has a real false-positive rate, reports what many projects do on purpose, or matters only to a security review. The pedantic and auditor personas of zizmor go here. |
| No profile | Needs configuration to mean anything (`forbidden-uses`, `required-actions`, `timeout-too-long`). It runs only when the configuration enables it. |
| No profile, online | Needs the network. It runs only when the online checks are on (`--online`, `online: true` in the config file or `LinterOptions.Online` in the API), whatever the profile. |

The `correctness` profile holds only rules of the `correctness` group, except `template-injection` and `hardcoded-container-credentials`, which actionlint reports too, and `expired-ignore` and `unused-baseline-entry`, which act only on the ignores and the baseline that the user wrote. `testdata/profiles.txt` records the profile and the level of every rule, one line per rule: the ID, the profile (`online` or `configured` for a rule without profile), the group and the level.

These invariants keep the profiles simple:

- **One ID per check.** The noisier findings of a check are the same rule ID with the option `pedantic`, which is true under the `pedantic` profile and false otherwise. Do not add an ID with a suffix such as `-expansion` or `-trusted`.
- **Level `error` only.** Every rule of `correctness` and `default` reports at level `error`, except `unused-baseline-entry`, which is `info` and which only `--baseline-check` produces. A rule that should not fail a build belongs to `pedantic` with its own level. `TestRuleProfilesInvariants` enforces this and the content of `correctness`.
- **A fix never invents a value.** A fixer that needs a number of minutes or days offers the fix only when a rule option supplies the value.

### Accepting a new check

A pull request that adds a check, or a request for one, is accepted when all of the following hold:

- **Stable ID.** The check has a stable, documented rule ID that is never renamed. For a security check, reuse the name of the equivalent [zizmor audit](docs/zizmor-parity.md) when there is one, so users can map the two.
- **Group and profile.** The check states its group and why, and the profile that enables it. `testdata/profiles.txt` records the profile and the level, so a new rule or a changed profile shows in the diff of the pull request and gets reviewed.
- **Docs section.** The [checks page](docs/checks.md) has a section for it with an example, the output and a playground link: see [Write the checks page](#write-the-checks-page). The rule appears in the generated [rules reference](docs/rules.md) through its `RuleInfo`.
- **Golden tests.** Tests under `testdata/` show what the check reports and what it intentionally leaves alone.
- **False-positive review.** The check ran on a corpus of real repositories, a set of well-known open source projects and not only your own, and every finding was reviewed. Put the corpus, the numbers and what you did not measure in the description of the pull request. When a zizmor audit matches, [`scripts/zizmor-diff`](scripts/zizmor-diff/README.md) compares the two tools, and its `--repos` option takes your checkouts: the corpus file of the repository lists checkouts on the machine of the maintainer. `testdata/realworld/dataset.zip` is a public set of workflow files.
- **Default only with essentially no false positives.** A check is in the `default` profile only when the review shows essentially none. Anything noisier goes to `pedantic`, whatever its group. A rule with no finding on the corpus has no measured false-positive rate, which is not the same as zero: say so, and prefer `pedantic` until a real case exists.
- **Configuration only when it is the point.** A policy check may need options, such as an allow list of actions. A correctness check does not need configuration to be useful.

A check that cannot meet these points, such as a personal style preference with no way to turn it off, is generally not accepted.

### Rules and rule IDs

Every finding has a stable rule ID in kebab-case, such as `unpinned-uses`, which the rule reports with `ReportID`. Configuration files, ignore comments and CI annotations name these IDs, so a released ID is never renamed or removed. `TestRuleIDsAreStable` fails when an ID that was recorded is no longer registered. [`docs/rules.md`](docs/rules.md) lists every rule with its group, level, profile and options.

<a id="adding-a-rule"></a>
## Add a rule

A rule is self-contained: it registers itself from an `init` function in its own file, so adding one edits neither `linter.go` nor `rule_registry.go`. This section is the procedure from the idea to a green pull request, with the tests that fail when you skip a step. Do the steps in order, and skip the sections on fixes and online rules when your rule has neither.

### Decide the group and the profile

Read the [policy](#policy-for-jactionlints-features) and choose the group and the profile. A rule of the `correctness` or `default` profile also runs on the workflows of this repository, which must pass the default profile.

### Write the rule

Put the rule in `rule_<name>.go`. Embed `RuleBase`, create it with `NewRuleBase` and report with `ReportID`. `ReportRange` also tells where the region ends; without it, the linter fills the end with the end of the token that starts at the position. [`rule_duplicate_triggers.go`](rule_duplicate_triggers.go) is a small example.

Register the rule at the end of the file:

```go
func init() {
	// One RuleInfo per ID that the rule can report. Registering an ID twice panics.
	registerRules(RuleInfo{
		ID: "my-rule", Group: RuleGroupPolicy, Summary: "A one-line description ending with a period.",
		DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-my-rule",
	})
	// How to create the rule for one file. Return nil when the rule is not needed, or call env.Skip(reason)
	// to log why it is disabled. env has the config, the project, the caches and the shellcheck and
	// pyflakes processes.
	registerRuleFactory("my-rule", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("my-rule") {
			return nil
		}
		return []Rule{NewRuleMyRule()}
	})
}
```

Keep the `RuleEnabled` gate. A factory without it creates the rule for every file, even when the rule is off, and the instance appears in `testdata/format/test.sarif`, which lists the rules that a run creates: `TestLinterFormatErrorMessageInSARIF` fails. The gate also saves the work of a rule that is off.

Rules run in a fixed order: the rules of `legacyRuleOrder` first, then all others sorted by name (`TestRuleFactoryOrderIsStable`). The legacy order decides which of two findings at the same position comes first, so it is frozen. Do not add your rule to it.

### Register the rule

| `RuleInfo` field | Meaning |
| --- | --- |
| `ID` | The kebab-case rule ID, unique and never renamed. |
| `Group` | `RuleGroupCorrectness`, `RuleGroupSecurity`, `RuleGroupPolicy` or `RuleGroupStyle`. |
| `Summary` | One line that ends with a period. `--list-rules`, `--explain` and `docs/rules.md` print it. |
| `DefaultLevel` | The level when the rule is enabled without an explicit one. |
| `Profile` | The first profile that enables the rule. Leave it empty for a rule that runs only when configured, and for an online rule. |
| `Online` | `true` for a rule that needs the GitHub API: it runs only with the online checks. |
| `Fixable` | `true` for a rule that attaches a fix (`Error.Fix`). `TestEveryRuleWhichSetsAFixIsMarkedFixable` lints the test data and fails when a finding has a fix but its rule is not marked. |
| `DocsAnchor` | The anchor of the section in `docs/checks.md`. `--explain` prints that section. |
| `Options` | The options of the rule under `rules:`. The config file validation, the JSON Schema and `docs/rules.md` take them from here. |

### Scope the rule to composite actions

Decide whether the rule applies to the `action.yml` files of composite actions. Add the name of its factory to `actionRuleScope` in `rule_action_scope.go`, and name the IDs of the rule in the composite actions section of `docs/checks.md`:

- `actionApplies` for a rule that checks steps.
- `actionCallerDependent` when the result depends on the events that trigger the calling workflows. Read them with `Workflow.TriggerEvents`, not `Workflow.On`.
- `actionNotApplicable` for a rule about workflow or job settings.

A rule without an entry does not run for actions. The `except` list of an entry names IDs that the rule does not report for actions.

### Rules with a fix

- A rule that fixes a finding sets `Error.Fix` and `Fixable: true`. A fix is a list of byte-range `TextEdit` values with a `Description`, and the edits of one fix must not overlap. `Unsafe` marks a fix that may change the behavior of the workflow: `--fix` applies the safe fixes, and `--fix=unsafe` also applies the others. Unsafe fixes are not in the SARIF output.
- When a finding could be fixed but the fix cannot be offered, set `Error.NoFix` with a `NoFixCode`. `option-required` names the rule option that supplies the missing value, and `online-required` says the fix needs the online checks. The [policy](#profiles-what-runs-without-configuration) forbids inventing a value.
- Never build YAML by concatenating strings. `RenderYAMLValue` renders a whole new value of a mapping entry: plain when that reads back as the same string, else double quoted. `YAMLSiteAt` and `YAMLSite.Insert` insert text into a scalar that exists, escaped for plain, flow, single quoted, double quoted and block scalars, or refuse it.
- `--fix` verifies every pass: the result must be valid YAML that differs only where the edits are. A fixer that breaks a file is refused and reported, but write a test that applies the fix and lints again anyway. The verification compares parsed trees, and it takes a node to end where the next node starts. It therefore accepts a difference next to an edit even when another edit caused it, and it cannot tell that an edit put a wrong value in the right key: assert the fixed text in the test. It compares anchors by the aliases that use them, not by name.
- When the edits of your fix can overlap with those of another rule, add the ID to `fixPriority` in `fix.go`. The rule with the lower number claims the text first, and the other fix is retried in the next pass, when its finding is reported again against the new text or is gone. The order is the map `fixPriority` in `fix.go`: fixes that remove a security problem first, then fixes that change values, then fixes that add metadata, then every rule that is not listed in alphabetical order, and `unused-ignore` last. A safe fix always goes before an unsafe one. [What fixing guarantees](docs/fix.md#what-fixing-guarantees) describes the passes.
- Run `mise run fix-corpus` on your own checkouts: it applies the safe fixes to a copy of each, and checks that the result is valid YAML, that a second run changes nothing and that no rule, jactionlint or zizmor, reports more findings. The default corpus lives on the machine of the maintainer, so build the binary and pass directories: `go build -o jactionlint ./cmd/jactionlint`, then `go run ./scripts/fix-corpus -jactionlint ./jactionlint DIR`. [`scripts/fix-corpus/README.md`](scripts/fix-corpus/README.md) has the flags.

### Online rules

An online rule has `Online: true` and no `Profile`. Its factory takes the online session from `env.online`, which is nil unless the online checks are on, and returns nothing when it is nil. It never talks to the network in tests: fixtures named `*_online` run with the answers of `testdata/online/github.json`, served by `NewFixtureGitHubClient`, and a lookup that the file does not hold fails the test, except the lookup of a file, which answers "not found". Add the repositories that your rule needs to the file. `NewRecordingGitHubClient` records the answers of the real client in the same format: trim the tag lists by hand.

An example in the checks page for an online rule starts with the line `# requires --online`, so that `scripts/check-checks` uses the same answers.

### Keep the rule fast and robust

A rule must take time linear in the size of the file. Parse once per file and look positions up through an index. Do not parse the whole source, scan it from the start or walk the whole syntax tree once for each finding or step.

- `sourceIndex` answers lines, byte offsets and `sites()` for `YAMLSiteAt` of many offsets, and `valuePosition` gives positions inside a multi-line scalar.
- `editSet` finds conflicts between edits.
- `TestScalingBudget` lints generated workflows of growing size and fails when the time grows faster than the size. `mise run bench` runs benchmarks at several sizes per workload, and `BenchmarkLintRealWorldCorpus` runs over `testdata/realworld/dataset.zip`.
- Add a native fuzz test for a parser, a scanner or a fixer (see [Fuzz](#fuzz)).

### Test the rule

Put the unit tests of the rule in `rule_<name>_test.go`, next to the rule file. Write the fixtures first and try the rule from the command line:

```sh
go build -o jactionlint ./cmd/jactionlint
./jactionlint --select=my-rule test.yaml
./jactionlint --list-rules
./jactionlint --explain my-rule
```

`--select` runs one rule whatever the profile. `--explain` prints the section of `docs/checks.md`, which is embedded in the binary, so rebuild after you edit the page. `node ./scripts/yaml-to-playground-url.js < test.yaml` prints a playground permalink for a file.

The fixtures of `testdata/err/` and `testdata/examples/` follow these conventions:

- `NAME.out` has one finding per line, in the order of the output: `test.yaml:LINE:COLUMN: message [kind]`. The kind is the name of the rule that reports the finding, which is not always its rule ID (`expression` for `template-injection`, `timeout-check` for `missing-timeout`). A line written as `/regexp/` matches by regular expression.
- A file `NAME.config` next to `NAME.yaml` holds the configuration of the fixture.
- A fixture named `RULE_ID` or `RULE_ID_example`, with dashes written as underscores, runs with that rule on. This is how a rule of the `default` or `pedantic` profile runs, because the tests that lint with the implicit profile use `correctness` (`TestMain` sets it). A test of a `default` rule calls `withDefaultProfile(t)` or sets `profile:` in its config.
- A name that ends with `_online` runs the online checks with the answers recorded in `testdata/online/github.json` and `GITHUB_REPOSITORY=example/environments`. `_security` turns on `unpinned-uses`, `_require_permissions` turns on `missing-permissions` and `_checkout` turns on `local-action-checkout`. `_asof_YYYY_MM_DD` lints as of that day, for checks that depend on a date.
- A name that contains `shellcheck`, `pyflakes` or `github_script` runs only when that tool exists.

Other areas keep their goldens elsewhere. Write a golden with the flag of the test, then review the diff:

| Directory | What it holds | Flag that rewrites it |
| --- | --- | --- |
| `testdata/policy/` | `NAME.yaml`, `NAME.out`, `NAME.cfg.yaml` and `ok/` for the permission, pinning and checkout rules. | `-update-policy-golden` |
| `testdata/fix/` | `NAME.yaml` and the fixed result `NAME.fixed.yaml`. | `-update-fix-goldens` |
| `testdata/nofix/` | The JSON and SARIF output for a workflow whose findings have no fix, with the `NoFix` reason of each. | `-update-nofix-goldens` |
| `testdata/actions/` | Cases of `action.yml` files. | `-update-action-goldens` |
| `testdata/online/` | The recorded GitHub answers and the online reports. | `-update-online-report-goldens` |

```sh
go test -run TestPolicyGolden -update-policy-golden .
```

A position is a line and a column counted in code points. `TestSARIFRegionsAreValid` checks every region of every fixture, so give a rule that sets positions of its own a fixture with non-ASCII text.

### Record the snapshots

1. `mise run rule-ids NAME` runs `go test -run TestRuleIDsAreStable -update-rule-ids=NAME .`, which writes the IDs that no file of `testdata/rule_ids.d/` lists to `testdata/rule_ids.d/NAME.txt`. Give each batch of rules its own name, so parallel work never touches the same file.
2. `go test -run TestRuleProfilesSnapshot -update-profiles .` writes `testdata/profiles.txt`. The rule appears as a line in the diff, which is where reviewers check its profile and level.
3. `mise run schema` regenerates the JSON Schemas, which list the rule ID and its options.
4. `mise run rules-doc` regenerates `docs/rules.md`, sorted by ID.

`hk fix --all` runs steps 3 and 4 and the outputs of the checks page in one go.

### Document the rule

Add a section to [`docs/checks.md`](docs/checks.md) with the anchor of the `DocsAnchor`, and follow [Write the checks page](#write-the-checks-page). Name the IDs of the rule in backticks, and add the IDs to the composite actions section, as [Scope the rule to composite actions](#scope-the-rule-to-composite-actions) says.

### What fails when you skip a step

| Test or check | Fails when | Fix |
| --- | --- | --- |
| `TestRuleRegistryIsWellFormed` | The ID is not kebab-case or not unique, the group or the profile is invalid, the summary does not end with a period, an online rule has a profile, or the `DocsAnchor` is not in `docs/checks.md`. | Correct the `RuleInfo` or add the section. |
| `TestEveryRuleCanBeExplained` | `--explain` cannot print the section of the `DocsAnchor`. | Add the section with the anchor. |
| `TestRuleIDsAreStable` | A registered ID is in no file of `testdata/rule_ids.d/`, or a recorded ID is no longer registered. | `mise run rule-ids NAME`. Never remove an ID. |
| `TestRuleIDsInSourceMatchRegistry` | A rule reports an ID that is not registered, or a registered ID is never reported. | Register the ID, or report it. |
| `TestRuleProfilesSnapshot` | `testdata/profiles.txt` differs from the registry. | `go test -run TestRuleProfilesSnapshot -update-profiles .` |
| `TestRuleProfilesInvariants` | A rule of `correctness` or `default` does not report `error`, or a rule of `correctness` is not of that group, outside the documented exceptions. | Change the level, the profile or the group. |
| `TestActionScopeIsComplete` | `actionRuleScope` has no entry for the factory. | Add the entry. |
| `TestActionScopeIsDocumented` | The composite actions section of `docs/checks.md` does not name the ID in backticks. | Add the ID to that section. |
| `TestSchemasAreUpToDate` | `schema/*.json` differ from the registry. | `mise run schema` |
| `TestRulesDocIsUpToDate` | `docs/rules.md` differs from the registry. | `mise run rules-doc` |
| `TestLinterFormatErrorMessageInSARIF` | The factory creates the rule although the rule is off. | Add the `RuleEnabled` gate. |
| `TestEveryRuleWhichSetsAFixIsMarkedFixable` | A finding has a fix but `Fixable` is false. | Set `Fixable: true`. |
| `TestOwnWorkflowsPassTheDefaultProfile` and the `jactionlint` step of hk | A new rule of the default profile reports a finding on a workflow of this repository. | Fix the workflow: pin every action to a commit SHA with a version comment, and set `permissions`, `timeout-minutes` and `concurrency`. |
| The `checks_doc` step of hk | An output or a link of `docs/checks.md` is outdated. | `go run ./scripts/check-checks -fix ./docs/checks.md` |

A complete pull request for a new rule contains the rule file, its tests and fixtures, the new line of `testdata/profiles.txt`, a file of `testdata/rule_ids.d/`, the regenerated JSON Schemas and `docs/rules.md`, the section of the checks page with its outputs, and a description with the profile table.

<a id="how-to-write-checks-document"></a>
<a id="about-checks-doc"></a>
## Write the checks page

[`docs/checks.md`](docs/checks.md) documents every check with examples, the output of jactionlint for each example and a link that opens the example in the playground. A tool writes the outputs and the links, `jactionlint --explain` prints one section of the page from the binary, and several tests read the page, so some rules of layout are contracts. This section holds the policy for examples and the contracts that tests enforce. [The README of check-checks](scripts/check-checks/README.md) owns the exact format of a section, its error messages and how the tool works.

### The layout of a section

A section is an anchor on its own line, a `##` heading, prose and at least one example. An example is the line `Example input:` with a `yaml` fence, the line `Output:` with a fence and a `[Playground](...)` line:

````markdown
<a id="check-my-rule"></a>
## Heading that names the problem

Prose that says what the check reports.

Example input:

```yaml
THE EXAMPLE, WITH AT LEAST ONE FINDING
```

Output:

```
THE TOOL WRITES THIS BLOCK
```

[Playground](THE TOOL WRITES THIS LINK)
````

[The format of a section](scripts/check-checks/README.md#the-format-of-a-section) specifies each part. These points cause most failures:

- **Anchor.** The id is unique and is the `DocsAnchor` of the rule. Several anchor lines in a row may name one section.
- **Heading.** Its text is unique. The start of the text chooses the kind of file of the example (`Dependabot`, `Composite action`, or a workflow for anything else), so renaming such a heading silently changes how its example is linted.
- **Example.** The input is not empty and makes jactionlint report at least one finding. The lines `Example input:` and `Output:`, the fences and the `[Playground](...)` line are exact: tools and tests find them by their text. Keep the blank lines of the layout.
- **Prose.** The rest of the section is free. Name the rule IDs that the section covers in backticks. Use inline links only, never `[text][ref]`, and never write "above" or "below": `--explain` prints one section alone.

### Examples the tool maintains

`go run ./scripts/check-checks -fix ./docs/checks.md` lints every example input, replaces the `Output:` block with what jactionlint prints and replaces the `[Playground](...)` line with a permalink. Without `-fix` the tool fails with a diff when the page is outdated. What you write depends on the rule that the example shows:

| The rule that the example shows | What you write |
| --- | --- |
| Runs under the example configuration: a rule of `correctness` or `default` that the configuration does not turn off | The example, then the generated output and link. |
| Does not run under it: a rule of `pedantic`, an opt-in rule, a rule that needs configuration | An `Example config:` block with the `rules:` that enable it, a `# select: id[,id]` first line when other rules would add findings, the generated output and `<!-- Skip playground link -->`. |
| Needs several files, or a message names a file of the repository | `Example file: PATH` blocks with the files, `# file: PATH` to name the example (for a workflow other than `test.yaml`), the generated output and `<!-- Skip playground link -->`. |
| Is an online rule | `# requires --online` as the first line of the input, the generated output and `<!-- Skip playground link -->`. The tool uses the recorded answers of `testdata/online/github.json`, and the playground has no network. |

The tool lints each example in a temporary repository that has the `Example file:` blocks and nothing else, with the example configuration and the `Example config:` block merged over it. [The README of check-checks](scripts/check-checks/README.md#examples-that-need-another-configuration-rules-or-files) specifies the markers and the blocks. The playground cannot reproduce such an example, so it keeps `<!-- Skip playground link -->`, and `docs/tests/wasm.test.ts` fails when a link does not reproduce its output.

Never write the output of the checks page by hand. A skipped output is checked by nothing and goes stale when a message changes, so `TestChecksPageSkipsNoOutput` fails on `<!-- Skip update output -->`. When an example needs something that the tool cannot set up, extend the tool and its README. Do not repeat the config in a second block of the prose: the copy is not checked.

Every example is a minimal workflow, linted with the configuration of [`internal/exampleconfig/exampleconfig.go`](internal/exampleconfig/exampleconfig.go): the `default` profile with the rules turned off that every minimal example breaks. [The example configuration](scripts/check-checks/README.md#the-example-configuration) says what it turns off and what to do when a new rule fires on most examples. The tool lints as of a fixed day, `exampleDay` in `scripts/check-checks/main.go`, so an output does not change when a date passes, for example the retirement of a runner label. The playground uses the current day.

A permalink drops the lines whose first non-blank character is `#`, so the playground can report other line numbers than the `Output:` block: [Playground links](scripts/check-checks/README.md#playground-links) has the details.

### Anchors are public

- `--explain` prints `https://jactionlint.jdx.dev/checks#<DocsAnchor>` as `Details`, [`docs/rules.md`](docs/rules.md) links the anchors and released binaries print them. Never remove or rename an anchor. When you rename a section, put the old anchor above the new one on its own line.
- One section may serve several rules: each rule names it in its `DocsAnchor`.
- `--explain` prints the section from its anchor to the next own-line anchor or to the next `##` heading, whichever comes first, and drops comment lines such as `<!-- Skip update output -->`. An own-line anchor inside a section therefore cuts it. A part that prints with its parent keeps an old id inline in its heading line, for example `### Part <a id="old-id"></a>`.
- `docs/checks.md` is embedded in the binary with `go:embed`: rebuild to see an edit in `--explain`.
- Every `DocsAnchor` must exist. When you change one, run `mise run rules-doc`, because `docs/rules.md` links it.
- The composite actions section (`check-composite-actions`) must name every rule ID in backticks.
- The list of checks at the top of the page links every section. Add your section to it: `TestChecksIndexListsEverySection` fails otherwise.

### Commands and tests

```sh
go run ./scripts/check-checks ./docs/checks.md        # fails with a diff when the page is outdated
go run ./scripts/check-checks -fix ./docs/checks.md   # rewrites the outputs and the links
mise run docs:test                                    # builds main.wasm and tests the permalinks
hk check --all -S checks_doc                          # the check that CI runs
```

The tool needs `shellcheck` and `pyflakes` and runs on Linux and macOS only. CI runs it in the `Lint` job and the wasm test in the `Wasm` job. These tests read the page:

| Reader | What it needs from the page |
| --- | --- |
| `--explain` and `TestExplain` | The own-line anchor, and a section that ends before the next heading. |
| `TestEveryRuleCanBeExplained`, `TestRuleRegistryIsWellFormed` | An anchor for the `DocsAnchor` of every rule. |
| `TestActionScopeIsDocumented` | Every rule ID in backticks between the anchor `check-composite-actions` and the next anchor. |
| `TestChecksIndexListsEverySection` | A link in the list at the top of the page for every section and every anchored subsection. |
| The docs example tests | Their literal lines, such as `Example input:` followed by a blank line and a `yaml` fence, found between the anchor and the next anchor. |
| `docs/tests/wasm.test.ts` | A blank line and a `[Playground](https://jactionlint.jdx.dev/#...)` link after every generated `Output:` block. It decodes every permalink, lints it with the real wasm build and compares the prefix (`warning: ` or `info: ` for a finding that is not an error), the message and the rule ID with the `Output:` block, and ignores the positions and the findings about files that do not exist. It also pins the first link of the page to a known example, so a change in the order of the sections can fail it. |

## Edit the site and the playground

The documentation site at https://jactionlint.jdx.dev is a [VitePress](https://vitepress.dev/) site in `docs/`. The playground is a page of the site that runs jactionlint compiled to WebAssembly in the browser. The site describes the development version, `main`.

### Edit a page

```sh
mise run docs:dev      # builds the wasm, then serves the site with live reload
mise run docs:build    # builds into docs/.vitepress/dist and verifies it
mise run docs:preview  # serves the built site
mise run docs:test     # the codec, the real wasm against the checks page, and the playground component
```

`mise run docs:build` builds the wasm too. VitePress fails the build on a dead link between pages but does not check `#fragments`, so the task then runs `docs/scripts/verify-docs.mjs` (`npm run verify` in `docs/`). The verifier fails on:

- an old `page#id` that neither exists nor has a redirect,
- a link between pages to a heading that does not exist,
- an entry of the redirect map whose target does not exist,
- a reference of the form `https://jactionlint.jdx.dev/<page>#<id>` or `docs/<page>.md#<id>` to an id that does not exist, in any file of the repository, this file included. Go sources may use a redirected location, because released binaries print it.

The `docs-build.yaml` workflow runs the build and the tests on every pull request that changes a path of its row in the [table of workflows](#open-a-pull-request): the site, the playground, the root Go files, the manual and the Markdown files `README.md`, `CONTRIBUTING.md`, `NOTICE.md` and `scripts/**/README.md`. Some tests tie the pages to the code:

- `TestCLIOptionsAreDocumented`: `docs/cli.md` and `man/jactionlint.1.ronn` name every option of the command line table `cliOptions` in `command_options.go`, as whole words. `docs/v2-migration.md` lists every option that replaces an old one as `` `--option` ``.
- `TestNoStaleV1Options`: the old single-dash spellings of the long options appear in no Markdown page, script, workflow or message, except in `docs/v2-migration.md` between `<!-- legacy-options:start -->` and `<!-- legacy-options:end -->`, and on lines that name `check-checks` together with its own `-fix` option. Write every option of jactionlint with two dashes.
- `TestConfigSchemaAcceptsRealConfigs`: the first `yaml` block after `## Configuration file` in `docs/config.md` validates against `schema/jactionlint-config.json`.
- `TestDocsDemoIsCurrent`: `README.md` and `docs/index.md` show the same demo, a workflow and its output without configuration. Each block follows a comment line (`<!-- demo:input -->`, `<!-- demo:output -->` and the optional `<!-- demo:output-correctness -->`), and the test runs jactionlint on the input and compares. When it fails after a change of a rule, run `jactionlint --no-color --no-hints test.yaml` on the input in an empty Git repository, add `--profile correctness` for the last block, and paste the output into both files.
- `docs/rules.md` is generated and `docs/checks.md` is machine-maintained: see [Generated files](#generated-files) and [Write the checks page](#write-the-checks-page).

`docs/.vitepress/llms.ts` builds `llms.txt` from the sidebar and takes the first prose paragraph after the heading of a page as its description, so every page opens with one paragraph that says what it covers. Link other pages with a relative `.md` path and an explicit fragment, such as `./config.md#profiles`.

### Add, move or rename a page

1. Add a new page to `docs/.vitepress/sidebar.ts` and to the index [`docs/README.md`](docs/README.md), which links only pages that exist.
2. Keep every URL that works. Released binaries print `https://jactionlint.jdx.dev/rules#<rule-id>` and `/checks#<anchor>` links, and other sites link to the pages. VitePress derives the id of a heading from its text, lowercase with punctuation turned into `-`, so renaming a heading changes its id.
3. Keep the old id. Keep the heading text, or put `<a id="old-id"></a>` on its own line directly above the new heading. When the heading moved to another page, add an entry `"/old-page#old-id": "/new-page#new-id"` to `docs/.vitepress/redirects.ts`. Never remove an entry.
4. Check the result with `mise run docs:build`. The verifier compares the build with `docs/tests/anchors-baseline.json`, the ids of the site before its pages were split, and reports a `LOST ANCHOR`.

The fragment of a URL never reaches the server, so the redirect happens in the browser. A head script in `docs/.vitepress/config.mts` redirects the first load. A router hook in `docs/.vitepress/theme/index.ts` redirects navigation inside the site.

A line that carries `x-release-please-version` must stay in a file that `release-please-config.json` lists: see [Version markers](#version-markers).

<a id="how-to-deploy-the-site-and-the-playground"></a>
### Deploy the site

The `pages.yaml` workflow builds the site with `mise run docs:build` and deploys it to GitHub Pages. It runs for a push to `main` that changes `docs/**`, `playground/**`, a Go file in the root directory, `go.mod`, `go.sum`, `mise.toml`, `scripts/build-wasm.bash` or the workflow itself, and by hand with `workflow_dispatch`. A change only under `internal/` or in the Go files of `scripts/` does not deploy. The deploy job runs only in the repository of `jdx`. The custom domain is a setting of the repository on GitHub Pages: `docs/public` holds no `CNAME` file.

To build the same site locally, run `mise run docs:build` and `mise run docs:preview`. The build writes `docs/.vitepress/dist`.

The build does not apply `wasm-opt` to `main.wasm`: the tool needs more memory than a GitHub-hosted runner has and shrinks the compressed file by about 2%. Commit `253f037a` has the measurements.

<a id="how-to-develop-playground"></a>
### The playground

The playground page is at https://jactionlint.jdx.dev/playground, and [`docs/playground.md`](docs/playground.md) describes how it behaves.

- [`playground/main.go`](playground/main.go) is the Go entry point of the wasm module. It is built with `GOOS=js GOARCH=wasm` and talks to the page through functions on `window`. It lints with the configuration of `internal/exampleconfig`.
- [`docs/.vitepress/theme/playground/`](docs/.vitepress/theme/playground) is the page: the Vue component `Playground.vue`, a [CodeMirror](https://codemirror.net/) editor in `editor.ts`, the glue to the wasm module in `wasm.ts`, the permalink encoding in `codec.ts` and the loading of a file from a URL in `remote.ts`.
- `docs/public/playground/main.wasm` and `wasm_exec.js`, the JavaScript runtime of Go, are build outputs and not committed.

```sh
mise run docs:wasm   # builds main.wasm and copies wasm_exec.js to docs/public/playground/
mise run docs:dev    # builds the wasm and serves the site with live reload
mise run docs:test   # the codec, the real wasm against the checks page, and the component
mise run lint:wasm   # staticcheck of ./playground with GOOS=js
```

Rebuild the wasm with `mise run docs:wasm` after you change Go code: the dev server does not do it.

A permalink is `https://jactionlint.jdx.dev/#` followed by the source compressed with zlib and encoded in base64. The checks page and external links use this format, so it must stay compatible. A hash that matches `^#e[A-Za-z0-9+/%]{15,}={0,2}$` on the home page moves to `/playground`, because a zlib stream starts with `e`. Any other hash stays, so `/#install` still opens the home page. The old query parameters `?s=<source>` and `?u=<url>` move to `/playground` too. `?u` loads a file from a URL, and a GitHub `blob` or gist URL is converted to the raw file.

## Cut a release

This section is for maintainers. Releases are automated with [release-please](https://github.com/googleapis/release-please): see [`release-please.yaml`](.github/workflows/release-please.yaml) and [`release-please-config.json`](release-please-config.json). The workflows that push to `main` or create tags use the secret `RELEASE_PLZ_TOKEN`, a personal access token, because tags and pushes made with `GITHUB_TOKEN` do not trigger workflows and cannot bypass the branch protection.

<a id="make-a-new-release"></a>
### How a release runs

1. **Merge pull requests into `main`.** The `release-please.yaml` workflow runs for every push to `main`, every Saturday and by hand. It opens or updates the release pull request, which bumps `.release-please-manifest.json`, the [version markers](#version-markers) and `CHANGELOG.md`. Never edit `CHANGELOG.md`: release-please writes it.
2. **The generated files join the release pull request.** After release-please, the `generate` job runs `go generate` and the detection of new major releases of popular actions, and pushes the commit `chore(generate): update generated files by go generate` to the release branch. It comments the output of the detection on the pull request. It does nothing when no release pull request is open, because a chore commit alone does not create one.
3. **The release pull request merges.** The `auto-merge-release.yaml` workflow runs every day and sets the pull request to squash-merge when its checks pass. It does nothing when the latest `v*` tag is less than seven days old, when no `fix` or `feat` commit follows that tag, or when the body of the pull request is empty. Run it by hand to skip the first two guards, or merge the pull request yourself to release earlier.
4. **release-please tags `vX.Y.Z` and creates the GitHub release as a draft.** A published release is immutable: its assets and its tag cannot change. So every asset must be on the draft before it is published.
5. **The tag starts `release.yaml`.** Its `binaries` job runs on a macOS runner, in this order:
   1. Build the manual with `ronn --roff`.
   2. Import the Apple signing certificate and pick the signing identity.
   3. Run GoReleaser. It builds the binaries for macOS, Linux, Windows and FreeBSD without cgo, signs the macOS binaries and archives them: tar.gz for macOS, Linux and FreeBSD, zip for Windows. Each archive holds `README.md`, `LICENSE.txt`, `NOTICE.md`, `CONTRIBUTING.md`, `docs` and `man/jactionlint.1`. GoReleaser attaches the archives and the checksum file to the draft and commits the updated Homebrew cask in `Casks` to `main`.
   4. Notarize the macOS binaries. The job fails when Apple does not accept them.
   5. Sign a [packslip](https://packslip.dev) bundle of the archives, `packslip.sigstore.json`, and attach it to the draft.
   6. **Publish the release** with `gh release edit --draft=false`.
   7. Attest the build provenance of the checksum file.
   8. Run the post-release download check: `scripts/download-jactionlint.bash` with the new version.
   9. Commit the version bump to `main`: the default version of `scripts/download-jactionlint.bash`, and the commit hash and the `# vX.Y.Z` comment of the `jdx/jactionlint` action examples in `docs/ci.md` and `docs/online.md`. They name a tag that exists only after the release is published, so the release pull request cannot hold them. `action.yml` is in the release pull request, because the tag must carry the version that it installs.
6. **The other jobs of the workflow** run beside `binaries` or after it. `docker` does not wait for `binaries`: it pushes the Docker image `ghcr.io/jdx/jactionlint` with the tags `X.Y.Z` and `latest` for `linux/amd64` and `linux/arm64`. `enhance-release` and `comment-release-fixes` need `binaries` to succeed. The first rewrites the release notes with [communique](https://github.com/jdx/communique) (`communique.toml` configures it) and appends a sponsor section, and the second comments on the issues and pull requests that the release fixes. Release notes stay editable on an immutable release.

### When a step fails

Before `Publish the release`, fix the cause and re-run the workflow. GoReleaser replaces the assets that are already on the draft, and the packslip action uploads its bundle with `--clobber`. The Homebrew cask commit lands on `main` before the release is published, so after a failed run it can point at a release that is still a draft until the re-run succeeds.

After `Publish the release`, a re-run of `binaries` cannot upload assets again. Finish its remaining steps by hand: the provenance attestation, the download check and the version bump. `docker` does not depend on `binaries`, so a failed `docker` job can run again on its own. A failed `binaries` skips `enhance-release` and `comment-release-fixes`, so do their work by hand too: the release notes and the comments on the issues that the release fixes.

### Version markers

Two mechanisms keep the release version in the repository current.

- **release-please.** A line that holds the version of the latest release carries the comment `x-release-please-version`, and release-please rewrites the version on that line in the release pull request. Find the lines with `grep -rn x-release-please-version .`. They are in `action.yml`, `.pre-commit-hooks.yaml`, `docs/.vitepress/version.ts` and the pages that show the Docker image tag (`docs/ci.md`) and the pre-commit revision (`docs/hooks.md`). Release-please edits only the files that `extra-files` of `release-please-config.json` lists. When you move a marker line to another file, list that file there.
- **The release job.** The `uses: jdx/jactionlint@<commit> # vX.Y.Z` examples of the action in `docs/ci.md` and `docs/online.md` pin the release by commit hash, as the default profile asks. They name a commit that exists only after the tag, so the last item of step 5 of [How a release runs](#how-a-release-runs) rewrites them with `sed`. The pattern needs a 40-character hash and a `# v` comment with a version, and the job fails when one of the two pages holds no such line: keep at least one action example in that form in each page.

Never edit these versions by hand.

<a id="maintain-auto-generated-sources"></a>
<a id="maintain-popular_actionsgo"></a>
<a id="maintain-all_webhooksgo"></a>
<a id="maintain-jactionlint-matcherjson"></a>
<a id="maintain-availabilitygo"></a>
## Generated files

Some files are generated. Do not edit them by hand. [The tool index](scripts/README.md#the-tools) lists each file with the command that writes it and the job or test that keeps it current, and the README of each script has its flags. [What to run after a change](scripts/README.md#what-to-run-after-a-change) says which command your change needs. These are the cases that a contributor meets:

- **You write them.** `testdata/profiles.txt` and `testdata/rule_ids.d/*.txt` record decisions that a reviewer checks, so a test fails until you update them: see [Record the snapshots](#record-the-snapshots). The goldens of the fixtures have flags too: see [Test the rule](#test-the-rule).
- **hk writes them.** `hk fix --all` rewrites `docs/rules.md`, the JSON Schemas and the outputs of `docs/checks.md`. `hk check --all`, `TestSchemasAreUpToDate` and `TestRulesDocIsUpToDate` fail when one is outdated.
- **The release pull request writes them.** `popular_actions.go`, `all_webhooks.go`, `availability.go` and `event_payloads.go` come from `go generate`, which fetches data from GitHub and needs the network. The `generate` job of `release-please.yaml` refreshes them in an open release pull request ([How a release runs](#how-a-release-runs)). Nothing refreshes them on `main` between releases, so run `mise run generate` when you need newer data.
- **A workflow writes them.** The `schema-pin.yaml` workflow moves `testdata/schema/runner` every week in a pull request.
- **A workflow checks them.** `.github/jactionlint-matcher.json` is generated because its regular expression must also match output with ANSI color escape sequences. After a change of the text output, run `mise run matcher:fixtures` and then `mise run matcher`, or `matcher.yaml` fails. [Problem matchers](docs/ci.md#problem-matchers) explains how to use the file.
- **Build outputs that git ignores.** `man/jactionlint.1` (`mise run man`) and `docs/public/playground/main.wasm` with `wasm_exec.js` (`mise run docs:wasm`, which the tasks `docs:dev`, `docs:build` and `docs:test` run).

<a id="how-to-generate-the-manual"></a>
### Build the manual

```sh
mise run man
```

The task runs `scripts/build-man.bash`. With the Ruby that mise provides for the task, it installs `ronn-ng`, the maintained fork of ronn, into a temporary gem directory and writes `man/jactionlint.1` from `man/jactionlint.1.ronn`. It writes roff only, because generating HTML fails inside ronn on current Ruby. The script needs a clone in which `.git` is a directory, so it stops in a git worktree. ronn-ng prints a line that starts with `warn:` for markup that it cannot render and builds the page anyway, so read the output. The release workflow builds the page the same way and ships it in the archives and the Homebrew cask. `TestCLIOptionsAreDocumented` requires the ronn source to name every option of the command line.

## Next steps

- [The tools of `scripts/`](scripts/README.md): the tool index and the commands to run after a change.
- [The README of check-checks](scripts/check-checks/README.md): the exact format of a section of the checks page.
- [Fuzz the parsers](fuzz/README.md): the go-fuzz targets, the native fuzz tests and their open failures.
- [Profiles](docs/config.md#profiles): what each profile enables.
- [The page index](docs/README.md): every page of the documentation.
