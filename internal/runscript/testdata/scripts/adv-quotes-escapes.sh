pip install 'pkg==1.0' "pkg2==2.0" pkg3\=\=3.0 "pkg4"'==4.0'
echo "\$GITHUB_ENV" >> \$GITHUB_ENV
echo \
  continued >> \
  "$GITHUB_ENV"
echo $'ansi\nquoted' >> $GITHUB_ENV
