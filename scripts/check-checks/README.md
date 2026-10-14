# check-checks

check-checks keeps the example outputs and the playground links of [the checks page](../../docs/checks.md) in step with the linter. It lints every example input of the page, rewrites the `Output:` block and the `[Playground]` link below it, and fails when the file differs from the result. This page also specifies the format of a section of the checks page, which the tool parses.

## When to run it

Run it after every change to `docs/checks.md`, and after every change that alters what jactionlint prints for an example: the text of a finding, the behavior of a rule or the example configuration. You rarely start it by hand, because hk runs it in the `checks_doc` step of `hk.pkl`:

| Where | Command |
| --- | --- |
| `hk check --all` (`mise run lint`), the `lint` job of [ci.yaml](../../.github/workflows/ci.yaml) and the pre-push hook | `go run ./scripts/check-checks -quiet ./docs/checks.md` |
| `hk fix --all` (`mise run lint-fix`) | `go run ./scripts/check-checks -fix ./docs/checks.md` |

The step does not run in the pre-commit hook. A failure means one of these:

- `checks document has some update`: an `Output:` block or a `[Playground]` link differs from what the tool generates. The message prints a diff and the command that fixes it. Read the diff, because it shows how your change reaches the documentation, then run the fix command and commit the result.
- `error at line N while generating section "..."`: the page does not follow the format below. The fix command cannot repair it. Correct the page by hand.

## Prerequisites

- Go.
- Linux or macOS. The tool does not support Windows, and the tests that run it are skipped there.
- `shellcheck` and `pyflakes` on `PATH`. Examples with a `run:` script print their findings. `mise install` installs both.
- The repository root as the working directory. The tool lints in the project of the working directory and reads `testdata/online/github.json` from its root.

## Usage

```
check-checks [-fix] [-quiet] FILE
```

The tool takes exactly one file.

| Command | What it does |
| --- | --- |
| `go run ./scripts/check-checks FILE` | Checks that FILE is up to date. It prints a diff and exits with status 1 when it is not. |
| `go run ./scripts/check-checks -quiet FILE` | Checks FILE without the trace log, which prints one line per parser step to stderr. |
| `go run ./scripts/check-checks -fix FILE` | Rewrites FILE in place when it is out of date. Combine with `-quiet` to silence the trace log. |

The exit status is 0 when the file is up to date or was fixed, and 1 for an outdated file, a format error, an unreadable file or wrong arguments. `-h` prints the usage and exits with status 0.

```sh
go run ./scripts/check-checks -quiet ./docs/checks.md
go run ./scripts/check-checks -fix ./docs/checks.md
```

## The format of a section

The tool reads the page line by line. Every line that starts with `## ` is the heading of a section and needs an anchor line above it. This is the smallest valid section:

````markdown
<a id="some-id"></a>
## This is title of the check

Example input:

```yaml
# This section is referred to generate the output and the playground link
on: push
jobs:
  test:
    runs-on: linux-latest # oops
    steps:
      - run: echo hello
```

Output:

```
THIS CODE BLOCK WILL BE AUTO-GENERATED
```

[Playground](URL_WILL_BE_AUTO_GENERATED)

Explanation for the check...
````

The parts, in order:

1. **Anchor.** `<a id="..."></a>` alone on a line. The id is not empty and appears once in the file. Several anchor lines in a row may precede one heading. The anchor is the public name of the section: `jactionlint --explain` and the documentation URLs use it. An anchor that no `## ` heading follows, such as one above a `###` heading, starts no section, but its id must be unique too.
2. **Heading.** The next line starts with `## ` and its text appears once in the file. The text also selects the kind of file the examples are (see below).
3. **Example input.** The line `Example input:`, then a fence of exactly ```` ```yaml ````. The input is not empty and must make jactionlint report at least one finding, otherwise the tool stops with `the input example caused no error`.
4. **Configuration and files (optional).** The blocks `Example config:` and `Example file: PATH` that an example needs, between the input and the output (see below).
5. **Output.** The line `Output:`, then a fence of exactly three backticks. The tool replaces the content of the fence.
6. **Link.** A line that starts with `[Playground](` and ends with `)`. The tool replaces the line.

Prose goes anywhere else in the section: between the heading and the first example, after the link, between examples. A section may have several examples: write another `Example input:` after the link, and each one is linted on its own. The page may use `###` headings inside a section.

Every section needs at least one example. A `## ` heading with no `Example input:` fails with `unexpected state "heading"`. The tool reads the lines `## `, `Example input:` and `Output:` wherever they are, also inside a code fence, so an example input cannot contain a line that starts with `## `.

### The kind of file an example is

| The heading starts with | The example is | Findings name it |
| --- | --- | --- |
| `Dependabot` | the Dependabot configuration | `.github/dependabot.yml` |
| `Composite action` | the metadata of a composite action | `.github/actions/example/action.yml` |
| anything else | a workflow file | `test.yaml` |

### Examples that need the online checks

Start the example input with the line `# requires --online`, as its very first line. The tool then lints the example with the online checks on, using the recorded GitHub answers of `testdata/online/github.json` instead of the network, and with `GITHUB_REPOSITORY` set to `example/environments`. The playground has no network, so give such a section the skip marker for the link (see below).

### Examples that need another configuration, rules or files

An example that shows a rule the example configuration turns off, a pedantic or opt-in rule, a check of several files, or a message about a file of the repository does not need a hand-written output. Describe what it needs and the tool generates the output like any other. Three things set it up. Each applies to one example: the next `Example input:` of the section starts again from the example configuration.

**Marker lines.** Comment lines at the very top of the input, before any other line, choose how it runs. The head ends at the first line that is not a marker, so a comment of the example that follows stays a comment. The playground link drops them like every comment line.

| Marker | Effect |
| --- | --- |
| `# requires --online` | The online checks, with the recorded answers (see above). |
| `# select: id[,id]` | Report only the findings of these rules, as `--select` does. A rule that is off in the configuration runs at its default level. Use it when the output of the section shows one rule (or a few) and other rules of the example would add findings. An unknown ID fails the tool. |
| `# profile: pedantic` | Run the profile, as `--profile` does. |
| `# file: .github/workflows/ci.yml` | Name the example. Findings print this path instead of `test.yaml`, and the project has the file, so a rule that reads the workflows of the repository sees it. |

**`Example config:`.** The line, a blank line and a fence of exactly ```` ```yaml ````, after the input. The block is the configuration file the example runs with. It is merged over the [example configuration](#the-example-configuration): a key of the block replaces the same key, and an entry of `rules` replaces the entry of the same rule, so every other rule keeps its level. To turn on an opt-in rule write `rules: {my-rule: warn}`. A section has one `Example config:` per example, the block is not empty and the configuration must be valid.

**`Example file:`.** The line `Example file: PATH`, a blank line and a fence, after the input. The tool writes the content to PATH in a temporary directory that is the repository of the example (it has `.git`, `.github/workflows` and nothing else), and lints the example there. The path is relative to the repository, uses slashes, and has no empty, `.`, `..` or `.git` part. The fence names any language (```` ```yaml ````, ```` ```go ````), and an empty fence makes an empty file, which is how an example shows that a directory exists. Use it for the local action or reusable workflow that the example calls, the files that a path filter matches, or the second workflow of a deadlock. Because the repository is a directory of its own, a message about a file of the jactionlint repository cannot leak into the output.

This is a complete example. Its rules are on in no profile of the default configuration, so it says which one it shows:

````markdown
Example input:

```yaml
# select: unknown-action-input
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/greet
        with:
          nmae: octocat
```

Example file: .github/actions/greet/action.yml

```yaml
name: Greet
description: Says hello
inputs:
  name:
    description: Who to greet
runs:
  using: composite
  steps:
    - run: echo hello
      shell: bash
```

Example config:

```yaml
rules:
  unknown-action-input: warn
```

Output:

```
THIS CODE BLOCK WILL BE AUTO-GENERATED
```

<!-- Skip playground link -->
````

The blocks and the markers reach the documentation as they are. Prose of the section should say, in a sentence, what the example config and the files are for, and must not repeat the config in a second block: a second copy is not checked.

### Playground links and hand-written outputs

Two comments switch the generation off for one example. Write them as plain lines of the page, outside any code fence:

- `<!-- Skip playground link -->` takes the place of the `[Playground](...)` line. The playground lints with the example configuration and one file, so it cannot reproduce an example that needs another configuration, a marker or a file. `docs/tests/wasm.test.ts` decodes every link and compares it with the output, so such an example must keep this comment.
- `<!-- Skip update output -->` goes after the `Output:` line and before the fence. The tool keeps the fence as you wrote it. Do not use it on the checks page (see below).

Without a link line or a skip comment after the output, the section fails with `unexpected state "after output"`.

Skip the link whenever the example runs with a configuration, a marker or files. Never skip the output of the checks page: a skipped output is not verified, goes stale when a message changes, and `TestChecksPageSkipsNoOutput` fails when the page has one. The tool still supports the comment for other documents and for its own tests.

| The example shows | What to write |
| --- | --- |
| A rule that runs under the example configuration | The generated output and link, with no marker. |
| A rule of the `pedantic` profile, an opt-in rule, a rule that needs configuration | `# select:` when other rules add findings, an `Example config:` block, the generated output and `<!-- Skip playground link -->`. |
| Several files, or a message about a file of the repository | `Example file:` blocks (and `# file:` to name the example), the generated output and `<!-- Skip playground link -->`. |
| An online rule | `# requires --online`, the generated output and `<!-- Skip playground link -->`. |

[The contributing guide](../../CONTRIBUTING.md#about-checks-doc) owns the policy of which rules get which kind of example.

## How it works

1. The tool parses the page with a small state machine: anchor, heading, example input, output header, output block and link.
2. For every example input it lints the text with the `jactionlint` package of the working tree, not an installed binary, under the configuration of `internal/exampleconfig` (with the `Example config:` block merged over it) and with these settings:
   - The example runs in a new temporary directory that is a repository: it has `.git`, `.github/workflows` and the `Example file:` blocks, and the tool changes into it. A file of the jactionlint repository is never seen, so a message about a missing file is the same on every machine.
   - The file has the name of its kind (see above), or the path of `# file:`, so findings read `test.yaml:5:14`.
   - The markers `# select:` and `# profile:` set the options `--select` and `--profile`.
   - Color is off.
   - `shellcheck` and `pyflakes` run from `PATH`.
   - The date is a fixed day (`exampleDay` in `main.go`), so an output does not change when a date-dependent check passes, such as the retirement of a runner label.
   - The root of the temporary repository in a message is replaced by `/path/to/repo`, so an output does not depend on your file system.
3. It writes the findings into the output block and the permalink into the link line.
4. It compares the result with the original. `check-checks -fix` writes the file. Without that flag the tool prints a diff and exits with status 1.

### The example configuration

The configuration of [internal/exampleconfig](../../internal/exampleconfig/exampleconfig.go) is the default profile with the rules `artipacked`, `concurrency-limits`, `dangerous-triggers`, `excessive-permissions`, `missing-permissions`, `missing-timeout`, `obfuscation`, `unpinned-images` and `unpinned-uses` turned off, and with the option `pedantic` of `unpinnable-action` on. An example is a minimal workflow, so it lacks the timeouts, permissions, pinned actions and concurrency group that these rules ask for, and their findings would hide the check the section explains.

Every other rule of the default and `correctness` profiles is on. An example for one rule must not trigger another one: give a job `runs-on`, pin `uses:` and so on. When a new rule fires on most examples, turn it off in `internal/exampleconfig`. That changes the CLI and the playground together, because the playground lints with the same configuration.

### Playground links

A link is `https://jactionlint.jdx.dev/#` followed by the example input compressed with zlib and encoded in base64. The tool removes every line whose first non-blank character is `#` before it compresses: comments such as `# ERROR: ...` and the `# requires --online` marker. The playground therefore reports other line numbers than the `Output:` block when the example has comments.

The link goes to the site root on purpose. A script of the home page moves a hash that looks like a permalink to [the playground](https://jactionlint.jdx.dev/playground), and `docs/tests/wasm.test.ts` decodes every link of the page, lints it with the real wasm build and compares the messages with the `Output:` block above it. Run that test with `mise run docs:test`. The playground cannot enable a rule that the example configuration turns off, so a section that shows such a rule skips the link.

## Pitfalls

- Write the markers of the section as plain lines. A skip comment inside a code fence is text of the fence, not a marker.
- The fences are exact: ```` ```yaml ```` around an input and an `Example config:`, and three backticks around an output. Any other info string, such as ```` ```text ````, is not recognized there. An `Example file:` fence may name any language.
- `Example config:` and `Example file:` go between the input and `Output:`, and a section must not repeat the config in another block: the copy is not checked and goes stale.
- A skipped playground link is not a reason to skip the output. Only `<!-- Skip update output -->` stops the check, and `TestChecksPageSkipsNoOutput` rejects it on the checks page.
- Do not edit a generated block or a `[Playground]` link by hand. The next run overwrites them.
- The index at the top of the page is written by hand. The tool leaves everything above the first anchor alone, and `TestChecksIndexListsEverySection` keeps the index complete.
- A change to the message of any rule changes the output of every example that shows it. Review the diff before you commit it.
- An example input always makes jactionlint report something. To show code that jactionlint accepts, put it in a fence that no `Example input:` line precedes. The tool ignores such a fence.

## Tests

`go test ./scripts/check-checks` runs the tool over the fixtures in `scripts/check-checks/testdata`: each `ok/*.in` must become the matching `ok/*.out`, and each `err/*.md` must fail. Other tests read the checks page itself:

- `TestEveryRuleCanBeExplained` and `TestRuleRegistryIsWellFormed` require an anchor for the `DocsAnchor` of every rule.
- `TestActionScopeIsDocumented` requires the section `check-composite-actions` to name every rule ID in backticks.
- `TestChecksIndexListsEverySection` requires the index above the first anchor of the page to link every section and every subsection that has an anchor of its own. Add the link there when you add a section.
- `TestChecksPageSkipsNoOutput` (in this directory) fails when the checks page has an `<!-- Skip update output -->` line.
- `TestActionsLockDocsExample` lints the repository of the lock file section, whose example is a whole repository and not an `Example input:`.

## Next steps

- [Contributing guide, writing the checks page](../../CONTRIBUTING.md#about-checks-doc): the policy of which rules get which kind of example.
- [The checks page](../../docs/checks.md): the file this tool maintains.
- [Scripts and generated files](../README.md): the other tools, and what to run after a change.
