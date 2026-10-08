set -e
fail=0
check() {
  code=$(curl -sS -o /dev/null -w '%{http_code}' --retry 5 --retry-delay 5 \
    --retry-all-errors "https://usage.sh$1" || echo 000)
  echo "$1 -> $code (want $2)"
  [ "$code" = "$2" ] || fail=1
}
check / 200
check /gh/jdx/tak 200
check /gh/nope-xyz-999/nope 404
[ "$fail" = "0" ] || { echo "::error::smoke test failed"; exit 1; }
