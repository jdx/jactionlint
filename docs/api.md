Go API
======
[![API Document][api-badge]][apidoc]

This document describes how to use [jactionlint](https://github.com/jdx/jactionlint) as Go library.

jactionlint can be used from Go programs by importing the module.

```go
import "github.com/jdx/jactionlint/v2"
```

See [the documentation][apidoc] to know the list of all APIs. It contains
a workflow file parser built on top of `yaml/go-yaml` library, expression `${{ }}` lexer/parser/checker, etc.

Followings are unexhaustive list of interesting APIs.

- `Command` struct represents entire `jactionlint` command. `Command.Main` takes command line arguments and runs command
  until the end and returns exit status.
- `Linter` manages linter lifecycle and applies checks to given files. If you want to run jactionlint checks in your
  program, please use this struct.
- `Project` and `Projects` detect a project (Git repository) in a given directory path and find configuration in it.
- `Config` represents structure of `jactionlint.yaml` config file. `ReadConfigFile()` reads a file and resolves its `extends`,
  and `ParseConfig()` parses bytes. `Config.RuleLevel()` tells at which `Severity` a rule runs. Unknown keys are errors.
- `Error` is a finding. It has the stable rule ID (`ID`), the `Severity`, the region (`Line`, `Column`, `EndLine` and `EndColumn`),
  the documentation URL (`DocURL`) and an optional automatic `Fix`, which is a list of byte-range `TextEdit`s.
- `Rules()` returns the `RuleInfo` of every rule (ID, group, summary, default level, profile and options). `LookupRule()`
  finds one by its ID. The IDs are stable. `RuleDocURL()` returns the URL of the documentation of a rule.
- `Severity` is the level of a finding: `SeverityInfo`, `SeverityWarning` or `SeverityError`. `SeverityOff` disables a rule.
  `Profile` is the set of rules enabled by the configuration: `ProfileDefault`, `ProfileStrict` or `ProfileAll`.
- `Linter.FixFiles()` and `Linter.FixRepository()` apply the fixes of the errors. `MigrateConfig()` rewrites the deprecated keys
  of a config file into the `rules` mapping.
- `Workflow`, `Job`, `Step`, ... are nodes of workflow syntax tree. `Workflow` is a root node.
- `Parse()` parses given contents into a workflow syntax tree. It tries to find syntax errors as much as possible and
  returns found errors as slice.
- `ParseUses()` parses the value of a `uses:` key, for steps and for reusable workflow calls, into a `UsesRef`: its `Kind`
  (`UsesAction`, `UsesReusableWorkflow`, `UsesDocker`, `UsesLocal`, `UsesInvalid`), owner, repo, subpath, ref and `RefKind`
  (`RefFullSHA`, `RefShortSHA`, `RefSemverTag`, `RefOther`, `RefDigest`, `RefNone`), and Docker image, tag and digest.
  `UsesRef.IsPinned()`, `SameRepo()` and `CanonicalName()` help rules that compare references.
- `Workflow.Comments` is the `CommentIndex` of the YAML comments of the file. A rule looks a node up by the line of its
  `Pos`: `Inline()` is the trailing comment, `Before()` and `After()` are the comment blocks directly above and below
  (a blank line ends a block), and `Documented()` tells whether a line has either. `NewCommentIndex()` builds an index
  from any YAML source.
- `LinterOptions.Online` turns on the [online checks](usage.md#online-checks) (`impostor-commit`, `known-vulnerable-actions`, and so
  on). They talk to GitHub through the `GitHubClient` interface; the built-in client sends REST requests with a token from the
  environment and caches the answers. Set `LinterOptions.GitHubClient` to serve your own, for example
  `NewFixtureGitHubClient()` with answers recorded by `NewRecordingGitHubClient()`, so that tests need no network. Without
  `Online` no client is ever called, and the WebAssembly build has no built-in client. `LinterOptions.OnlineOptions` sets the
  mode (`OnlineModeCache` answers from the cache only, `OnlineModeStrict` makes `Linter.OnlineFailed()` true when a lookup was
  skipped), the API URL, the token source, the allow and deny lists and the retry behavior; `Linter.OnlineSkipped()` is the
  number of lookups that failed and were skipped. A failed lookup never makes a `Lint` call fail.
- `Pass` is a visitor to traverse a workflow syntax tree. Multiple passes can be applied at single pass using `Visitor`.
- `Rule` is an interface for rule checkers and `RuleBase` is a base struct to implement a rule checker. `RuleBase.ReportID()`
  reports an error with the stable ID of the diagnostic. `RuleBase.Error()` reports it with the name of the rule as the ID.
  - `RuleExpression` is a rule checker to check expression syntax in `${{ }}`.
  - `RuleShellcheck` is a rule checker to apply `shellcheck` command to `run:` sections and collect errors from it.
  - `RuleJobNeeds` is a rule checker to check dependencies in `needs:` section. It can detect cyclic dependencies.
  - ...
- `ExprLexer` lexes expression syntax in `${{ }}` and returns slice of `Token`.
- `ExprParser` parses given slice of `Token` and returns syntax tree for expression in `${{ }}`. `ExprNode` is an
  interface for nodes in the expression syntax tree.
- `ExprType` is an interface of types in expression syntax `${{ }}`. `ObjectType`, `ArrayType`, `StringType`,
  `NumberType`, ... are structs to represent actual types of expression.
- `ExprSemanticsChecker` checks semantics of expression syntax `${{ }}`. It traverses given expression syntax tree and
  deduces its type, checking types and resolving variables (contexts).
- `ValidateRefGlob()` and `ValidatePathGlob()` validate [glob filter pattern][filter-pattern-doc] and returns all errors
  found by the validator.
- `ActionMetadata` is a struct for action metadata file (`action.yml`). It is used to check inputs specified at `with:`
  and typing `steps.{id}.outputs` object strictly.
- `PopularActions` global variable is the data set of popular actions' metadata collected by [the script](https://github.com/jdx/jactionlint/blob/main/scripts/generate-popular-actions).
- `AllWebhookTypes` global variable is the mapping from all webhook names to their types collected by [the script](https://github.com/jdx/jactionlint/blob/main/scripts/generate-webhook-events).
- `WorkflowKeyAvailability()` returns available context names and special function names for the given workflow key like
  `jobs.<job_id>.outputs.<output_id>`. This function uses the data collected by [the script](https://github.com/jdx/jactionlint/blob/main/scripts/generate-availability).

## Library versioning

The version of this repository is for command line tool `jactionlint`. So it does not represent the version of the library.
It means that the library does not follow semantic versioning and any patch version bump may introduce some breaking changes.

Since jactionlint v2 the Go module is `github.com/jdx/jactionlint/v2`. The package name is still `jactionlint`.

## Go version compatibility

Following the Go's official policy, last two major Go versions are supported. For example, when the latest Go version is
v1.22, v1.21 and v1.22 are supported. Minimum supported Go version is written in the [`go.mod`](https://github.com/jdx/jactionlint/blob/main/go.mod) file in this
repository.

---

[Checks](checks.md) | [Rules](rules.md) | [Installation](install.md) | [Usage](usage.md) | [Configuration](config.md) | [References](reference.md)

[api-badge]: https://pkg.go.dev/badge/github.com/jdx/jactionlint/v2.svg
[apidoc]: https://pkg.go.dev/github.com/jdx/jactionlint/v2
[go-yaml]: https://github.com/yaml/go-yaml
[filter-pattern-doc]: https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet
