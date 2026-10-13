package jactionlint

import "strings"

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

// powerShellCommandRunsWorkspace reads the words of one command.
func powerShellCommandRunsWorkspace(words []psWord) bool {
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
	switch strings.ToLower(strings.TrimSuffix(head.text, ".exe")) {
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
	case "import-module", "ipmo":
		for _, w := range rest {
			if isWorkspacePath(w.text) {
				return true
			}
		}
	}
	return false
}

// isWorkspacePath reports whether the word is a relative path (`./x`, `.\x`, `../x`) or a path below the workspace
// (`$env:GITHUB_WORKSPACE\x`, `${{ github.workspace }}/x`). A URL, an absolute path and a bare name are not.
func isWorkspacePath(w string) bool {
	l := strings.ToLower(strings.TrimSpace(w))
	switch {
	case strings.Contains(l, "://"):
		return false
	case strings.HasPrefix(l, "./"), strings.HasPrefix(l, `.\`), strings.HasPrefix(l, "../"), strings.HasPrefix(l, `..\`):
		return true
	case strings.HasPrefix(l, "$env:github_workspace"), strings.HasPrefix(l, "${env:github_workspace}"), strings.HasPrefix(l, "${{ github.workspace }}"):
		return true
	}
	return false
}
