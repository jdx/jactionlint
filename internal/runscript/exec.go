package runscript

import (
	"path"
	"regexp"
	"strings"
)

// This file describes the commands which run text as code: a shell with `-c STRING`, an interpreter with an inline
// program (`python -c`, `node -e`), and the commands which read code from their input. Like the rest of the
// package it only describes the script; the rules decide what is dangerous.

// ShellString returns the command string of `SHELL [options] -c STRING` among the words of the command, and the
// name of the shell. The shell does not have to be the command itself: `docker run img sh -c "..."`,
// `sudo bash -c "..."` and `xargs -I{} sh -c "..."` have one too. It returns a nil word when there is none.
// The string that follows is what the shell parses as code, so the arguments after it (`_ "$BODY"`) do not count.
func (c *Command) ShellString() (shell string, str *Word) {
	if c.Decl {
		return "", nil
	}
	for i, w := range c.Words {
		if w.Dynamic() || !isShellName(w.Value) {
			continue
		}
		if w != c.NameWord && !launchers[c.Name] {
			continue // `echo bash -c x` prints the words
		}
		if s := shellStringAfter(c.Words[i+1:]); s != nil {
			return shellBase(w.Value), s
		}
	}
	return "", nil
}

// launchers are the commands that start the program they are given as arguments, so that a shell among their
// arguments runs.
var launchers = set("docker podman nerdctl kubectl oc ssh nsenter chroot xargs env nohup timeout flock watch script parallel su runuser setsid unshare find lxc multipass vagrant sshpass exec")

func shellBase(name string) string {
	return strings.TrimSuffix(path.Base(name), ".exe")
}

func isShellName(name string) bool {
	n := shellBase(name)
	return shells[n] && n != "busybox"
}

// shellStringAfter walks the options of a shell and returns the word that `-c` takes.
func shellStringAfter(words []*Word) *Word {
	foundC := false
	for i := 0; i < len(words); i++ {
		w := words[i]
		v := w.Value
		if w.Dynamic() && !strings.HasPrefix(v, "-") {
			if foundC {
				return w
			}
			return nil
		}
		if !strings.HasPrefix(v, "-") && !strings.HasPrefix(v, "+") {
			if foundC {
				return w
			}
			return nil // a script file
		}
		switch {
		case v == "--" || v == "-":
			continue
		case v == "-o" || v == "+o" || v == "-O" || v == "+O" || v == "--rcfile" || v == "--init-file":
			i++ // the value
		case strings.HasPrefix(v, "--"):
			// --login, --norc, --posix ...
		case len(v) > 1 && v[0] == '-':
			if strings.Contains(v[1:], "c") {
				foundC = true
			}
			if last := v[len(v)-1]; last == 'o' || last == 'O' {
				i++
			}
		}
	}
	return nil
}

// ReadsStdin reports whether the shell, `eval`-like command or `source` reads the code it runs from its standard
// input: `sh`, `bash -s`, `bash -` and `source /dev/stdin` do, `bash file` and `bash -c cmd` do not.
func (c *Command) ReadsStdin() bool { return readsStdin(c) }

// IsShell reports whether the command is a shell (sh, bash, zsh, ...), which can run what it is given as code.
func (c *Command) IsShell() bool { return shells[c.Tool] && c.Name != "busybox" }

// ReplaceString returns the replacement string of `xargs -I STR` (`{}` for -i and --replace), "" when the
// command has none.
func (c *Command) ReplaceString() string {
	if c.Name != "xargs" {
		return ""
	}
	for i, a := range c.Args {
		v := a.Value
		switch {
		case v == "-I" && i+1 < len(c.Args):
			return c.Args[i+1].Value
		case strings.HasPrefix(v, "-I") && len(v) > 2:
			return v[2:]
		case v == "-i" || v == "--replace":
			return "{}"
		case strings.HasPrefix(v, "-i") && len(v) > 2 && !strings.HasPrefix(v, "--"):
			return v[2:]
		case strings.HasPrefix(v, "--replace="):
			return strings.TrimPrefix(v, "--replace=")
		}
	}
	return ""
}

// inlineFlags lists, per interpreter, the options that take a program text as their next word.
var inlineFlags = []struct {
	interp string
	name   *regexp.Regexp
	flags  *regexp.Regexp
}{
	{"python", regexp.MustCompile(`^(python|py)[0-9.]*$`), regexp.MustCompile(`^-[A-Za-z]*c$`)},
	{"node", regexp.MustCompile(`^(node|nodejs|bun)$`), regexp.MustCompile(`^(-e|-p|-pe|-ep|--eval|--print)$`)},
	{"perl", regexp.MustCompile(`^perl[0-9.]*$`), regexp.MustCompile(`^-[A-Za-z]*[eE]$`)},
	{"ruby", regexp.MustCompile(`^ruby[0-9.]*$`), regexp.MustCompile(`^-[A-Za-z]*e$`)},
	{"php", regexp.MustCompile(`^php[0-9.]*$`), regexp.MustCompile(`^-r$`)},
	{"pwsh", regexp.MustCompile(`^(?i:pwsh|powershell)$`), regexp.MustCompile(`^(?i:-c|-command)$`)},
	{"lua", regexp.MustCompile(`^(lua|luajit)[0-9.]*$`), regexp.MustCompile(`^-e$`)},
	{"Rscript", regexp.MustCompile(`^Rscript$`), regexp.MustCompile(`^-e$`)},
}

// InlineCode returns the program text of an interpreter that is given on the command line (`python -c "..."`,
// `node -e "..."`, `perl -pe "..."`) and the name of the interpreter. The word is nil when the command has none.
func (c *Command) InlineCode() (interp string, code *Word) {
	if c.Decl || c.NameWord == nil || c.Name == "" {
		return "", nil
	}
	start := -1
	for i, w := range c.Words {
		if w == c.NameWord {
			start = i
			break
		}
	}
	if start < 0 {
		return "", nil
	}
	for _, in := range inlineFlags {
		if !in.name.MatchString(strings.TrimSuffix(c.Name, ".exe")) {
			continue
		}
		words := c.Words[start+1:]
		for i, w := range words {
			if w.Dynamic() {
				continue
			}
			if in.flags.MatchString(w.Value) && i+1 < len(words) {
				return in.interp, words[i+1]
			}
			if in.interp == "node" && strings.HasPrefix(w.Value, "--eval=") {
				return in.interp, w
			}
		}
	}
	return "", nil
}
