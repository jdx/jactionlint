echo A=1 >> $RUNNER_TEMP/$GITHUB_ENV_x
echo A=1 >> "$GITHUB_ENV_x"
echo A=1 >> $GITHUB_ENVIRONMENT
echo A=1 >> "${GITHUB_ENV}.bak"
echo A=1 >> '$GITHUB_ENV'
echo A=1 >> $GITHUB_ENV/
echo A=1 >> ${GITHUB_ENV:-/dev/null}
echo A=1 >> $env_GITHUB_ENV
