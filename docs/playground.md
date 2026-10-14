---
title: Playground
description: The jactionlint playground runs the real jactionlint, compiled to WebAssembly, in your browser, so you can check a workflow without installing anything.
layout: page
---

<Playground />

The playground runs the real jactionlint, compiled to WebAssembly, in your browser. Paste a workflow file to see the findings jactionlint reports, without installing anything. What you type stays in your browser.

## Use it

1. Paste or type a workflow into the editor. The findings update after a short pause while you type, and at once after a paste.
2. Read the list of findings. Each shows its line and column, its message and its rule ID. Select a finding to move the cursor to its position, or a link in a message to open it.
3. Select **Permalink** to put the workflow into the address of the page and copy that address. Anyone who opens it sees the same workflow, which the address holds in its fragment. Permalinks of the form `https://jactionlint.jdx.dev/#...`, made before the playground moved to this page, redirect here.
4. To load a file from GitHub, enter its URL in the **GitHub or Gist URL** field and select **Check**. A GitHub `blob` URL and a gist URL are converted to the raw file, and any other URL is fetched as it is.

The page also opens a workflow from its address. `?s=` takes the workflow text, percent-encoded, and `?u=` takes a URL like the one of the field. When both are present, `?s=` wins over `?u=`, and both win over a permalink fragment.

The browser asks before you leave the page with changes that no permalink holds.

## How it differs from the command line

The playground checks one workflow with a fixed configuration, so it does not replace a run in your repository.

| Setting | Playground | Command line |
| ------- | ---------- | ------------ |
| Profile | `default`, with the rules below turned off | The profile of your config file, `default` when you configure nothing |
| Config file and baseline | None of yours: the fixed configuration below | `.github/jactionlint.yaml`, or else the [user-global config file](./config.md#where-jactionlint-reads-the-config-file), with its durable ignores, and a baseline |
| shellcheck and pyflakes | Not run, so the scripts of `run:` get no shellcheck or pyflakes findings | Run when installed |
| Network | None, so the [online checks](./online.md#online-checks) are not available | Used by the online checks when you turn them on |
| Files | One workflow | Workflows, composite actions, `dependabot.yml` and the Actions lock file |

The fixed configuration is the one that the examples of the [checks page](./checks.md) use. Each example is a minimal workflow, so it lacks what the default profile asks of every real workflow. The configuration turns these rules off, so that an output shows the check it explains:

- `artipacked`
- `concurrency-limits`
- `dangerous-triggers`
- `excessive-permissions`
- `missing-permissions`
- `missing-timeout`
- `obfuscation`
- `unpinned-images`
- `unpinned-uses`

It also sets the option `pedantic` of the online rule `unpinnable-action`, because the example of that rule needs it. The rule needs the network, so it does not run in the playground. A Playground link under an example of the checks page opens that example here, with the same configuration.

To use another [profile](./config.md#profiles), check scripts with shellcheck or run the online checks, [install jactionlint](./install.md) and run it on your repository.

## Next steps

- [Install jactionlint](./install.md) to check your own repository.
- [Run jactionlint](./usage.md) for the first run and the choice of a profile.
- [Online checks](./online.md) for the checks that need the network.
- [Profiles](./config.md#profiles) for what `correctness`, `default` and `pedantic` enable.
- [Checks](./checks.md) for every check, with examples that open in the playground.
