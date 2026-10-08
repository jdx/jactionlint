set -euo pipefail
# Blank lines and # comments are for people; the action takes entries only.
entries() {
  local file="withdrawals/.github/packslip/$1"
  # A missing file must fail the run, not publish a list that quietly
  # restores every withdrawn release.
  [ -f "$file" ] || { echo "$file is missing" >&2; exit 1; }
  grep -Ev '^[[:space:]]*(#|$)' "$file" || [ $? -eq 1 ]
}
delimiter="packslip_$(openssl rand -hex 16)"
{
  echo "yank<<$delimiter"
  entries yanked
  echo "$delimiter"
  echo "security<<$delimiter"
  entries security
  echo "$delimiter"
} >> "$GITHUB_OUTPUT"
