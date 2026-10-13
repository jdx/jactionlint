package jactionlint

import (
	"path"
	"strings"

	"mvdan.cc/sh/v3/syntax"
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

// gnuBrewPackages are the formulas of Homebrew that install GNU tools in place of, or next to, the BSD ones.
// Every formula whose name starts with gnu- is one as well.
var gnuBrewPackages = map[string]bool{
	"coreutils": true, "findutils": true, "diffutils": true, "grep": true, "gawk": true, "gsed": true,
}

// wrapperCommands run the command that follows them.
var wrapperCommands = map[string]bool{"sudo": true, "doas": true, "env": true, "command": true, "exec": true, "time": true, "nohup": true}

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
//   - one of its command words is a GNU tool by its Homebrew name (`gsed`, `gdate`, ...), or it calls `brew install` (or
//     reinstall, upgrade) with one of the formulas of the GNU tools as an argument, or it names a `gnubin` directory;
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

// callIsForMoreSystems looks at the command word and, for brew, the arguments of a simple command.
func callIsForMoreSystems(c *syntax.CallExpr) bool {
	var words []string // the literal words, "" for a word that is not
	for _, a := range c.Args {
		words = append(words, literalWord(a))
	}
	// The command word is the first one that is not a wrapper, an option of it or an assignment
	i := 0
	for ; i < len(words); i++ {
		w := words[i]
		if w == "" {
			return false // a command word which is not known
		}
		if wrapperCommands[path.Base(w)] || strings.HasPrefix(w, "-") || strings.Contains(w, "=") {
			continue
		}
		break
	}
	if i >= len(words) {
		return false
	}
	name := path.Base(words[i])
	switch {
	case name == "uname" || name == "sw_vers" || gnuPrefixedCommands[name]:
		return true
	case name == "brew":
		installing := false
		for _, a := range words[i+1:] {
			switch {
			case a == "install" || a == "reinstall" || a == "upgrade":
				installing = true
			case installing && !strings.HasPrefix(a, "-") && (gnuBrewPackages[a] || strings.HasPrefix(a, "gnu-")):
				return true
			}
		}
	}
	return false
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
