python3 -m venv "$RUNNER_TEMP/copr-cli"
# copr-cli 2.5 imports rich but omits it from its package dependencies.
"$RUNNER_TEMP/copr-cli/bin/pip" install copr-cli==2.5 rich==14.2.0
"$RUNNER_TEMP/copr-cli/bin/copr-cli" --version
