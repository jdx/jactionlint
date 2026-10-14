# Go API

Use jactionlint from Go to lint workflow files or text, read findings, apply fixes, run the online checks, add your own rules and reuse the parsers. The module is `github.com/jdx/jactionlint/v2`, the package is `jactionlint`, and [pkg.go.dev](https://pkg.go.dev/github.com/jdx/jactionlint/v2) lists every identifier. This page covers what that reference does not: which entry point to call, how the library differs from the command and what stays stable between releases.

## Install and import

Add the module to your project.

```sh
go get github.com/jdx/jactionlint/v2
```

The major version is part of the import path, and the package name stays `jactionlint`. [Go version compatibility](#go-version-compatibility) says which Go version your module needs.

This site describes the development version. When an identifier of this page is missing from the version you imported, run `go get github.com/jdx/jactionlint/v2@main`, or read `go doc github.com/jdx/jactionlint/v2` for the version you have.

## Lint a workflow

This program lints one workflow file and prints one line for each finding. Run it in the root of a Git repository that holds this file as `.github/workflows/ci.yaml`.

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo ${{ foo }}
```

Save the program as `main.go` in a module that requires jactionlint, then run it with `go run .`.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/jdx/jactionlint/v2"
)

func main() {
	// Always pass a non-nil *LinterOptions. The zero value is the default behavior.
	// io.Discard keeps the linter from printing: the findings come back as values.
	linter, err := jactionlint.NewLinter(io.Discard, &jactionlint.LinterOptions{})
	if err != nil {
		log.Fatal(err)
	}

	// A nil project means: find the repository from the path.
	findings, err := linter.LintFile(".github/workflows/ci.yaml", nil)
	if err != nil {
		log.Fatal(err) // jactionlint itself failed; this is not a finding
	}

	failed := false
	for _, f := range findings {
		fmt.Printf("%s:%d:%d %s %s\n", f.Filepath, f.Line, f.Column, f.Severity, f.ID)
		if f.Severity == jactionlint.SeverityError {
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}
```

The output, with the exit status 1 of the program:

```
.github/workflows/ci.yaml:1:1 error concurrency-limits
.github/workflows/ci.yaml:3:3 error missing-timeout
.github/workflows/ci.yaml:3:3 error missing-permissions
.github/workflows/ci.yaml:6:15 error unpinned-uses
.github/workflows/ci.yaml:6:15 error artipacked
.github/workflows/ci.yaml:7:23 error undefined-property
```

How the program behaves:

- `NewLinter(out, opts)` writes the findings in the output format to `out` and returns them as values. Pass `io.Discard` when you only want the values.
- `opts` must not be nil: `NewLinter(out, nil)` panics. The zero value `&jactionlint.LinterOptions{}` is the default behavior.
- The second result of every `Lint*` method is a failure of jactionlint itself, such as an unreadable file or a broken config file. Findings are never in it.
- The command exits with status 1 when a finding has the level `error`, or any level with `--strict-exit`. The program does the same for the level `error`: it compares `f.Severity` with `jactionlint.SeverityError`.
- With `Profile: jactionlint.ProfileCorrectness` in the options, the same file reports one finding, `undefined-property`.

### Choose the entry point

Each method lints, prints the findings to the writer of `NewLinter` and returns them.

| Call | Lints | Details |
| --- | --- | --- |
| `Lint(path, content, project)` | the text in `content`, reported under the name `path` | For text that is not a file, such as an editor buffer. With a nil `project`, the repository is found from `path` only when `path` exists on disk. For a path that does not exist, the config file of the repository is not read. |
| `LintFile(path, project)` | one file | |
| `LintFiles(paths, project)` | several files | One report for all files. |
| `LintDir(dir, project)` | every `.yml` and `.yaml` file below `dir`, recursively | It fails when there is no such file. |
| `LintRepository(dir)` | the workflow files, composite actions, Dependabot configuration and Actions lock file of the nearest repository | An empty `dir` is the working directory. It fails when no repository is found. |
| `LintStdin(reader)` | the content of `reader`, reported under `LinterOptions.StdinFileName` (default `<stdin>`) | |
| `Command.Main(args)` | what the `jactionlint` command does | See [Run the command from Go](#run-the-command-from-go). |

A `*Project` is a repository: the nearest parent directory that holds `.git` (or `.jj`, a Jujutsu workspace) and has `.github/workflows`, `.github/actions` or an `action.yml` (or `action.yaml`) at its root. `NewProjects().At(path)` finds the project of a path and caches it (nil when there is none), and `NewProject(root)` reads the repository at `root`, including its config file. Pass a project to a `Lint*` method to skip the search.

### Defaults differ from the command

`&jactionlint.LinterOptions{}` configures a run like the command without options, apart from the differences below. The profile, the config files, the ignores and the baseline work as in the command: the `default` profile applies when no config file sets another.

| What | `&jactionlint.LinterOptions{}` | `jactionlint` command |
| --- | --- | --- |
| Output | The text format goes to the writer you pass to `NewLinter`, and the findings are returned. | The text format goes to stdout. |
| shellcheck and pyflakes | They do not run until you set `Shellcheck` and `Pyflakes`. | They run when found on `PATH`. A missing one is noted in the log and skipped. |
| Logs, warnings and notes | They are discarded until you set `LogWriter`. | They go to stderr. |
| Hints | None are printed until you set `RunHints`. | A hint line follows a text run with many findings, unless `--no-hints` or `JACTIONLINT_NO_HINTS` is set. |
| Environment variables | `JACTIONLINT_ONLINE` is ignored. `XDG_CONFIG_HOME` still decides where the user-global config file is. | `JACTIONLINT_ONLINE` is the default of `--online` and `--no-online`. |
| Exit status | There is none: decide from the `Severity` of the findings. | 0, 1, 2 or 3, see [exit status](./cli.md#exit-status). |
| Interruption | Online lookups stop when `Context` is canceled. A nil `Context` never cancels. | Online lookups stop on Ctrl-C. |

To run the external tools as the command does, set `Shellcheck` and `Pyflakes` to a command name or a path. To get the note about a missing tool, set `RunHints` and give `LogWriter` a writer. An empty value disables a tool.

```go
linter, err := jactionlint.NewLinter(io.Discard, &jactionlint.LinterOptions{
	Shellcheck: "shellcheck",
	Pyflakes:   "pyflakes",
	LogWriter:  os.Stderr,
})
```

A missing tool only costs a note in the log, and only with `RunHints`. Set `ShellcheckRequired` or `PyflakesRequired` to make `NewLinter` fail instead. After a run, `linter.ExternalTools()` returns a map with an entry for each tool that checked or could not check a script: `shellcheck`, `pyflakes` and `node` (for the scripts of `actions/github-script`). An entry has the `Status` (`ran` or `not found`), the number of `Scripts` that the tool checked and the number of `SkippedScripts` that it could not check. A tool that you turned off has no entry.

A program that used the v1 library runs the `default` profile after you upgrade it. Set `Profile: jactionlint.ProfileCorrectness` to get what actionlint checks plus the bug detectors of jactionlint. See [the Go API of v2](./v2-migration.md#go-api) for what else changed.

## Read a finding

Each finding is an `*jactionlint.Error`. Its fields:

| Field | Meaning |
| --- | --- |
| `ID` | The rule ID, such as `unpinned-uses`. For a rule you add, the name you gave it. |
| `Message` | The text of the finding, without the file position. |
| `Filepath` | The path as the linter reports it. |
| `Line`, `Column` | The start of the region, 1-based. `Column` counts Unicode code points. |
| `EndLine`, `EndColumn` | The end of the region. `EndColumn` is the column after the last character. |
| `Severity` | `jactionlint.SeverityError`, `SeverityWarning` or `SeverityInfo`. `String()` gives `error`, `warn` or `info`. It is never `SeverityOff`. |
| `DocURL` | The URL of the documentation of the rule. Empty for a rule you add. |
| `Fix` | An automatic correction, or nil. |
| `NoFix` | Why a finding that the rule can fix has no `Fix`. Nil when `Fix` is set, and for findings that no fix exists for. |
| `Baselined` | The `Lint*` methods never return a finding that the baseline accepts. The field is set only on the way to the SARIF log and the summary format. |
| `Events`, `Permissions` | The events of the workflow and the permissions of the job token. Set only with `FindingContext`. |
| `RetiredID` | The retired rule ID that the finding had before its rule was merged into `ID`. Ignores of the old ID still match. |
| `Kind` | The legacy name of the checker that reported the finding. Several rules share a kind. Use `ID`. |

This loop tells apart the findings that have a fix, the ones that need something first and the ones that are advice only. Its output for the workflow of the first example:

```go
for _, f := range findings {
	switch {
	case f.Fix != nil:
		fmt.Printf("%s: fix %q, unsafe=%t, edits=%d\n", f.ID, f.Fix.Description, f.Fix.Unsafe, len(f.Fix.Edits))
	case f.NoFix != nil:
		fmt.Printf("%s: no fix, %s %q\n", f.ID, f.NoFix.Code, f.NoFix.Option)
	default:
		fmt.Printf("%s: advice only\n", f.ID)
	}
}
```

```
concurrency-limits: advice only
missing-timeout: no fix, option-required "missing-timeout.default-minutes"
missing-permissions: fix "Add permissions: contents: read", unsafe=false, edits=1
unpinned-uses: no fix, online-required ""
artipacked: fix "Set persist-credentials: false", unsafe=false, edits=1
undefined-property: advice only
```

A `Fix` lists `TextEdit`s. An edit replaces the bytes from `Start` up to `End` of the file with `NewText`: `Start == End` inserts, and an empty `NewText` deletes. An unsafe fix may change what the workflow does, and the fixing methods apply it only on request.

`NoFix.Code` says what to do. `option-required` and `option-invalid` name the rule option in `NoFix.Option`, `online-required` and `lookup-failed` concern the online checks, and `unsupported-shape` and `needs-judgment` leave the change to you. [Why a finding was not fixed](./fix.md#why-a-finding-was-not-fixed) lists every code, including the ones that the SARIF log adds.

To print a finding yourself, pick one of these:

- `f.Error()` returns the one-line form `path:line:column: message [kind]`. The brackets hold `Kind`, not `ID`.
- `f.PrettyPrint(w, source)` writes the text format with the source snippet, and also shows `Kind` where the command shows the rule ID. With a nil `source`, it prints no snippet.
- `NewErrorFormatter(template)` and its `PrintErrors(w, findings, source)` write a [Go template](./output.md#format-error-messages).

To write the documented JSON, set `Format: jactionlint.FormatJSON` and pass a real writer to `NewLinter`. Do not pass an `Error` to `encoding/json`: it has no JSON field tags, so it writes the Go field names (`Line`, `EndColumn`) and not the names of the [output schema](./output.md#fields-of-a-finding).

## Configure a run

`LinterOptions` holds the options of a run. Most fields have a command-line option, and some also have a config file key. The tables below map each one.

The linter takes its configuration from the first source that applies:

1. `LinterOptions.ConfigFile`, the path of a config file.
2. `LinterOptions.Config`, a `*Config` that you built or parsed. It replaces the config files of the repository and of the user.
3. The repository config file of the project that holds the file, or else the user-global config file. [Where jactionlint reads the config file](./config.md#where-jactionlint-reads-the-config-file) lists the names and the locations, which are the same as for the command.

`LinterOptions.Profile`, when set, overrides the `profile` of whichever source applies. A config that sets no profile means `ProfileDefault`.

A run that reads files of the machine can differ between machines. For a result that does not, pass a `Config`. `ParseConfig` parses the bytes of a config file, rejects unknown keys with a suggestion, and rejects `extends`, which needs a file path. Use `ReadConfigFile(path)` to read a file with its `extends`. [`playground/main.go`](https://github.com/jdx/jactionlint/blob/main/playground/main.go) lints this way in the browser, with no file system.

```go
config, err := jactionlint.ParseConfig([]byte("profile: correctness\n"))
if err != nil {
	log.Fatal(err)
}
linter, err := jactionlint.NewLinter(io.Discard, &jactionlint.LinterOptions{Config: config})
```

`config.RuleLevel("unpinned-uses")` tells at which `Severity` a rule runs under that configuration (`SeverityOff` when it is off). It also works on a nil `*Config`, which is the default configuration.

A `Config` also carries what a tool needs to inspect a config file. `Config.Ignores` holds the [durable ignores](./config.md#durable-ignores) as `ConfigIgnore` values, and `ConfigIgnore.Snippet()` renders one as YAML for tools that write entries. `Config.Deprecations` lists the messages about deprecated keys, `Config.Notices` the messages that are not warnings, and `Config.Path` the file it was read from.

### Options of a run

Fields in the same row belong together. [Command-line options](./cli.md#command-line-options) describes each option, and [Configuration](./config.md#configuration-file) each key.

| `LinterOptions` field | Option | Config key | Effect |
| --- | --- | --- | --- |
| `Format` | `--format` | none | A `Format*` constant (`FormatText` when empty, `FormatOneline`, `FormatJSON`, `FormatJSONL`, `FormatSARIF`, `FormatGCC`, `FormatSummary`, `FormatGitHub`) or a Go template. |
| `Oneline` | `--oneline` | none | The same as `FormatOneline`. It affects only the text format. |
| `Color` | `--color`, `--no-color` | none | `ColorOptionKindAuto` (the zero value), `ColorOptionKindAlways` or `ColorOptionKindNever`. `Always` and `Never` set a process-wide switch of the color library. |
| `ShowDocURLs` | `--show-doc-urls` | none | Prints the documentation URL of the rule under each finding of the text format and in the message of the `github` format. |
| `ShowRuleIDs` | `--rule-ids` | none | Accepted and without effect: the text format always shows the rule ID. |
| `FindingContext` | `--finding-context` | none | Fills `Error.Events` and `Error.Permissions`, which the JSON, JSON Lines and SARIF formats print. |
| `MinSeverity` | `--min-severity` | none | Hides the findings less severe than the value. The zero value shows every finding. |
| `StdinFileName` | `--stdin-filename` | none | The name that `LintStdin` reports. Empty means `<stdin>`. |
| `Verbose`, `Debug` | `--verbose`, `--debug` | none | Write logs to `LogWriter`. |
| `LogWriter` | none (the command uses stderr) | none | Receives logs, deprecation warnings and notes. Nil discards them. |
| `RunHints` | `--no-hints` turns it off | none | Lets a text run with many findings print one hint line to `LogWriter`, on a terminal or in CI. It also lets a run print the note about a missing shellcheck or pyflakes, except with the SARIF format. `HintsDisabledByEnv()` reports `JACTIONLINT_NO_HINTS`. |
| `Profile` | `--profile` | `profile` | Selects `ProfileCorrectness`, `ProfileDefault` or `ProfilePedantic` for every file, each including the rules of the one before. Empty leaves the choice to the config. See [profiles](./config.md#profiles). |
| `Config` | none | the whole file | A config to use instead of any file. `ConfigFile` wins when both are set. |
| `ConfigFile` | `--config-file` | none | The path of the config file to use. |
| `Select` | `--select` | none | Reports only these rule IDs. A selected rule that the profile turns off runs at its default level. An unknown ID makes `NewLinter` fail. An empty list applies no selection: every rule follows the configuration. |
| `IgnorePatterns` | `--ignore` | none | Each entry is a rule ID, or a regular expression that is matched against the message. |
| `NoIgnores` | `--no-ignores` | none | Applies no ignore comment, no `ignores` or `paths` ignore of the config and no baseline. `IgnorePatterns` still apply. |
| `Now` | none | none | The clock that decides when an `expires` date of `ignores` has passed. Nil means `time.Now`. It is for tests. |
| `WorkingDir` | none | none | The working directory. Empty means `os.Getwd`. |
| `Context` | none | none | Stops online lookups when canceled. Nil means `context.Background`. |
| `OnRulesCreated`, `OnDependabotRulesCreated` | none | none | Hooks that add rules of your own. See [Add your own rule](#add-your-own-rule). |
| `Baseline`, `BaselineFile` | `--baseline`, `--baseline=FILE` | `baseline` | Applies the baseline file, which must exist, even when the config does not ask for it. An empty `BaselineFile` is `.github/jactionlint-baseline.json`. |
| `NoBaseline` | `--no-baseline` | none | Ignores the baseline even when the config asks for it. |
| `BaselineCheck` | `--baseline-check` | none | Reports baseline entries that match no finding. It implies `Baseline`. |
| `SARIFHideBaselined` | `--sarif-hide-baselined` | none | Leaves the findings that the baseline accepts out of the SARIF log. |
| `Shellcheck`, `ShellcheckRequired` | `--shellcheck` | none | The command or path of shellcheck. Empty disables it. When `ShellcheckRequired` is set, `NewLinter` fails if the command is missing. The command sets it when you give `--shellcheck`. |
| `ShellcheckRCFile` | `--shellcheck-rcfile` | `shellcheck.rcfile` | A shellcheckrc file for shellcheck to read instead of `--norc`. A relative path is relative to the working directory (to the repository root in the key), and the file must exist. The keys `shellcheck.args` and `shellcheck.enable` come from the config only. |
| `Pyflakes`, `PyflakesRequired` | `--pyflakes` | none | The command or path of pyflakes, with the same rules as shellcheck. |
| `Ruff` | `--ruff` | none | The command or path of ruff. When set, ruff checks the Python scripts instead of pyflakes. `NewLinter` fails when the command is missing. |
| `Online` | `--online` | `online` | Turns the online checks on. See [Online checks from Go](#online-checks-from-go). |
| `OnlineOff` | `--no-online` | none | Turns the online checks off, whatever `Online` and the config say. |
| `OnlineOptions` | `--online=MODE` and the `--online-*` options | `online-options` | See the next table. It wins over the `online-options` of the config. |
| `OnlineCacheTTL` | none | none | How long the built-in GitHub client uses a cached answer. Zero means one hour and a negative value revalidates every answer. `OnlineOptions.CacheTTL` wins. |
| `GitHubClient` | none | none | Replaces the built-in GitHub client. |

`LinterOptions` has gained fields over time. Set them by name, never by position.

The fields of `OnlineOptions` map like this. The zero value of every field means the default. To set a pointer field such as `CacheTTL`, take the address of a value.

| `OnlineOptions` field | Option | `online-options` key |
| --- | --- | --- |
| `Mode` (`OnlineModeDefault`, `OnlineModeCache`, `OnlineModeStrict`, `OnlineModeCacheStrict`) | `--online=MODE` | `mode` |
| `ModeSet` | set by `--online` | none |
| `APIURL` | `--online-api-url` | `api-url` |
| `TokenEnv` | `--online-token-env` | `token-env` |
| `TokenFile` | `--online-token-file` | `token-file` |
| `Allow`, `Deny` | `--online-allow`, `--online-deny` | `allow`, `deny` |
| `CacheTTL` | `--online-cache-ttl` | `cache-ttl` |
| `MaxRateLimitWait` | `--online-max-wait` | `max-rate-limit-wait` |
| `Retries` | none | `retries` |
| `Concurrency` | none | `concurrency` |
| `GitHubCLI` | none | `gh-cli` |
| `GitHubComFallback` | `--online-github-com-fallback` | `github-com-fallback` |
| `GitHubComTokenEnv` | `--online-github-com-token-env` | `github-com-token-env` |
| `GitHubComTokenFile` | `--online-github-com-token-file` | `github-com-token-file` |

`ParseOnlineMode(s)` parses the value of `--online` and of the `mode` key. [Online options](./config.md#online-options) lists the defaults and limits.

## Run the command from Go

`Command` is the whole `jactionlint` command. Use it to get the behavior and the exit status of the command in a wrapper or a test, without starting a process. `cmd/jactionlint/main.go` is `os.Exit(cmd.Main(os.Args))` around it.

```go
cmd := jactionlint.Command{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
status := cmd.Main([]string{"jactionlint", "--profile=correctness", ".github/workflows/ci.yaml"})
os.Exit(status)
```

`Main` takes the arguments including the program name, like `os.Args`: it skips `args[0]`, so an argument in first place is lost. It returns the exit status as one of `ExitStatusSuccessNoProblem`, `ExitStatusSuccessProblemFound`, `ExitStatusInvalidCommandOption` and `ExitStatusFailure`, which [the exit status table](./cli.md#exit-status) describes.

The signature of `Main` is the same as in v1, and only the parsing of the arguments changed. `Main` parses POSIX/GNU options. A single-dash long option of v1 fails with status 2 and a message that names its replacement, and [Migrating to v2](./v2-migration.md#command-line-options) maps the old spellings. `Command` reads the environment variables of the command, such as `JACTIONLINT_ONLINE`, which `Linter` ignores.

## Fix, baseline and migrate

The library applies fixes, writes the baseline and rewrites old config. This table maps each task to its call and to the command option that does the same. Names that start with `linter.` are methods of `*Linter`, and the others are package functions.

| Task | Call | Command |
| --- | --- | --- |
| Fix files in place | `linter.FixFilesWithOptions(paths, project, options)` and `linter.FixRepositoryWithOptions(dir, options)`, or the short forms `linter.FixFiles(paths, project, mode)` and `linter.FixRepository(dir, mode)` | `--fix`, `--diff`, `--fix-rules` |
| Write the baseline | `linter.WriteBaseline(files, path)` | `--baseline-write` |
| Read a baseline file | `ReadBaselineFile(path)`, `ParseBaseline(bytes)` | none |
| Rewrite deprecated config keys | `MigrateConfig(src)` on bytes, `MigrateConfigFile(path)` on a file, `linter.MigrateConfig(dir)` on the config file of a repository | `--migrate-config` |
| Rewrite zizmor ignore comments | `MigrateZizmorIgnores(src)` on bytes, `MigrateZizmorIgnoreFiles(paths)` on files, `linter.MigrateIgnores(paths)` on the files that you name or on the workflow files of the repository | `--migrate-ignores` |
| Write the default config file | `linter.GenerateDefaultConfig(dir)` | `--init-config` |

### Apply fixes

The `...WithOptions` calls take a `FixOptions`, and the short forms take only a mode. All of them fix the files in place. This dry run computes the fixes of the first example and writes nothing:

```go
result, err := linter.FixFilesWithOptions(
	[]string{".github/workflows/ci.yaml"},
	nil,
	jactionlint.FixOptions{Mode: jactionlint.FixModeSafe, DryRun: true},
)
if err != nil {
	log.Fatal(err)
}
fmt.Println("applied:", result.Applied, result.ByRule)
fmt.Println("findings left:", len(result.Errors))
for _, failure := range result.Failures {
	fmt.Println("refused:", failure)
}
fmt.Print(result.Diff)
```

```
applied: 2 map[artipacked:1 missing-permissions:1]
findings left: 4
--- a/.github/workflows/ci.yaml
+++ b/.github/workflows/ci.yaml
@@ -1,7 +1,12 @@
 on: push
+
+permissions:
+  contents: read
 jobs:
   test:
     runs-on: ubuntu-latest
     steps:
       - uses: actions/checkout@v4
+        with:
+          persist-credentials: false
       - run: echo ${{ foo }}
```

`FixOptions` has these fields:

| Field | Meaning |
| --- | --- |
| `Mode` | `FixModeSafe` or `FixModeUnsafe`. It is required. |
| `Rules` | Fixes only the findings of these rule IDs. When empty, the `fix.rules` key of the config applies, and when that is empty too, every rule is fixed. |
| `DryRun` | Computes the fixes without writing files. The changes are in `FixResult.Diff`. |
| `MaxPasses` | The number of passes after which fixing gives up. Zero means the default. |

`FixResult` holds `Fixed` (the files that were rewritten, or would be), `Applied`, `ByRule`, `Errors` (the findings that remain, which refer to the text after the fixes), `Failures` and `Diff` (set only with `DryRun`). A fix that would damage the file is refused and listed in `Failures`. It is not an error return, so check `Failures`: the command exits with status 3 when it is not empty.

A fix run also prints. `LogWriter` gets the summary of what was fixed. Without `DryRun`, the writer of `NewLinter` gets the findings that remain. With `DryRun`, it gets the diff and `LogWriter` gets the remaining findings, so pass `io.Discard` when you read `Diff` and `Errors` yourself. [What fixing guarantees](./fix.md#what-fixing-guarantees) describes the passes, and [Which rules have a fix](./fix.md#which-rules-have-a-fix) lists the rules. The fixes that pin tags and images need the [online checks](#online-checks-from-go).

### Write and apply the baseline

`linter.WriteBaseline(files, path)` records the current findings, after the ignores and the minimum severity. Without files it lints the whole repository and replaces the baseline. With files it replaces only the entries of those files. An empty `path` is `.github/jactionlint-baseline.json`. Set `Baseline: true` on the options of a later run to hide the recorded findings.

```go
result, err := linter.WriteBaseline(nil, "")
if err != nil {
	log.Fatal(err)
}
fmt.Println("wrote", result.Entries, "entries for", result.Files, "files")

withBaseline, err := jactionlint.NewLinter(io.Discard, &jactionlint.LinterOptions{Baseline: true})
if err != nil {
	log.Fatal(err)
}
newFindings, err := withBaseline.LintRepository("")
if err != nil {
	log.Fatal(err)
}
fmt.Println("findings that are not in the baseline:", len(newFindings))
```

`WriteBaselineResult` also says whether the config already applies the baseline (`Applied`) and which config line would (`ConfigFile`, `ConfigValue`). A tool that reads the file uses `ReadBaselineFile(path)` or `ParseBaseline(bytes)`, which return a `*Baseline` with `Marshal()`. [Adopt a stricter configuration with a baseline](./baseline.md#baseline) explains how findings match entries.

### Migrate config and ignores

`MigrateConfig(src)` rewrites the deprecated keys of a config into the `rules` mapping and returns the new content and the migrated keys. `MigrateConfigFile(path)` does it in place.

```go
rewritten, migrated, err := jactionlint.MigrateConfig([]byte("require-shell: true\n"))
if err != nil {
	log.Fatal(err)
}
fmt.Printf("%s%v\n", rewritten, migrated)
```

```
rules:
  require-shell: error
[require-shell]
```

`MigrateZizmorIgnores(src)` rewrites the `# zizmor: ignore[...]` comments of a workflow into `# jactionlint ignore=...` comments and returns the new content and the migrated audit names. `MigrateZizmorIgnoreFiles(paths)` does the same in files and returns the names by file. [zizmor ignore comments](./ignore.md#zizmor-ignore-comments) says which comments migrate.

## Online checks from Go

The online checks query the GitHub API, which the other rules never do. They run in three cases:

- `LinterOptions.Online` is set.
- The config file sets `online: true`.
- `LinterOptions.OnlineOptions.Mode`, or the `online-options.mode` key, is `cache`, `strict` or `cache,strict`.

`OnlineOff` forces them off. An explicit `online: false` in a config file beats the `online-options.mode` of that file, but not `LinterOptions.OnlineOptions.Mode`, which turns the checks on whatever the config says. A config that you pass as `Config` replaces the config of the repository, so it also replaces an `online: true` in it. [Online checks](./online.md#online-checks) explains what they check.

The built-in client sends REST requests with the token that [Find the token](./online.md#find-the-token) describes (`OnlineOptions.TokenEnv`, `OnlineOptions.TokenFile`, `GITHUB_TOKEN`, `GH_TOKEN`, then the output of `gh auth token` unless `OnlineOptions.GitHubCLI` is false) and caches the answers on disk (see [cache and limits](./online.md#cache-and-limits)). Set `LinterOptions.GitHubClient` to serve the answers yourself, for instance in a test that must not touch the network. The client replaces the built-in client and its disk cache and implies nothing by itself: the rules still need one of the three cases above. The WebAssembly build has no built-in client, so `Online` there is an error unless you set `GitHubClient`.

This program serves one recorded answer. It selects the `archived-uses` rule so that the fixture can stay small.

```go
package main

import (
	"fmt"
	"io"
	"log"

	"github.com/jdx/jactionlint/v2"
)

// Recorded answers of the GitHub API, by lower-case owner/repo. A lookup that is not
// recorded fails, so a test notices when a rule starts to ask for something new.
const fixtures = `{
  "repos": {
    "actions/checkout": {
      "repo": {"archived": true, "default_branch": "main"}
    }
  }
}`

func main() {
	client, err := jactionlint.NewFixtureGitHubClient([]byte(fixtures))
	if err != nil {
		log.Fatal(err)
	}

	linter, err := jactionlint.NewLinter(io.Discard, &jactionlint.LinterOptions{
		Select:       []string{"archived-uses"},
		Online:       true,
		GitHubClient: client, // no request leaves the process
	})
	if err != nil {
		log.Fatal(err)
	}

	findings, err := linter.LintFile(".github/workflows/ci.yaml", nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range findings {
		fmt.Printf("%d:%d %s %s\n", f.Line, f.Column, f.Severity, f.ID)
	}

	coverage := linter.OnlineCoverage() // nil when the online checks did not run
	fmt.Println("coverage:", coverage.Status, "skipped lookups:", linter.OnlineSkipped())
}
```

```
6:15 warn archived-uses
coverage: complete skipped lookups: 0
```

### Serve answers for tests

`NewFixtureGitHubClient(data)` reads JSON and serves it. The JSON has the keys `repos`, `images` and `owners`:

- `repos` maps a lower-case `owner/repo` to its recorded data: `repo`, `tags`, `branches`, `refs` (keyed `heads/main` or `tags/v1`), `compare` (keyed `base...head`), `advisories`, `files` (keyed `ref:path`, each with `found` and `content`), `releases`, `environments` and `missing` for a repository that does not exist.
- `images` maps `registry/repository:tag` to a digest, or to `not-found`.
- `owners` maps a lower-case login to whether the user or organization exists.

[`testdata/online/github.json`](https://github.com/jdx/jactionlint/blob/main/testdata/online/github.json) is a larger example. A fixture client is strict: a call for data that is not recorded fails, and a failed lookup counts as skipped. A file that `files` does not list does not exist. `client.Calls()` returns how many calls it served.

`NewRecordingGitHubClient(inner)` wraps a client that you supply and records its answers. Its `Fixtures()` returns the JSON that `NewFixtureGitHubClient` reads. The built-in client has no exported constructor, so you cannot wrap it.

### Implement your own client

A `GitHubClient` has six methods: `Repository`, `Tags`, `ResolveRef`, `Branches`, `Compare` and `Advisories`. A client must be safe for concurrent use. It returns `ErrGitHubNotFound` (wrapped is fine) for something that does not exist or is not visible, and an error that satisfies `errors.Is(err, ErrGitHubRateLimited)` when the API refuses a request because of the rate limit.

A client can also implement these interfaces. The linter checks for each one and degrades when it is missing.

| Interface | Method | It enables | Without it |
| --- | --- | --- | --- |
| `GitHubContentClient` | `FileContent` | Reading the `action.yml` of an action that is not in `PopularActions`, and the declaration of a reusable workflow of another repository ([inputs and outputs of any action](./checks.md#check-remote-action-metadata)). | The checks that need those files report nothing for such actions. |
| `GitHubReleaseClient` | `Release` | The `immutable-release` policy of `unpinned-uses`, which accepts the tag of an [immutable release](./checks.md#check-immutable-release). | The policy behaves like `hash-pin`. |
| `GitHubEnvironmentClient` | `Environment` | Telling whether an environment has required reviewers ([environments without reviewers](./checks.md#check-untrusted-checkout-environment)). | The environment is trusted as it is without the online checks. |
| `GitHubOwnerClient` | `Owner` | Telling whether a user or organization exists, for the [`repojacking`](./checks.md#check-repojacking) check. | The `repojacking` check does nothing. |
| `GitHubPackageAdvisoryClient` | `AdvisoriesForPackages` | Finding the advisories that GitHub files under the full package name of an action in a subdirectory, such as `gradle/actions/setup-gradle`, in one lookup. | The `owner/repo` lookup of `Advisories` does not find them. |
| `ImageDigestClient` | `ImageDigest` | The digest of a container image tag, which `--online --fix=unsafe` writes after the tag ([pin images to digests](./online.md#pin-container-images-to-digests-with-online-fix-unsafe)). It returns `ErrImageNotFound` for an image that does not exist. | No digest is known, so images are not pinned. |
| `GitHubBranchScanner` | `CommitOnAnyBranch` | Telling whether a commit is on any branch in fewer requests ([`impostor-commit`](./checks.md#check-impostor-commit)). It returns `ErrGitHubBranchScanUnavailable` when it cannot answer, for example without a token. | The rule compares the branches one by one. |

### Read the result of a run

A lookup that fails never makes a `Lint*` call fail. The checks that needed it report nothing for those actions, and the run counts the lookup as skipped.

- `linter.OnlineSkipped()` is the number of lookups that were skipped. It is zero when the online checks did not run, and lookups that the allow and deny lists leave out are not counted.
- `linter.OnlineFailed()` is true when the mode is strict and a lookup was skipped. The command exits with status 3 then. A program that must not pass silently checks it.
- `linter.OnlineCoverage()` returns `*OnlineCoverage`, or nil when the online checks did not run. It has `Status` (`complete` or `incomplete`), `Mode`, `SkippedLookups`, `ExcludedRepositories`, `Evidence`, `SkippedBy` and `Answers`. `Answers` counts the answers by origin (network, cache, stale) and the time of the oldest cached one, as the local clock saw it. It is nil for a client of your own, which means unknown and not that every answer was fresh.
- `OnlineReport` is the JSON that `--online-report` writes: a `Version` and the coverage, with the status `disabled` when the checks were off.

Warnings about the online checks, such as a rate limit, go to `LogWriter`, except with the SARIF format, which carries them in its own output.

## Add your own rule

A rule of your own is a struct that embeds `RuleBase` and overrides the `Visit*` methods it needs. `OnRulesCreated` adds it. This program reports every step without a name.

```go
package main

import (
	"fmt"
	"io"
	"log"

	"github.com/jdx/jactionlint/v2"
)

// StepName reports every step that has no name.
type StepName struct {
	jactionlint.RuleBase // implements the Rule interface; override the Visit* methods you need
}

func NewStepName() *StepName {
	return &StepName{RuleBase: jactionlint.NewRuleBase("step-name", "every step has a name")}
}

func (r *StepName) VisitStep(n *jactionlint.Step) error {
	if n.Name == nil {
		r.Error(n.Pos, "every step must have a name")
	}
	return nil
}

func main() {
	linter, err := jactionlint.NewLinter(io.Discard, &jactionlint.LinterOptions{
		Profile: jactionlint.ProfileCorrectness,
		// Called for every file with the built-in rules. Return the rules to run.
		OnRulesCreated: func(rules []jactionlint.Rule) []jactionlint.Rule {
			return append(rules, NewStepName())
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	findings, err := linter.LintFile(".github/workflows/ci.yaml", nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range findings {
		fmt.Printf("%d:%d %s %s %q\n", f.Line, f.Column, f.Severity, f.ID, f.DocURL)
	}
}
```

```
6:9 error step-name ""
7:9 error step-name ""
7:23 error undefined-property "https://jactionlint.jdx.dev/rules#undefined-property"
```

The hook runs once for every workflow file and composite action that has a syntax tree: a file that is not valid YAML gets only its syntax finding. `RuleBase` implements the whole `Rule` interface with callbacks that do nothing, so you override only the ones you need: `VisitWorkflowPre`, `VisitWorkflowPost`, `VisitJobPre`, `VisitJobPost` and `VisitStep`. A rule reports with `Error(pos, message)` and `Errorf`, or with `ReportID(id, pos, message)`, `ReportIDf` and `ReportRange(id, pos, end, message)`, which also takes the end of the region. `OnDependabotRulesCreated` is the same hook for `.github/dependabot.yml`, with `DependabotRule` and `DependabotRuleBase`.

A rule you add differs from a built-in rule in these ways:

- Its findings have the level `error`, no `DocURL` and no `Fix`. `Error` and `Errorf` use the name of the rule as the `ID`. `ReportID` sets another `ID`, and `Kind` stays the name of the rule.
- Its `ID` is not a rule ID. A `rules` entry for it makes the config fail with `unknown rule ID`, `Select` refuses it, and `Rules()` does not list it.
- An ignore by its `ID` does not match. An ignore comment or an `IgnorePatterns` entry that is not the ID of a built-in rule is a regular expression that is matched against the message, so `IgnorePatterns: []string{"must have a name"}` ignores the findings above.

The slice that `OnRulesCreated` receives holds the built-in rules that the configuration enables for the file, so its length changes with the profile. `Rule.Name()` is the kind of a checker, and one checker can report several rule IDs: dropping a rule by its name drops every ID it reports. To turn a built-in rule off, use the `rules` mapping of the config. Syntax findings are reported even when the hook returns an empty slice. To add a built-in rule to jactionlint itself, follow [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md).

## Parse workflows, expressions and uses

The parsers work without a `Linter`. These are the building blocks:

| Call | Parses | Result |
| --- | --- | --- |
| `Parse(src)` | A workflow file. | A `*Workflow`, the root of the syntax tree, and one finding for each mistake in the syntax. It keeps going after the first. |
| `ParseAction(src)` | An `action.yml`. | A `*Workflow` with `Action` set, which is nil only when the input is not valid YAML. The steps of a composite action are in `Jobs` as one job. |
| `ParseDependabot(src)` | A `.github/dependabot.yml`. | A `*Dependabot` and the findings. |
| `ParseUses(spec)` | The value of a `uses:` key, of a step or a reusable workflow call. | A `*UsesRef` with `Kind` (`UsesAction`, `UsesReusableWorkflow`, `UsesDocker`, `UsesLocal`, `UsesInvalid`), `Owner`, `Repo`, `Subpath`, `Ref`, `RefKind` (`RefFullSHA`, `RefShortSHA`, `RefSemverTag`, `RefOther`, `RefDigest`, `RefNone`) and the image, tag and digest of a Docker reference. It never fails: an invalid value is `UsesInvalid` with `Problem` set. |
| `Workflow.Comments`, `NewCommentIndex(src)` | The YAML comments of a file. | A `*CommentIndex`. Look a node up by the line of its `Pos`: `Inline` is the trailing comment, `Before` and `After` are the comment blocks directly above and below (a blank line ends a block), and `Documented` tells whether a line has either. |
| `NewExprLexer(src)`, `NewExprParser()` | The inside of `${{ }}`. | An `ExprNode`, the root of the expression syntax tree. `LexExpression(src)` lexes a whole expression into a slice of `*Token`. |
| `NewExprSemanticsChecker(...)` | An expression syntax tree. | The `ExprType` of the expression (an interface that `ObjectType`, `ArrayType`, `StringType`, `NumberType`, `BoolType`, `NullType` and `AnyType` implement) and the findings about its types, contexts and functions. |
| `ValidateRefGlob(pattern)`, `ValidatePathGlob(pattern)` | A [filter pattern](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet) of `branches`, `tags` or `paths`. | The `[]InvalidGlobPattern` that the pattern breaks. |
| `NewProject(root)`, `NewProjects().At(path)` | A repository. | A `*Project` with `RootDir`, `WorkflowsDir`, `Config`, `ActionFiles` and `DependabotFiles`. |

`UsesRef` has the helpers `IsPinned`, `IsRepo`, `SameRepo` and `CanonicalName`. This program lists the `uses:` of the first example and tells whether each is pinned:

```go
package main

import (
	"fmt"
	"log"
	"maps"
	"os"
	"slices"

	"github.com/jdx/jactionlint/v2"
)

func main() {
	source, err := os.ReadFile(".github/workflows/ci.yaml")
	if err != nil {
		log.Fatal(err)
	}

	// Parse reports every mistake in the syntax and still returns the tree.
	workflow, syntaxFindings := jactionlint.Parse(source)
	for _, f := range syntaxFindings {
		fmt.Printf("%d:%d %s\n", f.Line, f.Column, f.Message)
	}
	if workflow == nil {
		return
	}

	for _, id := range slices.Sorted(maps.Keys(workflow.Jobs)) {
		for _, step := range workflow.Jobs[id].Steps {
			action, ok := step.Exec.(*jactionlint.ExecAction)
			if !ok {
				continue // a run: step
			}
			uses := jactionlint.ParseUses(action.Uses.Value)
			fmt.Printf("%s line %d: %s %s ref=%s pinned=%t\n",
				id, action.Uses.Pos.Line, uses.Kind, uses.CanonicalName(), uses.RefKind, uses.IsPinned())
		}
	}
}
```

```
test line 6: action actions/checkout ref=semver-tag pinned=false
```

To walk the tree with several passes in one traversal, add them to a `NewVisitor()` and call `Visit(workflow)`: every `Rule` is a `Pass`.

To check an expression, lex it, parse it and check the result. Pass the text inside `${{ }}` without the opening `${{` and with the closing `}}`, as the example does.

```go
lexer := jactionlint.NewExprLexer("github.ref == 'refs/heads/main' }}")
node, syntaxErr := jactionlint.NewExprParser().Parse(lexer)
if syntaxErr != nil {
	log.Fatal(syntaxErr)
}

// No untrusted-input check, and no list of vars or secrets to compare names with.
checker := jactionlint.NewExprSemanticsChecker(false, nil, nil)
// Without these two calls no context and no special function is available, and every context is rejected.
contexts, functions := jactionlint.WorkflowKeyAvailability("jobs.<job_id>.steps.if")
checker.SetContextAvailability(contexts)
checker.SetSpecialFunctionAvailability(functions)

exprType, findings := checker.Check(node)
fmt.Println("type:", exprType, "findings:", len(findings))
```

```
type: bool findings: 0
```

`WorkflowKeyAvailability(key)` returns the contexts and the special functions that GitHub allows at a workflow key such as `jobs.<job_id>.concurrency`. For a key it does not know it returns empty slices, which the checker reads as "none available".

The data tables behind the checks are exported:

- `PopularActions` holds the metadata of popular actions, keyed like `actions/checkout@v4`. A [script](https://github.com/jdx/jactionlint/blob/main/scripts/generate-popular-actions) collects it.
- `AllWebhookTypes` maps each webhook name to its activity types. A [script](https://github.com/jdx/jactionlint/blob/main/scripts/generate-webhook-events) collects it.
- `ActionMetadata` is the metadata of an action (`action.yml`), which the checks use for the inputs of `with:` and the type of `steps.<id>.outputs`.
- The availability data of `WorkflowKeyAvailability` comes from a [third script](https://github.com/jdx/jactionlint/blob/main/scripts/generate-availability).

## Rule metadata and schemas

The registry of rules is data you can read:

| Call | Returns |
| --- | --- |
| `Rules()` | A copy of the `RuleInfo` of every rule, sorted by ID. |
| `LookupRule(id)` | The `RuleInfo` of one rule and whether it exists. |
| `RenamedRules()` | The retired rule IDs, the rule that reports their findings instead and the option that turns them on. Ignores still accept a retired ID. |
| `RuleDocURL(id)` | The documentation URL of an ID. It does not check that the ID exists. |
| `OutputSchema()` | The JSON Schema of `--format json` and `jsonl` output. |
| `ConfigSchema()` | The JSON Schema of the config file. |

A `RuleInfo` has these fields:

| Field | Meaning |
| --- | --- |
| `ID` | The rule ID. |
| `Group` | `correctness`, `security`, `policy` or `style`. |
| `Summary` | A one-line description. |
| `DefaultLevel` | The level when the rule is enabled without an explicit level. |
| `Profile` | The first profile that enables the rule. Empty for an opt-in rule, which runs only when the config turns it on, and for an online rule, which runs with the online checks whatever the profile. |
| `Online` | Whether the rule needs the online checks. |
| `Fixable` | Whether the rule can attach a `Fix` to its findings. |
| `DocsAnchor` | The anchor of the section on the [checks page](./checks.md). It can be empty. |
| `Options` | The rule options, each a `RuleOption` with `Name`, `Kind`, `Default` and `Summary`. |

To find the rules that use the network or that can fix a finding, filter on `Online` and `Fixable`:

```go
for _, rule := range jactionlint.Rules() {
	if rule.Online || rule.Fixable {
		fmt.Println(rule.ID, rule.Group, rule.DefaultLevel, rule.Online, rule.Fixable)
	}
}
```

`ConfigSchemaURL` and `OutputSchemaURL` are the URLs where the schemas are published. The [rules page](./rules.md#rules) lists every rule, and [JSON Schema of the config](./config.md#json-schema) describes the schema.

## Library versioning

The version of the module is the version of the command: the repository is tagged once, for the command. The package documentation says that the library does not follow semantic versioning and that any patch release may introduce breaking changes.

The code states two contracts:

- Rule IDs are never renamed or reused. The documentation of `RuleInfo` says so, and `TestRuleIDsAreStable` fails when a released ID disappears. The IDs that were merged into other rules before v2 are in `RenamedRules()`.
- The fields of the JSON output are never removed or renamed within a major version, and fields can be added, so ignore the fields you do not know. The documentation of `OutputSchema()` says so.

Treat everything else as open to change, in particular:

- the syntax tree (`Workflow`, `Job`, `Step` and the other nodes);
- the `Rule*` structs and their `NewRule*` constructors, whose parameters follow what each rule needs (the source text, a `*Project`, a cache, and for some a type that is not exported): run the built-in rules through a `Linter`;
- the fields of `LinterOptions` and `Config`, which have grown since v1.

Pin the version of jactionlint in your `go.mod` and read [CHANGELOG.md](https://github.com/jdx/jactionlint/blob/main/CHANGELOG.md) before you upgrade.

## Go version compatibility

The minimum Go version is the `go` line of [go.mod](https://github.com/jdx/jactionlint/blob/main/go.mod), and a module that imports jactionlint needs at least that version. [Build from source](./install.md#build-from-source) says how the `go` command picks a toolchain for it. The workflows of the repository build and test with one Go version, not with a matrix, so the minimum is not tested separately.

## Next steps

- [Profiles and config keys](./config.md#profiles) for what `Profile` and `Config` select.
- [Fields of a finding](./output.md#fields-of-a-finding) for the JSON that `Format: jactionlint.FormatJSON` writes.
- [Online checks](./online.md#online-checks) for tokens, caching and what each online rule asks.
- [The Go API of v2](./v2-migration.md#go-api) for what changed since the v1 library.
- [Add a rule](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#add-a-rule) to jactionlint itself.
