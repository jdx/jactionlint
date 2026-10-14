#!/usr/bin/env bash
# Go wrapper for go-to-wheel (`--go-binary`). go-to-wheel always runs `go build ... -o <out> .` in the module root, but
# the root of this module is the library: the command is cmd/jactionlint. This swaps the trailing `.` for it.
set -euo pipefail

args=("$@")
last=$((${#args[@]} - 1))
if [ "${args[0]:-}" = build ] && [ "${args[$last]}" = . ]; then
  args[last]=./cmd/jactionlint
fi
exec go "${args[@]}"
