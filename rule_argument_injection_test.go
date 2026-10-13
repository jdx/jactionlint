package jactionlint

import (
	"strings"
	"testing"
)

func countArgumentInjection(t *testing.T, src string) int {
	t.Helper()
	cfg := &Config{Rules: map[string]RuleConfig{"argument-injection": {Level: SeverityError}}}
	n := 0
	for _, e := range lintBatchD(t, "test.yaml", src, cfg) {
		if e.ID == "argument-injection" {
			n++
		}
	}
	return n
}

// TestArgumentInjection is the table of the rule. want is the number of findings.
func TestArgumentInjection(t *testing.T) {
	const head = "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          BODY: ${{ github.event.comment.body }}\n          TITLE: ${{ github.event.issue.title }}\n          REF: ${{ github.head_ref }}\n          LIT: fixed\n        run: |\n"
	tests := []struct {
		what   string
		script string
		want   int
	}{
		// --- the program of sed and awk ---
		{"sed replacement", `sed "s/PLACEHOLDER/$BODY/g" template.txt > out.txt`, 1},
		{"sed -e", `sed -e "s/a/$TITLE/" file`, 1},
		{"sed in place", `sed -i "s|x|$BODY|" file`, 1},
		{"sed with braces and a flag", `sed -E "s/a/${BODY}/" file`, 1},
		{"sed address", `sed -n "/$TITLE/p" file`, 1},
		{"awk program", `awk "BEGIN { print \"$BODY\" }"`, 1},
		{"awk after an option", `awk -v x=1 "{ print \"$TITLE\" }" file`, 1},
		{"gawk", `gawk "{ print \"$BODY\" }" file`, 1},
		// --- find ---
		{"find -exec with the value as the command", `find . -exec $BODY {} \;`, 1},
		{"find with an unquoted value", `find . -name $TITLE`, 1},
		// --- options of tar, zip, git clone ---
		{"tar with a quoted value", `tar -czf out.tgz "$BODY"`, 1},
		{"tar with an unquoted value", `tar -czf out.tgz $BODY`, 1},
		{"zip", `zip out.zip "$TITLE"`, 1},
		{"git clone", `git clone "$BODY"`, 1},
		{"wget", `wget "$BODY"`, 1},
		{"curl", `curl "$TITLE"`, 1},

		// --- not a program, not an option ---
		{"a literal program", `sed 's/a/b/' file`, 0},
		{"a single quoted program that names the variable", `sed 's/a/$BODY/' file`, 0},
		{"the name of a branch", `sed "s/a/$REF/" file`, 0},
		{"a literal in a double quoted program", `sed "s/a/$LIT/" file`, 0},
		{"the value is a file", `sed 's/a/b/' "$BODY"`, 0},
		{"a program file", `awk -f prog.awk "$BODY"`, 0},
		{"the value is a variable of awk", `awk -v v="$BODY" '{ print v }' file`, 0},
		{"the value is a pattern of find", `find . -name "$TITLE"`, 0},
		{"the value is a directory of find", `find "$TITLE" -type f`, 0},
		{"the end of the options", `tar -czf out.tgz -- "$BODY"`, 0},
		{"the value is in the middle of the word", `tar -czf "dir/$BODY.tgz" file`, 0},
		{"a url made of the value", `git clone "https://example.com/$BODY"`, 0},
		{"git clone after --", `git clone -- "$BODY"`, 0},
		{"an option that takes the value", `tar --file="$BODY" -t`, 0},
		{"the name of a branch is not an option", `tar -czf out.tgz "$REF"`, 0},
		{"another command", `echo "$BODY"`, 0},
		{"cut down to letters and digits", "V=$(echo \"$BODY\" | tr -cd 'a-z0-9')\nsed \"s/a/$V/\" file", 0},
		{"cut down with an expansion", `sed "s/a/${BODY//[^a-z0-9]/}/" file`, 0},
		{"validated", "[[ \"$BODY\" =~ ^[a-z0-9-]+$ ]] || exit 1\nsed \"s/a/$BODY/\" file", 0},
		{"a dot and a slash are enough to break the program", "V=$(echo \"$BODY\" | sed 's/[^a-z0-9./]//g')\nsed \"s/a/$V/\" file", 1},
		{"an expression in the script", `sed "s/a/${{ github.event.comment.body }}/" file`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head
			for _, l := range strings.Split(tc.script, "\n") {
				src += "          " + l + "\n"
			}
			if got := countArgumentInjection(t, src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}

func TestArgumentInjectionTextFromGh(t *testing.T) {
	src := "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          N: ${{ github.event.issue.number }}\n        run: |\n"
	for _, tc := range []struct {
		script string
		want   int
	}{
		{`T=$(gh pr view "$N" --json body --jq .body); sed "s/a/$T/" f`, 1},
		{`T=$(gh pr view "$N" --json title --jq .title); sed "s/a/$T/" f`, 1},
		{`T=$(gh pr view "$N" --json headRefName --jq .headRefName); sed "s/a/$T/" f`, 0},
	} {
		if got := countArgumentInjection(t, src+"          "+tc.script+"\n"); got != tc.want {
			t.Errorf("%q: %d findings, want %d", tc.script, got, tc.want)
		}
	}
}
