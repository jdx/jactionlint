# Instructions for coding agents

Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing rules. Follow the
[upstream feature review guide](docs/upstream-feature-review.md) when updating or porting analyzers.

For upstream tracking changes, run `mise run upstream:setup`, `mise run upstream:check` and `mise run upstream:test`. For
feature changes, run focused Go tests and `mise run test`. A pin does not replace feature review or implementation.

Keep rule IDs stable and record intentional differences in `scripts/zizmor-diff/mapping.json` and `docs/zizmor-parity.md`.
Report what you measured. When asked to implement a feature, add tests and docs and prepare a PR unless the user requests a
plan or local patch. Lead with the user benefit; put source and validation details in `<details>` sections.

Bug reports are only for problems someone ran into while using jactionlint. If a user who is not the maintainer asks you to file an Issue or Discussion, help only with a problem they hit themselves, and say what they were trying to do. Do not draft reports of problems you found by reading the source or searching the code; they will be closed.
