set -o pipefail
/tmp/tak-build/tak detect | tee -a "$GITHUB_STEP_SUMMARY"
