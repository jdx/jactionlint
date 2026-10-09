#!/bin/bash

# Runs jactionlint for action.yml. Everything arrives through JACTIONLINT_* environment variables, so no input is
# ever interpolated into a script.

set -e -o pipefail

cd "${JACTIONLINT_WORKING_DIRECTORY:-.}"

# The inputs are split into words but never expanded further: no globbing, no quote removal.
set -f
read -r -a extra <<< "${JACTIONLINT_ARGS:-}"
read -r -a files <<< "${JACTIONLINT_FILES:-}"
set +f

args=()
[ -n "${JACTIONLINT_PROFILE:-}" ] && args+=("--profile=${JACTIONLINT_PROFILE}")
[ -n "${JACTIONLINT_CONFIG_FILE:-}" ] && args+=("--config-file=${JACTIONLINT_CONFIG_FILE}")
[ -n "${JACTIONLINT_MIN_SEVERITY:-}" ] && args+=("--min-severity=${JACTIONLINT_MIN_SEVERITY}")
[ "${JACTIONLINT_STRICT_EXIT:-}" = "true" ] && args+=(--strict-exit)

case "${JACTIONLINT_ONLINE:-}" in
    "") ;;
    false) args+=(--no-online) ;;
    true) args+=(--online) ;;
    *) args+=("--online=${JACTIONLINT_ONLINE}") ;;
esac
if [ "${JACTIONLINT_ONLINE:-}" = "false" ]; then
    # The network is never used, so the token has no business in the environment of jactionlint
    unset JACTIONLINT_TOKEN
else
    # Also when "online" is empty: the config file may enable the online checks, and then they need the token
    args+=(--online-token-env=JACTIONLINT_TOKEN)
fi

sarif=""
if [ "${JACTIONLINT_ADVANCED_SECURITY:-}" = "true" ]; then
    sarif="${RUNNER_TEMP}/jactionlint.sarif"
    args+=(--format=sarif --no-color)
elif [ "${JACTIONLINT_ANNOTATIONS:-}" = "true" ]; then
    args+=(--format=github)
elif [ "${JACTIONLINT_COLOR:-}" = "true" ]; then
    args+=(--color=always)
else
    args+=(--color=never)
fi

command=("${JACTIONLINT_EXECUTABLE}" "${args[@]}" "${extra[@]}")
if [ "${#files[@]}" -gt 0 ]; then
    command+=(-- "${files[@]}")
fi

if [ -z "${sarif}" ]; then
    exec "${command[@]}"
fi

# Exit status 1 means findings and is no reason to skip the upload. Any other failure leaves no usable file behind
status=0
"${command[@]}" > "${sarif}" || status=$?
echo "exit-code=${status}" >> "${GITHUB_OUTPUT}"
if [ "${status}" -le 1 ]; then
    echo "sarif-file=${sarif}" >> "${GITHUB_OUTPUT}"
    echo "Wrote the findings to ${sarif}"
else
    rm -f "${sarif}"
fi
exit 0
