# Run workflows locally

`jactionlint run` runs the jobs of a workflow on your machine, so that you can debug a `run:` script, an expression, a matrix or the `needs` wiring without pushing a commit. It needs no container engine and no runner image.

This page describes what the command does today. Each section says what is still missing.

## See what would run

```sh
jactionlint run --list                     # every workflow of the repository
jactionlint run --list ci                  # .github/workflows/ci.yml (the extension is optional)
jactionlint run --list ci -j test          # the job `test` and the jobs it needs
```

The plan lists the jobs in the order in which they can run, grouped in stages (the jobs of a stage do not depend on each other), and expands each matrix into the cells that GitHub would create:

```text
.github/workflows/ci.yml  (CI)
  stage 1
    test (ubuntu-latest, 1.22)   runs-on: ${{ matrix.os }}
    test (ubuntu-latest, 1.23, true)   runs-on: ${{ matrix.os }}
    test (macos-latest, 1.23)   runs-on: ${{ matrix.os }}
    lint   runs-on: ubuntu-latest   if: github.event_name == 'push'   name: Lint
  stage 2
    shards (matrix known at run time: ${{ fromJSON(needs.lint.outputs.matrix) }})   needs: lint
```

The expansion follows the rules of the runner: the product of the rows, then `exclude` (a partial match removes the combination), then each `include` entry, which is added to every combination whose original values it does not overwrite and otherwise becomes a combination of its own. The name of a cell is the job ID and the scalar values of the combination, so a YAML `3.10` shows as `3.1`, like on GitHub.

A matrix that is built from the output of another job (`fromJSON(needs.setup.outputs.matrix)`) cannot be expanded before the run. The plan says so instead of guessing. A matrix that only needs `github`, `vars`, `inputs` or `env` (`fromJSON(inputs.oses)`) is expanded.

## Expressions

The plan evaluates the `${{ }}` expressions it can: the `name:` and `runs-on:` of each cell, and the `if:` of a job. An `if:` that reads only `github`, `vars`, `inputs`, `env`, `runner` or `matrix` is marked `=> if is true` or `=> if is false`, and `jactionlint run` will skip a job whose condition is false. A condition that reads `needs` or `steps` is left as it is written.

The evaluator follows the rules of GitHub: comparisons and property names ignore case, values of different types compare as numbers (`'1' == 1`, `null == 0`), `&&` and `||` return one of their operands, `a.*.b` filters an array or the values of an object, and `0`, `''`, `null` and `NaN` are false. It implements `contains`, `startsWith`, `endsWith`, `format`, `join`, `toJSON`, `fromJSON`, `hashFiles` and the status functions `success`, `failure`, `always` and `cancelled`.

The `github` context is filled from git: the repository comes from the `origin` remote, `sha` and `ref` from `HEAD`, and the actor from `git config user.name`. The `runner` context describes this machine, with `runner.environment` set to `self-hosted`.

Differences from GitHub that you can see in the plan: a matrix built from JSON lists its keys in alphabetical order instead of the order of the JSON text, which changes the order of the values in a job name, not which cells exist.

| Option | Meaning |
| --- | --- |
| `-l`, `--list` | Print the plan and exit. |
| `--json` | Print the plan as JSON (with `--list`). |
| `-j`, `--job ID` | Select a job and the jobs it needs. Repeat the option for more jobs. |
| `-e`, `--event NAME` | The event that triggers the workflow (default `push`). The plan notes a workflow that has no such trigger. |
| `--event-file FILE` | A JSON file with the event payload, which is `github.event`. Without it, jactionlint makes a small payload for the event (a `push` has `ref`, `after` and `head_commit`; a `pull_request` has `number` and `head` and `base` with `ref` and `sha`). |
| `-i`, `--input NAME=VALUE` | An input of `workflow_dispatch`, which is `inputs.NAME`. Repeat the option for more inputs. |
| `-C`, `--directory DIR` | Run as if started in `DIR`. |

Running the steps is not implemented yet.
