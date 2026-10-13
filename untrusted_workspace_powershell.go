package jactionlint

import (
	"regexp"
	"strings"
)

// powerShellRunsWorkspace reports whether a PowerShell script invokes a file of the workspace: a command that is a relative
// path (`./build.ps1`, `& .\build.ps1`, `. ./lib.ps1`), `pwsh -File ./x.ps1` (or `powershell`), or `Import-Module ./x.psm1`.
// Comments, strings that are only text, URLs and arguments (`Get-Content ./x.ps1`, `-OutFile install.ps1`) do not count.
func powerShellRunsWorkspace(script string) bool {
	for _, cmd := range powerShellCommands(script) {
		if powerShellCommandRunsWorkspace(cmd) {
			return true
		}
	}
	return false
}

// psWord is a word of a command, and whether it was written in quotes (a string at the place of the command is a command
// name or a path that is invoked with `&`, and text anywhere else).
type psWord struct {
	text   string
	quoted bool
}

// powerShellCommands splits the script into commands made of words: the comments (`# ...` and `<# ... #>`) are dropped and the
// commands end at a newline, `;`, `|`, `&&`, `||` or a brace.
func powerShellCommands(script string) [][]psWord {
	script = strings.ReplaceAll(script, "\r\n", "\n")
	var cmds [][]psWord
	var cur []psWord
	var word strings.Builder
	inWord, quoted := false, false
	flushWord := func() {
		if inWord {
			cur = append(cur, psWord{word.String(), quoted})
		}
		word.Reset()
		inWord, quoted = false, false
	}
	flushCmd := func() {
		flushWord()
		if len(cur) > 0 {
			cmds = append(cmds, cur)
		}
		cur = nil
	}
	rs := []rune(script)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '<' && i+1 < len(rs) && rs[i+1] == '#' && !inWord:
			// a block comment
			end := strings.Index(string(rs[i+2:]), "#>")
			if end < 0 {
				i = len(rs)
			} else {
				i += 2 + len([]rune(string(rs[i+2:])[:end])) + 1
			}
		case c == '#' && !inWord:
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
			flushCmd()
		case c == '\'' || c == '"':
			// a string: its text is a word, and what is inside is not read as commands
			if !inWord {
				inWord = true
			}
			quoted = true
			for i++; i < len(rs) && rs[i] != c; i++ {
				if c == '"' && rs[i] == '`' && i+1 < len(rs) {
					i++
				}
				word.WriteRune(rs[i])
			}
		case c == '$' && i+1 < len(rs) && rs[i+1] == '{':
			// ${{ github.workspace }} (an expression of the workflow) and ${env:X}: one word, with the braces
			inWord = true
			closer := "}"
			if i+2 < len(rs) && rs[i+2] == '{' {
				closer = "}}"
			}
			end := strings.Index(string(rs[i:]), closer)
			if end < 0 {
				word.WriteString(string(rs[i:]))
				i = len(rs)
			} else {
				chunk := string(rs[i : i+len([]rune(string(rs[i:])[:end]))+len(closer)])
				word.WriteString(chunk)
				i += len([]rune(chunk)) - 1
			}
		case c == '`' && i+1 < len(rs):
			i++
			inWord = true
			word.WriteRune(rs[i])
		case c == '\n' || c == ';' || c == '|' || c == '{' || c == '}' || c == '(' || c == ')':
			flushCmd()
		case c == '&' && i+1 < len(rs) && rs[i+1] == '&':
			i++
			flushCmd()
		case c == ' ' || c == '\t':
			flushWord()
		default:
			inWord = true
			word.WriteRune(c)
		}
	}
	flushCmd()
	return cmds
}

// reAssignedVar matches the start of an assignment: `$x`, `[string]$x`, `$env:X`, `${x}`, with the operator glued or not.
var reAssignedVar = regexp.MustCompile(`^(\[[^\]]*\])*\$(\{[^}]*\}|[\w:.]+)(\+|-|\*|/|%|\?\?)?=`)

var (
	reVarOnly  = regexp.MustCompile(`^(\[[^\]]*\])*\$(\{[^}]*\}|[\w:.]+)$`)
	reAssignOp = regexp.MustCompile(`^(\+|-|\*|/|%|\?\?)?=$`)
)

// workspaceRunners are the commands that run what is in the workspace whatever their arguments are: the tools that build,
// test and install a project. A script that calls one after a checkout of a pull request runs its code.
var workspaceRunners = map[string]bool{
	"npm": true, "npx": true, "yarn": true, "pnpm": true, "make": true, "dotnet": true, "msbuild": true, "cargo": true, "go": true,
	"gradle": true, "gradlew": true, "mvn": true, "mvnw": true, "pip": true, "pytest": true, "tox": true, "terraform": true,
	"bundle": true, "rake": true, "composer": true, "nuget": true, "cmake": true,
}

// workspaceInterpreters run a file that is given to them: they count when an argument is a path.
var workspaceInterpreters = map[string]bool{
	"bash": true, "sh": true, "python": true, "python3": true, "node": true, "ruby": true, "perl": true, "php": true, "bun": true, "deno": true,
	"start-process": true, "invoke-item": true, "ii": true, "invoke-command": true, "icm": true, "import-module": true, "ipmo": true,
	"invoke-expression": true, "iex": true,
}

// powerShellCommandRunsWorkspace reads the words of one command. A statement that is not understood is reported when a word of
// it is a path that is invoked, rather than left alone: a missed invocation is worse than an extra report.
func powerShellCommandRunsWorkspace(words []psWord) bool {
	// an assignment: what is on the right is the command (`$x = ./b.ps1`, `$x += & ./b.ps1`, `[int]$x=./b.ps1`)
	if len(words) > 0 && !words[0].quoted {
		if loc := reAssignedVar.FindStringIndex(words[0].text); loc != nil {
			rest := words[0].text[loc[1]:]
			words = words[1:]
			if rest != "" {
				words = append([]psWord{{text: rest}}, words...)
			}
		} else if len(words) > 1 && reVarOnly.MatchString(words[0].text) && !words[1].quoted && reAssignOp.MatchString(words[1].text) {
			words = words[2:] // `$x = ...`, `[string]$x += ...`
		}
	}
	i := 0
	// the call operator and the dot-source operator: the next word is what runs
	for i < len(words) && !words[i].quoted && (words[i].text == "&" || words[i].text == ".") {
		i++
	}
	if i >= len(words) {
		return false
	}
	head := words[i]
	// a string alone is a value, and a string after `&` is what runs
	if isWorkspacePath(head.text) && (!head.quoted || i > 0) {
		return true
	}
	rest := words[i+1:]
	name := strings.ToLower(strings.TrimSuffix(head.text, ".exe"))
	if workspaceRunners[name] {
		return true
	}
	switch name {
	case "pwsh", "powershell":
		// pwsh -File ./x.ps1, -f ./x.ps1; -Command is a script of its own and is read as one
		for j, w := range rest {
			if w.quoted {
				continue
			}
			switch strings.ToLower(w.text) {
			case "-file", "-f":
				if j+1 < len(rest) && isWorkspacePath(rest[j+1].text) {
					return true
				}
			case "-command", "-c":
				if j+1 < len(rest) {
					var parts []string
					for _, r := range rest[j+1:] {
						parts = append(parts, r.text)
					}
					return powerShellRunsWorkspace(strings.Join(parts, " "))
				}
			}
		}
	}
	if workspaceInterpreters[name] {
		for _, w := range rest {
			if isWorkspacePath(w.text) {
				return true
			}
		}
	}
	return false
}

var reWorkspaceExpr = regexp.MustCompile(`(?i)^\$\{\{\s*github\.workspace\s*\}\}`)

// isWorkspacePath reports whether the word is a path of the workspace: a relative path (a word with a separator, `./x`,
// `.\x`, `../x`, `scripts/x.ps1`) or a path below the workspace (`$env:GITHUB_WORKSPACE\x`, `${{ github.workspace }}/x`). In
// PowerShell a word with a separator is a path, and a bare name (`build.ps1`) is not run from the current directory. A URL, an
// absolute path and a variable are not.
func isWorkspacePath(w string) bool {
	l := strings.ToLower(strings.TrimSpace(w))
	switch {
	case l == "", strings.Contains(l, "://"):
		return false
	case strings.HasPrefix(l, "$env:github_workspace"), strings.HasPrefix(l, "${env:github_workspace}"), reWorkspaceExpr.MatchString(l):
		return true
	case strings.HasPrefix(l, "-"), strings.HasPrefix(l, "$"), strings.HasPrefix(l, "~"), strings.HasPrefix(l, "/"), strings.HasPrefix(l, `\`):
		return false // an option, a variable, a path of the home or the root
	case len(l) >= 2 && l[1] == ':':
		return false // a drive
	}
	return strings.ContainsAny(l, `/\`)
}
