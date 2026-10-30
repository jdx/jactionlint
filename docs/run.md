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

A matrix that is built from the output of another job (`fromJSON(needs.setup.outputs.matrix)`) cannot be expanded before the run. The plan says so instead of guessing.

| Option | Meaning |
| --- | --- |
| `-l`, `--list` | Print the plan and exit. |
| `--json` | Print the plan as JSON (with `--list`). |
| `-j`, `--job ID` | Select a job and the jobs it needs. Repeat the option for more jobs. |
| `-e`, `--event NAME` | The event that triggers the workflow (default `push`). The plan notes a workflow that has no such trigger. |
| `-C`, `--directory DIR` | Run as if started in `DIR`. |

Running the steps is not implemented yet.
