---
title: Playground
description: The jactionlint online playground runs jactionlint in your browser through WebAssembly.
layout: page
---

<Playground />

The playground lints with the `default` profile, except that a few rules every minimal example would break (`missing-timeout`, `missing-permissions`, `unpinned-uses` and a few more, see [the contributing guide](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#how-to-write-checks-document)) are off. It has no network, so the [online checks](usage.md#online-checks) are not available. Use the command line for the full set of rules and the other [profiles](config.md#profiles).
