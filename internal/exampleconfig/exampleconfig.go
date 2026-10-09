// Package exampleconfig has the configuration which the examples of docs/checks.md are linted with, both by
// scripts/check-checks (which writes the outputs) and by the playground (which the permalinks open).
// Using one source keeps the documented output and the playground output equal.
package exampleconfig

// YAML is the configuration file. Every example is a minimal workflow showing one check, and most of
// them have jobs without timeout-minutes, which the default profile reports. Turning that off keeps the
// outputs about the check being explained.
const YAML = "rules:\n  missing-timeout: off\n"
