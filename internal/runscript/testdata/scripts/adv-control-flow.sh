set -euo pipefail
f() {
  pip install in-function
}
f
if [[ "${{ github.event_name }}" == "push" ]]; then
  npm install in-if
elif true; then
  cargo install in-elif
else
  go install in/else@v1
fi
for p in a b; do pip install "$p"; done
while read -r l; do echo "$l" >> "$GITHUB_ENV"; done < list.txt
case "$X" in
  a) apt-get install -y a ;;
  *) echo other ;;
esac
until false; do break; done
x=$(pip install in-subst); y=`npm i in-backtick`
[ -f x ] && pip install and-list || pip install or-list
pip install bg & wait
(cd sub && npm ci)
echo $(( 1 + 2 ))
declare -a arr=(a b c)
export A=1 B="$(date)"
local_var=1 pip install with-assign
