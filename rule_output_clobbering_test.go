package jactionlint

import (
	"strings"
	"testing"
)

func countOutputClobbering(t *testing.T, name, src string) int {
	t.Helper()
	cfg := &Config{Rules: map[string]RuleConfig{"output-clobbering": {Level: SeverityError}}}
	n := 0
	for _, e := range lintBatchD(t, name, src, cfg) {
		if e.ID == "output-clobbering" {
			n++
		}
	}
	return n
}

// TestOutputClobbering is the table of the rule. want is the number of findings.
func TestOutputClobbering(t *testing.T) {
	const head = "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    outputs:\n      approved: ${{ steps.check.outputs.approved }}\n    steps:\n      - id: check\n        env:\n          BODY: ${{ github.event.comment.body }}\n          TITLE: ${{ github.event.issue.title }}\n          REF: ${{ github.head_ref }}\n          LIT: fixed\n          RUNID: ${{ github.run_id }}\n          SECRETDELIM: ${{ secrets.DELIM }}\n          COMPACTDELIM: ${{secrets.DELIM}}\n          TOKENDELIM: ${{github.token}}\n          MIXEDDELIM: ${{ github.run_id }}-${{ secrets.DELIM }}\n          LITDELIM: uuid-secrets\n        run: |\n"
	const tail = "      - if: steps.check.outputs.approved == 'true'\n        run: echo approved\n"
	const out = `>> "$GITHUB_OUTPUT"`
	tests := []struct {
		what   string
		script string
		want   int
	}{
		// --- a record that an attacker can end ---
		{"the record of a comment", `echo "approved=false" ` + out + "\n" + `echo "body=$BODY" ` + out, 1},
		{"a record without another write", `echo "body=$BODY" ` + out, 1},
		{"printf", `printf 'body=%s\n' "$BODY" ` + out, 1},
		{"printf with a format that has the variable", `printf "body=$BODY\n" ` + out, 1},
		{"tee", `echo "body=$BODY" | tee -a "$GITHUB_OUTPUT"`, 1},
		{"a group", `{ echo "approved=false"; echo "body=$BODY"; } ` + out, 1},
		{"a here document", "cat <<EOF " + out + "\nbody=$BODY\nEOF", 1},
		{"a script variable", "V=$BODY\necho \"v=$V\" " + out, 1},
		{"a substitution", `echo "v=$(echo "$BODY" | sed 's/a/b/')" ` + out, 1},
		{"the value alone", `echo "$BODY" ` + out, 1},
		{"braces and the file in a variable", "F=$GITHUB_OUTPUT\necho \"body=${BODY}\" >> \"$F\"", 1},
		{"an expression in the script", `echo "body=${{ github.event.comment.body }}" ` + out, 1},
		{"two writes of the comment", "echo \"a=$BODY\" " + out + "\necho \"b=$BODY\" " + out, 2},
		// --- what reaches the file ---
		{"through a filter", `echo "body=$BODY" | sed 's/a/b/' >> "$GITHUB_OUTPUT"`, 1},
		{"through a filter and tee", `echo "body=$BODY" | grep -v x | tee -a "$GITHUB_OUTPUT"`, 1},
		{"a program that parses it", `echo "$BODY" | python parse_args.py ` + out, 0},
		{"a program in the middle", `echo "$BODY" | python parse_args.py | sed 's/a/b/' ` + out, 0},
		{"the test of an if", "{\n  if printf '%s\n' \"$BODY\" | grep -Fqi -- '/deploy'; then echo \"clean=true\"; fi\n  echo \"approved=false\"\n} " + out, 0},
		{"output sent elsewhere", `{ echo "approved=false"; echo "body=$BODY" >&2; echo "b=$BODY" > /dev/null; } ` + out, 0},
		{"the output of grep -q", `{ echo "approved=false"; echo "$BODY" | grep -q x && echo "found=true"; } ` + out, 0},
		{"base64", `echo "body=$(echo "$BODY" | base64)" ` + out, 0},
		{"base64 decoded", `echo "body=$(echo "$BODY" | base64 -d)" ` + out, 1},
		// --- a constant delimiter ---
		{"a constant delimiter in a group", `{ echo "body<<EOF"; echo "$BODY"; echo "EOF"; } ` + out, 1},
		{"a constant delimiter in three writes", "echo \"body<<EOF\" " + out + "\necho \"$BODY\" " + out + "\necho \"EOF\" " + out, 1},
		{"a constant delimiter in a here document", "cat <<EOF " + out + "\nbody<<DELIM\n$BODY\nDELIM\nEOF", 1},
		{"a constant delimiter that looks random", "delimiter=ghadelimiter_1234\n{ echo \"body<<$delimiter\"; echo \"$BODY\"; echo \"$delimiter\"; } " + out, 1},
		{"a delimiter from nanoseconds", "d=\"EOF_$(date +%s%N)\"\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"a delimiter from the date", "d=$(date +%s)\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"a delimiter from the environment", "d=$LIT\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},

		{"a printf with the delimiter and the value", `printf 'body<<EOF\n%s\nEOF\n' "$BODY" ` + out, 1},
		{"a printf with a random delimiter", "d=$(uuidgen)\nprintf 'body<<%s\\n%s\\n%s\\n' \"$d\" \"$BODY\" \"$d\" " + out, 0},
		{"an echo with the delimiter and the value", `echo -e "body<<EOF\n$BODY\nEOF" ` + out, 1},
		{"the last assignment of the delimiter is a constant", "d=$(uuidgen)\nd=EOF\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"the last assignment of the delimiter is random", "d=EOF\nd=$(uuidgen)\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"a prefix assignment is not the delimiter", "d=EOF\nd=$(uuidgen) true\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"a random assignment after the write", "d=EOF\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out + "\nd=$(uuidgen)", 1},
		{"a delimiter copied from a variable nobody set here", "d=$NOBODYSET\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"a variable whose name ends with RANDOM", "NONRANDOM=x\nd=$NONRANDOM\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"newlines removed after the record", `echo "body=$BODY" | tr -d '\n' ` + out, 0},
		{"the first line after the record", `echo "body=$BODY" | head -n 1 ` + out, 0},
		{"a filter after the record that keeps lines", `echo "body=$BODY" | sed 's/a/b/' ` + out, 1},
		{"RANDOM in arithmetic", "d=\"EOF_$((RANDOM))\"\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"RANDOM in arithmetic with spaces", "d=\"EOF_$(( RANDOM % 100000 ))\"\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"RANDOM in the old arithmetic", "d=\"EOF_$[RANDOM]\"\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"SRANDOM in the delimiter itself", "{ echo \"body<<EOF$(( SRANDOM ))\"; echo \"$BODY\"; } " + out, 0},
		{"the word RANDOM in arithmetic of another variable", "d=\"EOF_$((NONRANDOM + 1))\"\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"the delimiter is the comment", "{ echo \"body<<$BODY\"; echo \"text\"; } " + out, 1},
		{"the delimiter is a script variable that holds the comment", "d=$BODY\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"the delimiter is the id of the run", "{ echo \"body<<$RUNID\"; echo \"$BODY\"; echo \"$RUNID\"; } " + out, 1},
		{"the delimiter is a secret", "{ echo \"body<<$SECRETDELIM\"; echo \"$BODY\"; echo \"$SECRETDELIM\"; } " + out, 0},
		{"the delimiter is a secret written without spaces", "{ echo \"body<<$COMPACTDELIM\"; echo \"$BODY\"; echo \"$COMPACTDELIM\"; } " + out, 0},
		{"the delimiter is the token", "{ echo \"body<<$TOKENDELIM\"; echo \"$BODY\"; echo \"$TOKENDELIM\"; } " + out, 0},
		{"the delimiter is made of the run id and a secret", "{ echo \"body<<$MIXEDDELIM\"; echo \"$BODY\"; echo \"$MIXEDDELIM\"; } " + out, 0},
		{"the delimiter is a literal that names a random source", "{ echo \"body<<$LITDELIM\"; echo \"$BODY\"; echo \"$LITDELIM\"; } " + out, 1},
		{"two records in one printf, the value in the second", `printf 'approved=false\nbody=%s\n' "$BODY" ` + out, 1},
		{"two records in one echo, the value in the second", `echo -e "approved=false\nbody=$BODY" ` + out, 1},
		{"two records in one printf, the value in the first", `printf 'body=%s\napproved=false\n' "$BODY" ` + out, 1},
		{"two records in one string with a line break", "echo \"approved=false\nbody=$BODY\" " + out, 1},
		// --- a whole line: one line is a record ---
		{"the first line is a whole record", `echo "$BODY" | head -n 1 ` + out, 1},
		{"the text without newlines is a whole record", `echo "$BODY" | tr -d '\n' ` + out, 1},
		{"a substitution that keeps one line is a whole record", `echo "$(echo "$BODY" | head -n 1)" ` + out, 1},
		{"a variable that holds one line is a whole record", "V=$(echo \"$BODY\" | head -n 1)\necho \"$V\" " + out, 1},
		{"a whole line cut down to a set", `echo "$BODY" | tr -cd 'a-z0-9' ` + out, 0},
		{"a whole line cut down in a substitution", `echo "$(echo "$BODY" | tr -cd 'a-z0-9')" ` + out, 0},
		{"a whole line that is a hash", `echo "$BODY" | sha256sum ` + out, 0},
		{"a whole line that was checked", "[[ \"$BODY\" =~ ^[0-9]+$ ]] || exit 1\necho \"$BODY\" " + out, 0},
		{"a whole line whose name is a variable", `echo "$LIT=$BODY" | head -n 1 ` + out, 1},
		{"a value after a constant name is cut to one line", `echo "x=$BODY" | head -n 1 ` + out, 0},
		// --- the delimiter ---
		{"a printf delimiter that is a constant", `printf 'body<<%s\n%s\n%s\n' "$LIT" "$BODY" "$LIT" ` + out, 1},
		{"a printf delimiter that is random", "d=$(uuidgen)\nprintf 'body<<%s\\n%s\\n%s\\n' \"$d\" \"$BODY\" \"$d\" " + out, 0},
		{"a printf delimiter that is a secret", `printf 'body<<%s\n%s\n%s\n' "$SECRETDELIM" "$BODY" "$SECRETDELIM" ` + out, 0},
		{"a delimiter that is the date in seconds", "{ echo \"body<<$(date +%s)\"; echo \"$BODY\"; echo \"x\"; } " + out, 1},
		{"a delimiter that is the date in nanoseconds", "{ echo \"body<<$(date +%s%N)\"; echo \"$BODY\"; echo \"x\"; } " + out, 0},
		{"a script variable copied from a secret", "d=$SECRETDELIM\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"a script variable copied from a secret in two steps", "a=$SECRETDELIM\nd=\"x-$a\"\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"a script variable copied from the run id", "d=$RUNID\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"a script variable copied from a literal", "d=$LIT\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		// --- the record is closed by a printf too ---
		{"a printf closer of a printf opener, random delimiter", "d=$(uuidgen)\n{ printf 'body<<%s\\n' \"$d\"; printf '%s\\n' \"$d\"; echo \"x=$BODY\"; } " + out, 1},
		{"an echo closer of a printf opener, random delimiter", "d=$(uuidgen)\n{ printf 'body<<%s\\n' \"$d\"; echo \"$d\"; echo \"x=$BODY\"; } " + out, 1},
		{"a printf closer of an echo opener, random delimiter", "d=$(uuidgen)\n{ echo \"body<<$d\"; printf '%s\\n' \"$d\"; echo \"x=$BODY\"; } " + out, 1},
		{"a printf closer that does not match stays open", "d=$(uuidgen)\n{ printf 'body<<%s\\n' \"$d\"; printf '%s\\n' \"$LIT\"; echo \"x=$BODY\"; } " + out, 0},
		{"a printf closer that does not match, constant delimiter", "{ printf 'body<<%s\\n' \"$LIT\"; printf '%s\\n' \"$TITLE\"; echo \"$BODY\"; } " + out, 1},
		{"a printf closer with a literal", "{ printf 'body<<%s\\n' EOF; printf 'EOF\\n'; echo \"x=$BODY\"; } " + out, 1},
		{"a trusted conversion before the value", `printf 'approved=%s\nbody=%s\n' "$LIT" "$BODY" ` + out, 1},
		{"a trusted variable before the value", `echo -e "approved=$LIT\nbody=$BODY" ` + out, 1},
		{"a trusted variable after the value", `echo -e "body=$BODY\napproved=$LIT" ` + out, 1},
		{"the value is in the record that the job reads", `printf 'approved=%s\nbody=%s\n' "$BODY" "$LIT" ` + out, 0},
		{"a title and a comment in two records", `printf 'approved=%s\nbody=%s\n' "$TITLE" "$BODY" ` + out, 1},
		{"two records in one printf, both hold the value", `printf 'approved=%s\nbody=%s\n' "$BODY" "$BODY" ` + out, 0},
		{"tee with a here string", `tee -a "$GITHUB_OUTPUT" <<< "body=$BODY"`, 1},
		{"tee with a here string of a literal", `tee -a "$GITHUB_OUTPUT" <<< "x=1"`, 0},
		{"tee with a here string of the title", `tee -a "$GITHUB_OUTPUT" <<< "t=$TITLE"`, 0},
		{"cat with a here string", `cat <<< "body=$BODY" ` + out, 1},
		{"a variable named uuid", "uuid=EOF\n{ echo \"body<<$uuid\"; echo \"$BODY\"; echo \"$uuid\"; } " + out, 1},
		{"a variable named mktemp that holds a constant", "mktemp=EOF\nd=${mktemp}\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"a constant that is the word RANDOM", "d=RANDOM\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 1},
		{"the uuid of the kernel", "d=$(cat /proc/sys/kernel/random/uuid)\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"mktemp", "d=$(mktemp -u)\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"a random value in backticks", "d=`uuidgen`\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		// --- a random delimiter ---
		{"uuidgen", "delimiter=$(uuidgen)\n{ echo \"body<<$delimiter\"; echo \"$BODY\"; echo \"$delimiter\"; } " + out, 0},
		{"openssl", "EOF=$(openssl rand -hex 8)\n{ echo \"body<<$EOF\"; echo \"$BODY\"; echo \"$EOF\"; } " + out, 0},
		{"RANDOM", "d=\"ghadelimiter_$RANDOM$RANDOM\"\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"urandom", "d=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \\n')\n{ echo \"body<<$d\"; echo \"$BODY\"; echo \"$d\"; } " + out, 0},
		{"random in the delimiter itself", "{ echo \"body<<EOF$(uuidgen)\"; echo \"$BODY\"; } " + out, 0},
		{"a delimiter that cannot be resolved", "{ echo \"body<<$D\"; echo \"$BODY\"; echo \"$D\"; } " + out, 0},

		// --- not an input of an outsider with line breaks ---
		{"a title is one line", `echo "title=$TITLE" ` + out, 0},
		{"a branch name is one line", `echo "ref=$REF" ` + out, 0},
		{"a literal", `echo "x=$LIT" ` + out, 0},
		{"a literal and a substitution", `echo "x=$(date +%s)" ` + out, 0},
		{"the title and the branch", `echo "t=$TITLE-$REF" ` + out, 0},
		{"a title and a comment", `echo "t=$TITLE $BODY" ` + out, 1},
		// --- the value was made safe ---
		{"validated", "[[ \"$BODY\" =~ ^[0-9]+$ ]] || exit 1\necho \"n=$BODY\" " + out, 0},
		{"first line", `echo "first=$(echo "$BODY" | head -n 1)" ` + out, 0},
		{"newlines removed", `echo "one=$(echo "$BODY" | tr -d '\n')" ` + out, 0},
		{"newlines replaced", `echo "one=$(echo "$BODY" | tr '\n' ' ')" ` + out, 0},
		{"cut down to a set", `echo "n=${BODY//[^0-9]/}" ` + out, 0},
		{"counted", `echo "n=$(echo "$BODY" | wc -l)" ` + out, 0},
		{"hashed", `echo "h=$(echo "$BODY" | sha256sum | cut -c1-8)" ` + out, 0},
		{"matches counted", `echo "n=$(echo "$BODY" | grep -c foo)" ` + out, 0},
		{"overwritten", "V=$BODY\nV=fixed\necho \"v=$V\" " + out, 0},
		// --- not a write to the file ---
		{"the environment file", `echo "body=$BODY" >> "$GITHUB_ENV"`, 0},
		{"a file of the workspace", `echo "body=$BODY" >> out.txt`, 0},
		{"a file written as a variable", `echo "body=$BODY" >> "$GITHUB_STEP_SUMMARY"`, 0},
		{"the value is only printed", `echo "body=$BODY"`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head
			for _, l := range strings.Split(tc.script, "\n") {
				src += "          " + l + "\n"
			}
			src += tail
			if got := countOutputClobbering(t, "test.yaml", src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}

// TestOutputClobberingGates: a step that only trusted people reach is not reported, the rules cannot judge the check.
func TestOutputClobberingGates(t *testing.T) {
	mk := func(jobIf, stepIf, script string) string {
		src := "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n"
		if jobIf != "" {
			src += "    if: " + jobIf + "\n"
		}
		src += "    steps:\n      - id: check\n"
		if stepIf != "" {
			src += "        if: " + stepIf + "\n"
		}
		src += "        env:\n          BODY: ${{ github.event.comment.body }}\n        run: |\n"
		for _, l := range strings.Split(script, "\n") {
			src += "          " + l + "\n"
		}
		return src + "      - if: steps.check.outputs.approved == 'true'\n        run: echo x\n"
	}
	const write = `echo "body=$BODY" >> "$GITHUB_OUTPUT"`
	tests := []struct {
		what, jobIf, stepIf, script string
		want                        int
	}{
		{"no gate", "github.event_name == 'issue_comment'", "", write, 1},
		{"the association in the job", "contains(fromJSON('[\"OWNER\",\"MEMBER\"]'), github.event.comment.author_association)", "", write, 0},
		{"the result of a needed job is no check of the user", "needs.build.result == 'success'", "", write, 1},
		{"an output of an earlier step is no check of the user", "", "steps.build.outputs.ok == 'true'", write, 1},
		{"the actor in the step", "", "github.actor == 'maintainer'", write, 0},
		{"the permission in the script", "", "", "gh api \"repos/$R/collaborators/$USER/permission\" --jq .permission\n" + write, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := mk(tc.jobIf, tc.stepIf, tc.script)
			if got := countOutputClobbering(t, "test.yaml", src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}

// TestOutputClobberingNeedsAnOutputToClobber: the attacker can only change what is read.
func TestOutputClobberingNeedsAnOutputToClobber(t *testing.T) {
	mk := func(id, outputs, reads string) string {
		src := "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n" + outputs + "    steps:\n"
		if id != "" {
			src += "      - id: " + id + "\n"
		} else {
			src += "      -\n"
		}
		src += "        env:\n          BODY: ${{ github.event.comment.body }}\n        run: |\n          echo \"approved=false\" >> \"$GITHUB_OUTPUT\"\n          echo \"body=$BODY\" >> \"$GITHUB_OUTPUT\"\n"
		if reads != "" {
			src += "      - if: " + reads + "\n        run: echo x\n"
		}
		return src
	}
	tests := []struct {
		what, id, outputs, reads string
		want                     int
	}{
		{"another output is read", "check", "", "steps.check.outputs.approved == 'true'", 1},
		{"another output is a job output", "check", "    outputs:\n      ok: ${{ steps.check.outputs.approved }}\n", "", 1},
		{"the whole object is read", "check", "", "contains(toJSON(steps.check.outputs), 'x')", 1},
		{"the id is written in other letters", "check", "", "steps.CHECK.outputs.approved == 'true'", 1},
		{"only the written output is read", "check", "", "steps.check.outputs.body == 'x'", 0},
		{"nothing is read", "check", "", "", 0},
		{"the output of another step is read", "check", "", "steps.other.outputs.approved == 'true'", 0},
		{"the step has no id", "", "", "steps.check.outputs.approved == 'true'", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := mk(tc.id, tc.outputs, tc.reads)
			if got := countOutputClobbering(t, "test.yaml", src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}

func TestOutputClobberingShells(t *testing.T) {
	mk := func(shell, run string) string {
		return "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - id: check\n        env:\n          BODY: ${{ github.event.comment.body }}\n" + shell + "        run: " + run + "\n      - if: steps.check.outputs.approved == 'true'\n        run: echo x\n"
	}
	bash := `echo "body=$BODY" >> "$GITHUB_OUTPUT"`
	if got := countOutputClobbering(t, "test.yaml", mk("", bash)); got != 1 {
		t.Errorf("default shell: %d findings, want 1", got)
	}
	if got := countOutputClobbering(t, "test.yaml", mk("        shell: sh\n", bash)); got != 1 {
		t.Errorf("sh: %d findings, want 1", got)
	}
	if got := countOutputClobbering(t, "test.yaml", mk("        shell: pwsh\n", `"body=$env:BODY" >> $env:GITHUB_OUTPUT`)); got != 0 {
		t.Errorf("pwsh: %d findings, want 0", got)
	}
	// An action: the outputs are those of the action, an input is not judged
	action := "name: x\ndescription: x\ninputs:\n  v:\n    description: v\noutputs:\n  approved:\n    description: a\n    value: ${{ steps.check.outputs.approved }}\nruns:\n  using: composite\n  steps:\n    - id: check\n      shell: bash\n      env:\n        BODY: ${{ github.event.comment.body }}\n        V: ${{ inputs.v }}\n      run: |\n        echo \"v=$V\" >> \"$GITHUB_OUTPUT\"\n        echo \"body=$BODY\" >> \"$GITHUB_OUTPUT\"\n"
	if got := countOutputClobbering(t, ".github/actions/x/action.yml", action); got != 1 {
		t.Errorf("action: %d findings, want 1 (only the comment)", got)
	}
}

func TestExprMayHaveNewline(t *testing.T) {
	tests := []struct {
		expr string
		want bool
	}{
		{"github.event.comment.body", true},
		{"github.event.issue.body", true},
		{"github.event.head_commit.message", true},
		{"github.event.commits.*.message", true},
		{"github.event.issue.title", false},
		{"github.event.pull_request.title", false},
		{"github.head_ref", false},
		{"github.event.pull_request.head.ref", false},
		{"github.event.head_commit.author.name", false},
		{"github.event.pull_request.title || 'x'", false},
		{"github.event.pull_request.title || github.event.pull_request.body", true},
		{"format('{0} {1}', github.event.issue.title, github.event.issue.body)", true},
		{"not an expression )", true},
	}
	for _, tc := range tests {
		if got := exprMayHaveNewline(tc.expr); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestOutputClobberingNamesTheRecordOfTheValue: the message names the record whose value an outsider writes, not the
// first record of the command.
func TestOutputClobberingNamesTheRecordOfTheValue(t *testing.T) {
	src := "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - id: gate\n        env:\n          BODY: ${{ github.event.comment.body }}\n          LIT: x\n        run: |\n          printf 'approved=%s\\nbody=%s\\n' \"$LIT\" \"$BODY\" >> \"$GITHUB_OUTPUT\"\n      - if: steps.gate.outputs.approved == 'true'\n        run: echo x\n"
	cfg := &Config{Rules: map[string]RuleConfig{"output-clobbering": {Level: SeverityError}}}
	n := 0
	for _, e := range lintBatchD(t, "test.yaml", src, cfg) {
		if e.ID != "output-clobbering" {
			continue
		}
		n++
		if !strings.Contains(e.Message, `the record "body=..."`) || strings.Contains(e.Message, `"approved=...`) {
			t.Errorf("the message names the wrong record: %s", e.Message)
		}
	}
	if n != 1 {
		t.Errorf("%d findings, want 1", n)
	}
}
