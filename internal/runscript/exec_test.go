package runscript

import "testing"

func lastCommand(t *testing.T, script string) *Command {
	t.Helper()
	s, err := Analyze(script, "bash")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Commands) == 0 {
		t.Fatalf("no command in %q", script)
	}
	return s.Commands[len(s.Commands)-1]
}

func TestShellString(t *testing.T) {
	tests := []struct {
		script, shell, str string
	}{
		{`bash -c "echo $X"`, "bash", `echo $X`},
		{`sh -c 'echo hi'`, "sh", `echo hi`},
		{`bash -ec "echo $X"`, "bash", `echo $X`},
		{`bash -lc "echo $X"`, "bash", `echo $X`},
		{`bash --noprofile --norc -eo pipefail -c "echo $X"`, "bash", `echo $X`},
		{`sudo -u root bash -c "echo $X"`, "bash", `echo $X`},
		{`docker run --rm alpine sh -c "echo $X"`, "sh", `echo $X`},
		{`xargs -I{} sh -c 'echo {}'`, "sh", `echo {}`},
		{`/bin/bash -c "$CMD"`, "bash", `$CMD`},
		{`bash script.sh -c`, "", ""},
		{`bash -e script.sh`, "", ""},
		{`bash`, "", ""},
		{`echo bash -c hi`, "", ""}, // echo prints its words
		{`git commit -m "bash"`, "", ""},
	}
	for _, tc := range tests {
		shell, str := lastCommand(t, tc.script).ShellString()
		got := ""
		if str != nil {
			got = str.Value
		}
		if shell != tc.shell || got != tc.str {
			t.Errorf("%q: got (%q, %q), want (%q, %q)", tc.script, shell, got, tc.shell, tc.str)
		}
	}
}

func TestInlineCode(t *testing.T) {
	tests := []struct {
		script, interp, code string
	}{
		{`python3 -c "print(1)"`, "python", "print(1)"},
		{`python -Bc "print(1)"`, "python", "print(1)"},
		{`python3.11 -c 'x'`, "python", "x"},
		{`node -e "console.log(1)"`, "node", "console.log(1)"},
		{`node --eval="x"`, "node", "--eval=x"},
		{`perl -pe 's/a/b/'`, "perl", "s/a/b/"},
		{`ruby -e "puts 1"`, "ruby", "puts 1"},
		{`php -r "echo 1;"`, "php", "echo 1;"},
		{`pwsh -Command "Get-Date"`, "pwsh", "Get-Date"},
		{`python3 script.py -c x`, "python", "x"}, // scripts can have options named -c: the rules judge the text
		{`python3 script.py`, "", ""},
		{`node app.js`, "", ""},
		{`echo python -c x`, "", ""},
	}
	for _, tc := range tests {
		interp, code := lastCommand(t, tc.script).InlineCode()
		got := ""
		if code != nil {
			got = code.Value
		}
		if interp != tc.interp || got != tc.code {
			t.Errorf("%q: got (%q, %q), want (%q, %q)", tc.script, interp, got, tc.interp, tc.code)
		}
	}
}

func TestReplaceString(t *testing.T) {
	tests := []struct {
		script, want string
	}{
		{`xargs -I{} echo {}`, "{}"},
		{`xargs -I % echo %`, "%"},
		{`xargs -i echo {}`, "{}"},
		{`xargs --replace=@ echo @`, "@"},
		{`xargs --replace echo {}`, "{}"},
		{`xargs -n1 echo`, ""},
		{`echo -I{}`, ""},
	}
	for _, tc := range tests {
		if got := lastCommand(t, tc.script).ReplaceString(); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.script, got, tc.want)
		}
	}
}

func TestReadsStdinAndIsShell(t *testing.T) {
	tests := []struct {
		script       string
		shell, stdin bool
	}{
		{`bash`, true, true},
		{`bash -s`, true, true},
		{`sh -`, true, true},
		{`bash script.sh`, true, false},
		{`bash -c "x"`, true, false},
		{`source /dev/stdin`, false, true},
		{`echo hi`, false, false},
	}
	for _, tc := range tests {
		c := lastCommand(t, tc.script)
		if c.IsShell() != tc.shell || c.ReadsStdin() != tc.stdin {
			t.Errorf("%q: IsShell %v ReadsStdin %v, want %v %v", tc.script, c.IsShell(), c.ReadsStdin(), tc.shell, tc.stdin)
		}
	}
}
