python3 -m venv "$RUNNER_TEMP/venv"
"$RUNNER_TEMP/venv/bin/pip" install --quiet pyyaml
echo "$RUNNER_TEMP/venv/bin" >> "$GITHUB_PATH"
