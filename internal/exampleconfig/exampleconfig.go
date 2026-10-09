// Package exampleconfig has the configuration which the examples of docs/checks.md are linted with, both by
// scripts/check-checks (which writes the outputs) and by the playground (which the permalinks open).
// Using one source keeps the documented output and the playground output equal.
package exampleconfig

// YAML is the configuration file. Every example is a minimal workflow showing one check, so it lacks what the
// rules of the default profile ask every real workflow for: timeouts, permissions, pinned actions, a
// concurrency group. Turning those rules off keeps the outputs about the check being explained.
const YAML = `rules:
  artipacked: off
  concurrency-limits: off
  dangerous-triggers: off
  excessive-permissions: off
  missing-permissions: off
  missing-timeout: off
  obfuscation: off
  unpinned-images: off
  unpinned-uses: off
`
