status=$(cat /tmp/tak-out/status)
test "$status" -eq 0 || { echo "::error::instruction-count comparison failed to run"; exit "$status"; }
