package jactionlint

import (
	"path"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// This file decides from the PARSED script (and the parsed expressions of the step) whether a step is written for more
// than one system, so that the rule macos-gnu-flags leaves it alone. Nothing here looks at the text of the script with a
// regular expression: it reads the command words, the arguments of a command, the parameters that are expanded and the
// words that a test or a case compares.

// gnuPrefixedCommands are the names that Homebrew gives to the GNU tools when it installs them next to the BSD ones.
var gnuPrefixedCommands = map[string]bool{
	"gsed": true, "gdate": true, "gstat": true, "gtar": true, "ggrep": true, "gawk": true, "gcp": true, "gmv": true,
	"gls": true, "greadlink": true, "gxargs": true, "gfind": true, "gtee": true, "gsort": true, "ghead": true,
	"gtail": true, "gwc": true, "gcut": true, "gtr": true, "gmktemp": true, "gbase64": true, "gtimeout": true,
	"gdu": true, "ginstall": true,
}

// wrapperSpec describes a command that runs the command after it: the options of the wrapper which take a value (the
// option and its value are skipped) and the number of positional words that come before the command (`timeout 5 cmd`).
type wrapperSpec struct {
	withValue  map[string]bool
	positional int
}

func options(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// wrapperCommands are the commands that run the command that follows them, with the options that take a value.
var wrapperCommands = map[string]wrapperSpec{
	"sudo":    {withValue: options("-u", "-g", "-h", "-p", "-C", "-D", "-r", "-t", "-U", "-T", "--user", "--group", "--host", "--prompt", "--chdir")},
	"doas":    {withValue: options("-u", "-C")},
	"env":     {withValue: options("-u", "-C", "--unset", "--chdir")},
	"command": {},
	"exec":    {withValue: options("-a")},
	"time":    {withValue: options("-f", "-o", "--format", "--output")},
	"nohup":   {},
	"nice":    {withValue: options("-n", "--adjustment")},
	"timeout": {withValue: options("-s", "-k", "--signal", "--kill-after"), positional: 1},
	"stdbuf":  {withValue: options("-i", "-o", "-e")},
	"ionice":  {withValue: options("-c", "-n", "-p", "-P", "-u")},
	"setsid":  {},
	"chronic": {},
	"xargs":   {withValue: options("-I", "-n", "-P", "-L", "-s", "-E", "-d", "-a", "-J", "-R", "-S")},
}

// isAssignmentWord reports whether the word is a variable assignment (`VAR=value`), as `env VAR=value cmd` has.
func isAssignmentWord(w string) bool {
	name, _, ok := strings.Cut(w, "=")
	if !ok || name == "" {
		return false
	}
	for i, c := range name {
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// cmdWord is a word of a simple command: its literal text ("" for a word that is not literal) and, when it comes from a
// script that runscript read, that word.
type cmdWord struct {
	text string
	src  *runscript.Word
}

// commandWordOf returns the words of a simple command and the index of the real command in them, after the wrappers,
// their options with the values of these, and the assignments; -1 when it is not a literal word. The words are the ones
// given, except that the value of `env -S` (which is a command line) is split into words and put in place of the option.
// It is the one place that knows the wrappers, for the check of what a script is written for and for the reporting.
func commandWordOf(words []cmdWord) ([]cmdWord, int) {
	i := 0
	for i < len(words) {
		w := words[i].text
		if w == "" {
			return words, -1
		}
		if isAssignmentWord(w) {
			i++
			continue
		}
		base := path.Base(w)
		spec, ok := wrapperCommands[base]
		if !ok {
			return words, i
		}
		i++
		for i < len(words) {
			o := words[i].text
			if o == "" {
				return words, -1
			}
			if o == "--" {
				i++
				break
			}
			if len(o) > 1 && strings.HasPrefix(o, "-") {
				if base == "env" {
					// env -S and --split-string: the value is a command line, which runs
					if value, used, isSplit := envSplitString(words, i); isSplit {
						if value == "" && used > 1 {
							return words, -1 // the value is not a literal word
						}
						// The words of the operand have no place of their own in the script (and are split on white space,
						// without the quoting of a shell): src is nil, so only the check of what a script is written for
						// uses them, and a command in an operand of env -S is not reported.
						expanded := append([]cmdWord{}, words[:i]...)
						for _, f := range strings.Fields(value) {
							expanded = append(expanded, cmdWord{text: f})
						}
						expanded = append(expanded, words[i+used:]...)
						words = expanded
						continue
					}
				}
				i += 1 + optionValueWords(spec, o)
				continue
			}
			if isAssignmentWord(o) {
				i++
				continue
			}
			break
		}
		i += spec.positional
	}
	return words, -1
}

// realCommand returns the command that a simple command of a script really runs. runscript has found it for the wrappers
// that it knows (sudo, env, nice, timeout, ...) and that is trusted, unless commandWordOf finds a command further on in
// the same words (`xargs`, `ionice`, `setsid`, `chronic`, or a cluster such as `sudo -Eu root`). A command in the operand
// of `env -S` is not found: its words have no place in the script.
func realCommand(c *runscript.Command) *runscript.Command {
	if c.NameWord == nil {
		return c
	}
	words := make([]cmdWord, 0, len(c.Words))
	for _, w := range c.Words {
		t := w.Value
		if w.Dynamic() {
			t = ""
		}
		words = append(words, cmdWord{t, w})
	}
	words, i := commandWordOf(words)
	if i < 0 || i >= len(words) {
		return c
	}
	// runscript knows wrappers that commandWordOf does not (so commandWordOf can stop earlier, at one of them), and
	// commandWordOf knows options that runscript does not (so it can go further). Only a command that is after the one of
	// runscript is taken, and the words of an operand of env -S, which have no place in the script, never are.
	name := path.Base(words[i].text)
	if words[i].src == nil || words[i].src.Offset <= c.NameWord.Offset || name == c.Name {
		return c
	}
	nc := *c
	nc.Name = name
	nc.Args = nil
	for _, w := range words[i+1:] {
		if w.src == nil {
			return c
		}
		nc.Args = append(nc.Args, w.src)
	}
	return &nc
}

// realCommands returns the commands of the script as they are really run (see realCommand).
func realCommands(s *runscript.Script) []*runscript.Command {
	out := make([]*runscript.Command, len(s.Commands))
	for i, c := range s.Commands {
		out[i] = realCommand(c)
	}
	return out
}

// optionValueWords returns how many of the words after the option the option takes as its value: 1 for an option that
// takes a value, or a cluster of short options whose last letter takes one (`-Eu user`), and 0 when the value is in the
// word (`-uuser`, `--user=x`) or the option has none.
func optionValueWords(spec wrapperSpec, o string) int {
	if spec.withValue[o] {
		return 1
	}
	if len(o) > 2 && o[1] != '-' {
		for k, c := range o[1:] {
			if spec.withValue["-"+string(c)] {
				if k == len(o)-2 {
					return 1 // the last letter takes the next word
				}
				return 0 // the rest of the word is its value
			}
		}
	}
	return 0
}

// envSplitString reads `-S value`, `-Svalue`, `--split-string value` and `--split-string=value` of env at words[i]. It
// returns the value, the number of words that the option and its value take, and whether it is such an option.
func envSplitString(words []cmdWord, i int) (value string, used int, ok bool) {
	o := words[i].text
	switch {
	case o == "-S" || o == "--split-string":
		if i+1 >= len(words) {
			return "", 1, true
		}
		return words[i+1].text, 2, true
	case strings.HasPrefix(o, "--split-string="):
		return strings.TrimPrefix(o, "--split-string="), 1, true
	case strings.HasPrefix(o, "-S") && len(o) > 2 && o[1] != '-':
		return o[2:], 1, true
	}
	return "", 0, false
}

// stepConditionReadsTheSystem reports whether the `if:` of a step reads the context runner or matrix, which can keep the
// step from running on macOS.
func stepConditionReadsTheSystem(cond *String) bool {
	if cond == nil {
		return false
	}
	var refs []exprRef
	if strings.Contains(cond.Value, "${{") {
		refs, _ = stringExprRefs(cond.Value)
	} else if n, err := cond.condExpr(); err == nil && n != nil {
		collectExprRefs(n, nil, &refs)
	}
	return refsRead(refs, []string{"runner"}) || refsRead(refs, []string{"matrix"})
}

// scriptIsForMoreSystems reports whether the script is written for more than one system, or for the GNU tools:
//
//   - it reads `runner.os` or `matrix.os` in an expression, or expands `$OSTYPE` or `$RUNNER_OS`;
//   - it calls `uname` or `sw_vers` (the words of a test or a case are compared with what they print);
//   - one of its command words is a GNU tool by its Homebrew name (`gsed`, `gdate`, ...), or it names a `gnubin` directory
//     (which Homebrew puts the GNU tools under with their plain names). `brew install coreutils` alone is not enough: the
//     formula is keg-only, so the BSD tool stays first on the PATH;
//   - it compares or prints the word GNU (`sed --version | grep GNU`).
//
// A script that cannot be parsed counts as written for more systems: nothing is known about it.
func scriptIsForMoreSystems(script string) bool {
	if refs, _ := stringExprRefs(script); refsRead(refs, []string{"runner", "os"}) || refsRead(refs, []string{"matrix", "os"}) {
		return true
	}
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(sanitizeExpressionsInScript(script)), "")
	if err != nil {
		return true
	}
	found := false
	syntax.Walk(f, func(node syntax.Node) bool {
		if found {
			return false
		}
		switch n := node.(type) {
		case *syntax.CallExpr:
			found = callIsForMoreSystems(n)
		case *syntax.ParamExp:
			if n.Param != nil && (n.Param.Value == "OSTYPE" || n.Param.Value == "RUNNER_OS") {
				found = true
			}
		case *syntax.Lit:
			found = literalNamesTheSystem(n.Value)
		case *syntax.SglQuoted:
			found = literalNamesTheSystem(n.Value)
		}
		return !found
	})
	return found
}

// literalNamesTheSystem reports whether a word of the script is the directory of the GNU names of Homebrew, or the
// word that a script looks for in the version of a tool (`sed --version | grep GNU`).
func literalNamesTheSystem(v string) bool {
	return v == "GNU" || strings.Contains(v, "gnubin")
}

// callIsForMoreSystems looks at the command word of a simple command, after the wrappers.
func callIsForMoreSystems(c *syntax.CallExpr) bool {
	var words []cmdWord // the literal words, "" for a word that is not
	for _, a := range c.Args {
		words = append(words, cmdWord{text: literalWord(a)})
	}
	words, i := commandWordOf(words)
	if i < 0 || i >= len(words) {
		return false
	}
	name := path.Base(words[i].text)
	return name == "uname" || name == "sw_vers" || gnuPrefixedCommands[name]
}

// literalWord returns the text of a word that has no expansion, with the quotes removed, and "" for any other word.
func literalWord(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				l, ok := q.(*syntax.Lit)
				if !ok {
					return ""
				}
				b.WriteString(l.Value)
			}
		default:
			return ""
		}
	}
	return b.String()
}
