package jactionlint

import (
	"io"
	"strings"
	"testing"
)

// The regression table of the shell model of unredacted-secrets: a snippet of a `run:` script, with the secret
// CREDS (the env of the workflow) and, for the snippets that say so, K cut from it, and how many findings it gets.
// Every row is a case that came up in review or that the documentation names, so a change to the model shows here.
func TestUnredactedSecretsShellTable(t *testing.T) {
	const k = `K=$(jq -r .k <<<"$CREDS")` + "\n"
	tests := []struct {
		what    string
		withK   bool
		snippet string
		want    int
	}{
		// printed
		{"echo", true, `echo "$K"`, 1},
		{"printf", true, `printf '%s\n' "$K"`, 1},
		{"cat of a here-string", true, `cat <<<"$K"`, 1},
		{"echo to stderr", true, `echo "$K" >&2`, 1},
		{"echo inside a text", true, `echo "prefix $K suffix"`, 1},
		{"echo of a substitution", true, `echo "$(echo "$K")"`, 1},
		{"echo of a cut substitution", false, `echo "$(jq -r .k <<<"$CREDS")"`, 1},
		{"through a pipe to cat", true, `echo "$K" | cat`, 1},
		{"a copy", true, "K2=\"$K\"\necho \"$K2\"", 1},
		{"an appended value", true, "K+=x\necho \"$K\"", 1},
		{"printf -v and then echo", true, "printf -v OUT '%s' \"$K\"\necho \"$OUT\"", 1},
		{"a heredoc", true, "cat <<EOF\n$K\nEOF", 1},
		{"a cut of the value in a heredoc", true, "cat <<EOF\n${K:0:3}\nEOF", 1},
		{"printf with -v as data", true, `printf "%s %s %s\n" -v OUT "$K"`, 1},
		{"a parameter cut of the secret", false, "P=${CREDS#a}\necho \"$P\"", 1},
		{"a substring from the start", false, "P=${CREDS::8}\necho \"$P\"", 1},
		{"base64 -d of a pipe", false, "B=$(echo \"$CREDS\" | base64 -d)\necho \"$B\"", 1},
		{"base64 -d of a here-string", false, "B=$(base64 -d <<<\"$CREDS\")\necho \"$B\"", 1},
		{"set -x", false, "set -x\n" + k, 1},
		{"set -x and a substitution in an argument", false, "set -x\n./deploy \"$(jq -r .k <<<\"$CREDS\")\"", 1},

		// the mask comes first
		{"mask with echo", true, "echo \"::add-mask::$K\"\necho \"$K\"", 0},
		{"mask with printf", true, "printf '::add-mask::%s\\n' \"$K\"\necho \"$K\"", 0},
		{"mask of a value from a substitution", false, `echo "::add-mask::$(jq -r .k <<<"$CREDS")"`, 0},

		// a mask that is none
		{"mask with more text", true, "echo \"::add-mask::$K extra\"\necho \"$K\"", 1},
		{"mask of a cut", true, "echo \"::add-mask::${K:0:4}\"\necho \"$K\"", 1},
		{"mask redirected to a file", true, "echo \"::add-mask::$K\" > mask.txt\necho \"$K\"", 1},
		{"mask piped on", true, "echo \"::add-mask::$K\" | wc -c\necho \"$K\"", 1},
		{"mask with two arguments", true, "echo \"::add-mask::$K\" \"$K\"\necho \"$K\"", 1},
		{"a cut of a masked value is not masked", true, "echo \"::add-mask::$K\"\ncat <<EOF\n${K:0:3}\nEOF", 1},

		// not printed
		{"redirected to a file", true, `echo "$K" > f`, 0},
		{"piped to a command that counts", true, `echo "$K" | wc -c`, 0},
		{"into GITHUB_ENV", true, `echo "k=$K" >> "$GITHUB_ENV"`, 0},
		{"as a header of curl", true, `curl -H "Authorization: $K" https://example.com`, 0},
		{"as an argument", true, `./deploy "$K"`, 0},
		{"piped to tee", true, `echo "$K" | tee f`, 0},
		{"printf -v alone", true, `printf -v OUT '%s' "$K"`, 0},
		{"a quoted heredoc", true, "cat <<'EOF'\n$K\nEOF", 0},
		{"single quotes", true, `echo '$K'`, 0},
		{"a process substitution", true, `echo <(echo "$K")`, 0},
		{"reassigned", true, "K=literal\necho \"$K\"", 0},
		{"set -x turned off", false, "set -x\nset +x\n" + k, 0},
		{"a subshell assignment", false, "OUT=$(KEY=$(jq -r .k <<<\"$CREDS\"); echo done)\necho \"$KEY\"", 0},
		{"a here-string substitution that is an input", true, `jq . <<<"$(echo "$K")" > /dev/null`, 0},

		// the secret itself is masked by the runner
		{"the secret", false, `echo "$CREDS"`, 0},
		{"a copy of the secret", false, "C=\"$CREDS\"\necho \"$C\"", 0},
		{"the secret in a text", false, "L=\"x:$CREDS\"\necho \"$L\"", 0},
		{"a default", false, "D=${CREDS:-none}\necho \"$D\"", 0},

		// a program computed it
		{"the answer of a service", false, "S=$(curl -s -u \"$CREDS\" https://example.com | jq -r .id)\necho \"$S\"", 0},
		{"a hash", false, "H=$(sha256sum <<<\"$CREDS\")\necho \"$H\"", 0},
		{"a count", false, "N=$(echo \"$CREDS\" | wc -c)\necho \"$N\"", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			body := tc.snippet
			if tc.withK {
				body = k + body
			}
			var sb strings.Builder
			sb.WriteString("on: push\nenv:\n  CREDS: ${{ secrets.CREDENTIALS }}\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n")
			for _, line := range strings.Split(body, "\n") {
				sb.WriteString("          " + line + "\n")
			}
			l, err := NewLinter(io.Discard, &LinterOptions{})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = &Config{Profile: ProfilePedantic}
			errs, err := l.Lint("test.yaml", []byte(sb.String()), nil)
			if err != nil {
				t.Fatal(err)
			}
			got := 0
			for _, e := range errs {
				if e.ID == "unredacted-secrets" && strings.Contains(e.Message, "extracted from") {
					got++
				}
			}
			if got != tc.want {
				t.Errorf("want %d findings, got %d for:\n%s\n%v", tc.want, got, body, errs)
			}
		})
	}
}
