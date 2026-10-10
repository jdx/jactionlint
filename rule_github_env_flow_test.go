package jactionlint

import (
	"strings"
	"testing"
)

// TestGitHubEnvDataFlow is the table of the data-flow of github-env, see the invariant at the top of
// rule_github_env.go. want is the number of findings: 1 means the write is untrusted (reported), 0 trusted. why
// names the reason, which is what the case is about.
func TestGitHubEnvDataFlow(t *testing.T) {
	const head = "on: pull_request_target\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          TITLE: ${{ github.event.pull_request.title }}\n          REF: ${{ github.head_ref }}\n          REPO: ${{ github.repository }}\n          CMD: ${{ github.event.comment.body }}\n        run: |\n"
	const env = `echo "V=$V" >> "$GITHUB_ENV"`
	const path = `echo "$V" >> "$GITHUB_PATH"`
	tests := []struct {
		what   string
		script string
		want   int
		why    string
	}{
		// --- untrusted ---
		{"plain value", `V=$TITLE
` + env, 1, "an outsider's value, nothing done to it"},
		{"deletion of another character", `V=$(echo "$TITLE" | tr -d ' ')
` + env, 1, "the newline stays"},
		{"sed replaces the first match only", `V=$(echo "$TITLE" | sed 's/[^a-z]/-/')
` + env, 1, "no g flag, later characters stay"},
		{"newline removed, written to the path", `V=$(echo "$TITLE" | tr -d '\n')
` + path, 1, "a directory like ./bin is still possible"},
		{"tr keeps dot and slash for the path", `V=$(echo "$REF" | tr -cd 'a-z./')
` + path, 1, "the set holds . and /"},
		{"tr keeps the colon for the path", `V=$(echo "$REF" | tr -cd 'a-z:')
` + path, 1, "the set holds :"},
		{"sed whitelist with a dot for the path", `V=$(echo "$REF" | sed 's/[^a-z.]/-/g')
` + path, 1, "the set holds ."},
		{"expansion with a dot for the path", `V=${TITLE//[^a-z.]/}
` + path, 1, "the set holds ."},
		{"charset then sed that adds dots for the path", `V=$(echo "$REF" | tr -cd 'a-z' | sed 's/a/..\//g')
` + path, 1, "a later stage can put . and / back"},
		{"unanchored regex", `V=$TITLE
[[ "$V" =~ [a-z]+ ]] || exit 1
` + env, 1, "the regex does not pin the whole value"},
		{"regex that lets everything through", `V=$TITLE
[[ "$V" =~ ^.*$ ]] || exit 1
` + env, 1, ". matches the newline"},
		{"check after the write", env + `
V=$TITLE
[[ "$V" =~ ^[a-z]+$ ]] || exit 1`, 1, "the check comes too late"},
		{"sanitized next to a raw one", `echo "B=${TITLE//[^a-z]/} $REF" >> "$GITHUB_ENV"`, 1, "$REF is raw"},
		{"sanitized once, then the raw value", `S=$(echo "$TITLE" | tr -d '\n')
echo "B=$S $TITLE" >> "$GITHUB_ENV"`, 1, "$TITLE is raw"},
		{"read after the write", `echo "TITLE=$TITLE" >> "$GITHUB_ENV"
IFS=/ read -r OWNER TITLE <<< "$REPO"`, 1, "the read has not happened at the write"},
		{"read of an untrusted value", `IFS=/ read -r OWNER V <<< "$TITLE"
` + env, 1, "the value read is untrusted"},
		{"trusted read, then an untrusted one overwrites", `IFS=/ read -r OWNER V <<< "$REPO"
IFS=/ read -r OWNER V <<< "$TITLE"
` + env, 1, "the last read wins (Bugbot: first read wins over later overwrite)"},
		{"read without a here string", `read -r V < file.txt
` + env, 1, "unknown source"},
		{"tr then tr that turns a space into a newline", `V=$(echo "$TITLE" | tr -d '\n' | tr ' ' '\n')
` + env, 1, "a later stage restores the newline (Bugbot: preserving pipeline)"},
		{"tr then sed that writes a newline", `V=$(echo "$TITLE" | tr -d '\n' | sed 's/x/\n/')
` + env, 1, "a later stage restores the newline"},
		{"newline removal then awk", `V=$(echo "$TITLE" | tr -d '\n' | awk '{ gsub(/,/, "\n"); print }')
` + env, 1, "awk can print a newline"},
		{"newline removal then xargs", `V=$(echo "$TITLE" | tr -d '\n' | xargs -n1)
` + env, 1, "xargs splits into lines"},
		{"newline removal then cut with a newline delimiter", `V=$(echo "$TITLE" | tr -d '\n' | cut -d, -f1- --output-delimiter=$'\n')
` + env, 1, "cut can print a newline"},
		{"newline removal then printf", `V=$(echo "$TITLE" | tr -d '\n' | xargs printf '%s\n')
` + env, 1, "printf can print a newline"},
		{"negated bare test", `V=$TITLE
[[ ! "$V" =~ ^[a-z]+$ ]]
` + env, 1, "going on means the value did NOT match (Bugbot: test polarity)"},
		{"test that exits on a match", `V=$TITLE
[[ "$V" =~ ^[a-z]+$ ]] && exit 1
` + env, 1, "going on means the value did not match"},
		{"if that exits when the value is valid", `V=$TITLE
if [[ "$V" =~ ^[a-z]+$ ]]; then exit 1; fi
` + env, 1, "going on means the value did not match"},
		{"test without a failing exit", `V=$TITLE
[[ "$V" =~ ^[a-z]+$ ]] || echo bad
` + env + `
exit 0`, 1, "a later exit is not the exit of the test (Bugbot: any later exit)"},
		{"test in a subshell exit", `V=$TITLE
[[ "$V" =~ ^[a-z]+$ ]] || (exit 1)
` + env, 1, "exit of a subshell does not end the script"},
		{"bare test with set +e", `set +e
V=$TITLE
[[ "$V" =~ ^[a-z]+$ ]]
` + env, 1, "the bare test does not stop the script"},
		{"test inside an if", `V=$TITLE
if [ -n "$CI" ]; then
[[ "$V" =~ ^[a-z]+$ ]] || exit 1
fi
` + env, 1, "the test may not run"},
		{"validated, then overwritten", `V=$TITLE
[[ "$V" =~ ^[a-z]+$ ]] || exit 1
V=$REF
` + env, 1, "the last assignment is after the check (Bugbot: validated vars ignore later assignments)"},
		{"validated, then appended", `V=$TITLE
[[ "$V" =~ ^[a-z]+$ ]] || exit 1
V+=$REF
` + env, 1, "the append adds a raw value"},
		{"validated, then read", `V=$TITLE
[[ "$V" =~ ^[a-z]+$ ]] || exit 1
IFS=/ read -r V X <<< "$REF"
` + env, 1, "the read replaces the value"},
		{"safe value, then a branch with an untrusted one", `V=safe
if [ -n "$CI" ]; then V=$TITLE; fi
` + env, 1, "the branch may have run"},
		{"untrusted value assigned later in the loop", `for i in 1 2; do
echo "V=$V" >> "$GITHUB_ENV"
V=$TITLE
done`, 1, "the next round writes the value of the last"},
		{"case without a default that leaves", `V=$TITLE
case "$V" in
  a|b) ;;
esac
` + env, 1, "other values go on"},
		{"case with a glob pattern", `V=$TITLE
case "$V" in
  a*) ;;
  *) exit 1 ;;
esac
` + env, 1, "a* matches a newline"},
		{"eval before the write", `V=safe
eval "$CMD"
` + env, 1, "eval can set anything"},
		{"for loop over an untrusted list", `for V in $TITLE; do
` + env + `
done`, 1, "the items come from the outsider"},
		{"printf -v sets the variable", `V=safe
printf -v V '%s' "$TITLE"
` + env, 1, "printf -v assigns"},

		{"guard in a branch, write after the branch", `V=$TITLE
if [ -n "$CI" ]; then
  [[ "$V" =~ ^[a-z]+$ ]] || exit 1
fi
` + env, 1, "the guard only holds inside its branch"},
		{"guard in one branch, write in the other", `V=$TITLE
if [ -n "$CI" ]; then
  [[ "$V" =~ ^[a-z]+$ ]] || exit 1
else
  ` + env + `
fi`, 1, "the else branch was not checked"},
		{"guard in a branch, value replaced before the write", `V=$TITLE
if [ -n "$CI" ]; then
  [[ "$V" =~ ^[a-z]+$ ]] || exit 1
  V=$TITLE
  ` + env + `
fi`, 1, "the value is assigned again after the check"},
		{"guard in a branch that does not leave", `V=$TITLE
if [ -n "$CI" ]; then
  [[ "$V" =~ ^[a-z]+$ ]] || echo bad
  ` + env + `
fi`, 1, "the failing test does not stop the script"},
		{"guard in a branch, the write comes first", `V=$TITLE
if [ -n "$CI" ]; then
  ` + env + `
  [[ "$V" =~ ^[a-z]+$ ]] || exit 1
fi`, 1, "the check comes too late"},
		{"guard in a loop body", `V=$TITLE
for i in 1; do
  [[ "$V" =~ ^[a-z]+$ ]] || exit 1
  ` + env + `
done`, 1, "loops are not followed"},
		// --- trusted ---
		{"guard in the same branch as the write", `V=$TITLE
if [ -n "$CI" ]; then
  [[ "$V" =~ ^[a-z]+$ ]] || exit 1
  ` + env + `
fi`, 0, "the write is after the check in its own branch"},
		{"guard in the else branch of the write", `V=$TITLE
if [ -z "$CI" ]; then :; else
  [[ "$V" =~ ^[a-z]+$ ]] || exit 1
  ` + env + `
fi`, 0, "the write is after the check in its own branch"},
		{"guard in a nested branch", `V=$TITLE
if [ -n "$CI" ]; then
  if [ -n "$X" ]; then
    [[ "$V" =~ ^[a-z]+$ ]] || exit 1
    ` + env + `
  fi
fi`, 0, "the write is after the check in the nested branch"},
		{"newline deleted", `V=$(echo "$TITLE" | tr -d "\n\r")
` + env, 0, "no newline left"},
		{"newlines turned into spaces", `V=$(printf %s "$TITLE" | tr '\n' ' ')
` + env, 0, "no newline left"},
		{"first line only", `V=$(printf '%s\n' "$TITLE" | head -n 1)
` + env, 0, "one line"},
		{"expansion removing everything else", `V=${TITLE//[^a-zA-Z0-9]/}
` + env, 0, "a whitelist"},
		{"expansion removing the newline", `V=${TITLE//$'\n'/}
` + env, 0, "no newline left"},
		{"sed whitelist and cut", `V=$(echo "$REF" | sed -e 's/[^a-zA-Z0-9-]/-/g' | cut -c1-63)
` + env, 0, "a whitelist, then a cut"},
		{"sed whitelist for the path", `V=$(echo "$REF" | sed 's/[^a-zA-Z0-9-]/-/g')
` + path, 0, "no dot, slash or colon"},
		{"sed that joins lines", `V=$(echo "$TITLE" | sed ':a;N;$!ba;s/\n/ /g')
` + env, 0, "no newline left"},
		{"tr keeps a character set", `V=$(echo "$TITLE" | tr -cd 'a-zA-Z0-9_-')
` + env, 0, "a whitelist"},
		{"tr that keeps the dot for the environment", `V=$(echo "$TITLE" | tr -cd 'a-z.')
` + env, 0, "no newline in the set"},
		{"newline removal, then head and sort", `V=$(echo "$TITLE" | tr -d '\n' | head -c 20 | sort)
` + env, 0, "stages that only select"},
		{"newline removal, then a translation without a newline", `V=$(echo "$TITLE" | tr -d '\n' | tr 'a-z' 'A-Z')
` + env, 0, "tr to letters adds no newline"},
		{"newline removal, then a plain sed substitution", `V=$(echo "$TITLE" | tr -d '\n' | sed 's/ /-/g')
` + env, 0, "the replacement has no newline"},
		{"charset for the path, then head", `V=$(echo "$REF" | tr -cd 'a-z' | head -c 5)
` + path, 0, "head only selects"},
		{"last assignment wins", `V=$TITLE
V=safe
` + env, 0, "the untrusted value is gone at the write"},
		{"sanitized in place", `V=$TITLE
V=${V//[^a-z]/}
` + env, 0, "the last assignment is a whitelist"},
		{"conditional assignments that are both trusted", `V=a
if [ -n "$CI" ]; then V=b; fi
` + env, 0, "every possible value is a literal"},
		{"bare anchored test", `V=$TITLE
[[ "$V" =~ ^[a-z0-9-]+$ ]]
` + env, 0, "the test holds or -e stops the script"},
		{"anchored test or exit", `V=$TITLE
[[ "$V" =~ ^[a-z0-9-]+$ ]] || { echo bad; exit 1; }
` + env, 0, "the failing branch exits"},
		{"negated test and exit", `V=$TITLE
[[ ! "$V" =~ ^[a-z0-9-]+$ ]] && exit 1
` + env, 0, "exits when the value does not match"},
		{"negated if with exit", `V=$TITLE
if [[ ! "$V" =~ ^[0-9]+\.[0-9]+$ ]]; then exit 1; fi
` + env, 0, "exits when the value does not match"},
		{"if with exit in the else", `V=$TITLE
if [[ "$V" =~ ^[a-z]+$ ]]; then :; else exit 1; fi
` + env, 0, "exits when the value does not match"},
		{"literal comparison", `V=$TITLE
[[ "$V" == "stable" ]] || exit 1
` + env, 0, "one constant value"},
		{"case with a default that leaves", `V=$TITLE
case "$V" in
  stable|beta) ;;
  *) echo bad; exit 1 ;;
esac
` + env, 0, "only constants go on"},
		{"owner and name split from the repository", `IFS=/ read -r OWNER V <<< "$REPO"
` + env, 0, "a trusted source"},
		{"untrusted read, then a trusted one", `IFS=/ read -r OWNER V <<< "$TITLE"
IFS=/ read -r OWNER V <<< "$REPO"
` + env, 0, "the last read wins"},
		{"for loop over literals", `for V in a b; do
` + env + `
done`, 0, "constants"},
		{"date of the run", `echo "D=$(date -d "${{ github.event.workflow_run.run_started_at }}" '+%b %d')" >> "$GITHUB_ENV"`, 0, "a time"},
	}
	if len(tests) < 40 {
		t.Fatalf("the table needs at least 40 cases, has %d", len(tests))
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head
			for _, l := range strings.Split(tc.script, "\n") {
				src += "          " + l + "\n"
			}
			if got := countGitHubEnv(t, src); got != tc.want {
				t.Errorf("%s: %d findings, want %d (%s)\n%s", tc.what, got, tc.want, tc.why, src)
			}
		})
	}
}

func TestGitHubEnvTotalBranches(t *testing.T) {
	const head = "on: pull_request_target\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          TITLE: ${{ github.event.pull_request.title }}\n        run: |\n"
	tests := []struct {
		what, script string
		want         int
	}{
		{"case that sets it everywhere or leaves", "case \"$(uname -m)\" in\n  x86_64) V=a ;;\n  *) exit 1 ;;\nesac\necho \"V=$V\" >> \"$GITHUB_ENV\"", 0},
		{"case without a default", "case \"$(uname -m)\" in\n  x86_64) V=a ;;\nesac\necho \"V=$V\" >> \"$GITHUB_ENV\"", 1},
		{"case with an untrusted branch", "case \"$(uname -m)\" in\n  x86_64) V=a ;;\n  *) V=$TITLE ;;\nesac\necho \"V=$V\" >> \"$GITHUB_ENV\"", 1},
		{"case with a branch that does not set it", "case \"$(uname -m)\" in\n  x86_64) V=a ;;\n  *) echo hi ;;\nesac\necho \"V=$V\" >> \"$GITHUB_ENV\"", 1},
		{"if with else", "if [ -n \"$CI\" ]; then V=a; else V=b; fi\necho \"V=$V\" >> \"$GITHUB_ENV\"", 0},
		{"if with else and an untrusted branch", "if [ -n \"$CI\" ]; then V=a; else V=$TITLE; fi\necho \"V=$V\" >> \"$GITHUB_ENV\"", 1},
		{"if without else", "if [ -n \"$CI\" ]; then V=a; fi\necho \"V=$V\" >> \"$GITHUB_ENV\"", 1},
		{"if with elif and else", "if [ -n \"$CI\" ]; then V=a; elif [ -n \"$X\" ]; then V=b; else V=c; fi\necho \"V=$V\" >> \"$GITHUB_ENV\"", 0},
		{"if with elif and no else", "if [ -n \"$CI\" ]; then V=a; elif [ -n \"$X\" ]; then V=b; fi\necho \"V=$V\" >> \"$GITHUB_ENV\"", 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head
			for _, l := range strings.Split(tc.script, "\n") {
				src += "          " + l + "\n"
			}
			if got := countGitHubEnv(t, src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}

// TestGitHubEnvReviewRound3 covers cases where the untrusted value must be seen on a trigger that is not
// privileged (only a known-untrusted write is reported there, so a value judged "unknown" hides the finding) and
// cases that must stay quiet on pull_request_target.
func TestGitHubEnvReviewRound3(t *testing.T) {
	mk := func(trigger, extra, script string) string {
		src := "on: " + trigger + "\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          TITLE: ${{ github.event.pull_request.title }}\n          CMD: ${{ github.event.comment.body }}\n" + extra + "        run: |\n"
		for _, l := range strings.Split(script, "\n") {
			src += "          " + l + "\n"
		}
		return src
	}
	const env = `echo "V=$V" >> "$GITHUB_ENV"`
	tests := []struct {
		what, trigger, extra, script string
		want                         int
	}{
		{"assignment in the branch of the write", "issue_comment", "", "if [ -n \"$X\" ]; then V=$TITLE; " + env + "; else V=a; fi", 1},
		{"assignment in the case branch of the write", "issue_comment", "", "case \"$X\" in\n  a) V=$TITLE; " + env + " ;;\n  *) V=b ;;\nesac", 1},
		{"trusted assignment in the branch of the write", "issue_comment", "", "if [ -n \"$X\" ]; then V=a; " + env + "; else V=$TITLE; fi", 0},
		{"assignment after the group write", "issue_comment", "", `{ echo "T=$TITLE"; TITLE=safe; } >> "$GITHUB_ENV"`, 1},
		{"function body sees the prefix assignment", "issue_comment", "", "w() { " + env + "; }\nV=$TITLE w", 1},
		{"function body sees a later assignment", "issue_comment", "", "w() { " + env + "; }\nV=$TITLE\nw", 1},
		{"sed whitelist that keeps = and the newline", "issue_comment", "", "V=$(echo \"$CMD\" | sed 's/[^A-Za-z0-9_=./-]/-/g')\n" + env, 1},
		{"sed whitelist without = is harmless", "issue_comment", "", "V=$(echo \"$CMD\" | sed 's/[^A-Za-z0-9_./-]/-/g')\n" + env, 0},
		{"sed -z whitelist with =", "issue_comment", "", "V=$(echo \"$CMD\" | sed -z 's/[^A-Za-z0-9_=./-]/-/g')\n" + env, 0},
		{"bare test under a template without -e", "issue_comment", "        shell: bash {0}\n", "V=$TITLE\n[[ \"$V\" =~ ^[a-z]+$ ]]\n" + env, 1},
		{"bare test under a template with -e", "issue_comment", "        shell: bash -e {0}\n", "V=$TITLE\n[[ \"$V\" =~ ^[a-z]+$ ]]\n" + env, 0},
		{"bare test after set -e in a template", "issue_comment", "        shell: bash {0}\n", "set -e\nV=$TITLE\n[[ \"$V\" =~ ^[a-z]+$ ]]\n" + env, 0},
		{"bare test under shell: bash", "issue_comment", "        shell: bash\n", "V=$TITLE\n[[ \"$V\" =~ ^[a-z]+$ ]]\n" + env, 0},
		{"benign substitution next to a sanitized one", "pull_request_target", "", `echo "B=$(date +%s)-$(echo "$TITLE" | tr -d '\n')" >> "$GITHUB_ENV"`, 0},
		{"benign substitution next to a raw one", "pull_request_target", "", `echo "B=$(date +%s)-$(echo "$TITLE" | sed s/a/b/)" >> "$GITHUB_ENV"`, 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := mk(tc.trigger, tc.extra, tc.script)
			if got := countGitHubEnv(t, src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}
