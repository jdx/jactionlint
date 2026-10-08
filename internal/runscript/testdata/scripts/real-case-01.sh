set -euo pipefail
case "$MODE" in
  oidc|otp) ;;
  *)
    echo "::error::Unsupported mode: $MODE"
    exit 1
    ;;
esac
