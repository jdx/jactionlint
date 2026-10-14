# References

Find the GitHub documentation, the specifications and the tools that the checks of jactionlint are based on. Use it to look up where a check comes from or where to read more. The last section lists the places where jactionlint itself lives.

## GitHub Actions documentation

| Page | What the checks take from it |
| ---- | ---------------------------- |
| [Workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax) | The keys and values of a workflow file. |
| [Filter pattern cheat sheet](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#filter-pattern-cheat-sheet) | The glob syntax of `branches`, `tags` and `paths`. |
| [Expressions](https://docs.github.com/en/actions/reference/workflows-and-actions/expressions) | The operators and types of `${{ }}` expressions. |
| [Built-in functions](https://docs.github.com/en/actions/reference/workflows-and-actions/expressions#functions) | The functions that an expression can call. |
| [Contexts](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts) | The properties of each context and where each context is available. |
| [Events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows) | The event names and their activity types. |
| [Self-hosted runners](https://docs.github.com/en/actions/concepts/runners/self-hosted-runners) | The labels of runners that you host. |
| [Secure use reference](https://docs.github.com/en/actions/reference/security/secure-use) | The guidance behind the findings about script injection, pinning and permissions. |
| [Metadata syntax for actions](https://docs.github.com/en/actions/reference/workflows-and-actions/metadata-syntax) | The keys of the `action.yml` of a composite action. |
| [Reusing workflow configurations](https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations) | The inputs, outputs and secrets of reusable workflows. |
| [Dependabot options reference](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference) | The keys of `dependabot.yml`. |

## Tools

| Tool | Relation to jactionlint |
| ---- | ----------------------- |
| [shellcheck](https://github.com/koalaman/shellcheck) | jactionlint runs it on the shell scripts of `run:`. |
| [pyflakes](https://github.com/PyCQA/pyflakes) | jactionlint runs it on the Python scripts of `run:`. |
| [ruff](https://docs.astral.sh/ruff/) | `--ruff` runs its pyflakes rules instead of pyflakes. |
| [actionlint](https://github.com/rhysd/actionlint) | The project that jactionlint is forked from. [Coming from actionlint](./actionlint.md) compares the two. |
| [zizmor](https://docs.zizmor.sh/) | The audits that inform the security rules. [jactionlint and zizmor](./zizmor-parity.md) compares the two. |
| [hk](https://hk.jdx.dev) | A Git hook manager that runs jactionlint ([setup](./hooks.md#hk)). |
| [mise](https://mise.jdx.dev/) | A tool manager that installs jactionlint ([setup](./install.md#mise)). |

## Specifications

| Specification | Used for |
| ------------- | -------- |
| [POSIX crontab](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/crontab.html#tag_20_25_07) | The cron syntax of `on.schedule`. |
| [IANA time zone database](https://www.iana.org/time-zones) | The values of `timezone` in `on.schedule`. |
| [SARIF](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) | The format of `--format sarif` ([Output formats](./output.md)). |
| [Go text/template](https://pkg.go.dev/text/template) | The syntax of a template given to `--format`. |

## jactionlint

| Resource | Contents |
| -------- | -------- |
| [Repository](https://github.com/jdx/jactionlint) | The source, the issues and the pull requests. |
| [Releases](https://github.com/jdx/jactionlint/releases) | The release archives and their checksums. |
| [Go API reference](https://pkg.go.dev/github.com/jdx/jactionlint/v2) | The documentation of the Go package ([Go API](./api.md)). |
| [Playground](https://jactionlint.jdx.dev/playground) | jactionlint in the browser. |
| [llms.txt](https://jactionlint.jdx.dev/llms.txt) | An index of these pages for tools that read them, with `llms-full.txt` for the full text. |

## Next steps

- [Checks](./checks.md) for what each check reports, with the documentation it follows.
- [Command line](./cli.md#programs-and-network) for the programs that jactionlint runs and the hosts it contacts.
- [Go API](./api.md) for the reference of the Go package.
