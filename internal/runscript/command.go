package runscript

import (
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Command is a simple command: `name args...` with its assignments and redirections.
type Command struct {
	Loc
	// Words are all words of the command as written, wrappers included.
	Words []*Word
	// Wrappers are the commands in front of the real one: `sudo -E env A=1 pip install x` has [sudo env].
	// Understood: sudo, doas, env, command, exec, builtin, time, nohup, nice, timeout, stdbuf, xvfb-run.
	Wrappers []string
	// Name is the base name of the real command (`/usr/bin/pip3` is `pip3`), "" if it is not static. For a
	// dynamic name, see NameWord.
	Name string
	// NameWord is the word of the command name, nil if the command has no name (only `export`-like forms).
	NameWord *Word
	// Tool is the canonical name of the tool, if it is one of the tools the analyzer knows: pip (also pip3,
	// pip3.11, python -m pip), npm, pnpm, yarn, bun, aube, npx, cargo, go, gem, apt (also apt-get), brew, uv, pipx,
	// twine, gh, curl, wget, tee, and the shells (sh, bash, zsh, ...). Otherwise "".
	Tool string
	// Args are the words after the tool. For `python -m pip install x` they start after `pip`, so
	// Args[0] is `install` there like for `pip install x`.
	Args []*Word
	// Flags are the options in Args, in order, with their values.
	Flags []*Flag
	// Positional are the words of Args which are not flags or flag values.
	Positional []*Word
	// Assigns are the variable assignments in front of the command (`A=1 B=2 cmd`).
	Assigns []*Assignment
	// Redirects are the redirections attached to this command, including heredocs.
	Redirects []*Redirect
	// Pipeline is the pipeline the command is a stage of, nil if it is not in one. Stage is the index.
	Pipeline *Pipeline
	Stage    int
	// Decl is true for declaration builtins: export, declare, local, readonly, typeset. Their assignments are in
	// Assigns.
	Decl bool

	install *Install
}

// Flag is an option of a command.
type Flag struct {
	Word *Word
	// Name is the option without a value: `--version` for `--version=1.2`, `-r` for `-rreq.txt` and `-r req.txt`.
	// Short options which are clustered (`-yq`) are one flag named `-yq`, see Command.HasFlag.
	Name string
	// Value is the value, nil if the option has none. It is a separate word for `--version 1.2`.
	Value *Word
}

// Assignment is a variable assignment.
type Assignment struct {
	Loc
	Name   string
	Value  *Word // nil for `A=` or `declare A`
	Append bool  // +=
	Array  bool  // A=(...)
	// Cmd is the command the assignment is a prefix of or the declaration builtin it belongs to, nil for a
	// plain `A=1` statement.
	Cmd *Command
}

// HasFlag returns whether the command has any of the flags, given with their dashes (`-y`, `--yes`). Single
// letter options are also found in clusters (`-yq` has `-y`).
func (c *Command) HasFlag(names ...string) bool { return c.Flag(names...) != nil }

// Flag returns the first flag with one of the names, nil if there is none.
func (c *Command) Flag(names ...string) *Flag {
	for _, f := range c.Flags {
		for _, n := range names {
			if f.Name == n {
				return f
			}
			if len(n) == 2 && n[0] == '-' && n[1] != '-' && len(f.Name) > 2 && f.Name[0] == '-' && f.Name[1] != '-' && strings.Contains(f.Name[1:], n[1:]) {
				return f
			}
		}
	}
	return nil
}

// FlagValues returns the values of all flags with one of the names.
func (c *Command) FlagValues(names ...string) []*Word {
	var out []*Word
	for _, f := range c.Flags {
		for _, n := range names {
			if f.Value == nil {
				continue
			}
			// A cluster ending in a flag which takes a value (-qr req.txt) has the value of that last flag.
			if f.Name == n || (len(n) == 2 && n[0] == '-' && n[1] != '-' && len(f.Name) > 2 && f.Name[0] == '-' && f.Name[1] != '-' && f.Name[len(f.Name)-1] == n[1]) {
				out = append(out, f.Value)
			}
		}
	}
	return out
}

// Verb returns the first positional argument (`install` of `pip install x`), "" if there is none or it is
// dynamic.
func (c *Command) Verb() string { return c.Sub(0) }

// Sub returns the i-th positional argument, "" if there is none or it contains an expression or expansion.
func (c *Command) Sub(i int) string {
	if i >= len(c.Positional) || c.Positional[i].Dynamic() {
		return ""
	}
	return c.Positional[i].Value
}

// Installs describes the command as a package installation, nil if it is not one. See [Install].
func (c *Command) Installs() *Install { return c.install }

// Publishes describes the command as a publish of a package or release, nil if it is not one.
func (c *Command) Publishes() *Publish { return publishOf(c) }

// builder converts the syntax tree.
type builder struct {
	s    *Script
	sub  string
	cmds map[syntax.Command]*Command
	// pipes whose chain is already collected by the outermost BinaryCmd; negated pipelines
	seenPipe map[*syntax.BinaryCmd]bool
	negated  map[*syntax.BinaryCmd]bool
	tested   map[*syntax.BinaryCmd]bool // pipelines whose status is tested, see Pipeline.Tested
	pending  []pendingPipeline
	groups   []groupRedirect
	done     map[syntax.Node]bool
	sorted   []*Command // commands by offset
}

func (b *builder) src(start, end int) string {
	if start < 0 || end > len(b.s.Source) || start > end {
		return ""
	}
	return b.s.Source[start:end]
}

func (b *builder) build(f *syntax.File) {
	b.seenPipe = map[*syntax.BinaryCmd]bool{}
	b.negated = map[*syntax.BinaryCmd]bool{}
	b.tested = map[*syntax.BinaryCmd]bool{}
	b.done = map[syntax.Node]bool{}
	s := b.s
	syntax.Walk(f, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Stmt:
			b.stmt(n)
		case *syntax.CallExpr:
			b.call(n)
		case *syntax.DeclClause:
			b.decl(n)
		case *syntax.IfClause:
			b.markTested(n.Cond...)
		case *syntax.WhileClause:
			b.markTested(n.Cond...)
		case *syntax.BinaryCmd:
			if n.Op == syntax.AndStmt || n.Op == syntax.OrStmt {
				b.markTested(n.X, n.Y)
			}
			if (n.Op == syntax.Pipe || n.Op == syntax.PipeAll) && !b.seenPipe[n] {
				b.pipeline(n)
			}
		}
		return true
	})
	b.sorted = slices.Clone(s.Commands)
	slices.SortStableFunc(b.sorted, func(x, y *Command) int { return x.Offset - y.Offset })
	// link substitution commands to the words they are in
	for _, c := range s.Commands {
		for _, w := range c.Words {
			if w.Subst {
				w.Subs = b.commandsWithin(w, c)
			}
		}
		for _, a := range c.Assigns {
			if a.Value != nil && a.Value.Subst {
				a.Value.Subs = b.commandsWithin(a.Value, c)
			}
		}
	}
	for _, r := range s.Redirects {
		if r.Target != nil && r.Target.Subst {
			r.Target.Subs = b.commandsWithin(r.Target, nil)
		}
	}
	b.finishPipelines()
	b.finishGroups()
	b.aliases()
	for _, c := range s.Commands {
		c.install = installOf(c)
	}
	sortRedirects(s.Redirects)
}

func (b *builder) commandsWithin(w *Word, self *Command) []*Command {
	i := sort.Search(len(b.sorted), func(i int) bool { return b.sorted[i].Offset >= w.Offset })
	var out []*Command
	for ; i < len(b.sorted) && b.sorted[i].Offset < w.End; i++ {
		if c := b.sorted[i]; c != self && c.End <= w.End {
			out = append(out, c)
		}
	}
	return out
}

func (b *builder) decl(n *syntax.DeclClause) {
	if b.done[n] {
		return
	}
	b.done[n] = true
	if len(n.Args) == 0 && n.Variant == nil {
		return
	}
	start, end := int(n.Pos().Offset()), int(n.End().Offset())
	c := &Command{Loc: b.s.loc(start, end), Decl: true}
	if n.Variant != nil {
		c.Name = n.Variant.Value
		c.NameWord = &Word{Loc: b.s.loc(int(n.Variant.Pos().Offset()), int(n.Variant.End().Offset())), Raw: n.Variant.Value, Value: n.Variant.Value}
		c.Words = []*Word{c.NameWord}
	}
	for _, a := range n.Args {
		if as := b.assign(a, c); as != nil {
			c.Assigns = append(c.Assigns, as)
		}
	}
	b.s.Commands = append(b.s.Commands, c)
	b.cmds[n] = c
}

func (b *builder) assign(a *syntax.Assign, owner *Command) *Assignment {
	if a == nil || a.Name == nil {
		return nil
	}
	as := &Assignment{
		Loc:    b.s.loc(int(a.Pos().Offset()), int(a.End().Offset())),
		Name:   a.Name.Value,
		Value:  b.word(a.Value),
		Append: a.Append,
		Array:  a.Array != nil,
		Cmd:    owner,
	}
	b.s.Assignments = append(b.s.Assignments, as)
	return as
}

func (b *builder) call(n *syntax.CallExpr) {
	if b.done[n] {
		return
	}
	b.done[n] = true
	if len(n.Args) == 0 {
		// `A=1` statement
		for _, a := range n.Assigns {
			b.assign(a, nil)
		}
		return
	}
	start, end := int(n.Pos().Offset()), int(n.End().Offset())
	c := &Command{Loc: b.s.loc(start, end)}
	for _, a := range n.Assigns {
		if as := b.assign(a, c); as != nil {
			c.Assigns = append(c.Assigns, as)
		}
	}
	for _, w := range n.Args {
		c.Words = append(c.Words, b.word(w))
	}
	b.resolve(c)
	b.cmds[n] = c
	b.s.Commands = append(b.s.Commands, c)
}

// wrapper commands: name -> options which take a value
var wrappers = map[string]map[string]bool{
	"sudo":     set("-u -g -h -p -C -D -R -T -U -r -t --user --group --host --prompt --close-from --chdir --chroot --role --type"),
	"doas":     set("-u -C"),
	"env":      set("-u -C -S --unset --chdir --split-string"),
	"command":  nil,
	"exec":     set("-a"),
	"builtin":  nil,
	"time":     set("-f -o --format --output"),
	"nohup":    nil,
	"nice":     set("-n --adjustment"),
	"timeout":  set("-s -k --signal --kill-after"),
	"stdbuf":   set("-i -o -e --input --output --error"),
	"xvfb-run": set("-e -f -n -p -s --error-file --auth-file --server-num --server-args"),
}

var reAssignWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// resolve finds the real command behind the wrappers and splits its arguments.
func (b *builder) resolve(c *Command) {
	i := 0
	for i < len(c.Words) {
		w := c.Words[i]
		if w.Dynamic() {
			break
		}
		base := path.Base(w.Value)
		opts, ok := wrappers[base]
		if !ok {
			break
		}
		c.Wrappers = append(c.Wrappers, base)
		i++
		for i < len(c.Words) {
			a := c.Words[i]
			v := a.Value
			if base == "env" && reAssignWord.MatchString(v) {
				i++
				continue
			}
			if base == "timeout" && !strings.HasPrefix(v, "-") { // the duration
				i++
				break
			}
			if base == "command" && (v == "-v" || v == "-V") { // a query, nothing is run
				b.finish(c, len(c.Words))
				return
			}
			if !strings.HasPrefix(v, "-") || a.Dynamic() || v == "-" || v == "--" {
				if v == "--" {
					i++
				}
				break
			}
			i++
			if opts[v] && i < len(c.Words) {
				i++
			}
		}
	}
	b.finish(c, i)
}

func (b *builder) finish(c *Command, i int) {
	if i >= len(c.Words) {
		return
	}
	c.NameWord = c.Words[i]
	if !c.NameWord.Dynamic() {
		c.Name = path.Base(c.NameWord.Value)
	}
	args := c.Words[i+1:]
	c.Tool, args = canonicalTool(c.Name, args)
	c.Args = args
	splitArgs(c)
}

var (
	rePip    = regexp.MustCompile(`^pip[0-9.]*$`)
	rePython = regexp.MustCompile(`^(python|py)[0-9.]*$`)
)

var shells = set("sh bash zsh dash ash ksh fish busybox")

// canonicalTool maps the command name (and for python -m the module) to a tool name and returns the arguments
// after the tool.
func canonicalTool(name string, args []*Word) (string, []*Word) {
	name = strings.TrimSuffix(name, ".exe")
	switch {
	case name == "":
		return "", args
	case rePip.MatchString(name):
		return "pip", args
	case rePython.MatchString(name):
		// python -m MODULE, also with a few leading bool flags
		for i := 0; i+1 < len(args); i++ {
			a := args[i].Value
			if a == "-m" && !args[i+1].Dynamic() {
				switch m := args[i+1].Value; m {
				case "pip", "pipx", "twine", "uv":
					return m, args[i+2:]
				}
				return "", args
			}
			if a != "-u" && a != "-B" && a != "-E" && a != "-s" && a != "-S" && a != "-I" && a != "-O" && a != "-W" {
				break
			}
		}
		return "", args
	case name == "apt-get" || name == "aptitude":
		return "apt", args
	case name == "cargo-binstall":
		return "cargo", append([]*Word{{Raw: "binstall", Value: "binstall"}}, args...)
	case name == "pnpx":
		return "pnpm-dlx", args
	}
	switch name {
	case "pip", "npm", "pnpm", "yarn", "bun", "aube", "npx", "bunx", "cargo", "go", "gem", "apt", "brew", "uv", "uvx", "pipx", "twine",
		"gh", "curl", "wget", "tee", "poetry", "flit", "hatch", "eval", "source", ".":
		return name, args
	}
	if shells[name] {
		return name, args
	}
	return "", args
}

// splitArgs fills Flags and Positional.
func splitArgs(c *Command) {
	valueFlags := toolValueFlags[c.Tool]
	endOfFlags := false
	for i := 0; i < len(c.Args); i++ {
		w := c.Args[i]
		v := w.Value
		if endOfFlags || !strings.HasPrefix(v, "-") || v == "-" || strings.HasPrefix(w.Raw, "$") {
			c.Positional = append(c.Positional, w)
			continue
		}
		if v == "--" {
			endOfFlags = true
			continue
		}
		f := &Flag{Word: w, Name: v}
		if eq := strings.IndexByte(v, '='); eq > 0 && strings.HasPrefix(v, "--") {
			f.Name = v[:eq]
			f.Value = subWord(w, eq+1)
		} else if valueFlags[v] {
			if i+1 < len(c.Args) {
				i++
				f.Value = c.Args[i]
			}
		} else if len(v) > 2 && v[1] != '-' && valueFlags[v[:2]] {
			// -rreq.txt
			f.Name = v[:2]
			f.Value = subWord(w, 2)
		} else if len(v) > 2 && v[1] != '-' && !strings.ContainsRune(v, '=') {
			// cluster like -yq, optionally ending in a value flag letter: -qr req.txt
			if valueFlags[string([]byte{'-', v[len(v)-1]})] && i+1 < len(c.Args) {
				i++
				f.Value = c.Args[i]
			}
		}
		c.Flags = append(c.Flags, f)
	}
}

// subWord is the part of a word starting at byte i of its value (used for `--k=v` and `-kv`).
func subWord(w *Word, i int) *Word {
	if i > len(w.Value) {
		i = len(w.Value)
	}
	v := &Word{Loc: w.Loc, Raw: w.Raw, Value: w.Value[i:], Quoted: w.Quoted, Glob: w.Glob, Subst: w.Subst}
	// expressions and variables are attributed to the value when it contains them
	for _, e := range w.Exprs {
		if strings.Contains(v.Value, e) {
			v.Exprs = append(v.Exprs, e)
		}
	}
	for _, name := range w.Vars {
		if strings.Contains(v.Value, name) {
			v.Vars = append(v.Vars, name)
		}
	}
	return v
}

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(s) {
		m[f] = true
	}
	return m
}
