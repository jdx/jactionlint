F=$GITHUB_ENV
echo A=1 >> "$F"
{
  echo "MULTI<<EOF"
  echo "${{ github.event.pull_request.title }}"
  echo "EOF"
} >> "$GITHUB_ENV"
( echo B=2 ) >> $GITHUB_PATH
if true; then echo C=3; fi >> $GITHUB_ENV
for i in 1 2; do echo "I=$i"; done >> "$GITHUB_OUTPUT"
exec >> $GITHUB_ENV
