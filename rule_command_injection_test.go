package jactionlint

import (
	"strings"
	"testing"
)

func commandInjectionConfig() *Config {
	return &Config{Rules: map[string]RuleConfig{"command-injection": {Level: SeverityError}}}
}

func countCommandInjection(t *testing.T, src string) int {
	t.Helper()
	n := 0
	for _, e := range lintBatchD(t, "test.yaml", src, commandInjectionConfig()) {
		if e.ID == "command-injection" {
			n++
		}
	}
	return n
}

// TestCommandInjectionShell is the table of the shell sinks of command-injection. want is the number of findings.
func TestCommandInjectionShell(t *testing.T) {
	const head = "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          BODY: ${{ github.event.comment.body }}\n          REF: ${{ github.head_ref }}\n          NUM: ${{ github.event.issue.number }}\n          REPO: ${{ github.repository }}\n          LIT: hello\n        run: |\n"
	tests := []struct {
		what   string
		script string
		want   int
	}{
		// --- eval and source ---
		{"eval of the variable", `eval "$BODY"`, 1},
		{"eval with the variable in a longer string", `eval "echo $BODY"`, 1},
		{"eval unquoted", `eval $BODY`, 1},
		{"eval of a substitution that echoes it", `eval "$(echo "$BODY" | tr a b)"`, 1},
		{"eval of a script variable", "V=$BODY\neval \"$V\"", 1},
		{"eval of a head ref", `eval "git checkout $REF"`, 1},
		{"eval of a value read from it", "read -r CMD <<< \"$BODY\"\neval \"$CMD\"", 1},
		{"eval in a loop over its words", "for x in $BODY; do eval \"$x\"; done", 1},
		{"source of a process substitution", `source <(echo "$BODY")`, 1},
		{"dot of a process substitution", `. <(echo "$BODY")`, 1},
		{"source of stdin", `source /dev/stdin <<< "$BODY"`, 1},
		// --- a shell with -c ---
		{"bash -c with the variable in the string", `bash -c "echo $BODY"`, 1},
		{"sh -c", `sh -c "echo $BODY"`, 1},
		{"bash -ec", `bash -ec "echo $BODY"`, 1},
		{"bash with options before -c", `bash --noprofile --norc -eo pipefail -c "echo $BODY"`, 1},
		{"sudo bash -c", `sudo bash -c "echo $BODY"`, 1},
		{"docker run sh -c", `docker run --rm alpine sh -c "echo $BODY"`, 1},
		{"the whole string is the variable", `bash -c "$BODY"`, 1},
		{"string made by a substitution", `bash -c "$(echo "$BODY")"`, 1},
		// --- a variable as the command ---
		{"the variable is the command", `$BODY`, 1},
		{"the quoted variable is the command", `"$BODY" --version`, 1},
		{"the braced variable is the command", `${BODY} --version`, 1},
		{"sudo and the variable", `sudo $BODY`, 1},
		{"a command made from the comment", "CMD=$(echo \"$BODY\" | sed -n 's#^/run ##p')\n$CMD", 1},
		// --- the shell reads the program from its input ---
		{"pipe into bash", `echo "$BODY" | bash`, 1},
		{"pipe into sh -s", `echo "$BODY" | sh -s`, 1},
		{"pipe through a filter into bash", `echo "$BODY" | sed 's/a/b/' | bash`, 1},
		{"here string into bash", `bash <<< "$BODY"`, 1},
		{"here document into bash", "bash <<EOF\necho $BODY\nEOF", 1},
		{"the here document of the producer", "cat <<EOF | bash\necho $BODY\nEOF", 1},
		{"the here document of an earlier stage", "cat <<EOF | sed 's/a/b/' | bash\necho $BODY\nEOF", 1},
		{"a quoted here document of the producer", "cat <<'EOF' | bash\necho $BODY\nEOF", 0},
		{"a here document of the producer without the variable", "cat <<EOF | bash\necho hi\nEOF", 0},
		{"process substitution to bash", `bash <(echo "$BODY")`, 1},
		// --- xargs ---
		{"xargs -I with a shell", `echo "$BODY" | xargs -I{} sh -c 'echo {}'`, 1},
		{"xargs -I with a space", `printf '%s\n' "$BODY" | xargs -I % bash -c "echo %"`, 1},
		{"xargs with a here string", `xargs -I{} sh -c 'echo {}' <<< "$BODY"`, 1},
		{"xargs -i", `echo "$BODY" | xargs -i sh -c 'echo {}'`, 1},
		// --- an inline program ---
		{"python -c", `python3 -c "print('$BODY')"`, 1},
		{"node -e", `node -e "console.log('$BODY')"`, 1},
		{"perl -pe", `perl -pe "s/a/$BODY/"`, 1},
		{"ruby -e", `ruby -e "puts '$BODY'"`, 1},

		// --- not reported: the value is data ---
		{"plain argument", `echo "$BODY"`, 0},
		{"argument of a program", `git checkout "$REF"`, 0},
		{"the value after the command string", `bash -c 'echo "$1"' _ "$BODY"`, 0},
		{"single quoted command string", `bash -c 'echo "$BODY"'`, 0},
		{"single quoted eval", `eval 'echo "$BODY"'`, 0},
		{"a literal command string", `bash -c "echo hello"`, 0},
		{"a trusted variable is evaluated", `eval "$LIT"`, 0},
		{"the repository is trusted", `eval "echo $REPO"`, 0},
		{"a number of the event", `eval "echo $NUM"`, 0},
		{"a literal piped into a shell", `echo "set -e" | bash`, 0},
		{"a script of the repository", `bash ./build.sh "$BODY"`, 0},
		{"a command of a directory", `$HOME/bin/tool "$BODY"`, 0},
		{"a variable that only starts the command name", "DIR=$BODY\n$DIR/tool", 0},
		{"xargs without a replacement string", `echo "$BODY" | xargs sh -c 'echo "$@"' _`, 0},
		{"xargs without a shell", `echo "$BODY" | xargs -I{} echo {}`, 0},
		{"python script with the value as an argument", `python3 script.py "$BODY"`, 0},
		{"python reads the environment", `python3 -c 'import os; print(os.environ["BODY"])'`, 0},
		{"ssh-agent", `eval "$(ssh-agent -s)"`, 0},
		{"tool setup", `eval "$(brew shellenv)"`, 0},
		{"a command with a flag", `grep -c x <<< "$BODY"`, 0},
		{"a function argument is not followed", "run() { eval \"$1\"; }\nrun \"$BODY\"", 0},

		// --- not reported: the value was made safe ---
		{"validated with an anchored pattern", "[[ \"$BODY\" =~ ^[a-z0-9-]+$ ]] || exit 1\neval \"echo $BODY\"", 0},
		{"cut down to a character set", "V=$(echo \"$BODY\" | tr -cd 'a-z0-9')\neval \"echo $V\"", 0},
		{"cut down with an expansion", `eval "echo ${BODY//[^a-z0-9]/}"`, 0},
		{"cut down with sed", "V=$(echo \"$BODY\" | sed 's/[^a-zA-Z0-9]/-/g')\nbash -c \"echo $V\"", 0},
		{"quoted with printf %q", `eval "echo $(printf '%q' "$BODY")"`, 0},
		{"quoted with @Q", `eval "echo ${BODY@Q}"`, 0},
		{"counted", `eval "echo $(echo "$BODY" | wc -c)"`, 0},
		{"hashed", `eval "echo $(echo "$BODY" | sha256sum)"`, 0},
		{"only the newlines are removed", "V=$(echo \"$BODY\" | tr -d '\\n')\neval \"echo $V\"", 1},
		{"a pattern that allows shell syntax", "[[ \"$BODY\" =~ ^[a-z$]+$ ]] || exit 1\neval \"echo $BODY\"", 1},
		{"an unanchored pattern", "[[ \"$BODY\" =~ [a-z]+ ]] || exit 1\neval \"echo $BODY\"", 1},
		{"the check comes too late", "eval \"echo $BODY\"\n[[ \"$BODY\" =~ ^[a-z]+$ ]] || exit 1", 1},
		{"a set that holds the dollar sign", "V=$(echo \"$BODY\" | tr -cd 'a-z$()')\neval \"echo $V\"", 1},
		{"the value is overwritten before", "V=$BODY\nV=safe\neval \"$V\"", 0},
		{"the value is overwritten in a branch only", "V=safe\nif [ -n \"$X\" ]; then V=$BODY; fi\neval \"$V\"", 1},
		{"template expression in the script", `eval "${{ github.event.comment.body }}"`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head
			for _, l := range strings.Split(tc.script, "\n") {
				src += "          " + l + "\n"
			}
			if got := countCommandInjection(t, src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}

// TestCommandInjectionOtherShells: pwsh and cmd scripts are not parsed, and the rule works the same under a custom bash.
func TestCommandInjectionContext(t *testing.T) {
	mk := func(shell string) string {
		return "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          BODY: ${{ github.event.comment.body }}\n" + shell + "        run: eval \"$BODY\"\n"
	}
	for _, tc := range []struct {
		what, shell string
		want        int
	}{
		{"default shell", "", 1},
		{"bash", "        shell: bash\n", 1},
		{"sh", "        shell: sh\n", 1},
		{"pwsh", "        shell: pwsh\n", 0},
	} {
		t.Run(tc.what, func(t *testing.T) {
			if got := countCommandInjection(t, mk(tc.shell)); got != tc.want {
				t.Errorf("%d findings, want %d", got, tc.want)
			}
		})
	}
	t.Run("job env and workflow env", func(t *testing.T) {
		src := "on: issue_comment\npermissions: {}\nenv:\n  BODY: ${{ github.event.comment.body }}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: eval \"$BODY\"\n"
		if got := countCommandInjection(t, src); got != 1 {
			t.Errorf("%d findings, want 1", got)
		}
	})
	t.Run("step env overrides with a literal", func(t *testing.T) {
		src := "on: issue_comment\npermissions: {}\nenv:\n  BODY: ${{ github.event.comment.body }}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          BODY: fixed\n        run: eval \"$BODY\"\n"
		if got := countCommandInjection(t, src); got != 0 {
			t.Errorf("%d findings, want 0", got)
		}
	})
	t.Run("the trigger does not matter", func(t *testing.T) {
		src := "on: pull_request\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          REF: ${{ github.head_ref }}\n        run: eval \"git checkout $REF\"\n"
		if got := countCommandInjection(t, src); got != 1 {
			t.Errorf("%d findings, want 1", got)
		}
	})
	t.Run("an input of a composite action is not judged", func(t *testing.T) {
		src := "name: x\ndescription: x\ninputs:\n  cmd:\n    description: x\nruns:\n  using: composite\n  steps:\n    - shell: bash\n      env:\n        CMD: ${{ inputs.cmd }}\n      run: eval \"$CMD\"\n"
		l, err := NewLinter(discardWriter{}, &LinterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = commandInjectionConfig()
		errs, err := l.Lint(".github/actions/x/action.yml", []byte(src), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range errs {
			if e.ID == "command-injection" {
				t.Errorf("unexpected finding: %s", e.Message)
			}
		}
	})
	t.Run("an event property of a composite action is judged", func(t *testing.T) {
		src := "name: x\ndescription: x\nruns:\n  using: composite\n  steps:\n    - shell: bash\n      env:\n        BODY: ${{ github.event.comment.body }}\n      run: eval \"$BODY\"\n"
		l, err := NewLinter(discardWriter{}, &LinterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = commandInjectionConfig()
		errs, err := l.Lint(".github/actions/x/action.yml", []byte(src), nil)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, e := range errs {
			if e.ID == "command-injection" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%d findings, want 1", n)
		}
	})
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// TestCommandInjectionGitHubScript is the table of the github-script sinks.
func TestCommandInjectionGitHubScript(t *testing.T) {
	const head = "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/github-script@v7\n        env:\n          BODY: ${{ github.event.comment.body }}\n          REPO: ${{ github.repository }}\n        with:\n          script: |\n"
	const cp = "const { execSync, spawnSync, execFileSync, execFile, exec: cpExec } = require('child_process');\n"
	tests := []struct {
		what   string
		script string
		want   int
	}{
		// --- reported ---
		{"execSync with a template", cp + "execSync(`echo ${process.env.BODY}`);", 1},
		{"execSync with a concatenation", cp + `execSync("echo " + process.env.BODY);`, 1},
		{"execSync with the value alone", cp + `execSync(process.env.BODY);`, 1},
		{"bracket access", cp + `execSync("echo " + process.env['BODY']);`, 1},
		{"a constant of the value", cp + "const body = process.env.BODY;\nexecSync(`echo ${body}`);", 1},
		{"a destructured value", cp + "const { BODY } = process.env;\nexecSync(`echo ${BODY}`);", 1},
		{"a destructured value with a fallback", cp + "const { BODY } = process.env || {};\nexecSync(`echo ${BODY}`);", 1},
		{"a destructured value with a nullish fallback", cp + "const { BODY } = process.env ?? {};\nexecSync(`echo ${BODY}`);", 1},
		{"a destructured value in parentheses", cp + "const { BODY } = (process.env);\nexecSync(`echo ${BODY}`);", 1},
		{"a destructured value with an alternative that is the source", cp + "const { BODY } = {} || process.env;\nexecSync(`echo ${BODY}`);", 1},
		{"a destructured payload with a fallback", cp + "const { body } = context.payload.comment ?? {};\nexecSync(`echo ${body}`);", 1},
		{"a destructured payload with a fallback and an await", cp + "const { body } = (await context.payload.comment) || {};\nexecSync(`echo ${body}`);", 1},
		{"a destructured value of a literal", cp + "const { BODY } = { BODY: 'x' } || {};\nexecSync(`echo ${BODY}`);", 0},
		{"a destructured value of another object", cp + "const { BODY } = config ?? {};\nexecSync(`echo ${BODY}`);", 0},
		{"a destructured payload of a literal", cp + "const { body } = { body: 'x' };\nexecSync(`echo ${body}`);", 0},
		{"a renamed destructured value", cp + "const { BODY: b } = process.env;\nexecSync(`echo ${b}`);", 1},
		{"a command made of the value", cp + "const cmd = `echo ${process.env.BODY}`;\nexecSync(cmd);", 1},
		{"through the module", "const cp = require('child_process');\ncp.execSync(`echo ${process.env.BODY}`);", 1},
		{"through the module name", "const child_process = require('node:child_process');\nchild_process.exec(`echo ${process.env.BODY}`);", 1},
		{"require in the call", "require('child_process').execSync(`echo ${process.env.BODY}`);", 1},
		{"promisified exec", "const util = require('util');\nconst { exec } = require('child_process');\nconst run = util.promisify(exec);\nawait run(`echo ${process.env.BODY}`);", 1},
		{"exec of the module with a callback", cp + "cpExec(`echo ${process.env.BODY}`, () => {});", 1},
		{"spawnSync with a shell", cp + "spawnSync(`echo ${process.env.BODY}`, { shell: true });", 1},
		{"execFileSync with a shell and arguments", cp + "execFileSync('echo', [process.env.BODY], { shell: true });", 1},
		{"a shell with -c", cp + "spawnSync('bash', ['-c', `echo ${process.env.BODY}`]);", 1},
		{"a shell with -c in the arguments", cp + "spawnSync('sh', '-c', `echo ${process.env.BODY}`);", 1},
		{"a shell with a path", cp + "spawnSync('/bin/bash', ['-c', process.env.BODY]);", 1},
		{"a shell with a longer path", cp + "spawnSync(\"/usr/local/bin/zsh\", [\"-c\", process.env.BODY]);", 1},
		{"a shell in backticks", cp + "spawnSync(`sh`, [`-c`, process.env.BODY]);", 1},
		{"a shell with a path in backticks", cp + "const cmd = process.env.BODY;\nspawnSync(`/bin/dash`, [`-lc`, cmd]);", 1},
		{"a shell with double quotes and -ec", cp + "execFileSync(\"ksh\", [\"-ec\", process.env.BODY]);", 1},
		{"a path to a program that is not a shell", cp + "spawnSync('/usr/bin/python3', ['-c', process.env.BODY]);", 0},
		{"a program whose name ends with sh", cp + "spawnSync('/usr/bin/gnuplotsh', ['-c', process.env.BODY]);", 0},
		{"a shell without -c", cp + "spawnSync('/bin/bash', ['script.sh', process.env.BODY]);", 0},
		{"an option that is not the one of the shell", cp + "spawnSync('/bin/bash', ['-x', process.env.BODY]);", 0},
		{"-c that holds an expression", cp + "spawnSync('bash', ['-c' + x, process.env.BODY]);", 0},
		{"exec of child_process bound to the name exec", "const exec = require('child_process');\nexec.exec(`echo ${process.env.BODY}`);", 1},
		{"exec of child_process, the value in the middle", "const exec = require('child_process');\nexec.exec('echo ' + process.env.BODY + ' | wc');", 1},
		{"exec.exec of @actions/exec, the value in the middle", "const exec = require('@actions/exec');\nawait exec.exec(`echo ${process.env.BODY}`);", 0},
		{"a function taken from the module", "const execSync = require('child_process').execSync;\nexecSync(`echo ${process.env.BODY}`);", 1},
		{"a function taken from the module with node:", "const run = require('node:child_process').exec;\nrun(`echo ${process.env.BODY}`, () => {});", 1},
		{"spawnSync taken from the module with a shell", "const sp = require('child_process').spawnSync;\nsp(`echo ${process.env.BODY}`, { shell: true });", 1},
		{"spawnSync taken from the module without a shell", "const sp = require('child_process').spawnSync;\nsp('echo', [process.env.BODY]);", 0},
		{"a name taken from the module that is not a call of child_process", "const run = require('child_process').ChildProcess;\nrun(`echo ${process.env.BODY}`);", 0},
		{"the program of exec.exec", `await exec.exec(process.env.BODY);`, 1},
		{"the program of exec.exec in a template", "await exec.exec(`${process.env.BODY} --version`);", 1},
		{"exec.exec with a shell -c", "await exec.exec('bash', ['-c', `echo ${process.env.BODY}`]);", 1},
		{"the payload of the event", cp + "execSync(`echo ${context.payload.comment.body}`);", 1},
		{"a constant of the payload", cp + "const title = context.payload.issue.title;\nexecSync(`echo ${title}`);", 1},
		{"a destructured payload", cp + "const { title } = context.payload.issue;\nexecSync(`echo ${title}`);", 1},
		{"a trimmed value", cp + "execSync(`echo ${process.env.BODY.trim()}`);", 1},

		// --- not reported ---
		{"arguments of a program", cp + "execFileSync('echo', [process.env.BODY]);", 0},
		{"arguments of exec.exec", `await exec.exec('echo', [process.env.BODY]);`, 0},
		{"the value is an argument of the command line of exec.exec", "await exec.exec(`echo ${process.env.BODY}`);", 0},
		{"the shell reads the variable", cp + `execSync('echo "$BODY"');`, 0},
		{"the value goes through the options", cp + `execSync('echo "$BODY"', { env: { ...process.env, BODY: process.env.BODY } });`, 0},
		{"a trusted variable", cp + "execSync(`echo ${process.env.REPO}`);", 0},
		{"a number", cp + "execSync(`echo ${Number(process.env.BODY)}`);", 0},
		{"parseInt", cp + "execSync(`echo ${parseInt(process.env.BODY, 10)}`);", 0},
		{"a cleaned value", cp + "execSync(`echo ${process.env.BODY.replace(/[^a-z]/g, '')}`);", 0},
		{"a function of the script", cp + "execSync(`echo ${shellQuote(process.env.BODY)}`);", 0},
		{"validated with an anchored pattern", cp + "if (!/^[a-z0-9-]+$/.test(process.env.BODY)) throw new Error('bad');\nexecSync(`echo ${process.env.BODY}`);", 0},
		{"validated through a constant", cp + "const b = process.env.BODY;\nif (!/^[a-z0-9-]+$/.test(b)) throw new Error('bad');\nexecSync(`echo ${b}`);", 0},
		{"a regular expression exec", "const m = /a(.*)/.exec(process.env.BODY);", 0},
		{"a regular expression object", "const re = /a/;\nre.exec(process.env.BODY);", 0},
		{"a comment", cp + "// execSync(`echo ${process.env.BODY}`);", 0},
		{"a string", cp + "const s = 'execSync(`echo ${process.env.BODY}`)';", 0},
		{"without a shell option", cp + "spawnSync('echo', [process.env.BODY]);", 0},
		{"a shell option of a nested object", cp + "spawnSync(`echo ${process.env.BODY}`, { env: { shell: 'bash' } });", 0},
		{"a shell option in a comment", cp + "spawnSync(`echo ${process.env.BODY}`, {\n  // shell: true\n  cwd: '.' });", 0},
		{"a shell option in a string", cp + "spawnSync(`echo ${process.env.BODY}`, { cwd: 'shell: true' });", 0},
		{"a shell option in a block comment", cp + "spawnSync(`echo ${process.env.BODY}`, { /* shell: true */ cwd: '.' });", 0},
		{"a shell option of another name", cp + "spawnSync(`echo ${process.env.BODY}`, { myshell: true });", 0},
		{"a top level shell option after a nested object", cp + "spawnSync(`echo ${process.env.BODY}`, { env: { shell: 'x' }, shell: true });", 1},
		{"a shell option that is a path", cp + "spawnSync(`echo ${process.env.BODY}`, { shell: '/bin/bash' });", 1},
		{"a shell option with the arguments array", cp + "spawnSync('echo', [process.env.BODY], { cwd: '.', shell: true });", 1},
		{"shell false", cp + "spawnSync(`echo ${process.env.BODY}`, { shell: false });", 0},
		{"an expression of the workflow", cp + "execSync(`echo ${{ github.event.comment.body }}`);", 0},
		{"exec of an unknown object", "const { exec } = require('./exec');\nexec(`echo ${process.env.BODY}`);", 0},
		{"the value is only logged", "console.log(process.env.BODY);", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head
			for _, l := range strings.Split(tc.script, "\n") {
				src += "            " + l + "\n"
			}
			if got := countCommandInjection(t, src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}
