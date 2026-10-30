package runner

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// shellSpec is a resolved `shell:` of a run step.
type shellSpec struct {
	argv []string // with {0} still in place
	ext  string   // extension of the script file
	// prelude and postlude wrap the script (PowerShell needs them to stop on errors and to return the exit code).
	prelude, postlude string
}

// resolveShell maps the `shell:` value of a step to the command that runs its script, like the hosted runner does:
//
//	bash   bash --noprofile --norc -eo pipefail {0}     sh   sh -e {0}
//	pwsh   pwsh -command ". '{0}'"                      python   python {0}
//
// An empty value is bash when the machine has it and sh otherwise. Any other value is a command line with {0}.
// lookPath finds a program on the PATH of the step.
func resolveShell(shell string, lookPath func(string) bool) (shellSpec, error) {
	switch s := strings.TrimSpace(shell); s {
	case "":
		if lookPath("bash") {
			return resolveShell("bash", lookPath)
		}
		return resolveShell("sh", lookPath)
	case "bash":
		return shellSpec{argv: []string{"bash", "--noprofile", "--norc", "-eo", "pipefail", "{0}"}, ext: ".sh"}, nil
	case "sh":
		return shellSpec{argv: []string{"sh", "-e", "{0}"}, ext: ".sh"}, nil
	case "python":
		return shellSpec{argv: []string{"python", "{0}"}, ext: ".py"}, nil
	case "pwsh", "powershell":
		return shellSpec{argv: []string{s, "-command", ". '{0}'"}, ext: ".ps1",
			prelude:  "$ErrorActionPreference = 'stop'\n",
			postlude: "\nif ((Test-Path -LiteralPath variable:\\LASTEXITCODE)) { exit $LASTEXITCODE }\n"}, nil
	case "cmd":
		if runtime.GOOS != "windows" {
			return shellSpec{}, fmt.Errorf("shell cmd only exists on Windows")
		}
		return shellSpec{argv: []string{"cmd", "/D", "/E:ON", "/V:OFF", "/S", "/C", "CALL \"{0}\""}, ext: ".cmd"}, nil
	default:
		fields := strings.Fields(s)
		if len(fields) < 2 || !strings.Contains(s, "{0}") {
			return shellSpec{}, fmt.Errorf("shell %q must be bash, sh, python, pwsh, powershell, cmd, or a command line with {0} for the script file", s)
		}
		return shellSpec{argv: fields, ext: ".sh"}, nil
	}
}

func (s shellSpec) command(script string) []string {
	argv := make([]string, len(s.argv))
	for i, a := range s.argv {
		argv[i] = strings.ReplaceAll(a, "{0}", script)
	}
	return argv
}

func onPath(path []string) func(string) bool {
	return func(name string) bool {
		for _, d := range path {
			if p, err := exec.LookPath(d + "/" + name); err == nil && p != "" {
				return true
			}
		}
		return false
	}
}
